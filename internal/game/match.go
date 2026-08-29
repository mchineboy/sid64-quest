package game

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNoMatch   = errors.New("no matching name")
	ErrAmbiguous = errors.New("ambiguous name")
)

func NormalizeDirection(command string) string {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "n", "north":
		return "north"
	case "s", "south":
		return "south"
	case "e", "east":
		return "east"
	case "west":
		return "west"
	default:
		return ""
	}
}

// MatchesName reports whether query identifies name. A query matches when it
// equals the name, is a substring of it, or every query word appears in it.
func MatchesName(name, query string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" || n == "" {
		return false
	}
	if n == q || strings.Contains(n, q) {
		return true
	}
	for _, word := range strings.Fields(q) {
		if !strings.Contains(n, word) {
			return false
		}
	}
	return true
}

func findNamed[T any](items []T, query string, name func(T) string) (T, error) {
	var zero T
	var matches []T
	for _, item := range items {
		if MatchesName(name(item), query) {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return zero, fmt.Errorf("%w: %s", ErrNoMatch, query)
	case 1:
		return matches[0], nil
	default:
		return zero, fmt.Errorf("%w: %s", ErrAmbiguous, query)
	}
}
