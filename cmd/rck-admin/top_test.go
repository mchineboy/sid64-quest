package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

func TestTopDisplay(t *testing.T) {
	sessions := []topSession{
		{id: "12345678-abcd", user: "alice\x1b\n\t", character: "Ranger", state: "4", mode: "1", heartbeat: 1, room: "Town Square"},
		{id: "abcdef01-abcd", user: "bob", state: "2", mode: "0", heartbeat: 10},
	}
	var out bytes.Buffer
	renderTop(&out, sessions, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC), 120, 24)
	for _, want := range []string{"2 sessions, 1 playing", "ANSI: 1  PETSCII: 1", "pairing", "Town Square", "not player idle time"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	if strings.ContainsAny(out.String(), "\x1b\t") {
		t.Fatal("unsanitized terminal control")
	}
	out.Reset()
	renderTop(&out, sessions, time.Now(), 80, 8)
	if !strings.Contains(out.String(), "1 more") || strings.Contains(out.String(), "SESSION") {
		t.Fatal(out.String())
	}
	out.Reset()
	renderTop(&out, nil, time.Now(), 80, 24)
	if !strings.Contains(out.String(), "No active terminal sessions") {
		t.Fatal(out.String())
	}
	if topText("a\u202eb\u009bc", 12) != "abc" {
		t.Fatal("format/control characters not removed")
	}
}

func TestTopArguments(t *testing.T) {
	for _, args := range [][]string{{"--interval", "0s"}, {"--interval", "bad"}, {"extra"}} {
		if err := runTop(nil, args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

// The temporary table shadows the real table only on this one connection.
// No persistent data or schema is changed, and only a local test DB is allowed.
func TestTopDatabase(t *testing.T) {
	if os.Getenv("RCK_ADMIN_INTEGRATION") != "1" {
		t.Skip("set RCK_ADMIN_INTEGRATION=1 with local PostgreSQL")
	}
	cfg := config.LoadFromEnv()
	if host := cfg.Database.PostgreSQL.Host; host != "localhost" && host != "127.0.0.1" && host != "::1" {
		t.Fatal("local database required")
	}
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TEMP TABLE terminal_sessions (id text, checkpoint jsonb, last_seen timestamptz, closed boolean);
 INSERT INTO terminal_sessions VALUES
 ('live', '{"Username":"alice","Character":{"name":"Ranger"},"Room":{"name":"Town Square"},"State":4,"Presentation":1,"AuthToken":"secret-do-not-display"}', now(), false),
 ('pairing', '{"Username":"bob","State":2,"Presentation":0,"PairingCode":"secret-code"}', now(), false),
 ('closed', '{}', now(), true),
 ('stale', '{}', now()-interval '3 minutes', false);`)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := readTop(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].character != "Ranger" || sessions[1].state != "2" {
		t.Fatalf("unexpected sessions: %+v", sessions)
	}
	var out bytes.Buffer
	renderTop(&out, sessions, time.Now(), 120, 0)
	if strings.Contains(out.String(), "secret") {
		t.Fatal("sensitive checkpoint fields leaked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readTop(ctx, db); err == nil {
		t.Fatal("query ignored cancellation")
	}
}
