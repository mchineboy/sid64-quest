package telnet

import (
	"strings"
)

// Presentation selects the byte stream written to a client.
type Presentation int

const (
	PresentationANSI Presentation = iota
	PresentationPETSCII
)

func (p Presentation) String() string {
	if p == PresentationPETSCII {
		return "petscii"
	}
	return "ansi"
}

func presentationForTerminalType(terminalType string) Presentation {
	terminalType = strings.ToUpper(terminalType)
	for _, marker := range []string{
		"PETSCII", "COMMODORE", "C64", "C128", "CCGMS", "CGTERM", "NOVATERM", "ULTIMATETERM",
	} {
		if strings.Contains(terminalType, marker) {
			return PresentationPETSCII
		}
	}
	return PresentationANSI
}

// encodePETSCII converts ordinary server text to the C64 lowercase/uppercase
// character set selected by CHR$(14). Native PETSCII control and graphics bytes are
// written with sendPETSCII and deliberately bypass this conversion.
func encodePETSCII(text string) string {
	var result strings.Builder
	result.Grow(len(text))

	for _, character := range strings.ReplaceAll(text, "\r\n", "\r") {
		switch {
		case character == '\r' || character == '\n':
			result.WriteByte('\r')
		case character >= 'A' && character <= 'Z':
			result.WriteByte(byte(character - 'A' + 0xc1))
		case character >= 'a' && character <= 'z':
			result.WriteByte(byte(character - ('a' - 'A')))
		case character >= ' ' && character <= '~':
			result.WriteByte(byte(character))
		case character == '…':
			result.WriteString("...")
		default:
			result.WriteByte('?')
		}
	}

	return result.String()
}

func petsciiWelcome() string {
	return petHeading("SID64 Quest") + "\r" +
		petLine("A small world. A new adventure.") + "\r" +
		petLine("Enter your username to begin.") +
		petLine("New players can create an account in") +
		petLine("the browser sign-in step.") + "\r" +
		petCyan + petLine("Display: TERMINAL ANSI / PETSCII") +
		petLine("QUIT leaves the game.") + "\r" + petWhite
}

// petsciiAuthInstructions draws the sign-in screen for a 40x25 Commodore
// screen.
//
// QR opens a separate compact quadrant-block screen; HELP retains the URL fallback.
func petsciiAuthInstructions(entryURL, pairingCode string) string {
	code := " " + pairingCode + " "
	padding := (petsciiTextWidth - len(code)) / 2
	if padding < 0 {
		padding = 0
	}
	return petHeading("Sign in to play") + "\r" +
		petLine("1. On your phone or computer, open:") +
		petCyan + petLine(entryURL) + "\r" +
		petWhite + petLine("2. Sign in with this pairing code:") + "\r" +
		petYellow + strings.Repeat(" ", padding) + "\x12" +
		petsciiText(code) + "\x92\r\r" +
		petWhite + petLine("3. Return here and type CHECK.") + "\r" +
		petCyan + petLine("The code expires in 5 minutes.") +
		petLine("QR to scan. HELP repeats. QUIT exits.") + "\r" +
		petWhite + encodePETSCII("Check> ")
}
