package game

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

func (ws *WorldService) ListRoomNPCs(ctx context.Context, roomID uuid.UUID) ([]*models.NPC, error) {
	rows, err := ws.db.QueryContext(ctx, `
		SELECT id, name, description, room_id, health, max_health, level, created_at
		FROM npcs WHERE room_id = $1 ORDER BY name`, roomID)
	if err != nil {
		return nil, fmt.Errorf("list npcs: %w", err)
	}
	defer rows.Close()

	var result []*models.NPC
	for rows.Next() {
		var npc models.NPC
		if err := rows.Scan(&npc.ID, &npc.Name, &npc.Description, &npc.RoomID, &npc.Health, &npc.MaxHealth, &npc.Level, &npc.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan npc: %w", err)
		}
		result = append(result, &npc)
	}
	return result, rows.Err()
}

func (ws *WorldService) Talk(ctx context.Context, characterID, roomID uuid.UUID, query string) (string, error) {
	npcs, err := ws.ListRoomNPCs(ctx, roomID)
	if err != nil {
		return "", err
	}
	if len(npcs) == 0 {
		return "", fmt.Errorf("there is no one here to talk to")
	}

	var npc *models.NPC
	if query == "" {
		if len(npcs) != 1 {
			return "", fmt.Errorf("talk to whom?")
		}
		npc = npcs[0]
	} else {
		npc, err = findNamed(npcs, query, func(n *models.NPC) string { return n.Name })
		if err != nil {
			if errors.Is(err, ErrNoMatch) {
				return "", fmt.Errorf("there is no one called %q here", query)
			}
			if errors.Is(err, ErrAmbiguous) {
				return "", fmt.Errorf("%q is ambiguous", query)
			}
			return "", err
		}
	}

	if npc.Name != npcTownCrier {
		return fmt.Sprintf("%s nods, but has nothing useful to say.", npc.Name), nil
	}

	inventory, err := ws.ListInventory(ctx, characterID)
	if err != nil {
		return "", err
	}
	hasManifest := false
	for _, item := range inventory {
		if item.Item != nil && item.Item.Name == itemManifest {
			hasManifest = true
			break
		}
	}

	character, err := ws.LoadCharacter(ctx, characterID)
	if err != nil {
		return "", err
	}

	if hasManifest {
		return "The Town Crier's eyes light up. \"That's the harbor ledger! Give it to me and I'll see you paid.\"", nil
	}
	if character.Deliveries == 0 {
		return "The Town Crier waves a damp scrap of paper. \"The harbor ledger washed up at the Moonlit Docks. Fetch the misplaced manifest and bring it back. I'll make it worth your while, every time it goes missing.\"", nil
	}
	return fmt.Sprintf("The Town Crier grins. \"You've returned that ledger %d time(s). It keeps drifting back to the docks. Fetch it again if you want another purse.\"", character.Deliveries), nil
}

