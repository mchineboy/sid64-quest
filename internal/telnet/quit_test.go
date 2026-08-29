package telnet

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/tylerhardison/race-condition-kingdom/internal/ansi"
)

// drainedConnection returns a connection whose output is collected, so a test
// can assert on what the player actually saw.
func drainedConnection(t *testing.T) (*Connection, func() string) {
	t.Helper()

	server, client := net.Pipe()
	t.Cleanup(func() { server.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	output := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(client)
		output <- string(data)
	}()

	connection := &Connection{
		Conn:         server,
		Reader:       bufio.NewReader(server),
		Writer:       bufio.NewWriter(server),
		Formatter:    ansi.NewFormatter(false),
		Presentation: PresentationANSI,
		Context:      ctx,
		Cancel:       cancel,
	}

	return connection, func() string {
		server.Close()
		select {
		case data := <-output:
			return data
		case <-time.After(2 * time.Second):
			t.Fatal("timed out collecting connection output")
			return ""
		}
	}
}

func TestQuitCommandsReportDisconnectRatherThanFailure(t *testing.T) {
	server := &Server{}

	cases := []struct {
		name  string
		input string
		run   func(conn *Connection, input string) error
	}{
		{"in game", "quit", server.handleGameCommandWithoutPrompt},
		{"in game short", "q", server.handleGameCommandWithoutPrompt},
		{"in game exit", "exit", server.handleGameCommandWithoutPrompt},
		{"at username prompt", "quit", server.handleUsernameInput},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			connection, collect := drainedConnection(t)

			err := testCase.run(connection, testCase.input)
			seen := collect()

			if !errors.Is(err, errQuit) {
				t.Fatalf("quit returned %v, want errQuit so the loop closes the session", err)
			}
			if !strings.Contains(seen, "Goodbye!") {
				t.Errorf("player saw %q, want a goodbye", seen)
			}
			if strings.Contains(seen, "ERROR") {
				t.Errorf("player saw an error on a clean quit: %q", seen)
			}
		})
	}
}
