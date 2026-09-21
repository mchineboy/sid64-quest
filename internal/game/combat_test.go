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
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

func TestCombatEquipment(t *testing.T) {
	attack, armor := combatEquipment(nil)
	require.Equal(t, 5, attack)
	require.Zero(t, armor)
	items := []*models.InventoryItem{
		{Quantity: 1, Equipped: true, Item: &models.Item{ItemType: "weapon", Properties: map[string]interface{}{"damage": 5}}},
		{Quantity: 1, Equipped: true, Item: &models.Item{ItemType: "armor", Properties: map[string]interface{}{"defense": 3}}},
		{Quantity: 1, Equipped: false, Item: &models.Item{ItemType: "weapon", Properties: map[string]interface{}{"damage": 999}}},
	}
	attack, armor = combatEquipment(items)
	require.Equal(t, 10, attack)
	require.Equal(t, 3, armor)
}

func TestPvECombatPersistenceAndSafety(t *testing.T) {
	parent := openTestDB(t)
	ctx := context.Background()
	name := "rck_combat_" + uuid.New().String()[:8]
	_, err := parent.Exec(`CREATE DATABASE ` + name)
	require.NoError(t, err)
	cfg := config.LoadFromEnv()
	cfg.Database.PostgreSQL.Database = name
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close(); _, err := parent.Exec(`DROP DATABASE ` + name); require.NoError(t, err) })
	require.NoError(t, database.Migrate(ctx, db))
	ws := NewWorldService(db)
	require.NoError(t, ws.EnsureStarterWorld(ctx))
	roomID := func(key string) uuid.UUID {
		var id uuid.UUID
		require.NoError(t, db.QueryRow(`SELECT room_id FROM world_content_rooms WHERE content_key=$1`, key).Scan(&id))
		return id
	}
	choir, hall := roomID("crypt_choir"), roomID("hall_returning")
	sentinel := uuid.NewSHA1(uuid.NameSpaceURL, []byte("sid64.quest/world/monsters/crypt_sentinel"))
	user, player, other := uuid.New(), uuid.New(), uuid.New()
	_, err = db.Exec(`INSERT INTO users(id,username,email,password_hash) VALUES($1,'fighter','fighter@test.invalid','test')`, user)
	require.NoError(t, err)
	for i, id := range []uuid.UUID{player, other} {
		_, err = db.Exec(`INSERT INTO characters(id,user_id,name,current_room_id,health,stamina,gold) VALUES($1,$2,$3,$4,100,100,0)`, id, user, []string{"Fighter", "Bystander"}[i], choir)
		require.NoError(t, err)
	}
	for _, query := range []string{"Bystander", other.String(), "", "not here"} {
		_, err = ws.Attack(ctx, player, choir, query)
		require.Error(t, err)
	}
	untouched, err := ws.LoadCharacter(ctx, other)
	require.NoError(t, err)
	require.Equal(t, 100, untouched.Health)
	_, err = ws.Attack(ctx, player, hall, "sentinel")
	require.Error(t, err, "reject stale/forged location")
	_, err = db.Exec(`INSERT INTO npcs(name,description,room_id) VALUES('Friendly Watcher','A friend',$1)`, choir)
	require.NoError(t, err)
	_, err = ws.Attack(ctx, player, choir, "watcher")
	require.ErrorContains(t, err, "friendly")
	_, err = db.Exec(`INSERT INTO inventory(character_id,item_id,equipped) SELECT $1,id,true FROM items WHERE name IN ('Rusty Sword','Leather Armor')`, player)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE npcs SET loot_chance=10000 WHERE id=$1`, sentinel)
	require.NoError(t, err)
	round, err := ws.Attack(ctx, player, choir, "sentinel")
	require.NoError(t, err)
	require.Contains(t, round.Message, "9 damage")
	current, err := ws.LoadCharacter(ctx, player)
	require.NoError(t, err)
	require.Equal(t, 97, current.Health)
	require.Equal(t, 98, current.Stamina)
	require.NoError(t, ws.EnsureStarterWorld(ctx))
	var enemyHP int
	require.NoError(t, db.QueryRow(`SELECT health FROM npcs WHERE id=$1`, sentinel).Scan(&enemyHP))
	require.Equal(t, 15, enemyHP, "restart keeps injuries")
	_, err = ws.Attack(ctx, player, choir, "sentinel")
	require.NoError(t, err)
	round, err = ws.Attack(ctx, player, choir, "sentinel")
	require.NoError(t, err)
	require.Contains(t, round.Message, "Victory")
	current, err = ws.LoadCharacter(ctx, player)
	require.NoError(t, err)
	require.Equal(t, 94, current.Health, "no retaliation on lethal hit")
	require.Zero(t, current.Gold, "coin remains on the corpse until looted")
	require.Equal(t, int64(15), current.Experience)
	message, err := ws.LootCorpse(ctx, player, choir, "sentinel")
	require.NoError(t, err)
	require.Contains(t, message, "Warden Mail")
	current, err = ws.LoadCharacter(ctx, player)
	require.NoError(t, err)
	require.Greater(t, current.Gold, GoldValue(8))
	require.Less(t, current.Gold, GoldValue(9))
	_, err = ws.Attack(ctx, player, choir, "sentinel")
	require.Error(t, err)
	npcs, err := ws.ListRoomNPCs(ctx, choir)
	require.NoError(t, err)
	for _, npc := range npcs {
		require.NotEqual(t, sentinel, npc.ID)
	}
	require.NoError(t, ws.EnsureStarterWorld(ctx))
	require.NoError(t, db.QueryRow(`SELECT health FROM npcs WHERE id=$1`, sentinel).Scan(&enemyHP))
	require.Zero(t, enemyHP, "startup must not respawn early")
	_, err = db.Exec(`UPDATE npcs SET respawn_at=now()-interval '1 second' WHERE id=$1`, sentinel)
	require.NoError(t, err)
	npcs, err = ws.ListRoomNPCs(ctx, choir)
	require.NoError(t, err)
	found := false
	for _, npc := range npcs {
		if npc.ID == sentinel {
			found = true
			require.Equal(t, 24, npc.Health)
		}
	}
	require.True(t, found)
	// Two players racing for a final blow can earn only one reward.
	_, err = db.Exec(`UPDATE npcs SET health=1 WHERE id=$1`, sentinel)
	require.NoError(t, err)
	var wg sync.WaitGroup
	success := make(chan bool, 2)
	for _, id := range []uuid.UUID{player, other} {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			_, err := ws.Attack(ctx, id, choir, "sentinel")
			success <- err == nil
		}(id)
	}
	wg.Wait()
	close(success)
	wins := 0
	for ok := range success {
		if ok {
			wins++
		}
	}
	require.Equal(t, 1, wins)
	var winner uuid.UUID
	require.NoError(t, db.QueryRow(`SELECT owner_id FROM monster_corpses WHERE npc_id=$1`, sentinel).Scan(&winner))
	_, err = ws.LootCorpse(ctx, winner, choir, "sentinel")
	require.NoError(t, err)
	var total int
	require.NoError(t, db.QueryRow(`SELECT sum(gold) FROM characters WHERE user_id=$1`, user).Scan(&total))
	require.Greater(t, int64(total), GoldValue(16))
	// Exhaustion rejects the entire action, then death relocates without losses.
	gallery := roomID("mine_gallery")
	_, err = db.Exec(`UPDATE characters SET current_room_id=$2,health=1,stamina=1,gold=$3 WHERE id=$1`, player, gallery, GoldValue(150))
	require.NoError(t, err)
	_, err = ws.Attack(ctx, player, gallery, "rat")
	require.ErrorContains(t, err, "stamina")
	_, err = db.Exec(`UPDATE characters SET stamina=20 WHERE id=$1`, player)
	require.NoError(t, err)
	before, err := ws.LoadCharacter(ctx, player)
	require.NoError(t, err)
	round, err = ws.Attack(ctx, player, gallery, "rat")
	require.NoError(t, err)
	require.True(t, round.Defeated)
	require.Equal(t, hall, round.RoomID)
	current, err = ws.LoadCharacter(ctx, player)
	require.NoError(t, err)
	require.Zero(t, current.Health)
	require.True(t, current.IsDead)
	require.Equal(t, before.Gold, current.Gold)
	inventory, err := ws.ListInventory(ctx, player)
	require.NoError(t, err)
	require.Len(t, inventory, 3)
	_, _, _, err = ws.MoveCharacter(ctx, player, "west")
	require.ErrorContains(t, err, "dead")
	_, err = ws.Resurrect(ctx, player, true, 100)
	require.NoError(t, err)
	current, err = ws.LoadCharacter(ctx, player)
	require.NoError(t, err)
	require.False(t, current.IsDead)
	require.Equal(t, 50, current.Health)
	require.Equal(t, GoldValue(50), current.Gold)
	afterResurrectionGold := current.Gold
	// Core command checkpoint rollback also rolls back combat and rewards.
	_, err = db.Exec(`UPDATE characters SET current_room_id=$2 WHERE id=$1`, player, choir)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE npcs SET health=1,respawn_at=NULL WHERE id=$1`, sentinel)
	require.NoError(t, err)
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = NewTransactionalWorldService(tx).Attack(ctx, player, choir, "sentinel")
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, db.QueryRow(`SELECT health FROM npcs WHERE id=$1`, sentinel).Scan(&enemyHP))
	require.Equal(t, 1, enemyHP)
	current, err = ws.LoadCharacter(ctx, player)
	require.NoError(t, err)
	require.Equal(t, afterResurrectionGold, current.Gold)
	// Even a hostile NPC placed by an operator in a safe room cannot be fought.
	_, err = db.Exec(`UPDATE rooms SET room_type='safe' WHERE id=$1`, choir)
	require.NoError(t, err)
	_, err = ws.Attack(ctx, player, choir, "sentinel")
	require.ErrorContains(t, err, "not allowed")
}
