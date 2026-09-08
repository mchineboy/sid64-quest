package telnet

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/tylerhardison/race-condition-kingdom/internal/database"
	"github.com/tylerhardison/race-condition-kingdom/internal/game"
	"github.com/tylerhardison/race-condition-kingdom/internal/terminalwire"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

type coreFixture struct {
	db    *sql.DB
	cache *redis.Client
	cfg   *config.Config
	core  *Core
	t     *testing.T
}

func newCoreFixture(t *testing.T) *coreFixture {
	t.Helper()
	if os.Getenv("RCK_CORE_INTEGRATION") != "1" {
		t.Skip("set RCK_CORE_INTEGRATION=1 with local PostgreSQL and Redis to run")
	}
	cfg := config.LoadFromEnv()
	for _, host := range []string{cfg.Database.PostgreSQL.Host, cfg.Redis.Host} {
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			t.Fatal("integration tests require local database hosts")
		}
	}
	admin, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	require.NoError(t, err)
	name := "rck_core_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec("CREATE DATABASE " + name)
	require.NoError(t, err)
	cfg.Database.PostgreSQL.Database = name
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	require.NoError(t, err)
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Address(), Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	require.NoError(t, cache.Ping(context.Background()).Err())
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	require.NoError(t, database.Migrate(context.Background(), db))
	require.NoError(t, game.NewWorldService(db).EnsureStarterWorld(context.Background()))
	f := &coreFixture{db: db, cache: cache, cfg: cfg, t: t, core: NewCore(db, cache, cfg, logger, strings.Repeat("t", 32))}
	t.Cleanup(func() {
		// Only remove Redis entries named by this disposable database's sessions.
		rows, err := db.Query(`SELECT id,checkpoint FROM terminal_sessions`)
		if err == nil {
			for rows.Next() {
				var id string
				var data []byte
				_ = rows.Scan(&id, &data)
				var p checkpoint
				_ = json.Unmarshal(data, &p)
				_ = cache.Del(context.Background(), "session:"+id, "terminal:presence:"+id, "auth_token:"+p.AuthToken, "pairing_code:"+strings.ReplaceAll(p.PairingCode, "-", "")).Err()
			}
			rows.Close()
		}
		cache.Close()
		db.Close()
		_, err = admin.Exec("DROP DATABASE " + name)
		if err != nil {
			t.Errorf("drop test database: %v", err)
		}
		admin.Close()
	})
	return f
}

func (f *coreFixture) player(name string) (uuid.UUID, uuid.UUID) {
	user, character := uuid.New(), uuid.New()
	_, err := f.db.Exec(`INSERT INTO users(id,username,email,password_hash,permissions) VALUES($1,$2,$3,'test','{"builder":true,"admin":true}')`, user, "core_"+user.String()[:8], user.String()+"@example.test")
	require.NoError(f.t, err)
	_, err = f.db.Exec(`INSERT INTO characters(id,user_id,name,current_room_id) VALUES($1,$2,$3,(SELECT id FROM rooms WHERE name='Town Square' LIMIT 1))`, character, user, name)
	require.NoError(f.t, err)
	return user, character
}

func (f *coreFixture) request(core *Core, req *terminalwire.Request, kind, input string) terminalwire.Response {
	f.t.Helper()
	req.Sequence++
	req.Kind = kind
	req.Input = input
	resp, err := core.dispatch(context.Background(), *req)
	require.NoError(f.t, err)
	for _, out := range resp.Output {
		req.Ack = out.ID
	}
	return resp
}

func outputText(resp terminalwire.Response) string {
	var out strings.Builder
	for _, p := range resp.Output {
		out.Write(p.Data)
	}
	return out.String()
}

func (f *coreFixture) login(core *Core, name string, user, character uuid.UUID) *terminalwire.Request {
	f.t.Helper()
	req := &terminalwire.Request{Version: terminalwire.Version, ID: uuid.NewString(), EdgeID: uuid.NewString()}
	f.request(core, req, "open", "")
	f.request(core, req, "input", name)
	var data []byte
	require.NoError(f.t, f.db.QueryRow(`SELECT checkpoint FROM terminal_sessions WHERE id=$1`, req.ID).Scan(&data))
	var saved checkpoint
	require.NoError(f.t, json.Unmarshal(data, &saved))
	require.NoError(f.t, core.auth.LinkTokenToSession(saved.AuthToken, req.ID, user, character))
	f.request(core, req, "poll", "")
	resp := f.request(core, req, "input", "1")
	require.Contains(f.t, outputText(resp), "Town Square")
	return req
}

