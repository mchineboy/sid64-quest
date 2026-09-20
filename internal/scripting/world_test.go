package scripting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorldBuilderValidation(t *testing.T) {
	for name, source := range map[string]string{
		"empty": "x = 1",
		"duplicate": `room("a", "A", "Room A")
room("a", "B", "Room B")`,
		"unknown": `room("a", "A", "Room A")
link("a", "north", "missing")`,
		"disconnected": `room("a", "A", "Room A")
room("b", "B", "Room B")`,
		"collision": `room("a", "A", "Room A")
room("b", "B", "Room B")
room("c", "C", "Room C")
link("a", "north", "b")
link("c", "north", "b")`,
		"direction": `room("a", "A", "Room A")
link("a", "sideways", "b")`,
		"self": `room("a", "A", "Room A")
link("a", "north", "a")`,
		"controls": `room("a", "A\x1b", "Room A")`,
		"path":     `room("a", "A", "Room A", script="../secret")`,
		"limit": `def build():
    for n in range(65):
        room("a"+str(n), "Room", "Description")
build()`,
		"effects": `award_gold(100)`,
	} {
		t.Run(name, func(t *testing.T) {
			out, err := Run(context.Background(), Input{Kind: "world", Source: source})
			require.Error(t, err)
			require.Nil(t, out.World)
		})
	}
	out, err := Run(context.Background(), Input{Kind: "world", Source: `room("a", "A", "Room A")
room("b", "B", "Room B", kind="dungeon")
link("a", "down", "b")`})
	require.NoError(t, err)
	require.Equal(t, "b", out.World.Rooms[0].Exits["down"])
	require.Equal(t, "a", out.World.Rooms[1].Exits["up"])
	// World-building APIs are not exposed to player event hooks.
	_, err = Run(context.Background(), Input{Kind: "room", Hook: "on_enter", Source: `def on_enter(e):
    room("a", "A", "Room A")`})
	require.Error(t, err)
}

func TestMonsterDefinitions(t *testing.T) {
	prefix := `room("a", "A", "Room A")
`
	for _, source := range []string{
		`monster("m","missing","Monster","Description")`,
		`monster("m","a","Monster","Description",health=0)`,
		`monster("m","a","Monster","Description",attack=-1)`,
		`monster("m","a","Monster","Description",gold=101)`,
		`monster("m","a","Monster","Description",respawn=0)`,
		`monster("m","a","Monster","Description")
monster("m","a","Monster","Description")`,
	} {
		_, err := Run(context.Background(), Input{Kind: "world", Source: prefix + source})
		require.Error(t, err)
	}
	_, err := Run(context.Background(), Input{Kind: "world", Source: `room("a","A","Room A",kind="safe")
monster("m","a","Monster","Description")`})
	require.Error(t, err)
	out, err := Run(context.Background(), Input{Kind: "world", Source: prefix + `monster("m","a","Monster","Description",health=30,gold=10)`})
	require.NoError(t, err)
	require.Len(t, out.World.Monsters, 1)
	require.Equal(t, 30, out.World.Monsters[0].Health)
}
