package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const (
	itemManifest      = "Misplaced Manifest"
	itemHealthPotion  = "Health Potion"
	itemStaminaPotion = "Stamina Potion"
	itemRustySword    = "Rusty Sword"
	itemLeatherArmor  = "Leather Armor"
	npcTownCrier      = "Town Crier"
	questRewardGold   = 15
	moveStaminaCost   = 1
	maxInventorySize  = 50
)

func ensurePersistentLoop(ctx context.Context, tx *sql.Tx, rooms map[string]uuid.UUID) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS room_items (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
			item_id UUID NOT NULL REFERENCES items(id),
			quantity INTEGER DEFAULT 1 CHECK (quantity > 0),
			UNIQUE (room_id, item_id)
		)`,
		`CREATE TABLE IF NOT EXISTS character_objectives (
			character_id UUID PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,
			deliveries INTEGER NOT NULL DEFAULT 0 CHECK (deliveries >= 0),
			last_delivered_at TIMESTAMP WITH TIME ZONE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_character_item ON inventory (character_id, item_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_items_name ON items (name)`,
		`CREATE INDEX IF NOT EXISTS idx_room_items_room ON room_items (room_id)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ensure loop schema: %w", err)
		}
	}

	items := []struct {
		name, description, itemType string
		weight                      float64
		value                       int64
		properties                  string
	}{
		{itemManifest, "A water-stained harbor ledger. The Town Crier will want this back.", "quest", 0.2, 0, `{"quest": true}`},
		{itemHealthPotion, "A small vial of red liquid that smells of herbs.", "consumable", 0.5, 15, `{"healing": 25, "consumable_type": "potion"}`},
		{itemStaminaPotion, "A blue liquid that bubbles with leftover energy.", "consumable", 0.5, 12, `{"stamina_restore": 30, "consumable_type": "potion"}`},
		{itemRustySword, "A well-worn blade, still sharp enough to hang on a belt.", "weapon", 3.5, 25, `{"damage": 5, "weapon_type": "sword"}`},
		{itemLeatherArmor, "Basic protection made from tanned hide.", "armor", 8.0, 50, `{"defense": 3, "armor_type": "light"}`},
	}
	itemIDs := make(map[string]uuid.UUID, len(items))
	for _, item := range items {
		var id uuid.UUID
		err := tx.QueryRowContext(ctx, `SELECT id FROM items WHERE name = $1`, item.name).Scan(&id)
		if err == sql.ErrNoRows {
			err = tx.QueryRowContext(ctx, `
				INSERT INTO items (name, description, item_type, weight, value, properties)
				VALUES ($1, $2, $3, $4, $5, $6::jsonb)
				RETURNING id`, item.name, item.description, item.itemType, item.weight, item.value, item.properties).Scan(&id)
		}
		if err != nil {
			return fmt.Errorf("ensure item %q: %w", item.name, err)
		}
		itemIDs[item.name] = id
	}

	squareID := rooms["Town Square"]
	var crierID uuid.UUID
	err := tx.QueryRowContext(ctx, `SELECT id FROM npcs WHERE name = $1 ORDER BY created_at LIMIT 1`, npcTownCrier).Scan(&crierID)
	if err == sql.ErrNoRows {
		props, _ := json.Marshal(map[string]bool{"friendly": true, "quest_giver": true})
		err = tx.QueryRowContext(ctx, `
			INSERT INTO npcs (name, description, room_id, properties)
			VALUES ($1, $2, $3, $4::jsonb)
			RETURNING id`,
			npcTownCrier,
			"An enthusiastic man in colorful robes who shouts the latest news and lost-and-found notices.",
			squareID,
			props,
		).Scan(&crierID)
	} else if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE npcs SET room_id = $1 WHERE id = $2`, squareID, crierID)
	}
	if err != nil {
		return fmt.Errorf("ensure town crier: %w", err)
	}

	placements := []struct {
		room string
		item string
	}{
		{"Moonlit Docks", itemManifest},
		{"The Prancing Pony Inn", itemHealthPotion},
		{"Market Lane", itemRustySword},
		{"Market Lane", itemLeatherArmor},
		{"Market Lane", itemStaminaPotion},
	}
	for _, place := range placements {
		roomID, ok := rooms[place.room]
		if !ok {
			return fmt.Errorf("missing room %q for item placement", place.room)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO room_items (room_id, item_id, quantity)
			VALUES ($1, $2, 1)
			ON CONFLICT (room_id, item_id) DO NOTHING`, roomID, itemIDs[place.item]); err != nil {
			return fmt.Errorf("place %q in %q: %w", place.item, place.room, err)
		}
	}
	return nil
}
