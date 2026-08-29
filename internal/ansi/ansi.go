package ansi

import (
	"fmt"
	"strings"
)

// ANSI escape codes for colors and formatting
const (
	// Reset
	Reset = "\033[0m"
	
	// Text styles
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Italic    = "\033[3m"
	Underline = "\033[4m"
	Blink     = "\033[5m"
	Reverse   = "\033[7m"
	Strike    = "\033[9m"
	
	// Foreground colors (standard)
	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
	
	// Foreground colors (bright)
	BrightBlack   = "\033[90m"
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"
	
	// Background colors (standard)
	BgBlack   = "\033[40m"
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
	BgWhite   = "\033[47m"
	
	// Background colors (bright)
	BgBrightBlack   = "\033[100m"
	BgBrightRed     = "\033[101m"
	BgBrightGreen   = "\033[102m"
	BgBrightYellow  = "\033[103m"
	BgBrightBlue    = "\033[104m"
	BgBrightMagenta = "\033[105m"
	BgBrightCyan    = "\033[106m"
	BgBrightWhite   = "\033[107m"
	
	// Cursor movement
	CursorUp    = "\033[A"
	CursorDown  = "\033[B"
	CursorRight = "\033[C"
	CursorLeft  = "\033[D"
	
	// Screen control
	ClearScreen     = "\033[2J"
	ClearLine       = "\033[K"
	ClearLineLeft   = "\033[1K"
	ClearLineRight  = "\033[0K"
	SaveCursor      = "\033[s"
	RestoreCursor   = "\033[u"
	HideCursor      = "\033[?25l"
	ShowCursor      = "\033[?25h"
	
	// Special sequences
	Bell = "\007"
)

// Color represents a color code
type Color string

// Style represents text styling options
type Style struct {
	Foreground Color
	Background Color
	Bold       bool
	Dim        bool
	Italic     bool
	Underline  bool
	Blink      bool
	Reverse    bool
	Strike     bool
}

// Formatter provides ANSI formatting utilities
type Formatter struct {
	enabled bool
}

// NewFormatter creates a new ANSI formatter
func NewFormatter(enabled bool) *Formatter {
	return &Formatter{enabled: enabled}
}

// Enabled reports whether the client accepts ANSI escape sequences. Callers
// that depend on colour for meaning, rather than decoration, must check this.
func (f *Formatter) Enabled() bool {
	return f.enabled
}

// Format applies ANSI formatting to text
func (f *Formatter) Format(text string, style Style) string {
	if !f.enabled {
		return text
	}
	
	var codes []string
	
	// Add foreground color
	if style.Foreground != "" {
		codes = append(codes, string(style.Foreground))
	}
	
	// Add background color
	if style.Background != "" {
		codes = append(codes, string(style.Background))
	}
	
	// Add text styles
	if style.Bold {
		codes = append(codes, Bold)
	}
	if style.Dim {
		codes = append(codes, Dim)
	}
	if style.Italic {
		codes = append(codes, Italic)
	}
	if style.Underline {
		codes = append(codes, Underline)
	}
	if style.Blink {
		codes = append(codes, Blink)
	}
	if style.Reverse {
		codes = append(codes, Reverse)
	}
	if style.Strike {
		codes = append(codes, Strike)
	}
	
	if len(codes) == 0 {
		return text
	}
	
	return strings.Join(codes, "") + text + Reset
}

// Colorize applies a simple color to text
func (f *Formatter) Colorize(text string, color Color) string {
	if !f.enabled {
		return text
	}
	return string(color) + text + Reset
}

// Bold makes text bold
func (f *Formatter) Bold(text string) string {
	return f.Format(text, Style{Bold: true})
}

// Dim makes text dim
func (f *Formatter) Dim(text string) string {
	return f.Format(text, Style{Dim: true})
}

// Italic makes text italic
func (f *Formatter) Italic(text string) string {
	return f.Format(text, Style{Italic: true})
}

// Underline underlines text
func (f *Formatter) Underline(text string) string {
	return f.Format(text, Style{Underline: true})
}

