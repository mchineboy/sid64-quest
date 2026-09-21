package telnet

import (
	"context"
	"net"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
	"github.com/tylerhardison/race-condition-kingdom/internal/game"
)

func TestCoreIntegrationCombatReplayAndDefeat(t *testing.T) {
	cache := miniredis.RunT(t)
	host, port, err := net.SplitHostPort(cache.Addr())
	require.NoError(t, err)
	t.Setenv("REDIS_HOST", host)
	t.Setenv("REDIS_PORT", port)
	t.Setenv("REDIS_PASSWORD", "")
	f := newCoreFixture(t)
	user, character := f.player("Dungeon Fighter")
	req := f.login(f.core, "fighter", user, character)
	for _, direction := range []string{"west", "west", "down", "north", "east"} {
		f.request(f.core, req, "input", direction)
	}
	require.Contains(t, outputText(f.request(f.core, req, "input", "look")), "Bronze Sentinel [hostile, HP 24/24]")
	require.Contains(t, outputText(f.request(f.core, req, "input", "look sentinel")), "ceremonial blade")
	green := NewCore(f.db, f.cache, f.cfg, f.core.logger, f.core.Token)
	f.db.SetMaxOpenConns(1)
	req.Sequence++
	req.Kind = "input"
	req.Input = "attack sentinel"
	resp, err := f.core.dispatch(context.Background(), *req)
	require.NoError(t, err)
	require.Contains(t, outputText(resp), "4 damage")
	replay, err := green.dispatch(context.Background(), *req)
	require.NoError(t, err)
	require.Equal(t, outputText(resp), outputText(replay))
	for _, out := range replay.Output {
		req.Ack = out.ID
	}
	var hp, stamina int
	require.NoError(t, f.db.QueryRow(`SELECT health,stamina FROM characters WHERE id=$1`, character).Scan(&hp, &stamina))
	require.Equal(t, 94, hp)
	require.Equal(t, 93, stamina)
	_, err = f.db.Exec(`UPDATE characters SET health=1,gold=$2 WHERE id=$1`, character, game.GoldValue(100))
	require.NoError(t, err)
	resp = f.request(green, req, "input", "hit sentinel")
	require.Contains(t, outputText(resp), "Hall of Returning")
	var room string
	var dead bool
	require.NoError(t, f.db.QueryRow(`SELECT c.health,c.is_dead,r.name FROM characters c JOIN rooms r ON r.id=c.current_room_id WHERE c.id=$1`, character).Scan(&hp, &dead, &room))
	require.Zero(t, hp)
	require.True(t, dead)
	require.Equal(t, "Hall of Returning", room)
	require.Contains(t, outputText(f.request(green, req, "input", "west")), "You are dead")
	require.Contains(t, outputText(f.request(green, req, "input", "resurrect pay")), "return with")
	require.Contains(t, outputText(f.request(green, req, "input", "where")), "Hall of Returning")
}
