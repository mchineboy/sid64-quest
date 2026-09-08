package telnet

import (
	"bufio"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

func TestReadLineSplitLineEndings(t *testing.T) {
	for _, ending := range []string{"\n", "\x00", ""} {
		t.Run("CR then "+ending, func(t *testing.T) {
			conn, _ := drainedConnection(t)
			// Replace the reader between calls to simulate packets arriving after CR.
			conn.Reader = bufio.NewReader(strings.NewReader("look\r"))
			line, err := conn.ReadLine()
			if err != nil || line != "look" {
				t.Fatalf("first line = %q, %v", line, err)
			}
			conn.Reader = bufio.NewReader(strings.NewReader(ending + "who\r"))
			line, err = conn.ReadLine()
			if err != nil || line != "who" {
				t.Fatalf("second line = %q, %v", line, err)
			}
		})
	}
}

func TestReadLineSkipsNegotiation(t *testing.T) {
	conn, _ := drainedConnection(t)
	negotiation := string([]byte{telnetIAC, telnetSB, telnetTTYPE, ttypeIS, 'C', '6', '4', telnetIAC, telnetSE})
	options := strings.Repeat(string([]byte{telnetIAC, telnetWILL, telnetTTYPE}), 10000)
	conn.Reader = bufio.NewReader(strings.NewReader(options + "sa" + negotiation + "y\thello\r"))
	line, err := conn.ReadLine()
	if err != nil || line != "say hello" {
		t.Fatalf("line = %q, %v", line, err)
	}
}

func TestReadLineRejectsOversizeAndRecovers(t *testing.T) {
	conn, _ := drainedConnection(t)
	conn.Reader = bufio.NewReader(strings.NewReader(strings.Repeat("a", 1025) + "\r\nlook\r"))
	line, err := conn.ReadLine()
	if !errors.Is(err, errLineTooLong) || line != "" {
		t.Fatalf("oversize = %q, %v", line, err)
	}
	line, err = conn.ReadLine()
	if err != nil || line != "look" {
		t.Fatalf("next line = %q, %v", line, err)
	}
	conn.Reader = bufio.NewReader(strings.NewReader(strings.Repeat("a", 1024) + "\n"))
	line, err = conn.ReadLine()
	if err != nil || len(line) != 1024 {
		t.Fatalf("boundary: len=%d, err=%v", len(line), err)
	}
}

func TestTerminalOverrideDuringLogin(t *testing.T) {
	conn, collect := drainedConnection(t)
	conn.State = StateAwaitingUsername
	server := &Server{}
	if err := server.processInput(conn, "TERMINAL PETSCII"); err != nil {
		t.Fatal(err)
	}
	if conn.Presentation != PresentationPETSCII || conn.State != StateAwaitingUsername {
		t.Fatal("override changed login state or failed to switch display")
	}
	if err := server.processInput(conn, "terminal ansi"); err != nil {
		t.Fatal(err)
	}
	if conn.Presentation != PresentationANSI {
		t.Fatal("failed to return to ANSI")
	}
	seen := collect()
	for _, expected := range []string{string([]byte{telnetIAC, telnetWILL, telnetECHO}), string([]byte{telnetIAC, 252, telnetECHO}), "Enter your username:"} {
		if !strings.Contains(seen, expected) {
			t.Errorf("missing %q in output", expected)
		}
	}
}

func TestInvalidTerminalDoesNotChangeState(t *testing.T) {
	conn, collect := drainedConnection(t)
	conn.State = StateAuthenticated
	if err := (&Server{}).processInput(conn, "terminal invalid"); err != nil {
		t.Fatal(err)
	}
	if conn.State != StateAuthenticated || conn.Presentation != PresentationANSI {
		t.Fatal("invalid option changed state")
	}
	if !strings.Contains(collect(), "Usage:") {
		t.Fatal("missing usage")
	}
}

func TestWhereReportsRoomAndSortedExits(t *testing.T) {
	conn, collect := drainedConnection(t)
	conn.Room = &models.Room{Name: "Town Square", Exits: map[string]uuid.UUID{"south": uuid.New(), "north": uuid.New()}}
	if err := (&Server{}).handleGameCommandWithoutPrompt(conn, "where"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(collect(), "You are in Town Square. Exits: north, south.") {
		t.Fatal("missing location")
	}
}

func TestOversizedSubnegotiationIsBounded(t *testing.T) {
	_, _, err := readTelnetSubnegotiation(bufio.NewReader(strings.NewReader(string(byte(telnetTTYPE)) + strings.Repeat("x", 2048))))
	if err == nil {
		t.Fatal("oversized negotiation accepted")
	}
}
