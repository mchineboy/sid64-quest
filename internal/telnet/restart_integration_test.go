package telnet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func freeTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

func startTestProcess(t *testing.T, binary string, env []string) *exec.Cmd {
	t.Helper()
	log, err := os.CreateTemp(t.TempDir(), "process-*.log")
	require.NoError(t, err)
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = log
	cmd.Stderr = log
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		data, _ := os.ReadFile(log.Name())
		if bytes.Contains(data, []byte("WARNING: DATA RACE")) {
			t.Errorf("race in %s", binary)
		}
		if t.Failed() {
			t.Logf("%s logs:\n%s", filepath.Base(binary), data)
		}
		log.Close()
	})
	return cmd
}

func waitTestCore(t *testing.T, target, token string) {
	t.Helper()
	require.Eventually(t, func() bool {
		req, _ := http.NewRequest("GET", target+"/ready", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		client := &http.Client{Timeout: time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == 200
	}, 15*time.Second, 50*time.Millisecond)
}

type testTerminal struct {
	t       *testing.T
	conn    net.Conn
	petscii bool
	pending []byte
}

func openTestTerminal(t *testing.T, port int, pet bool) *testTerminal {
	t.Helper()
	var conn net.Conn
	require.Eventually(t, func() bool {
		var err error
		conn, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		return err == nil
	}, 10*time.Second, 50*time.Millisecond)
	t.Cleanup(func() { conn.Close() })
	client := &testTerminal{t: t, conn: conn, petscii: pet}
	client.until("sername")
	return client
}
func (c *testTerminal) send(text string) {
	c.t.Helper()
	if c.petscii {
		text = encodePETSCII(text)
	}
	_, err := c.conn.Write([]byte(text))
	require.NoError(c.t, err)
}
func (c *testTerminal) until(marker string) string {
	c.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if at := bytes.Index(c.pending, []byte(marker)); at >= 0 {
			end := at + len(marker)
			out := string(c.pending[:end])
			c.pending = c.pending[end:]
			return out
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 4096)
		n, err := c.conn.Read(buf)
		if err != nil {
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				continue
			}
			c.t.Fatalf("terminal disconnected waiting for %q: %v; buffered %q", marker, err, c.pending)
		}
		data := buf[:n]
		if c.petscii {
			for i, b := range data {
				if b >= 65 && b <= 90 {
					data[i] = b + 32
				} else if b >= 193 && b <= 218 {
					data[i] = b - 128
				}
			}
		}
		c.pending = append(c.pending, data...)
	}
	c.t.Fatalf("timed out waiting for %q; buffered %q", marker, c.pending)
	return ""
}

