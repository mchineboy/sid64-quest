package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

type GroundItem struct {
	ItemID   uuid.UUID
	Name     string
	Quantity int
	Item     *models.Item
}

func (ws *WorldService) ListInventory(ctx context.Context, characterID uuid.UUID) ([]*models.InventoryItem, error) {
	rows, err := ws.db.QueryContext(ctx, `
		SELECT inv.id, inv.character_id, inv.item_id, inv.quantity, inv.equipped, inv.acquired_at,
		       i.id, i.name, i.description, i.item_type, i.weight, i.value, i.properties
		FROM inventory inv
		JOIN items i ON i.id = inv.item_id
		WHERE inv.character_id = $1
		ORDER BY i.name`, characterID)
	if err != nil {
		return nil, fmt.Errorf("list inventory: %w", err)
	}
	defer rows.Close()

	var result []*models.InventoryItem
	for rows.Next() {
		entry, err := scanInventoryRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func (ws *WorldService) ListRoomItems(ctx context.Context, roomID uuid.UUID) ([]GroundItem, error) {
	rows, err := ws.db.QueryContext(ctx, `
		SELECT ri.item_id, ri.quantity, i.id, i.name, i.description, i.item_type, i.weight, i.value, i.properties
		FROM room_items ri
		JOIN items i ON i.id = ri.item_id
		WHERE ri.room_id = $1
		ORDER BY i.name`, roomID)
	if err != nil {
		return nil, fmt.Errorf("list room items: %w", err)
	}
	defer rows.Close()

	var result []GroundItem
	for rows.Next() {
		var ground GroundItem
		item, err := scanItem(rows, &ground.ItemID, &ground.Quantity)
		if err != nil {
			return nil, err
		}
		ground.Name = item.Name
		ground.Item = item
		result = append(result, ground)
	}
	return result, rows.Err()
}

func (ws *WorldService) TakeItem(ctx context.Context, characterID, roomID uuid.UUID, query string) (*models.Item, error) {
	tx, err := ws.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin take: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := requireCharacterInRoom(ctx, tx, characterID, roomID); err != nil {
		return nil, err
	}

	ground, err := ws.listRoomItemsTx(ctx, tx, roomID)
	if err != nil {
		return nil, err
	}
	chosen, err := findNamed(ground, query, func(item GroundItem) string { return item.Name })
	if err != nil {
		if errors.Is(err, ErrNoMatch) {
			return nil, fmt.Errorf("%w: you do not see %q here", ErrNoMatch, query)
		}
		if errors.Is(err, ErrAmbiguous) {
			return nil, fmt.Errorf("%q is ambiguous", query)
		}
		return nil, err
	}

	var inventoryCount int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(quantity), 0) FROM inventory WHERE character_id = $1`, characterID).Scan(&inventoryCount); err != nil {
		return nil, fmt.Errorf("count inventory: %w", err)
	}
	if inventoryCount >= maxInventorySize {
		return nil, fmt.Errorf("your inventory is full")
	}

	if propertyBool(chosen.Item.Properties, "quest") {
		var already int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM inventory WHERE character_id = $1 AND item_id = $2`, characterID, chosen.ItemID).Scan(&already); err != nil {
			return nil, fmt.Errorf("check quest item: %w", err)
		}
		if already > 0 {
			return nil, fmt.Errorf("you already have %s", chosen.Name)
		}
	}

	remaining := chosen.Quantity - 1
	if remaining <= 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM room_items WHERE room_id = $1 AND item_id = $2`, roomID, chosen.ItemID); err != nil {
			return nil, fmt.Errorf("remove room item: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE room_items SET quantity = $1 WHERE room_id = $2 AND item_id = $3`, remaining, roomID, chosen.ItemID); err != nil {
			return nil, fmt.Errorf("reduce room item: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory (character_id, item_id, quantity)
		VALUES ($1, $2, 1)
		ON CONFLICT (character_id, item_id) DO UPDATE SET quantity = inventory.quantity + 1`,
		characterID, chosen.ItemID); err != nil {
		return nil, fmt.Errorf("store inventory item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit take: %w", err)
	}
	return chosen.Item, nil
}

func (ws *WorldService) DropItem(ctx context.Context, characterID, roomID uuid.UUID, query string) (*models.Item, error) {
	tx, err := ws.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin drop: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := requireCharacterInRoom(ctx, tx, characterID, roomID); err != nil {
		return nil, err
	}

	inventory, err := ws.listInventoryTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}
	chosen, err := findNamed(inventory, query, func(item *models.InventoryItem) string { return item.Item.Name })
	if err != nil {
		return nil, inventoryMatchError(query, err)
	}
	if chosen.Equipped {
		return nil, fmt.Errorf("you must unequip %s first", chosen.Item.Name)
	}

	if chosen.Quantity <= 1 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM inventory WHERE id = $1`, chosen.ID); err != nil {
			return nil, fmt.Errorf("remove inventory item: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE inventory SET quantity = quantity - 1 WHERE id = $1`, chosen.ID); err != nil {
			return nil, fmt.Errorf("reduce inventory item: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO room_items (room_id, item_id, quantity)
		VALUES ($1, $2, 1)
		ON CONFLICT (room_id, item_id) DO UPDATE SET quantity = room_items.quantity + 1`,
		roomID, chosen.ItemID); err != nil {
		return nil, fmt.Errorf("place dropped item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit drop: %w", err)
	}
	return chosen.Item, nil
}

func (ws *WorldService) UseItem(ctx context.Context, characterID uuid.UUID, query string) (string, *models.Character, error) {
	tx, err := ws.db.BeginTx(ctx, nil)
	if err != nil {
		return "", nil, fmt.Errorf("begin use: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inventory, err := ws.listInventoryTx(ctx, tx, characterID)
	if err != nil {
		return "", nil, err
	}
	chosen, err := findNamed(inventory, query, func(item *models.InventoryItem) string { return item.Item.Name })
	if err != nil {
		return "", nil, inventoryMatchError(query, err)
	}

	var health, maxHealth, stamina, maxStamina int
	if err := tx.QueryRowContext(ctx, `SELECT health, max_health, stamina, max_stamina FROM characters WHERE id = $1 FOR UPDATE`, characterID).Scan(&health, &maxHealth, &stamina, &maxStamina); err != nil {
		return "", nil, fmt.Errorf("lock character: %w", err)
	}

	message := ""
	if heal, ok := propertyInt(chosen.Item.Properties, "healing"); ok {
		health += heal
		if health > maxHealth {
			health = maxHealth
		}
		message = fmt.Sprintf("You drink the %s. Health is now %d/%d.", chosen.Item.Name, health, maxHealth)
	} else if restore, ok := propertyInt(chosen.Item.Properties, "stamina_restore"); ok {
		stamina += restore
		if stamina > maxStamina {
			stamina = maxStamina
		}
		message = fmt.Sprintf("You drink the %s. Stamina is now %d/%d.", chosen.Item.Name, stamina, maxStamina)
	} else {
		return "", nil, fmt.Errorf("%s cannot be used that way; try equip", chosen.Item.Name)
	}

	if chosen.Quantity <= 1 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM inventory WHERE id = $1`, chosen.ID); err != nil {
			return "", nil, fmt.Errorf("consume item: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE inventory SET quantity = quantity - 1 WHERE id = $1`, chosen.ID); err != nil {
			return "", nil, fmt.Errorf("consume item stack: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE characters SET health = $1, stamina = $2 WHERE id = $3`, health, stamina, characterID); err != nil {
		return "", nil, fmt.Errorf("save vitals: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", nil, fmt.Errorf("commit use: %w", err)
	}
	character, err := ws.LoadCharacter(ctx, characterID)
	if err != nil {
		return message, nil, err
	}
	return message, character, nil
}

func (ws *WorldService) EquipItem(ctx context.Context, characterID uuid.UUID, query string) (string, error) {
	tx, err := ws.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin equip: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inventory, err := ws.listInventoryTx(ctx, tx, characterID)
	if err != nil {
		return "", err
	}
	chosen, err := findNamed(inventory, query, func(item *models.InventoryItem) string { return item.Item.Name })
	if err != nil {
		return "", inventoryMatchError(query, err)
	}
	if chosen.Item.ItemType != "weapon" && chosen.Item.ItemType != "armor" {
		return "", fmt.Errorf("%s is not equipment", chosen.Item.Name)
	}
	if chosen.Equipped {
		return fmt.Sprintf("%s is already equipped.", chosen.Item.Name), nil
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE inventory SET equipped = false
		WHERE character_id = $1 AND equipped = true AND item_id IN (
			SELECT id FROM items WHERE item_type = $2
		)`, characterID, chosen.Item.ItemType); err != nil {
		return "", fmt.Errorf("unequip previous: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inventory SET equipped = true WHERE id = $1`, chosen.ID); err != nil {
		return "", fmt.Errorf("equip item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit equip: %w", err)
	}
	verb := "wield"
	if chosen.Item.ItemType == "armor" {
		verb = "wear"
	}
	return fmt.Sprintf("You %s %s.", verb, chosen.Item.Name), nil
}

func (ws *WorldService) UnequipItem(ctx context.Context, characterID uuid.UUID, query string) (string, error) {
	inventory, err := ws.ListInventory(ctx, characterID)
	if err != nil {
		return "", err
	}
	chosen, err := findNamed(inventory, query, func(item *models.InventoryItem) string { return item.Item.Name })
	if err != nil {
		return "", inventoryMatchError(query, err)
	}
	if !chosen.Equipped {
		return "", fmt.Errorf("%s is not equipped", chosen.Item.Name)
	}
	if _, err := ws.db.ExecContext(ctx, `UPDATE inventory SET equipped = false WHERE id = $1`, chosen.ID); err != nil {
		return "", fmt.Errorf("unequip item: %w", err)
	}
	return fmt.Sprintf("You remove %s.", chosen.Item.Name), nil
}

func (ws *WorldService) listRoomItemsTx(ctx context.Context, tx *sql.Tx, roomID uuid.UUID) ([]GroundItem, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT ri.item_id, ri.quantity, i.id, i.name, i.description, i.item_type, i.weight, i.value, i.properties
		FROM room_items ri
		JOIN items i ON i.id = ri.item_id
		WHERE ri.room_id = $1
		ORDER BY i.name, ri.item_id FOR UPDATE OF ri`, roomID)
	if err != nil {
		return nil, fmt.Errorf("list room items: %w", err)
	}
	defer rows.Close()

	var result []GroundItem
	for rows.Next() {
		var ground GroundItem
		item, err := scanItem(rows, &ground.ItemID, &ground.Quantity)
		if err != nil {
			return nil, err
		}
		ground.Name = item.Name
		ground.Item = item
		result = append(result, ground)
	}
	return result, rows.Err()
}

func (ws *WorldService) listInventoryTx(ctx context.Context, tx *sql.Tx, characterID uuid.UUID) ([]*models.InventoryItem, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT inv.id, inv.character_id, inv.item_id, inv.quantity, inv.equipped, inv.acquired_at,
		       i.id, i.name, i.description, i.item_type, i.weight, i.value, i.properties
		FROM inventory inv
		JOIN items i ON i.id = inv.item_id
		WHERE inv.character_id = $1
		ORDER BY i.name`, characterID)
	if err != nil {
		return nil, fmt.Errorf("list inventory: %w", err)
	}
	defer rows.Close()

	var result []*models.InventoryItem
	for rows.Next() {
		entry, err := scanInventoryRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanInventoryRow(rows rowScanner) (*models.InventoryItem, error) {
	var entry models.InventoryItem
	item, err := scanItem(rows, &entry.ID, &entry.CharacterID, &entry.ItemID, &entry.Quantity, &entry.Equipped, &entry.AcquiredAt)
	if err != nil {
		return nil, err
	}
	entry.Item = item
	return &entry, nil
}

func scanItem(rows rowScanner, prefix ...any) (*models.Item, error) {
	item := &models.Item{}
	var properties []byte
	args := append(prefix, &item.ID, &item.Name, &item.Description, &item.ItemType, &item.Weight, &item.Value, &properties)
	if err := rows.Scan(args...); err != nil {
		return nil, fmt.Errorf("scan item: %w", err)
	}
	if len(properties) > 0 {
		if err := json.Unmarshal(properties, &item.Properties); err != nil {
			return nil, fmt.Errorf("decode item properties: %w", err)
		}
	}
	return item, nil
}

func inventoryMatchError(query string, err error) error {
	if errors.Is(err, ErrNoMatch) {
		return fmt.Errorf("you are not carrying %q", query)
	}
	if errors.Is(err, ErrAmbiguous) {
		return fmt.Errorf("%q is ambiguous", query)
	}
	return err
}

func requireCharacterInRoom(ctx context.Context, tx *sql.Tx, characterID, roomID uuid.UUID) error {
	var current uuid.UUID
	if err := tx.QueryRowContext(ctx, `SELECT current_room_id FROM characters WHERE id = $1 FOR UPDATE`, characterID).Scan(&current); err != nil {
		return fmt.Errorf("lock character: %w", err)
	}
	if current != roomID {
		return fmt.Errorf("you are not in that room")
	}
	return nil
}

// TakeAll transfers available stacks atomically, respecting capacity and quest uniqueness.
func (ws *WorldService) TakeAll(ctx context.Context, characterID, roomID uuid.UUID) ([]GroundItem, error) {
	tx, err := ws.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = requireCharacterInRoom(ctx, tx, characterID, roomID); err != nil {
		return nil, err
	}
	ground, err := ws.listRoomItemsTx(ctx, tx, roomID)
	if err != nil {
		return nil, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(quantity),0) FROM inventory WHERE character_id=$1`, characterID).Scan(&count); err != nil {
		return nil, err
	}
	var taken []GroundItem
	for _, item := range ground {
		quantity := min(item.Quantity, maxInventorySize-count)
		if quantity <= 0 {
			continue
		}
		if propertyBool(item.Item.Properties, "quest") {
			var owned int
			if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM inventory WHERE character_id=$1 AND item_id=$2`, characterID, item.ItemID).Scan(&owned); err != nil {
				return nil, err
			}
			if owned > 0 {
				continue
			}
			quantity = 1
		}
		if quantity == item.Quantity {
			_, err = tx.ExecContext(ctx, `DELETE FROM room_items WHERE room_id=$1 AND item_id=$2`, roomID, item.ItemID)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE room_items SET quantity=quantity-$3 WHERE room_id=$1 AND item_id=$2`, roomID, item.ItemID, quantity)
		}
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO inventory(character_id,item_id,quantity) VALUES($1,$2,$3) ON CONFLICT(character_id,item_id) DO UPDATE SET quantity=inventory.quantity+EXCLUDED.quantity`, characterID, item.ItemID, quantity)
		if err != nil {
			return nil, err
		}
		item.Quantity = quantity
		taken = append(taken, item)
		count += quantity
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return taken, nil
}