func (ws *WorldService) GiveItem(ctx context.Context, characterID, roomID uuid.UUID, itemQuery, npcQuery string) (string, *models.Character, error) {
	tx, err := ws.beginTx(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("begin give: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := requireCharacterInRoom(ctx, tx, characterID, roomID); err != nil {
		return "", nil, err
	}

	npcs, err := ws.listRoomNPCsTx(ctx, tx, roomID)
	if err != nil {
		return "", nil, err
	}
	var npc *models.NPC
	if npcQuery == "" {
		npc, err = findNamed(npcs, npcTownCrier, func(n *models.NPC) string { return n.Name })
		if err != nil {
			return "", nil, fmt.Errorf("there is no one here who wants that")
		}
	} else {
		npc, err = findNamed(npcs, npcQuery, func(n *models.NPC) string { return n.Name })
		if err != nil {
			if errors.Is(err, ErrNoMatch) {
				return "", nil, fmt.Errorf("there is no one called %q here", npcQuery)
			}
			return "", nil, err
		}
	}
	if npc.Name != npcTownCrier {
		return "", nil, fmt.Errorf("%s does not want that", npc.Name)
	}

	inventory, err := ws.listInventoryTx(ctx, tx, characterID)
	if err != nil {
		return "", nil, err
	}
	chosen, err := findNamed(inventory, itemQuery, func(item *models.InventoryItem) string { return item.Item.Name })
	if err != nil {
		return "", nil, inventoryMatchError(itemQuery, err)
	}
	if chosen.Item.Name != itemManifest {
		return "", nil, fmt.Errorf("the Town Crier only wants the misplaced manifest")
	}

	var gold int64
	var stamina, maxStamina int
	if err := tx.QueryRowContext(ctx, `SELECT gold, stamina, max_stamina FROM characters WHERE id = $1 FOR UPDATE`, characterID).Scan(&gold, &stamina, &maxStamina); err != nil {
		return "", nil, fmt.Errorf("lock character: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM inventory WHERE id = $1`, chosen.ID); err != nil {
		return "", nil, fmt.Errorf("remove quest item: %w", err)
	}

	gold += questRewardGold
	stamina += 20
	if stamina > maxStamina {
		stamina = maxStamina
	}
	if _, err := tx.ExecContext(ctx, `UPDATE characters SET gold = $1, stamina = $2 WHERE id = $3`, gold, stamina, characterID); err != nil {
		return "", nil, fmt.Errorf("pay reward: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO character_objectives (character_id, deliveries, last_delivered_at)
		VALUES ($1, 1, NOW())
		ON CONFLICT (character_id) DO UPDATE
		SET deliveries = character_objectives.deliveries + 1, last_delivered_at = NOW()`, characterID); err != nil {
		return "", nil, fmt.Errorf("record delivery: %w", err)
	}

	var docksID uuid.UUID
	if err := tx.QueryRowContext(ctx, `SELECT id FROM rooms WHERE name = 'Moonlit Docks' ORDER BY created_at LIMIT 1`).Scan(&docksID); err != nil {
		return "", nil, fmt.Errorf("find docks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO room_items (room_id, item_id, quantity)
		VALUES ($1, $2, 1)
		ON CONFLICT (room_id, item_id) DO UPDATE SET quantity = 1`, docksID, chosen.ItemID); err != nil {
		return "", nil, fmt.Errorf("respawn manifest: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", nil, fmt.Errorf("commit give: %w", err)
	}

	character, err := ws.LoadCharacter(ctx, characterID)
	if err != nil {
		return "", nil, err
	}
	message := fmt.Sprintf("The Town Crier snatches the ledger and presses %d gold into your hand. \"It'll wash up at the docks again. Come back when it does.\" Deliveries: %d.", questRewardGold, character.Deliveries)
	return message, character, nil
}

func (ws *WorldService) Rest(ctx context.Context, characterID, roomID uuid.UUID) (string, *models.Character, error) {
	room, err := ws.GetRoom(ctx, roomID)
	if err != nil {
		return "", nil, err
	}
	if room.RoomType != "inn" {
		return "", nil, fmt.Errorf("you can only rest at an inn")
	}
	var current uuid.UUID
	if err := ws.db.QueryRowContext(ctx, `SELECT current_room_id FROM characters WHERE id = $1`, characterID).Scan(&current); err != nil {
		return "", nil, fmt.Errorf("load character room: %w", err)
	}
	if current != roomID {
		return "", nil, fmt.Errorf("you are not in that room")
	}

	if _, err := ws.db.ExecContext(ctx, `
		UPDATE characters
		SET health = max_health, stamina = max_stamina, last_rest = NOW(), is_sleeping = false
		WHERE id = $1`, characterID); err != nil {
		return "", nil, fmt.Errorf("rest: %w", err)
	}
	character, err := ws.LoadCharacter(ctx, characterID)
	if err != nil {
		return "", nil, err
	}
	return "You rest by the fire. Health and stamina are restored.", character, nil
}

func (ws *WorldService) listRoomNPCsTx(ctx context.Context, tx transaction, roomID uuid.UUID) ([]*models.NPC, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, name, description, room_id, health, max_health, level, created_at
		FROM npcs WHERE room_id = $1 ORDER BY name`, roomID)
	if err != nil {
		return nil, fmt.Errorf("list npcs: %w", err)
	}
	defer rows.Close()

	var result []*models.NPC
	for rows.Next() {
		var npc models.NPC
		if err := rows.Scan(&npc.ID, &npc.Name, &npc.Description, &npc.RoomID, &npc.Health, &npc.MaxHealth, &npc.Level, &npc.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan npc: %w", err)
		}
		result = append(result, &npc)
	}
	return result, rows.Err()
}
