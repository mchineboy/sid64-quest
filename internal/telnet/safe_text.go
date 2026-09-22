package telnet

import (
	"strings"
	"unicode"
)

// terminalLabel is for untrusted labels, never server-authored ANSI formatting.
func terminalLabel(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return -1
		}
		return r
	}, value)
}
