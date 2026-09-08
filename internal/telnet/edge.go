package telnet

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tylerhardison/race-condition-kingdom/internal/ansi"
	"github.com/tylerhardison/race-condition-kingdom/internal/terminalwire"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

// Edge owns TCP sockets and byte framing only. It has no database credentials,
// authentication authority or world state. Updating TargetFile switches cores
// at the next request without disturbing terminal sockets.
type Edge struct {
	Config                    *config.Config
	Logger                    *logrus.Logger
	Target, TargetFile, Token string
	ID                        string
	clients                   sync.Map
	active                    atomic.Int64
}

func (e *Edge) Run(ctx context.Context) error {
	if len(e.Token) < 32 {
		return fmt.Errorf("CORE_TOKEN must contain at least 32 bytes")
	}
	e.ID = uuid.NewString()
	ansiListener, err := net.Listen("tcp", net.JoinHostPort(e.Config.Server.Host, fmt.Sprint(e.Config.Server.TelnetPort)))
	if err != nil {
		return err
	}
	defer ansiListener.Close()
	petListener, err := net.Listen("tcp", net.JoinHostPort(e.Config.Server.Host, fmt.Sprint(e.Config.Server.PETSCIIPort)))
	if err != nil {
		return err
	}
	defer petListener.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { <-ctx.Done(); ansiListener.Close(); petListener.Close() }()
	var wg sync.WaitGroup
	var accepting sync.WaitGroup
	accepting.Add(2)
	errs := make(chan error, 2)
	accept := func(listener net.Listener, dedicated bool) {
		defer accepting.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				errs <- err
				return
			}
			count := e.active.Add(1)
			if max := e.Config.Server.MaxConnections; max > 0 && count > int64(max) {
				e.active.Add(-1)
				conn.Close()
				continue
			}
			wg.Add(1)
			go func() { defer wg.Done(); defer e.active.Add(-1); e.serve(ctx, conn, dedicated) }()
		}
	}
	go accept(ansiListener, false)
	go accept(petListener, true)
	e.Logger.WithField("edge_id", e.ID).Info("Terminal edge listening")
	select {
	case <-ctx.Done():
	case err = <-errs:
		cancel()
	}
	cancel()
	ansiListener.Close()
	petListener.Close()
	accepting.Wait()
	wg.Wait()
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (e *Edge) call(ctx context.Context, req terminalwire.Request) (terminalwire.Response, error) {
	var result terminalwire.Response
	target := e.Target
	if e.TargetFile != "" {
		data, err := os.ReadFile(e.TargetFile)
		if err != nil {
			return result, err
		}
		target = strings.TrimSpace(string(data))
	}
	base := target
	if strings.HasPrefix(target, "unix:") {
		base = "http://core"
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return result, fmt.Errorf("invalid core endpoint")
	}
	var client *http.Client
	if cached, ok := e.clients.Load(target); ok {
		client = cached.(*http.Client)
	} else {
		transport := &http.Transport{MaxIdleConnsPerHost: 32, IdleConnTimeout: 30 * time.Second}
		if strings.HasPrefix(target, "unix:") {
			path := strings.TrimPrefix(target, "unix:")
			transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", path)
			}
		}
		client = &http.Client{Transport: transport, Timeout: 12 * time.Second}
		actual, _ := e.clients.LoadOrStore(target, client)
		client = actual.(*http.Client)
	}
	data, err := json.Marshal(req)
	if err != nil {
		return result, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/session", bytes.NewReader(data))
	if err != nil {
		return result, err
	}
	r.Header.Set("Authorization", "Bearer "+e.Token)
	r.Header.Set("Content-Type", "application/json")
	response, err := client.Do(r)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return result, fmt.Errorf("core status %d", response.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 256*1024)).Decode(&result)
	if err == nil && (result.Version != terminalwire.Version || result.Sequence != req.Sequence) {
		err = fmt.Errorf("incompatible core response")
	}
	return result, err
}

// ReadLine's ordinary idle deadline is managed by the edge loop instead: a
// backend outage must not expire a terminal while the player waits to recover.
type edgeConn struct{ net.Conn }

func (e edgeConn) SetReadDeadline(time.Time) error { return nil }