// MoveCursor moves the cursor to a specific position
func (f *Formatter) MoveCursor(row, col int) string {
	if !f.enabled {
		return ""
	}
	return fmt.Sprintf("\033[%d;%dH", row, col)
}

// MoveCursorUp moves the cursor up by n lines
func (f *Formatter) MoveCursorUp(n int) string {
	if !f.enabled {
		return ""
	}
	return fmt.Sprintf("\033[%dA", n)
}

// MoveCursorDown moves the cursor down by n lines
func (f *Formatter) MoveCursorDown(n int) string {
	if !f.enabled {
		return ""
	}
	return fmt.Sprintf("\033[%dB", n)
}

// MoveCursorRight moves the cursor right by n columns
func (f *Formatter) MoveCursorRight(n int) string {
	if !f.enabled {
		return ""
	}
	return fmt.Sprintf("\033[%dC", n)
}

// MoveCursorLeft moves the cursor left by n columns
func (f *Formatter) MoveCursorLeft(n int) string {
	if !f.enabled {
		return ""
	}
	return fmt.Sprintf("\033[%dD", n)
}

// ClearScreenAndHome clears the screen and moves cursor to home
func (f *Formatter) ClearScreenAndHome() string {
	if !f.enabled {
		return ""
	}
	return ClearScreen + f.MoveCursor(1, 1)
}

// Box draws a simple box around text
func (f *Formatter) Box(text string, width int) string {
	lines := strings.Split(text, "\n")
	
	// Calculate the actual width needed
	maxLen := 0
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	
	if width < maxLen+4 {
		width = maxLen + 4
	}
	
	var result strings.Builder
	
	// Top border
	result.WriteString("┌")
	result.WriteString(strings.Repeat("─", width-2))
	result.WriteString("┐\n")
	
	// Content lines
	for _, line := range lines {
		result.WriteString("│ ")
		result.WriteString(line)
		result.WriteString(strings.Repeat(" ", width-len(line)-3))
		result.WriteString("│\n")
	}
	
	// Bottom border
	result.WriteString("└")
	result.WriteString(strings.Repeat("─", width-2))
	result.WriteString("┘")
	
	return result.String()
}

// ProgressBar creates a progress bar
func (f *Formatter) ProgressBar(current, max int, width int, style Style) string {
	if max == 0 {
		return ""
	}
	
	percentage := float64(current) / float64(max)
	filled := int(percentage * float64(width))
	
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	
	return f.Format(bar, style)
}

// Table creates a simple table
func (f *Formatter) Table(headers []string, rows [][]string) string {
	if len(headers) == 0 || len(rows) == 0 {
		return ""
	}
	
	// Calculate column widths
	colWidths := make([]int, len(headers))
	for i, header := range headers {
		colWidths[i] = len(header)
	}
	
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}
	
	var result strings.Builder
	
	// Header separator
	result.WriteString("┌")
	for i, width := range colWidths {
		result.WriteString(strings.Repeat("─", width+2))
		if i < len(colWidths)-1 {
			result.WriteString("┬")
		}
	}
	result.WriteString("┐\n")
	
	// Headers
	result.WriteString("│")
	for i, header := range headers {
		result.WriteString(" ")
		result.WriteString(f.Bold(header))
		result.WriteString(strings.Repeat(" ", colWidths[i]-len(header)+1))
		result.WriteString("│")
	}
	result.WriteString("\n")
	
	// Header-body separator
	result.WriteString("├")
	for i, width := range colWidths {
		result.WriteString(strings.Repeat("─", width+2))
		if i < len(colWidths)-1 {
			result.WriteString("┼")
		}
	}
	result.WriteString("┤\n")
	
	// Rows
	for _, row := range rows {
		result.WriteString("│")
		for i, cell := range row {
			if i < len(colWidths) {
				result.WriteString(" ")
				result.WriteString(cell)
				result.WriteString(strings.Repeat(" ", colWidths[i]-len(cell)+1))
				result.WriteString("│")
			}
		}
		result.WriteString("\n")
	}
	
	// Bottom border
	result.WriteString("└")
	for i, width := range colWidths {
		result.WriteString(strings.Repeat("─", width+2))
		if i < len(colWidths)-1 {
			result.WriteString("┴")
		}
	}
	result.WriteString("┘")
	
	return result.String()
}

