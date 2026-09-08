package telnet

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	qrcode "github.com/skip2/go-qrcode"

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
	want := "\xc8ELLO, ADVENTURER...\r"
	if got != want {
		t.Errorf("encodePETSCII() = %q, want %q", got, want)
	}
}

func TestReadLineEchoesPETSCIIAndHandlesDelete(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connection := &Connection{
		Conn:         server,
		Reader:       bufio.NewReader(server),
		Writer:       bufio.NewWriter(server),
		Formatter:    ansi.NewFormatter(false),
		Presentation: PresentationPETSCII,
		Context:      ctx,
		Cancel:       cancel,
	}

	type readResult struct {
		line string
		err  error
	}
	result := make(chan readResult, 1)
	go func() {
		line, err := connection.ReadLine()
		result <- readResult{line: line, err: err}
	}()

	input := []byte{0xc1, 0xc2, 0x14, 0xc3, '\r'}
	writeDone := make(chan error, 1)
	go func() {
		_, err := client.Write(input)
		writeDone <- err
	}()

	echo := make([]byte, len(input))
	if _, err := io.ReadFull(client, echo); err != nil {
		t.Fatal(err)
	}
	if string(echo) != string(input) {
		t.Fatalf("echo = %v, want %v", echo, input)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.line != "AC" {
		t.Fatalf("line = %q, want %q", got.line, "AC")
	}
}

func TestPETSCIIAuthInstructionsFitFortyByTwentyFive(t *testing.T) {
	screen := petsciiAuthInstructions("http://symptom-pi:8080/pair", "9MDJ-XNAH")

	if !strings.Contains(screen, encodePETSCII("9MDJ-XNAH")) {
		t.Fatal("screen does not show the pairing code")
	}

	// Colour, charset, clear-screen and reverse-video codes occupy no column.
	zeroWidth := map[byte]bool{0x05: true, 0x12: true, 0x0e: true, 0x92: true, 0x93: true, 0x9e: true, 0x9f: true}

	var rows []string
	var row []byte
	for index := 0; index < len(screen); index++ {
		character := screen[index]
		switch {
		case character == '\r':
			rows = append(rows, string(row))
			row = row[:0]
		case zeroWidth[character]:
		default:
			row = append(row, character)
		}
	}
	if len(row) > 0 {
		rows = append(rows, string(row))
	}

	if len(rows) > 25 {
		t.Fatalf("screen uses %d rows, want no more than 25", len(rows))
	}
	for index, text := range rows {
		if len(text) > 40 {
			t.Fatalf("row %d is %d columns (%q), want no more than 40", index, len(text), text)
		}
	}
}

func TestANSIQRCodeMatchesSymbolAndFitsTerminal(t *testing.T) {
	const content = "http://symptom-pi:8080/p/ABCD-EFGH"

	rendered, err := ansiQRCode(content, 78)
	if err != nil {
		t.Fatal(err)
	}

	code, err := qrcode.New(content, qrcode.Low)
	if err != nil {
		t.Fatal(err)
	}
	want := code.Bitmap()

	rows := strings.Split(strings.TrimRight(rendered, "\r\n"), "\r\n")
	width := strings.Count(rows[0], upperHalfBlock)
	if width != len(want[0]) {
		t.Fatalf("rendered %d modules per row, want %d", width, len(want[0]))
	}

	// Two module rows per character row keeps modules square on a terminal
	// whose cells are twice as tall as wide. Anything else is unscannable.
	if height := len(rows) * 2; height != len(want) && height != len(want)+1 {
		t.Fatalf("rendered %d module rows, want %d", height, len(want))
	}

	// Reading the modules back out proves the polarity and row pairing are
	// faithful to the symbol, rather than merely the right shape.
	got := decodeANSIQRModules(t, rows, width, len(want))
	for row := range want {
		for column := range want[row] {
			if got[row][column] != want[row][column] {
				t.Fatalf("module (%d,%d) is %v, want %v", row, column, got[row][column], want[row][column])
			}
		}
	}

	if _, err := ansiQRCode(strings.Repeat("x", 2000), 78); err == nil {
		t.Fatal("expected a QR code wider than the terminal to be rejected")
	}
}

// decodeANSIQRModules recovers the module grid from rendered output. A dark
// module is a black foreground in the top half of a cell, or a black
// background in the bottom half.
func decodeANSIQRModules(t *testing.T, rows []string, width, height int) [][]bool {
	t.Helper()

	modules := make([][]bool, height)
	for index := range modules {
		modules[index] = make([]bool, width)
	}

	for rowIndex, row := range rows {
		cells := strings.Split(row, upperHalfBlock)
		if len(cells) != width+1 {
			t.Fatalf("row %d has %d cells, want %d", rowIndex, len(cells)-1, width)
		}
		for column := 0; column < width; column++ {
			top := strings.Contains(cells[column], ansi.Black)
			bottom := strings.Contains(cells[column], ansi.BgBlack)
			modules[rowIndex*2][column] = top
			if rowIndex*2+1 < height {
				modules[rowIndex*2+1][column] = bottom
			}
		}
	}
	return modules
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

func TestPETSCIIMixedCaseRoundTrip(t *testing.T) {
	c := &Connection{Presentation: PresentationPETSCII}
	const text = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz 0123456789"
	encoded := encodePETSCII(text)
	var decoded []byte
	for _, b := range []byte(encoded) {
		decoded = append(decoded, c.petsciiInputByte(b))
	}
	if string(decoded) != text {
		t.Fatalf("round trip = %q", decoded)
	}
	if encodePETSCII("AaZz") != "\xc1\x41\xda\x5a" {
		t.Fatal("incorrect letter byte mapping")
	}
	for b := byte(0x61); b <= 0x7a; b++ {
		if c.petsciiInputByte(b) != 'A'+b-0x61 {
			t.Fatal("uppercase alias decoded incorrectly")
		}
	}
	if !strings.HasPrefix(petsciiWelcome(), "\x93\x0e") {
		t.Fatal("welcome does not select mixed-case charset")
	}
	if strings.Contains(petsciiWelcome(), "\x8e") {
		t.Fatal("welcome selects graphics charset")
	}
}
