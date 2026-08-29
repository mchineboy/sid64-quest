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

// encodePETSCII converts ordinary server text to the C64's boot-time
// upper/graphics character set. Native PETSCII control and graphics bytes are
// written with sendPETSCII and deliberately bypass this conversion.
func encodePETSCII(text string) string {
	var result strings.Builder
	result.Grow(len(text))

	for _, character := range text {
		switch {
		case character == '\r' || character == '\n':
			result.WriteByte('\r')
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
	const (
		clearScreen = "\x93"
		// Select the uppercase/graphics charset. encodePETSCII folds text to
		// uppercase and the banner uses codes 0x6a-0x6c as line graphics, both
		// of which require this mode rather than the shifted one.
		upperCharset = "\x8e"
		cyan         = "\x9f"
		white        = "\x05"
		reverseOn    = "\x12"
		reverseOff   = "\x92"
		block        = "\xa0"
		graphicA     = "\x6a"
		graphicB     = "\x6b"
		graphicC     = "\x6c"
	)

	border := strings.Repeat(block, 40)
	graphics := strings.Repeat(graphicA+graphicB+graphicC, 13) + graphicA

	return clearScreen + upperCharset + cyan + border + "\r" +
		white + reverseOn + " RACE CONDITION KINGDOM             " + reverseOff + "\r" +
		cyan + graphics + "\r" +
		white + " A TELNET MUD FOR A 40-COLUMN REALM. \r" +
		cyan + border + "\r\r"
}

// petsciiAuthInstructions draws the sign-in screen for a 40x25 Commodore
// screen.
//
// It deliberately carries no QR code. The smallest QR symbol is 21x21 modules
// and needs a 4-module quiet zone, so 29x29. A Commodore character cell is
// roughly square, so square QR modules cost one cell each, and 29 rows do not
// fit in 25. Packing two modules per cell fits the height but halves module
// height, and scanners reject the resulting 2:1 modules. The pairing code is
// therefore the primary path here, and it is drawn large and centred.
func petsciiAuthInstructions(entryURL, pairingCode string) string {
	const (
		clearScreen  = "\x93"
		upperCharset = "\x8e"
		white        = "\x05"
		cyan         = "\x9f"
		yellow       = "\x9e"
		reverseOn    = "\x12"
		reverseOff   = "\x92"
		border       = "\xa0"
	)

	var screen strings.Builder
	screen.WriteString(clearScreen + upperCharset)

	screen.WriteString(cyan + strings.Repeat(border, 40) + "\r")
	screen.WriteString(white + centerPETSCIILine("SIGN IN TO PLAY", 40) + "\r")
	screen.WriteString(cyan + strings.Repeat(border, 40) + "\r\r")

	screen.WriteString(white + " 1. OPEN THIS ON A PHONE OR PC:\r\r")
	screen.WriteString(cyan + wrapPETSCII("    "+strings.ToUpper(entryURL), 40) + "\r")

	screen.WriteString(white + " 2. ENTER THIS CODE:\r\r")
	// Reverse video must cover only the code, not the centring whitespace.
	code := " " + strings.ToUpper(pairingCode) + " "
	screen.WriteString(yellow + strings.Repeat(" ", (40-len(code))/2))
	screen.WriteString(reverseOn + code + reverseOff + "\r\r")

	screen.WriteString(white + " 3. TYPE  CHECK  HERE WHEN DONE.\r\r")
	screen.WriteString(cyan + centerPETSCIILine("CODE EXPIRES IN 5 MINUTES", 40) + "\r")
	screen.WriteString(white)
	return screen.String()
}

func centerPETSCIILine(text string, width int) string {
	if len(text) >= width {
		return text
	}
	return strings.Repeat(" ", (width-len(text))/2) + text
}

func wrapPETSCII(text string, width int) string {
	var output strings.Builder
	for len(text) > width {
		output.WriteString(text[:width])
		output.WriteByte('\r')
		text = text[width:]
	}
	output.WriteString(text)
	output.WriteByte('\r')
	return output.String()
}
