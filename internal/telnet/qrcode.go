package telnet

import (
	"fmt"
	"strings"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/tylerhardison/race-condition-kingdom/internal/ansi"
)

// upperHalfBlock carries the module in the top half of a character cell; the
// cell's background colour carries the module in the bottom half.
const upperHalfBlock = "\u2580"

// ansiQRCode renders content as a scannable QR code, packing two module rows
// into every character row.
//
// A typical terminal character cell is about twice as tall as it is wide, so
// two stacked modules per cell yields roughly square modules, which is what a
// scanner requires. Colours are stated explicitly as black and white rather
// than inherited from the terminal theme, because polarity is what a scanner
// reads. This is unsuitable for PETSCII, whose cells are already square.
func ansiQRCode(content string, maxColumns int) (string, error) {
	code, err := qrcode.New(content, qrcode.Low)
	if err != nil {
		return "", fmt.Errorf("create QR code: %w", err)
	}

	bitmap := code.Bitmap()
	if len(bitmap) == 0 {
		return "", fmt.Errorf("QR code is empty")
	}
	width := len(bitmap[0])
	if width > maxColumns {
		return "", fmt.Errorf("QR code needs %d columns; terminal allows %d", width, maxColumns)
	}

	indent := strings.Repeat(" ", (maxColumns-width)/2)

	var output strings.Builder
	for row := 0; row < len(bitmap); row += 2 {
		output.WriteString(indent)
		for column := 0; column < width; column++ {
			// Bitmap reports true for a dark module. Rows past the end of the
			// bitmap are quiet zone, which is light.
			top := bitmap[row][column]
			bottom := false
			if row+1 < len(bitmap) {
				bottom = bitmap[row+1][column]
			}

			foreground := ansi.BrightWhite
			if top {
				foreground = ansi.Black
			}
			background := ansi.BgBrightWhite
			if bottom {
				background = ansi.BgBlack
			}
			output.WriteString(foreground + background + upperHalfBlock)
		}
		output.WriteString(ansi.Reset + "\r\n")
	}

	return output.String(), nil
}
