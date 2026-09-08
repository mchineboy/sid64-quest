package telnet

import (
	"github.com/skip2/go-qrcode"
	"os"
	"strings"
	"testing"
)

func TestPETSCIIQRModules(t *testing.T) {
	url := "https://symptom-pi.tail8762f9.ts.net:8443/p/ABCD-EFGH"
	screen, err := petsciiQRScreen(url)
	if err != nil {
		t.Fatal(err)
	}
	q, _ := qrcode.New(url, qrcode.Low)
	bitmap := q.Bitmap()
	cells := (len(bitmap) + 1) / 2
	rows := strings.Split(screen, "\r")
	if len(rows) > 25 {
		t.Fatal("screen would scroll")
	}
	for y, row := range rows[1 : 1+cells] {
		col := 0
		reverse := false
		for _, ch := range []byte(row) {
			if ch == 0x12 {
				reverse = true
				continue
			}
			if ch == 0x92 {
				reverse = false
				continue
			}
			x := col - (39-cells)/2
			col++
			if x < 0 {
				continue
			}
			mask := -1
			for i, shape := range petQuadrants {
				if shape.glyph == ch && shape.reverse == reverse {
					mask = i
					break
				}
			}
			if mask < 0 {
				t.Fatal("unknown block")
			}
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					r, c := y*2+dy, x*2+dx
					if r < len(bitmap) && c < len(bitmap) && ((mask&(1<<(dy*2+dx))) == 0) != bitmap[r][c] {
						t.Fatal("module mismatch")
					}
				}
			}
		}
	}
	if _, err := petsciiQRScreen(strings.Repeat("x", 500)); err == nil {
		t.Fatal("oversize QR accepted")
	}
	if path := os.Getenv("RCK_QR_PREVIEW"); path != "" {
		if err := os.WriteFile(path, []byte(screen), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
