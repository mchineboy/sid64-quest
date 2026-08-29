package telnet

import "strings"

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
		cyan        = "\x9f"
		white       = "\x05"
		reverseOn   = "\x12"
		reverseOff  = "\x92"
		block       = "\xa0"
		graphicA    = "\x6a"
		graphicB    = "\x6b"
		graphicC    = "\x6c"
	)

	border := strings.Repeat(block, 40)
	graphics := strings.Repeat(graphicA+graphicB+graphicC, 13) + graphicA

	return clearScreen + cyan + border + "\r" +
		white + reverseOn + " RACE CONDITION KINGDOM             " + reverseOff + "\r" +
		cyan + graphics + "\r" +
		white + " A TELNET MUD FOR A 40-COLUMN REALM. \r" +
		cyan + border + "\r\r"
}