func TestCoreIntegrationProcessRestart(t *testing.T) {
	f := newCoreFixture(t)
	binDir := os.Getenv("RCK_INTEGRATION_BIN_DIR")
	require.NotEmpty(t, binDir, "build game-core and terminal-edge, then set RCK_INTEGRATION_BIN_DIR")
	ansiPort, petPort := freeTestPort(t), freeTestPort(t)
	bluePort, greenPort := freeTestPort(t), freeTestPort(t)
	blueTarget := fmt.Sprintf("http://127.0.0.1:%d", bluePort)
	greenTarget := fmt.Sprintf("http://127.0.0.1:%d", greenPort)
	targetFile := filepath.Join(t.TempDir(), "core-target")
	require.NoError(t, os.WriteFile(targetFile, []byte(blueTarget), 0600))
	env := []string{"POSTGRES_DB=" + f.cfg.Database.PostgreSQL.Database, "HOST=127.0.0.1", "MONGODB_URI=", "CORE_TOKEN=" + f.core.Token, "TELNET_PORT=" + fmt.Sprint(ansiPort), "PETSCII_PORT=" + fmt.Sprint(petPort), "CORE_TARGET_FILE=" + targetFile, "IDLE_TIMEOUT=900"}
	blue := startTestProcess(t, filepath.Join(binDir, "game-core"), append(env, "CORE_LISTEN=127.0.0.1:"+fmt.Sprint(bluePort)))
	waitTestCore(t, blueTarget, f.core.Token)
	edge := startTestProcess(t, filepath.Join(binDir, "terminal-edge"), env)
	a := openTestTerminal(t, ansiPort, false)
	b := openTestTerminal(t, petPort, true)
	for i, client := range []*testTerminal{a, b} {
		name := []string{"edgealice", "edgebob"}[i]
		user, character := f.player([]string{"Edge Alice", "Edge Bob"}[i])
		client.send(name + "\r")
		var saved checkpoint
		var id string
		require.Eventually(t, func() bool {
			var data []byte
			err := f.db.QueryRow(`SELECT id,checkpoint FROM terminal_sessions WHERE checkpoint->>'Username'=$1`, name).Scan(&id, &data)
			if err != nil {
				return false
			}
			return json.Unmarshal(data, &saved) == nil && saved.AuthToken != ""
		}, 10*time.Second, 25*time.Millisecond)
		require.NoError(t, f.core.auth.LinkTokenToSession(saved.AuthToken, id, user, character))
		client.until("character")
		client.send("1\r")
		client.until("Town Square")
	}
	// Nothing from the observer until the final CR: its input reader must not
	// gate room events, server announcements, or another player's movement.
	a.until("Edge Bob has entered the realm")
	b.send("say half typed ")
	a.send("announce live server notice\r")
	b.until("[Realm] live server notice")
	a.send("east\r")
	b.until("Edge Alice leaves east")
	a.send("west\r")
	b.until("Edge Alice arrives")
	var squareID string
	require.NoError(t, f.db.QueryRow(`SELECT id FROM rooms WHERE name='Town Square' LIMIT 1`).Scan(&squareID))
	// Real persisted room changes, not cosmetic heartbeat text.
	_, err := f.db.Exec(`INSERT INTO room_items(room_id,item_id,quantity) SELECT $1,id,1 FROM items WHERE name='Health Potion' ON CONFLICT(room_id,item_id) DO UPDATE SET quantity=room_items.quantity+1`, squareID)
	require.NoError(t, err)
	b.until("Health Potion appears here")
	a.until("Health Potion appears here")
	_, err = f.db.Exec(`INSERT INTO npcs(name,description,room_id,health,max_health,level) VALUES('Socket Goblin','A test visitor',$1,10,10,1)`, squareID)
	require.NoError(t, err)
	b.until("Socket Goblin appears here")
	a.until("Socket Goblin appears here")
	b.send("survived\r")
	a.until("Edge Bob says: half typed survived")
	a.send("script new room socket_survivor\r")
	var scriptID string
	require.Eventually(t, func() bool {
		return f.db.QueryRow(`SELECT id FROM scripts WHERE name='socket_survivor'`).Scan(&scriptID) == nil
	}, 5*time.Second, 25*time.Millisecond)
	a.send("script edit " + scriptID + "\r")
	a.until("Editing socket_survivor")
	// Leave a partial source line in the edge's decoder when the core dies.
	a.send("    tell(\"still ")
	require.NoError(t, blue.Process.Kill())
	_ = blue.Wait()
	a.until("taking a breath")
	b.until("taking a breath")
	a.send("connected\")\r")
	b.send("east\r")
	green := startTestProcess(t, filepath.Join(binDir, "game-core"), append(env, "CORE_LISTEN=127.0.0.1:"+fmt.Sprint(greenPort)))
	waitTestCore(t, greenTarget, f.core.Token)
	require.NoError(t, os.WriteFile(targetFile+".next", []byte(greenTarget), 0600))
	require.NoError(t, os.Rename(targetFile+".next", targetFile))
	a.until("realm stirs again")
	b.until("realm stirs again")
	b.until("Market Lane")
	a.send(".save\r")
	require.Eventually(t, func() bool {
		var source string
		err := f.db.QueryRow(`SELECT content FROM scripts WHERE id=$1`, scriptID).Scan(&source)
		return err == nil && strings.Contains(source, `tell("still connected")`)
	}, 10*time.Second, 25*time.Millisecond)
	b.send("west\r")
	b.until("Town Square")
	a.send("say sockets survived\r")
	b.until("sockets survived")
	b.send("who\r")
	b.until("Edge Alice")
	// Planned switch with both cores live: the same edge PID and sockets stay up.
	blue2 := startTestProcess(t, filepath.Join(binDir, "game-core"), append(env, "CORE_LISTEN=127.0.0.1:"+fmt.Sprint(bluePort)))
	waitTestCore(t, blueTarget, f.core.Token)
	badSwitch := exec.Command(filepath.Join(binDir, "core-switch"), "-target", blueTarget, "-file", targetFile)
	badSwitch.Env = append(os.Environ(), "CORE_TOKEN="+strings.Repeat("wrong", 10))
	_, err = badSwitch.CombinedOutput()
	require.Error(t, err)
	previous, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	require.Equal(t, greenTarget, strings.TrimSpace(string(previous)))
	switcher := exec.Command(filepath.Join(binDir, "core-switch"), "-target", blueTarget, "-file", targetFile)
	switcher.Env = append(os.Environ(), "CORE_TOKEN="+f.core.Token)
	result, err := switcher.CombinedOutput()
	require.NoError(t, err, string(result))
	a.until("realm stirs again")
	b.until("realm stirs again")
	require.NoError(t, green.Process.Kill())
	_ = green.Wait()
	a.send("look\r")
	a.until("Town Square")
	b.send("say green to blue\r")
	a.until("green to blue")
	var count int
	require.NoError(t, f.db.QueryRow(`SELECT count(*) FROM terminal_sessions WHERE NOT closed`).Scan(&count))
	require.Equal(t, 2, count)
	require.Nil(t, edge.ProcessState)
	require.Nil(t, blue2.ProcessState)
	a.send("quit\r")
	b.until("Edge Alice has left the realm")
	b.send("quit\r")
	require.Eventually(t, func() bool {
		_ = f.db.QueryRow(`SELECT count(*) FROM terminal_sessions WHERE NOT closed`).Scan(&count)
		return count == 0
	}, 5*time.Second, 25*time.Millisecond)
	t.Log("PASS: SIGKILL core, queued commands, partial input, editor recovery, ANSI/PETSCII, shared presence/chat, live blue/green switch; edge PID and sockets unchanged")
}
