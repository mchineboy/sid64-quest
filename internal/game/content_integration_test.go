package game

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tylerhardison/race-condition-kingdom/internal/database"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

func TestWorldContentUpgradeAndPersistence(t *testing.T) {
	parent := openTestDB(t)
	ctx := context.Background()
	name := "rck_content_" + uuid.New().String()[:8]
	_, err := parent.ExecContext(ctx, `CREATE DATABASE `+name)
	require.NoError(t, err)
	cfg := config.LoadFromEnv()
	cfg.Database.PostgreSQL.Database = name
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
		_, err := parent.Exec(`DROP DATABASE ` + name)
		if err != nil {
			t.Errorf("drop test database: %v", err)
		}
	})
	require.NoError(t, database.Migrate(ctx, db))
	// Adopt an existing starter room without moving its saved character.
	square, user, character := uuid.New(), uuid.New(), uuid.New()
	_, err = db.Exec(`INSERT INTO rooms(id,name,description) VALUES($1,'Town Square','Old square')`, square)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO users(id,username,email,password_hash,permissions) VALUES($1,'explorer','explorer@test.invalid','test','{"admin":true}')`, user)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO characters(id,user_id,name,current_room_id) VALUES($1,$2,'Explorer',$3)`, character, user, square)
	require.NoError(t, err)
	ws := NewWorldService(db)
	require.NoError(t, ws.EnsureStarterWorld(ctx))
	saved, err := ws.LoadCharacter(ctx, character)
	require.NoError(t, err)
	require.Equal(t, square, *saved.CurrentRoomID)
	for _, direction := range []string{"w", "w", "d", "n", "n"} {
		_, _, _, err = ws.MoveCharacter(ctx, character, direction)
		require.NoError(t, err)
	}
	saved, err = ws.LoadCharacter(ctx, character)
	require.NoError(t, err)
	finale := *saved.CurrentRoomID
	room, err := ws.GetRoom(ctx, finale)
	require.NoError(t, err)
	require.Equal(t, "Bellkeeper Reliquary", room.Name)
	// Concurrent solves award once, with state and reward committed together.
	var wg sync.WaitGroup
	rewards := make(chan int, 2)
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := ws.RunScript(ctx, character, finale, finale, "room", "on_command", "answer", "bell")
			rewards <- out.Gold
			errors <- err
		}()
	}
	wg.Wait()
	close(rewards)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	total := 0
	for reward := range rewards {
		total += reward
	}
	require.Equal(t, 30, total)
	// Admin can edit a bundled script whose owner is NULL.
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("sid64.quest/world/scripts/crypt"))
	drafts, err := ws.ScriptCommand(ctx, user, ScriptRequest{Action: "show", ID: id})
	require.NoError(t, err)
	_, err = ws.ScriptCommand(ctx, user, ScriptRequest{Action: "save", ID: id, Revision: drafts[0].Revision, Content: drafts[0].Content + "\n# operator edit\n"})
	require.NoError(t, err)
	// Startup retains edits, detached hooks, extra exits and puzzle progress.
	_, err = db.Exec(`UPDATE rooms SET script_id=NULL,exits=exits || jsonb_build_object('west',$2::text) WHERE id=$1`, square, finale)
	require.NoError(t, err)
	ws = NewWorldService(db)
	require.NoError(t, ws.EnsureStarterWorld(ctx))
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM rooms`).Scan(&count))
	require.Equal(t, 83, count)
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM scripts`).Scan(&count))
	require.Equal(t, 8, count)
	var detached bool
	require.NoError(t, db.QueryRow(`SELECT script_id IS NULL FROM rooms WHERE id=$1`, square).Scan(&detached))
	require.True(t, detached)
	room, err = ws.GetRoom(ctx, square)
	require.NoError(t, err)
	require.Equal(t, finale, room.Exits["west"])
	drafts, err = ws.ScriptCommand(ctx, user, ScriptRequest{Action: "show", ID: id})
	require.NoError(t, err)
	require.Contains(t, drafts[0].Content, "# operator edit")
	out, err := ws.RunScript(ctx, character, finale, finale, "room", "on_command", "answer", "bell")
	require.NoError(t, err)
	require.Zero(t, out.Gold)
	saved, err = ws.LoadCharacter(ctx, character)
	require.NoError(t, err)
	require.Equal(t, GoldValue(30), saved.Gold)
	_, room, _, err = ws.MoveCharacter(ctx, character, "south")
	require.NoError(t, err)
	_, room, _, err = ws.MoveCharacter(ctx, character, "south")
	require.NoError(t, err)
	_, room, _, err = ws.MoveCharacter(ctx, character, "u")
	require.NoError(t, err)
	require.Equal(t, "Lantern Cemetery", room.Name)
	// Simulate an older pack without the tower, with an operator occupying its
	// proposed entrance. A failed extension must roll back every inserted room.
	var tower, spring, crown uuid.UUID
	require.NoError(t, db.QueryRow(`SELECT room_id FROM world_content_rooms WHERE content_key='tower'`).Scan(&tower))
	require.NoError(t, db.QueryRow(`SELECT room_id FROM world_content_rooms WHERE content_key='spring'`).Scan(&spring))
	require.NoError(t, db.QueryRow(`SELECT room_id FROM world_content_rooms WHERE content_key='highland_crown'`).Scan(&crown))
	_, err = db.Exec(`DELETE FROM world_content_rooms WHERE content_key='tower'`)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM rooms WHERE id=$1`, tower)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE rooms SET exits=exits-'down' WHERE id=$1`, crown)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE rooms SET exits=exits || jsonb_build_object('east',$2::text) WHERE id=$1`, spring, finale)
	require.NoError(t, err)
	require.ErrorContains(t, ws.EnsureStarterWorld(ctx), "new content conflicts")
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM rooms`).Scan(&count))
	require.Equal(t, 82, count)
	_, err = db.Exec(`UPDATE rooms SET exits=exits-'east' WHERE id=$1`, spring)
	require.NoError(t, err)
	require.NoError(t, ws.EnsureStarterWorld(ctx))
	require.NoError(t, db.QueryRow(`SELECT room_id FROM world_content_rooms WHERE content_key='tower'`).Scan(&tower))
	room, err = ws.GetRoom(ctx, spring)
	require.NoError(t, err)
	require.Equal(t, tower, room.Exits["east"])
	room, err = ws.GetRoom(ctx, tower)
	require.NoError(t, err)
	require.Equal(t, spring, room.Exits["west"])

}
