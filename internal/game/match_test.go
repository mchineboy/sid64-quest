package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchesName(t *testing.T) {
	assert.True(t, MatchesName("Misplaced Manifest", "manifest"))
	assert.True(t, MatchesName("Misplaced Manifest", "misplaced"))
	assert.True(t, MatchesName("Health Potion", "health potion"))
	assert.True(t, MatchesName("Town Crier", "crier"))
	assert.False(t, MatchesName("Town Crier", "docks"))
	assert.False(t, MatchesName("Rusty Sword", ""))
}

func TestNormalizeDirection(t *testing.T) {
	assert.Equal(t, "north", NormalizeDirection("n"))
	assert.Equal(t, "south", NormalizeDirection("S"))
	assert.Equal(t, "east", NormalizeDirection("east"))
	assert.Equal(t, "west", NormalizeDirection("west"))
	assert.Equal(t, "", NormalizeDirection("w"))
	assert.Equal(t, "", NormalizeDirection("up"))
}

func TestPropertyIntAndBool(t *testing.T) {
	props := map[string]interface{}{"healing": 25.0, "quest": true}
	value, ok := propertyInt(props, "healing")
	require.True(t, ok)
	assert.Equal(t, 25, value)
	assert.True(t, propertyBool(props, "quest"))
	assert.False(t, propertyBool(props, "healing"))
	_, ok = propertyInt(props, "missing")
	assert.False(t, ok)
}

func TestFindNamed(t *testing.T) {
	names := []string{"Health Potion", "Stamina Potion"}
	got, err := findNamed(names, "health", func(name string) string { return name })
	require.NoError(t, err)
	assert.Equal(t, "Health Potion", got)

	_, err = findNamed(names, "potion", func(name string) string { return name })
	require.ErrorIs(t, err, ErrAmbiguous)

	_, err = findNamed(names, "ledger", func(name string) string { return name })
	require.ErrorIs(t, err, ErrNoMatch)
}
