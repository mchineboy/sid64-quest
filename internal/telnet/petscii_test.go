package telnet

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/tylerhardison/race-condition-kingdom/internal/ansi"
)

func TestPresentationForTerminalType(t *testing.T) {
	for _, terminalType := range []string{"C64", "petscii", "CCGMS 5.5", "Commodore-128", "UltimateTerm"} {
		if got := presentationForTerminalType(terminalType); got != PresentationPETSCII {
			t.Errorf("presentationForTerminalType(%q) = %s, want petscii", terminalType, got)
		}
	}

	if got := presentationForTerminalType("xterm-256color"); got != PresentationANSI {
		t.Errorf("presentationForTerminalType(xterm-256color) = %s, want ansi", got)
	}
}

func TestNegotiateTerminalType(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connection := &Connection{
		Conn:      server,
		Reader:    bufio.NewReader(server),
		Writer:    bufio.NewWriter(server),
		Formatter: ansi.NewFormatter(true),
		Context:   ctx,
		Cancel:    cancel,
	}

	type result struct {
		terminalType string
		err          error
	}
	resultChannel := make(chan result, 1)
	go func() {
		terminalType, err := connection.NegotiateTerminalType(time.Second)
		resultChannel <- result{terminalType: terminalType, err: err}
	}()

	request := make([]byte, 3)
	if _, err := io.ReadFull(client, request); err != nil {
		t.Fatal(err)
	}
	if got, want := string(request), string([]byte{telnetIAC, telnetDO, telnetTTYPE}); got != want {
		t.Fatalf("initial negotiation = %v, want %v", request, []byte(want))
	}

	if _, err := client.Write([]byte{telnetIAC, telnetWILL, telnetTTYPE}); err != nil {
		t.Fatal(err)
	}
	request = make([]byte, 6)
	if _, err := io.ReadFull(client, request); err != nil {
		t.Fatal(err)
	}
	if got, want := string(request), string([]byte{telnetIAC, telnetSB, telnetTTYPE, ttypeSEND, telnetIAC, telnetSE}); got != want {
		t.Fatalf("terminal type request = %v, want %v", request, []byte(want))
	}

	if _, err := client.Write([]byte{telnetIAC, telnetSB, telnetTTYPE, ttypeIS, 'C', '6', '4', telnetIAC, telnetSE}); err != nil {
		t.Fatal(err)
	}

	select {
	case result := <-resultChannel:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.terminalType != "C64" {
			t.Errorf("terminal type = %q, want C64", result.terminalType)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal negotiation did not complete")
	}
}

func TestEncodePETSCII(t *testing.T) {
	got := encodePETSCII("Hello, adventurer…\r\n")
	want := "HELLO, ADVENTURER...\r\r"
	if got != want {
		t.Errorf("encodePETSCII() = %q, want %q", got, want)
	}
}

func TestReadTelnetSubnegotiation(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(string([]byte{
		telnetTTYPE, ttypeIS, 'C', '6', '4', telnetIAC, telnetSE,
	})))

	option, data, err := readTelnetSubnegotiation(reader)
	if err != nil {
		t.Fatal(err)
	}
	if option != telnetTTYPE || string(data) != string([]byte{ttypeIS, 'C', '6', '4'}) {
		t.Errorf("readTelnetSubnegotiation() = (%d, %q), want terminal type C64", option, data)
	}
}
