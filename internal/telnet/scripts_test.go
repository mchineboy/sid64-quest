package telnet

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tylerhardison/race-condition-kingdom/internal/database"
	"github.com/tylerhardison/race-condition-kingdom/internal/game"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

func TestScriptSourceIndentation(t *testing.T) {
	c, _ := drainedConnection(t)
	c.Reader = bufio.NewReader(strings.NewReader("    tell(\"hello\")  \r\n"))
	line, err := c.ReadLine()
	require.NoError(t, err)
	require.Equal(t, "    tell(\"hello\")  ", line)
}

func TestInGameScriptingWorkflow(t *testing.T) {
	cfg := config.LoadFromEnv()
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Skipf("Postgres unavailable: %v", err)
	}
	require.NoError(t, database.Migrate(context.Background(), db))
	owner, room, character, npc, item := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err = db.Exec(`INSERT INTO users(id,username,email,password_hash,permissions) VALUES($1,$2,$3,'test','{"admin":true}')`, owner, "editor_"+owner.String()[:8], owner.String()+"@test.invalid")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO rooms(id,name,description) VALUES($1,'Editor room','A room')`, room)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO characters(id,user_id,name,current_room_id) VALUES($1,$2,'Editor',$3)`, character, owner, room)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO npcs(id,name,description,room_id) VALUES($1,'Oracle','A seer',$2)`, npc, room)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO items(id,name,description,item_type) VALUES($1,'Crystal','A crystal','trinket')`, item)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO inventory(character_id,item_id,quantity) VALUES($1,$2,1)`, character, item)
	require.NoError(t, err)
	defer func() {
		db.Exec(`DELETE FROM inventory WHERE character_id=$1`, character)
		db.Exec(`DELETE FROM npcs WHERE id=$1`, npc)
		db.Exec(`DELETE FROM items WHERE id=$1`, item)
		db.Exec(`DELETE FROM characters WHERE id=$1`, character)
		db.Exec(`DELETE FROM rooms WHERE id=$1`, room)
		db.Exec(`DELETE FROM scripts WHERE created_by=$1`, owner)
		db.Exec(`DELETE FROM users WHERE id=$1`, owner)
	}()
	c, collect := drainedConnection(t)
	c.State = StateInGame
	// Real authenticated connections identify their owner through Character.
	c.User = nil
	c.Character = &models.Character{ID: character, UserID: owner, Name: "Editor"}
	c.Room = &models.Room{ID: room, Name: "Editor room"}
	s := &Server{world: game.NewWorldService(db), hub: newPlayerHub()}
	command := func(input string) { require.NoError(t, s.processInput(c, input)) }
	command("script new room puzzle")
	var id uuid.UUID
	require.NoError(t, db.QueryRow(`SELECT id FROM scripts WHERE created_by=$1`, owner).Scan(&id))
	command("script edit " + id.String())
	require.NotNil(t, c.ScriptEditor)
	command(".set 1 def on_command(event):")
	command(".set 2     tell(\"A door opens.\")")
	command("    return True")
	command(".save")
	require.Nil(t, c.ScriptEditor)
	command("script test " + id.String() + " on_command knock")
	command("script publish " + id.String() + " 2")
	command("script attach " + id.String() + " here")
	command("knock")
	command("script history " + id.String())
	for _, v := range []struct {
		kind, hook, command, text string
		target                    uuid.UUID
	}{{"npc", "on_talk", "talk Oracle", "The oracle speaks.", npc}, {"item", "on_use", "use Crystal", "The crystal glows.", item}} {
		ds, err := s.world.ScriptCommand(c.Context, owner, game.ScriptRequest{Action: "new", Kind: v.kind, Name: "test-" + v.kind})
		require.NoError(t, err)
		sid := ds[0].ID
		_, err = s.world.ScriptCommand(c.Context, owner, game.ScriptRequest{Action: "save", ID: sid, Revision: 1, Content: fmt.Sprintf("def %s(event):\n    tell(%q)\n    return True\n", v.hook, v.text)})
		require.NoError(t, err)
		command(fmt.Sprintf("script publish %s 2", sid))
		command(fmt.Sprintf("script attach %s %s", sid, v.target))
		command(v.command)
	}
	command("script edit " + id.String())
	_, err = db.Exec(`UPDATE users SET permissions='{}' WHERE id=$1`, owner)
	require.NoError(t, err)
	command(".save")
	require.Nil(t, c.ScriptEditor)
	seen := collect()
	require.Contains(t, seen, "[preview you] A door opens.")
	require.Contains(t, seen, "The oracle speaks.")
	require.Contains(t, seen, "The crystal glows.")
	require.Contains(t, seen, "builder permission required")
	require.NotContains(t, seen, "Unknown command: knock")
	require.NotContains(t, seen, "nothing useful to say")
}