// Predefined color constants for convenience
const (
	ColorBlack   Color = Black
	ColorRed     Color = Red
	ColorGreen   Color = Green
	ColorYellow  Color = Yellow
	ColorBlue    Color = Blue
	ColorMagenta Color = Magenta
	ColorCyan    Color = Cyan
	ColorWhite   Color = White
	
	ColorBrightBlack   Color = BrightBlack
	ColorBrightRed     Color = BrightRed
	ColorBrightGreen   Color = BrightGreen
	ColorBrightYellow  Color = BrightYellow
	ColorBrightBlue    Color = BrightBlue
	ColorBrightMagenta Color = BrightMagenta
	ColorBrightCyan    Color = BrightCyan
	ColorBrightWhite   Color = BrightWhite
)

// Game-specific color schemes
var (
	// Health colors
	HealthCritical = ColorBrightRed
	HealthLow      = ColorRed
	HealthMedium   = ColorYellow
	HealthHigh     = ColorGreen
	HealthFull     = ColorBrightGreen
	
	// Stamina colors
	StaminaEmpty = ColorRed
	StaminaLow   = ColorYellow
	StaminaHigh  = ColorCyan
	StaminaFull  = ColorBrightCyan
	
	// Item rarity colors
	ItemCommon    = ColorWhite
	ItemUncommon  = ColorGreen
	ItemRare      = ColorBlue
	ItemEpic      = ColorMagenta
	ItemLegendary = ColorYellow
	ItemArtifact  = ColorBrightYellow
	
	// Chat colors
	ChatSay     = ColorWhite
	ChatTell    = ColorCyan
	ChatYell    = ColorYellow
	ChatOOC     = ColorBrightBlack
	ChatAdmin   = ColorBrightRed
	ChatSystem  = ColorBrightBlue
	
	// Combat colors
	CombatAttack = ColorRed
	CombatDefend = ColorBlue
	CombatHeal   = ColorGreen
	CombatMiss   = ColorBrightBlack
	
	// UI colors
	UIPrompt    = ColorBrightWhite
	UIError     = ColorBrightRed
	UISuccess   = ColorBrightGreen
	UIWarning   = ColorBrightYellow
	UIInfo      = ColorBrightCyan
	UISecondary = ColorBrightBlack
)

// GetHealthColor returns the appropriate color for a health percentage
func GetHealthColor(current, max int) Color {
	if max == 0 {
		return HealthCritical
	}
	
	percentage := float64(current) / float64(max)
	
	switch {
	case percentage <= 0.1:
		return HealthCritical
	case percentage <= 0.25:
		return HealthLow
	case percentage <= 0.5:
		return HealthMedium
	case percentage <= 0.75:
		return HealthHigh
	default:
		return HealthFull
	}
}

// GetStaminaColor returns the appropriate color for a stamina percentage
func GetStaminaColor(current, max int) Color {
	if max == 0 {
		return StaminaEmpty
	}
	
	percentage := float64(current) / float64(max)
	
	switch {
	case percentage <= 0.1:
		return StaminaEmpty
	case percentage <= 0.5:
		return StaminaLow
	case percentage <= 0.75:
		return StaminaHigh
	default:
		return StaminaFull
	}
}

// StripANSI removes ANSI escape codes from text
func StripANSI(text string) string {
	// Simple regex would be better, but avoiding external dependencies
	// This is a basic implementation that handles most common cases
	result := strings.Builder{}
	inEscape := false
	
	for i, r := range text {
		if r == '\033' && i+1 < len(text) && text[i+1] == '[' {
			inEscape = true
			continue
		}
		
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		
		result.WriteRune(r)
	}
	
	return result.String()
}

// Length returns the display length of text (excluding ANSI codes)
func Length(text string) int {
	return len(StripANSI(text))
}