func TestCoreIntegrationRecoveryAndExactlyOnce(t *testing.T) {
	f := newCoreFixture(t)
	u, ch := f.player("Core Alice")
	req := f.login(f.core, "alice", u, ch)
	green := NewCore(f.db, f.cache, f.cfg, f.core.logger, f.core.Token)
	// Reads for authorization must reuse the transaction, even with a one-slot
	// pool. Otherwise the active command deadlocks behind its own connection.
	f.db.SetMaxOpenConns(1)
	// Simulate a response lost after commit: both versions receive exactly the
	// same command concurrently. Only one movement/stamina deduction may commit.
	req.Sequence++
	req.Kind = "input"
	req.Input = "east"
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, core := range []*Core{f.core, green} {
		wg.Add(1)
		go func(c *Core) { defer wg.Done(); _, err := c.dispatch(context.Background(), *req); errs <- err }(core)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var stamina int
	var room string
	require.NoError(t, f.db.QueryRow(`SELECT c.stamina,r.name FROM characters c JOIN rooms r ON r.id=c.current_room_id WHERE c.id=$1`, ch).Scan(&stamina, &room))
	require.Equal(t, 99, stamina)
	require.Equal(t, "Market Lane", room)
	resp, err := green.dispatch(context.Background(), *req)
	require.NoError(t, err)
	require.Contains(t, outputText(resp), "Market Lane")
	for _, out := range resp.Output {
		req.Ack = out.ID
	}
	// Presence and chat work across independently constructed cores.
	u2, ch2 := f.player("Core Bob")
	b := f.login(green, "bob", u2, ch2)
	f.request(f.core, req, "input", "west")
	require.Contains(t, outputText(f.request(green, b, "input", "who")), "Core Alice")
	f.request(f.core, req, "input", "say across versions")
	require.Contains(t, outputText(f.request(green, b, "poll", "")), "across versions")
	// Unsaved editor state survives a replacement too.
	f.request(f.core, req, "input", "script new room restart_test")
	var script string
	require.NoError(t, f.db.QueryRow(`SELECT id FROM scripts WHERE name='restart_test'`).Scan(&script))
	f.request(f.core, req, "input", "script edit "+script)
	f.request(f.core, req, "input", "    tell(\"survived\")")
	f.request(green, req, "input", ".save")
	var content string
	require.NoError(t, f.db.QueryRow(`SELECT content FROM scripts WHERE id=$1`, script).Scan(&content))
	require.Contains(t, content, "survived")
	// Current permission is checked after restoration; disable applies immediately.
	_, err = f.db.Exec(`UPDATE users SET is_active=false WHERE id=$1`, u)
	require.NoError(t, err)
	require.True(t, f.request(green, req, "input", "look").Closed)
}

func TestCoreIntegrationRollbackAndOwnership(t *testing.T) {
	f := newCoreFixture(t)
	u, ch := f.player("Owner")
	req := f.login(f.core, "owner", u, ch)
	green := NewCore(f.db, f.cache, f.cfg, f.core.logger, f.core.Token)
	other := &terminalwire.Request{Version: 1, ID: uuid.NewString(), EdgeID: uuid.NewString()}
	f.request(green, other, "open", "")
	f.request(green, other, "input", "owner")
	var raw []byte
	require.NoError(t, f.db.QueryRow(`SELECT checkpoint FROM terminal_sessions WHERE id=$1`, other.ID).Scan(&raw))
	var p checkpoint
	require.NoError(t, json.Unmarshal(raw, &p))
	require.NoError(t, green.auth.LinkTokenToSession(p.AuthToken, other.ID, u, ch))
	f.request(green, other, "poll", "")
	require.Contains(t, outputText(f.request(green, other, "input", "1")), "already online")
	// Reject the checkpoint write after movement. The character update must roll
	// back too, then a retry after removing the fault must execute exactly once.
	_, err := f.db.Exec(`CREATE FUNCTION reject_checkpoint() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected checkpoint failure'; END $$; CREATE TRIGGER checkpoint_fault BEFORE UPDATE ON terminal_sessions FOR EACH ROW EXECUTE FUNCTION reject_checkpoint()`)
	require.NoError(t, err)
	req.Sequence++
	req.Kind = "input"
	req.Input = "east"
	_, err = f.core.dispatch(context.Background(), *req)
	require.Error(t, err)
	var stamina int
	require.NoError(t, f.db.QueryRow(`SELECT stamina FROM characters WHERE id=$1`, ch).Scan(&stamina))
	require.Equal(t, 100, stamina)
	_, err = f.db.Exec(`DROP TRIGGER checkpoint_fault ON terminal_sessions`)
	require.NoError(t, err)
	resp, err := green.dispatch(context.Background(), *req)
	require.NoError(t, err)
	require.Contains(t, outputText(resp), "Market Lane")
	for _, out := range resp.Output {
		req.Ack = out.ID
	}
	f.request(f.core, req, "close", "")
	require.Contains(t, outputText(f.request(green, other, "input", "1")), "Market Lane")
	// Redis presence is leased, and disconnect clears it.
	require.Equal(t, int64(0), f.cache.Exists(context.Background(), "terminal:presence:"+req.ID).Val())
	require.Greater(t, f.cache.TTL(context.Background(), "terminal:presence:"+other.ID).Val(), time.Duration(0))
}