func (e *Edge) serve(parent context.Context, socket net.Conn, dedicated bool) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	defer socket.Close()
	go func() { <-ctx.Done(); socket.Close() }()
	c := &Connection{ID: uuid.NewString(), Conn: socket, Reader: bufio.NewReader(socket), Writer: bufio.NewWriter(socket), Context: ctx, Cancel: cancel, Formatter: ansi.NewFormatter(true)}
	if tcp, ok := socket.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}
	if dedicated {
		c.SetTerminalType("PETSCII")
	} else {
		terminal, err := c.NegotiateTerminalType(250 * time.Millisecond)
		if err != nil {
			return
		}
		c.SetTerminalType(terminal)
		if c.Presentation == PresentationPETSCII {
			if c.EnableServerEcho() != nil {
				return
			}
		}
	}
	var mode atomic.Int32
	mode.Store(int32(c.Presentation))
	c.inputPresentation = func() Presentation { return Presentation(mode.Load()) }
	c.Conn = edgeConn{socket}
	_ = socket.SetReadDeadline(time.Time{})
	var lastInput atomic.Int64
	lastInput.Store(time.Now().UnixNano())
	notice := func(text string) error {
		if Presentation(mode.Load()) == PresentationPETSCII {
			return c.write("\r" + petCyan + petLine(text) + petWhite)
		}
		return c.write("\r\n" + text + "\r\n")
	}
	input := make(chan string, 8)
	go func() {
		for {
			line, err := c.ReadLine()
			if errors.Is(err, errLineTooLong) {
				_ = notice("That line is too long. Please try a shorter command.")
				continue
			}
			if err != nil {
				cancel()
				return
			}
			lastInput.Store(time.Now().UnixNano())
			// Backpressure keeps the queue bounded without dropping commands.
			select {
			case input <- line:
			case <-ctx.Done():
				return
			}
		}
	}()
	req := terminalwire.Request{Version: terminalwire.Version, EdgeID: e.ID, ID: c.ID, Sequence: 1, Kind: "open", Terminal: c.TerminalType, Dedicated: dedicated}
	defer func() {
		cancel()
		cleanup, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		req.Kind = "close"
		req.Input = ""
		_, _ = e.call(cleanup, req) // An unreachable core releases the presence lease in two minutes.
	}()
	paused := false
	lastNotice := time.Time{}
	coreID := ""
	for {
		response, err := e.call(ctx, req)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if !paused {
				if notice("The realm is taking a breath. Stay connected; we'll be right back.") != nil {
					return
				}
				paused = true
				lastNotice = time.Now()
				e.Logger.WithError(err).WithField("connection", c.ID).Info("Waiting for core")
			} else if time.Since(lastNotice) > 30*time.Second {
				if notice("Still here, adventurer. Your connection is safe. Please wait.") != nil {
					return
				}
				lastNotice = time.Now()
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue // Retry the exact sequence; PostgreSQL deduplicates it.
		}
		if response.Reset {
			if notice("The realm has been away too long. Please sign in again here.") != nil {
				return
			}
			req.ID = uuid.NewString()
			req.Sequence = 1
			req.Ack = 0
			req.Kind = "open"
			req.Input = ""
			req.Terminal = Presentation(mode.Load()).String()
			continue
		}
		mode.Store(int32(response.Presentation))
		recovered := paused || (coreID != "" && coreID != response.CoreID)
		if recovered {
			if notice("The realm stirs again. Your journey continues.") != nil {
				return
			}
			lastInput.Store(time.Now().UnixNano())
		}
		coreID = response.CoreID
		paused = false
		for _, output := range response.Output {
			if output.ID <= req.Ack {
				continue
			}
			if err := c.write(string(output.Data)); err != nil {
				return
			}
			req.Ack = output.ID
		}
		if recovered && len(response.Output) == 0 {
			if c.write(string(response.Prompt)) != nil {
				return
			}
		}
		if response.Closed {
			return
		}
		req.Sequence++
		select {
		case <-ctx.Done():
			return
		case line := <-input:
			req.Kind = "input"
			req.Input = line
		case <-time.After(time.Second):
			idle := time.Duration(e.Config.Server.IdleTimeout) * time.Second
			if idle <= 0 {
				idle = 15 * time.Minute
			}
			if time.Since(time.Unix(0, lastInput.Load())) > idle {
				_ = notice("You have been idle a while. Come back soon, adventurer.")
				req.Kind = "close"
				req.Input = ""
			} else {
				req.Kind = "poll"
				req.Input = ""
			}
		}
	}
}
