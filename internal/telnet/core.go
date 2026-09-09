package telnet

import (
	"bufio"
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/tylerhardison/race-condition-kingdom/internal/ansi"
	"github.com/tylerhardison/race-condition-kingdom/internal/auth"
	"github.com/tylerhardison/race-condition-kingdom/internal/game"
	"github.com/tylerhardison/race-condition-kingdom/internal/terminalwire"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

// Core is deliberately stateless between requests. PostgreSQL serializes commands
// across old and new cores and commits gameplay, session state and terminal output
// together. Retrying an unacknowledged sequence never executes it twice.
type Core struct {
	db     *sql.DB
	redis  *redis.Client
	cfg    *config.Config
	auth   *auth.AuthService
	logger *logrus.Logger
	ID     string
	Token  string
}

func NewCore(db *sql.DB, cache *redis.Client, cfg *config.Config, logger *logrus.Logger, token string) *Core {
	return &Core{db: db, redis: cache, cfg: cfg, auth: auth.NewAuthService(db, cache, cfg, logger), logger: logger, ID: uuid.NewString(), Token: token}
}

type checkpoint struct {
	Version                          int
	State                            ConnectionState
	Username                         string
	Session                          *models.Session
	Character                        *models.Character
	Room                             *models.Room
	AuthToken, PairingCode, Terminal string
	Presentation                     Presentation
	Dedicated                        bool
	Editor                           *scriptEditor
	Output                           []terminalwire.Output
	NextOutput                       uint64
	Closed                           bool
	Scene                            *roomScene
}

type coreSession struct {
	id, edge string
	sequence int64
	saved    checkpoint
	conn     *Connection
	dirty    bool
}

func (c *Core) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if len(c.Token) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+c.Token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if r.URL.Path == "/ready" && r.Method == http.MethodGet {
		if c.db.PingContext(ctx) != nil || c.redis.Ping(ctx).Err() != nil {
			http.Error(w, "dependencies unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(terminalwire.Response{Version: terminalwire.Version, CoreID: c.ID})
		return
	}
	if r.URL.Path != "/session" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req terminalwire.Request
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if _, err := uuid.Parse(req.ID); err != nil {
		http.Error(w, "invalid connection", 400)
		return
	}
	if _, err := uuid.Parse(req.EdgeID); err != nil {
		http.Error(w, "invalid edge", 400)
		return
	}
	if req.Version != terminalwire.Version || req.Sequence < 1 || len(req.Input) > 1024 || len(req.Terminal) > 128 {
		http.Error(w, "unsupported protocol or input", 400)
		return
	}
	switch req.Kind {
	case "open", "input", "poll", "close":
	default:
		http.Error(w, "invalid operation", 400)
		return
	}
	response, err := c.dispatch(ctx, req)
	if err != nil {
		c.logger.WithError(err).WithField("connection", req.ID).Warn("Core request failed")
		http.Error(w, "realm temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (c *Core) dispatch(ctx context.Context, req terminalwire.Request) (terminalwire.Response, error) {
	var response terminalwire.Response
	// Pairing requires Redis. Fail before accepting commands when it is down.
	if err := c.redis.Ping(ctx).Err(); err != nil {
		return response, err
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return response, err
	}
	defer tx.Rollback()
	// This is a correctness-first alpha implementation: a single world command
	// lane, including across versions. Never use an expiring Redis lock for this.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(72632003)`); err != nil {
		return response, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE terminal_sessions SET character_id=NULL WHERE last_seen < now()-interval '2 minutes'`); err != nil {
		return response, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM terminal_sessions WHERE last_seen < now()-interval '24 hours'`); err != nil {
		return response, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,edge_id,sequence,checkpoint FROM terminal_sessions WHERE (NOT closed AND last_seen > now()-interval '2 minutes') OR id=$1`, req.ID)
	if err != nil {
		return response, err
	}
	sessions := map[string]*coreSession{}
	for rows.Next() {
		s := &coreSession{}
		var data []byte
		if err = rows.Scan(&s.id, &s.edge, &s.sequence, &data); err != nil {
			rows.Close()
			return response, err
		}
		if err = json.Unmarshal(data, &s.saved); err != nil {
			rows.Close()
			return response, err
		}
		if s.saved.Version != terminalwire.Version {
			rows.Close()
			return response, fmt.Errorf("incompatible checkpoint")
		}
		sessions[s.id] = s
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return response, err
	}
	s := sessions[req.ID]
	if s == nil {
		if req.Kind == "close" {
			return terminalwire.Response{Version: terminalwire.Version, CoreID: c.ID, Sequence: req.Sequence, Closed: true}, nil
		}
		if req.Sequence != 1 || req.Kind != "open" {
			return terminalwire.Response{Version: terminalwire.Version, CoreID: c.ID, Sequence: req.Sequence, Reset: true}, nil
		}
		s = &coreSession{id: req.ID, edge: req.EdgeID, saved: checkpoint{Version: terminalwire.Version, Terminal: req.Terminal, Dedicated: req.Dedicated}}
		s.saved.Presentation = presentationForTerminalType(req.Terminal)
		sessions[s.id] = s
	}
	if s.edge != req.EdgeID {
		return response, fmt.Errorf("connection belongs to another edge")
	}
	wasClosed := s.saved.Closed
	// A socket-close notification is idempotent even if the last request's
	// commit was not observed by the edge. It cannot execute a game command.
	if req.Kind == "close" {
		s.saved.Closed = true
		if req.Sequence > s.sequence {
			s.sequence = req.Sequence
		}
	}
	if req.Sequence > s.sequence+1 {
		return response, fmt.Errorf("sequence gap")
	}
	if req.Ack > s.saved.NextOutput {
		return response, fmt.Errorf("output acknowledgement ahead of core")
	}
	for len(s.saved.Output) > 0 && s.saved.Output[0].ID <= req.Ack {
		s.saved.Output = s.saved.Output[1:]
	}
	s.dirty = true
	world := game.NewTransactionalWorldService(tx)
	server := NewServer(c.cfg, c.auth.WithReadTransaction(tx), nil, world, c.logger)
	defer server.cancel()
	for _, other := range sessions {
		other.restore(ctx)
		if other == s {
			continue
		}
		if !other.saved.Closed && other.conn.State == StateInGame && other.conn.Character != nil {
			if !server.hub.claim(other.conn.Character.ID, other.id) {
				return response, fmt.Errorf("conflicting character ownership")
			}
			server.hub.update(other.conn)
		}
	}
	if !s.saved.Closed && s.conn.State == StateInGame && s.conn.Character != nil {
		if !server.hub.claim(s.conn.Character.ID, s.id) {
			s.conn.State = StateAuthenticated
			s.conn.Character, s.conn.Room, s.conn.ScriptEditor = nil, nil, nil
			_ = s.conn.SendMessage("That character is playing elsewhere. Please choose another character.")
		} else {
			server.hub.update(s.conn)
		}
	}
	server.forcePETSCII = s.saved.Dedicated
	wasPlaying := s.conn.State == StateInGame && s.conn.Character != nil
	fresh := req.Sequence == s.sequence+1 && !s.saved.Closed
	if fresh {
		// Observe background changes before an input command replaces the room
		// view, so an active typist does not miss changes between polls either.
		if req.Kind == "input" && wasPlaying {
			if err = s.observeRoom(world, true); err != nil {
				return response, err
			}
		}
		before := make(map[string][]byte, len(sessions))
		for id, other := range sessions {
			other.capture()
			before[id], err = json.Marshal(other.saved)
			if err != nil {
				return response, err
			}
		}
		if _, err = tx.ExecContext(ctx, "SAVEPOINT terminal_command"); err != nil {
			return response, err
		}
		var commandErr error
		switch req.Kind {
		case "open":
			if s.sequence != 0 {
				return response, fmt.Errorf("session already open")
			}
			commandErr = s.conn.SendWelcome()
			if commandErr == nil {
				commandErr = server.handleInitialConnection(s.conn)
			}
		case "input":
			commandErr = server.processInput(s.conn, req.Input)
		case "poll":
			if s.conn.State == StateAwaitingAuth {
				commandErr = server.checkAuthentication(s.conn, false)
			}
		case "close":
			commandErr = errQuit
		}
		if errors.Is(commandErr, errQuit) {
			s.saved.Closed = true
			server.hub.remove(s.id)
		} else if commandErr != nil {
			// Invalid scripts and other command failures must not trap the edge in
			// an infinite retry loop. Roll back this command, including its output,
			// and durably return a normal error. A dead DB/transaction still retries.
			if _, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT terminal_command"); err != nil {
				return response, err
			}
			for id, other := range sessions {
				if err = json.Unmarshal(before[id], &other.saved); err != nil {
					return response, err
				}
				other.restore(ctx)
				other.dirty = other == s
			}
			c.logger.WithError(commandErr).Warn("Game command rolled back")
			_ = s.conn.SendError("That action could not be completed. Please try again.")
		}
		if _, err = tx.ExecContext(ctx, "RELEASE SAVEPOINT terminal_command"); err != nil {
			return response, err
		}
		s.sequence = req.Sequence
	}
	// Commit lifecycle notifications in the recipients' outboxes as well. The
	// nil legacy event bus does not deliver these for a stateless core.
	if !wasClosed && s.saved.Closed && wasPlaying {
		server.hub.remove(s.id)
		server.BroadcastMessage(s.conn.Character.Name + " has left the realm.")
	}
	if fresh && !s.saved.Closed && s.conn.State == StateInGame {
		if err = s.observeRoom(world, req.Kind == "poll"); err != nil {
			return response, err
		}
		if req.Kind == "poll" && len(s.saved.Output) > 0 {
			if err = s.conn.SendPrompt(s.conn.currentPrompt()); err != nil {
				return response, err
			}
		}
	}
	if s.saved.Closed && req.Sequence == s.sequence+1 {
		s.sequence = req.Sequence
	}
	for _, other := range sessions {
		if !other.dirty {
			continue
		}
		other.capture()
		data, err := json.Marshal(other.saved)
		if err != nil {
			return response, err
		}
		var character interface{}
		if !other.saved.Closed && other.conn.State == StateInGame && other.conn.Character != nil {
			character = other.conn.Character.ID
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO terminal_sessions(id,edge_id,character_id,sequence,checkpoint,closed) VALUES($1,$2,$3,$4,$5,$6)
		 ON CONFLICT(id) DO UPDATE SET character_id=$3,sequence=$4,checkpoint=$5,closed=$6,
		 last_seen=CASE WHEN terminal_sessions.id=$7 THEN now() ELSE terminal_sessions.last_seen END`, other.id, other.edge, character, other.sequence, data, other.saved.Closed, req.ID)
		if err != nil {
			return response, err
		}
	}
	if err = tx.Commit(); err != nil {
		return response, err
	}
	// Redis presence is a disposable directory for operators, never authority for
	// character ownership or command deduplication. PostgreSQL is authoritative.
	key := "terminal:presence:" + s.id
	if s.saved.Closed {
		_ = c.redis.Del(ctx, key).Err()
		_ = c.auth.DeleteSession(s.id)
	} else {
		_ = c.redis.Set(ctx, key, s.edge, 2*time.Minute).Err()
	}
	prompt := "Username> "
	switch s.conn.State {
	case StateInGame:
		prompt = s.conn.currentPrompt()
	case StateAwaitingAuth:
		prompt = "Check> "
	case StateAuthenticated:
		prompt = "Character> "
	}
	if s.saved.Presentation == PresentationPETSCII {
		prompt = petsciiText(prompt)
	}
	response = terminalwire.Response{Version: terminalwire.Version, CoreID: c.ID, Sequence: s.sequence, Output: s.saved.Output, Presentation: int(s.saved.Presentation), Closed: s.saved.Closed, Prompt: []byte(prompt)}
	return response, nil
}

func (s *coreSession) restore(ctx context.Context) {
	p := &s.saved
	ctx, cancel := context.WithCancel(ctx)
	c := &Connection{ID: s.id, State: p.State, Username: p.Username, Session: p.Session, Character: p.Character, Room: p.Room, AuthToken: p.AuthToken, PairingCode: p.PairingCode, TerminalType: p.Terminal, Presentation: p.Presentation, ScriptEditor: p.Editor, Context: ctx, Cancel: cancel, Formatter: ansi.NewFormatter(p.Presentation == PresentationANSI)}
	c.Conn = &outputConn{session: s}
	c.Writer = bufio.NewWriter(c.Conn)
	s.conn = c
}

func (s *coreSession) capture() {
	c, p := s.conn, &s.saved
	p.State, p.Username, p.Session, p.Character, p.Room = c.State, c.Username, c.Session, c.Character, c.Room
	p.AuthToken, p.PairingCode, p.Terminal, p.Presentation, p.Editor = c.AuthToken, c.PairingCode, c.TerminalType, c.Presentation, c.ScriptEditor
}

// Output is bounded and persists with the command. Slow/offline clients cannot
// grow an unbounded outbox. Overflow retires only that session.
type outputConn struct{ session *coreSession }

func (o *outputConn) Write(data []byte) (int, error) {
	s := o.session
	s.dirty = true
	n := len(data)
	for _, p := range s.saved.Output {
		n += len(p.Data)
	}
	if n > 64*1024 {
		s.saved.Closed = true
		return len(data), nil
	}
	s.saved.NextOutput++
	s.saved.Output = append(s.saved.Output, terminalwire.Output{ID: s.saved.NextOutput, Data: append([]byte(nil), data...)})
	return len(data), nil
}
func (o *outputConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (o *outputConn) Close() error                     { o.session.saved.Closed = true; o.session.dirty = true; return nil }
func (o *outputConn) LocalAddr() net.Addr              { return coreAddr("core") }
func (o *outputConn) RemoteAddr() net.Addr             { return coreAddr("edge") }
func (o *outputConn) SetDeadline(time.Time) error      { return nil }
func (o *outputConn) SetReadDeadline(time.Time) error  { return nil }
func (o *outputConn) SetWriteDeadline(time.Time) error { return nil }

type coreAddr string

func (a coreAddr) Network() string { return "terminal-core" }
func (a coreAddr) String() string  { return string(a) }
