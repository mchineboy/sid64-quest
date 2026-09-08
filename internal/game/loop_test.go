package game

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	cfg := config.LoadFromEnv()
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestPersistentPlayerLoop(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	world := NewWorldService(db)
	require.NoError(t, world.EnsureStarterWorld(ctx))

	userID := uuid.New()
	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, username, email, password_hash)
		VALUES ($1, $2, $3, 'test-hash')`,
		userID, "loop_"+userID.String()[:8], userID.String()+"@example.test")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID)
	})

	var squareID, docksID, innID uuid.UUID
	require.NoError(t, db.QueryRowContext(ctx, `SELECT id FROM rooms WHERE name = 'Town Square' ORDER BY created_at LIMIT 1`).Scan(&squareID))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT id FROM rooms WHERE name = 'Moonlit Docks' ORDER BY created_at LIMIT 1`).Scan(&docksID))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT id FROM rooms WHERE name = 'The Prancing Pony Inn' ORDER BY created_at LIMIT 1`).Scan(&innID))
	placeNamedItem(t, db, docksID, itemManifest)
	placeNamedItem(t, db, innID, itemHealthPotion)

	var characterID uuid.UUID
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO characters (user_id, name, health, stamina, gold, current_room_id)
		VALUES ($1, $2, 40, 40, 10, $3)
		RETURNING id`, userID, "Tester "+userID.String()[:8], docksID).Scan(&characterID))

	item, err := world.TakeItem(ctx, characterID, docksID, "manifest")
	require.NoError(t, err)
	require.Equal(t, itemManifest, item.Name)
	require.Len(t, mustInventory(t, world, characterID), 1)

	_, _, err = world.GiveItem(ctx, characterID, docksID, "manifest", "")
	require.Error(t, err)

	_, to, stamina, err := world.MoveCharacter(ctx, characterID, "south")
	require.NoError(t, err)
	require.Equal(t, "North Gate", to.Name)
	require.Equal(t, 39, stamina)
	_, to, _, err = world.MoveCharacter(ctx, characterID, "south")
	require.NoError(t, err)
	require.Equal(t, "Town Square", to.Name)

	message, character, err := world.GiveItem(ctx, characterID, squareID, "manifest", "crier")
	require.NoError(t, err)
	require.Contains(t, message, "gold")
	require.Equal(t, int64(25), character.Gold)
	require.Equal(t, 1, character.Deliveries)
	require.Empty(t, mustInventory(t, world, characterID))

	ground, err := world.ListRoomItems(ctx, docksID)
	require.NoError(t, err)
	require.True(t, hasItem(ground, itemManifest), "manifest should respawn at the docks")

	_, err = world.TakeItem(ctx, characterID, innID, "health")
	require.Error(t, err)

	_, err = db.ExecContext(ctx, `UPDATE characters SET current_room_id = $1, health = 40 WHERE id = $2`, innID, characterID)
	require.NoError(t, err)
	potion, err := world.TakeItem(ctx, characterID, innID, "health")
	require.NoError(t, err)
	require.Equal(t, itemHealthPotion, potion.Name)

	useMessage, character, err := world.UseItem(ctx, characterID, "potion")
	require.NoError(t, err)
	require.Contains(t, useMessage, "Health is now")
	require.Greater(t, character.Health, 40)

	restMessage, character, err := world.Rest(ctx, characterID, innID)
	require.NoError(t, err)
	require.Contains(t, restMessage, "restored")
	require.Equal(t, character.MaxHealth, character.Health)
	require.Equal(t, character.MaxStamina, character.Stamina)

	reloaded, err := world.LoadCharacter(ctx, characterID)
	require.NoError(t, err)
	require.Equal(t, innID, *reloaded.CurrentRoomID)
	require.Equal(t, 1, reloaded.Deliveries)
	require.Equal(t, int64(25), reloaded.Gold)
}

func TestTakeDropEquipPersists(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	world := NewWorldService(db)
	require.NoError(t, world.EnsureStarterWorld(ctx))

	userID := uuid.New()
	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, username, email, password_hash)
		VALUES ($1, $2, $3, 'test-hash')`,
		userID, "eq_"+userID.String()[:8], userID.String()+"@example.test")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID)
	})

	var marketID uuid.UUID
	require.NoError(t, db.QueryRowContext(ctx, `SELECT id FROM rooms WHERE name = 'Market Lane' ORDER BY created_at LIMIT 1`).Scan(&marketID))
	placeNamedItem(t, db, marketID, itemRustySword)

	var characterID uuid.UUID
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO characters (user_id, name, current_room_id)
		VALUES ($1, $2, $3)
		RETURNING id`, userID, "Equipper "+userID.String()[:8], marketID).Scan(&characterID))

	sword, err := world.TakeItem(ctx, characterID, marketID, "sword")
	require.NoError(t, err)
	require.Equal(t, itemRustySword, sword.Name)

	message, err := world.EquipItem(ctx, characterID, "sword")
	require.NoError(t, err)
	require.Contains(t, message, "wield")

	inventory := mustInventory(t, world, characterID)
	require.True(t, inventory[0].Equipped)

	_, err = world.DropItem(ctx, characterID, marketID, "sword")
	require.Error(t, err)

	_, err = world.UnequipItem(ctx, characterID, "sword")
	require.NoError(t, err)
	_, err = world.DropItem(ctx, characterID, marketID, "sword")
	require.NoError(t, err)
	require.Empty(t, mustInventory(t, world, characterID))
}

func mustInventory(t *testing.T, world *WorldService, characterID uuid.UUID) []*models.InventoryItem {
	t.Helper()
	items, err := world.ListInventory(context.Background(), characterID)
	require.NoError(t, err)
	return items
}

func placeNamedItem(t *testing.T, db *sql.DB, roomID uuid.UUID, name string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO room_items (room_id, item_id, quantity)
		SELECT $1, id, 1 FROM items WHERE name = $2
		ON CONFLICT (room_id, item_id) DO UPDATE SET quantity = GREATEST(room_items.quantity, 1)`,
		roomID, name)
	require.NoError(t, err)
}

func hasItem(items []GroundItem, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
