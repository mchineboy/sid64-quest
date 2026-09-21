package game

import (
	"fmt"
	"strings"
)

const (
	CopperPerSilver int64 = 100
	CopperPerGold   int64 = 10000
)

// GoldValue converts a configured or authored gold amount to storage units.
func GoldValue(gold int) int64 {
	return int64(gold) * CopperPerGold
}

// FormatCurrency renders one stored balance as conventional denominations.
func FormatCurrency(value int64) string {
	if value < 0 {
		value = 0
	}
	gold := value / CopperPerGold
	silver := value % CopperPerGold / CopperPerSilver
	copper := value % CopperPerSilver
	parts := make([]string, 0, 3)
	if gold > 0 {
		parts = append(parts, fmt.Sprintf("%d gold", gold))
	}
	if silver > 0 {
		parts = append(parts, fmt.Sprintf("%d silver", silver))
	}
	if copper > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d copper", copper))
	}
	return strings.Join(parts, ", ")
}
