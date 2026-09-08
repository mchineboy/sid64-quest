package telnet

import (
	"fmt"
	"github.com/skip2/go-qrcode"
	"strings"
)

// Each entry is a PETSCII glyph plus a reverse-video flag for four white
// quadrants: top-left, top-right, bottom-left, bottom-right. These shapes are
// identical in both C64 character sets. Background must be black.
var petQuadrants = [16]struct {
	glyph   byte
	reverse bool
}{
	{0x20, false}, {0xbe, false}, {0xbc, false}, {0xa2, true},
	{0xbb, false}, {0xa1, false}, {0xbf, true}, {0xac, true},
	{0xac, false}, {0xbf, false}, {0xa1, true}, {0xbb, true},
	{0xa2, false}, {0xbc, true}, {0xbe, true}, {0x20, true},
}

func petsciiQRScreen(url string) (string, error) {
	code, err := qrcode.New(url, qrcode.Low)
	if err != nil {
		return "", err
	}
	bitmap := code.Bitmap() // Includes the four-module quiet zone.
	cells := (len(bitmap) + 1) / 2
	if cells > 21 {
		return "", fmt.Errorf("pairing URL is too long for a C64 QR screen")
	}
	var b strings.Builder
	b.WriteString("\x93\x0e\x05\x92")
	b.WriteString(petLine("Scan to sign in"))
	for y := 0; y < cells; y++ {
		b.WriteString(strings.Repeat(" ", (39-cells)/2))
		for x := 0; x < cells; x++ {
			mask := 0
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					row, col := y*2+dy, x*2+dx
					if row >= len(bitmap) || col >= len(bitmap) || !bitmap[row][col] {
						mask |= 1 << (dy*2 + dx)
					}
				}
			}
			shape := petQuadrants[mask]
			if shape.reverse {
				b.WriteByte(0x12)
			} else {
				b.WriteByte(0x92)
			}
			b.WriteByte(shape.glyph)
		}
		b.WriteString("\x92\r")
	}
	b.WriteString(petLine("CHECK after sign-in. HELP for URL."))
	return b.String(), nil
}
