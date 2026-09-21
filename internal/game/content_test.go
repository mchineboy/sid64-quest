package game

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tylerhardison/race-condition-kingdom/internal/scripting"
)

func TestBundledWorldRoutes(t *testing.T) {
	world, _, err := bundledWorld(context.Background())
	require.NoError(t, err)
	require.Len(t, world.Rooms, 83)
	rooms := map[string]scripting.WorldRoom{}
	for _, room := range world.Rooms {
		rooms[room.Key] = room
	}
	reverse := map[string]string{"north": "south", "south": "north", "east": "west", "west": "east", "up": "down", "down": "up"}
	for _, room := range rooms {
		for direction, next := range room.Exits {
			require.Equal(t, room.Key, rooms[next].Exits[reverse[direction]])
		}
	}
	for _, route := range []struct {
		end        string
		directions []string
	}{
		{"crypt_stair", []string{"west", "west", "down"}},
		{"mine_lift", []string{"north", "east", "north", "north", "north", "north", "east", "down"}},
		{"grotto_steps", []string{"north", "north", "east", "east", "down"}},
		{"vault_stair", []string{"north", "east", "north", "east", "east", "east", "north", "east", "down"}},
		{"wreck_steps", []string{"south", "south", "south", "south", "south", "east", "south", "south", "down"}},
		{"frost_steps", []string{"north", "east", "north", "north", "north", "east", "east", "up", "north", "north", "east", "north", "down"}},
		{"square", []string{"north", "north", "east", "east", "south", "west", "north", "north", "north"}},
	} {
		here := "square"
		for _, direction := range route.directions {
			here = rooms[here].Exits[direction]
			require.NotEmpty(t, here)
		}
		require.Equal(t, route.end, here)
	}
	require.Equal(t, "docks", rooms["gate"].Exits["north"], "preserve delivery route")
}

func TestDungeonPuzzles(t *testing.T) {
	_, scripts, err := bundledWorld(context.Background())
	require.NoError(t, err)
	for _, tc := range []struct {
		key, room, command string
		answers            []string
		gold               int
	}{
		{"crypt", "Bellkeeper Reliquary", "answer", []string{"bell"}, 30},
		{"mine", "Silvervein Pump Chamber", "crank", []string{"intake", "wheel", "sluice"}, 40},
		{"grotto", "Tideglass Lens Chamber", "align", []string{"moon", "tide", "beacon"}, 50},
		{"vault", "Tithe Strongroom", "press", []string{"grain", "coin", "seal"}, 45},
		{"wreck", "Capstan Deck", "rig", []string{"anchor", "spar", "sail"}, 55},
		{"frost", "Frozen Forge", "stoke", []string{"tinder", "bellows", "flue"}, 60},
	} {
		t.Run(tc.key, func(t *testing.T) {
			in := scripting.Input{Kind: "room", Source: scripts[tc.key], Hook: "on_command", Room: map[string]string{"name": tc.room}, Command: tc.command}
			run := func(answer string) scripting.Output {
				in.Text = answer
				out, err := scripting.Run(context.Background(), in)
				require.NoError(t, err)
				in.State = out.State
				return out
			}
			require.Zero(t, run("wrong").Gold)
			if len(tc.answers) > 1 {
				require.Zero(t, run(tc.answers[0]).Gold)
				require.Zero(t, run(tc.answers[0]).Gold)
				require.Equal(t, "0", in.State["step"], "wrong order resets progress")
			}
			total := 0
			for _, answer := range tc.answers {
				out := run(answer)
				require.True(t, out.Handled)
				total += out.Gold
			}
			require.Equal(t, tc.gold, total)
			require.Equal(t, "yes", in.State["completed"])
			for _, answer := range tc.answers {
				require.Zero(t, run(answer).Gold, "cannot farm rewards")
			}
			in.Room["name"] = "Town Square"
			require.False(t, run(tc.answers[0]).Handled)
			in.Room["name"] = tc.room
			in.State = nil
			total = 0
			for _, answer := range tc.answers {
				total += run(answer).Gold
			}
			require.Equal(t, tc.gold, total, "another player has independent progress")
		})
	}
}
