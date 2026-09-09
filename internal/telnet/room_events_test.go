package telnet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSceneChanges(t *testing.T) {
	before := &roomScene{Items: map[string]sceneEntry{"loot": {"Coin", 2}}, NPCs: map[string]sceneEntry{"old": {"Rat", 1}}}
	after := &roomScene{Items: map[string]sceneEntry{"loot": {"Coin", 4}}, NPCs: map[string]sceneEntry{"new": {"Goblin", 1}}}
	require.Equal(t, []string{"Coin appears here. (+2)", "Goblin appears here. (+1)", "Rat is no longer here. (-1)"}, sceneChanges(before, after))
	require.Empty(t, sceneChanges(after, after))
}
