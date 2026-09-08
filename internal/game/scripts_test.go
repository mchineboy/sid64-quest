package game

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tylerhardison/race-condition-kingdom/internal/database"
)

func TestScriptingLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	require.NoError(t, database.Migrate(ctx, db))
	ws := NewWorldService(db)
	builder, admin, player, room, character := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, v := range []struct {
		id   uuid.UUID
		role string
	}{{builder, "builder"}, {admin, "admin"}, {player, "player"}} {
		_, err := db.Exec(`INSERT INTO users(id,username,email,password_hash,permissions) VALUES($1,$2,$3,'test',$4)`, v.id, "script_"+v.id.String()[:8], v.id.String()+"@test.invalid", fmt.Sprintf(`{"%s":true}`, v.role))
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM script_audit WHERE actor_id IN ($1,$2,$3)`, builder, admin, player)
		db.Exec(`DELETE FROM characters WHERE id=$1`, character)
		db.Exec(`DELETE FROM rooms WHERE id=$1`, room)
		db.Exec(`DELETE FROM scripts WHERE created_by=$1`, builder)
		db.Exec(`DELETE FROM users WHERE id IN ($1,$2,$3)`, builder, admin, player)
	})
	_, err := db.Exec(`INSERT INTO rooms(id,name,description) VALUES($1,'Script room','A room')`, room)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO characters(id,user_id,name,current_room_id) VALUES($1,$2,$4,$3)`, character, player, room, "Script "+character.String()[:8])
	require.NoError(t, err)
	_, err = ws.ScriptCommand(ctx, player, ScriptRequest{Action: "list"})
	require.Error(t, err)
	drafts, err := ws.ScriptCommand(ctx, builder, ScriptRequest{Action: "new", Kind: "room", Name: "welcome"})
	require.NoError(t, err)
	id := drafts[0].ID
	_, err = ws.ScriptCommand(ctx, player, ScriptRequest{Action: "show", ID: id})
	require.Error(t, err)
	source := `def on_enter(e):
    count = int(get_state("count", "0")) + 1
    set_state("count", str(count))
    tell("visit " + str(count))
    award_gold(7)
    heal(5)
`
	_, err = ws.ScriptCommand(ctx, builder, ScriptRequest{Action: "save", ID: id, Revision: 1, Content: source})
	require.NoError(t, err)
	_, err = ws.ScriptCommand(ctx, builder, ScriptRequest{Action: "save", ID: id, Revision: 1, Content: "stale"})
	require.Error(t, err)
	_, err = ws.ScriptCommand(ctx, builder, ScriptRequest{Action: "publish", ID: id, Revision: 2})
	require.Error(t, err)
	_, err = ws.ScriptCommand(ctx, admin, ScriptRequest{Action: "publish", ID: id, Revision: 1})
	require.Error(t, err)
	_, err = ws.ScriptCommand(ctx, admin, ScriptRequest{Action: "publish", ID: id, Revision: 2})
	require.NoError(t, err)
	_, err = ws.ScriptCommand(ctx, builder, ScriptRequest{Action: "attach", ID: id, Target: room})
	require.Error(t, err)
	_, err = ws.ScriptCommand(ctx, admin, ScriptRequest{Action: "attach", ID: id, Target: room})
	require.NoError(t, err)
	run := func() string {
		out, err := ws.RunScript(ctx, character, room, room, "room", "on_enter", "", "")
		require.NoError(t, err)
		require.Len(t, out.Messages, 1)
		return out.Messages[0].Text
	}
	require.Equal(t, "visit 1", run())
	// Saving even broken drafts never changes the published source.
	_, err = ws.ScriptCommand(ctx, builder, ScriptRequest{Action: "save", ID: id, Revision: 2, Content: "broken syntax!"})
	require.NoError(t, err)
	_, err = ws.ScriptCommand(ctx, admin, ScriptRequest{Action: "publish", ID: id, Revision: 3})
	require.Error(t, err)
	require.Equal(t, "visit 2", run())
	// Runtime failures discard state and output.
	broken := `def on_enter(e):
    set_state("count", "999")
    tell("must not appear")
    award_gold(100)
    fail("intentional")
`
	_, err = ws.ScriptCommand(ctx, builder, ScriptRequest{Action: "save", ID: id, Revision: 3, Content: broken})
	require.NoError(t, err)
	_, err = ws.ScriptCommand(ctx, admin, ScriptRequest{Action: "publish", ID: id, Revision: 4})
	require.NoError(t, err)
	out, err := ws.RunScript(ctx, character, room, room, "room", "on_enter", "", "")
	require.Error(t, err)
	require.Empty(t, out.Messages)
	_, err = ws.ScriptCommand(ctx, builder, ScriptRequest{Action: "restore", ID: id, Revision: 2})
	require.NoError(t, err)
	_, err = ws.ScriptCommand(ctx, admin, ScriptRequest{Action: "publish", ID: id, Revision: 5})
	require.NoError(t, err)
	require.Equal(t, "visit 3", run())
	var gold int
	require.NoError(t, db.QueryRow(`SELECT gold FROM characters WHERE id=$1`, character).Scan(&gold))
	require.Equal(t, 21, gold)
	// Concurrent commands must not lose increments or award from stale state.
	var wg sync.WaitGroup
	results := make(chan string, 4)
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := ws.RunScript(ctx, character, room, room, "room", "on_enter", "", "")
			if err != nil {
				failures <- err
				return
			}
			results <- out.Messages[0].Text
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	var visits []string
	for v := range results {
		visits = append(visits, v)
	}
	require.ElementsMatch(t, []string{"visit 4", "visit 5", "visit 6", "visit 7"}, visits)
	// Revocation is immediate, including saves from an already-open editor.
	_, err = db.Exec(`UPDATE users SET permissions='{}' WHERE id=$1`, builder)
	require.NoError(t, err)
	_, err = ws.ScriptCommand(ctx, builder, ScriptRequest{Action: "save", ID: id, Revision: 5, Content: source})
	require.Error(t, err)
	_, err = ws.ScriptCommand(ctx, admin, ScriptRequest{Action: "disable", ID: id})
	require.NoError(t, err)
	out, err = ws.RunScript(ctx, character, room, room, "room", "on_enter", "", "")
	require.NoError(t, err)
	require.Empty(t, out.Messages)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM script_audit WHERE script_id=$1`, id).Scan(&count))
	require.GreaterOrEqual(t, count, 9)
}
