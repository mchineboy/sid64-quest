package game

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type Corpse struct {
	ID          uuid.UUID
	MonsterName string
	OwnerID     uuid.UUID
}

func (ws *WorldService) ListRoomCorpses(ctx context.Context, roomID uuid.UUID) ([]Corpse, error) {
	if _, err := ws.db.ExecContext(ctx, `DELETE FROM monster_corpses WHERE expires_at<=now()`); err != nil {
		return nil, err
	}
	rows, err := ws.db.QueryContext(ctx, `
		SELECT id,monster_name,owner_id
		FROM monster_corpses
		WHERE room_id=$1 AND NOT (currency_looted AND item_looted)
		ORDER BY created_at`, roomID)
	if err != nil {
		return nil, fmt.Errorf("list corpses: %w", err)
	}
	defer rows.Close()
	var corpses []Corpse
	for rows.Next() {
		var corpse Corpse
		if err := rows.Scan(&corpse.ID, &corpse.MonsterName, &corpse.OwnerID); err != nil {
			return nil, err
		}
		corpses = append(corpses, corpse)
	}
	return corpses, rows.Err()
}

func (ws *WorldService) LootCorpse(ctx context.Context, characterID, roomID uuid.UUID, query string) (string, error) {
	tx, err := ws.beginTx(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err = requireCharacterInRoom(ctx, tx, characterID, roomID); err != nil {
		return "", err
	}
	var dead bool
	if err = tx.QueryRowContext(ctx, `SELECT is_dead FROM characters WHERE id=$1`, characterID).Scan(&dead); err != nil {
		return "", err
	}
	if dead {
		return "", fmt.Errorf("the dead cannot loot")
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM monster_corpses WHERE expires_at<=now()`); err != nil {
		return "", err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id,monster_name,owner_id
		FROM monster_corpses
		WHERE room_id=$1 AND NOT (currency_looted AND item_looted)
		ORDER BY created_at FOR UPDATE`, roomID)
	if err != nil {
		return "", err
	}
	var corpses []Corpse
	for rows.Next() {
		var corpse Corpse
		if err = rows.Scan(&corpse.ID, &corpse.MonsterName, &corpse.OwnerID); err != nil {
			rows.Close()
			return "", err
		}
		corpses = append(corpses, corpse)
	}
	if err = rows.Close(); err != nil {
		return "", err
	}
	chosen, err := findNamed(corpses, query, func(c Corpse) string { return c.MonsterName })
	if err != nil {
		if errors.Is(err, ErrNoMatch) {
			return "", fmt.Errorf("there is no matching corpse here")
		}
		return "", fmt.Errorf("that corpse name is ambiguous")
	}
	if chosen.OwnerID != characterID {
		return "", fmt.Errorf("that corpse belongs to another victor")
	}

	var currency int64
	var itemID sql.NullString
	var currencyLooted, itemLooted bool
	if err = tx.QueryRowContext(ctx, `
		SELECT currency_value,item_id,currency_looted,item_looted
		FROM monster_corpses WHERE id=$1 FOR UPDATE`, chosen.ID).
		Scan(&currency, &itemID, &currencyLooted, &itemLooted); err != nil {
		return "", err
	}
	parts := make([]string, 0, 2)
	if !currencyLooted {
		if _, err = tx.ExecContext(ctx, `UPDATE characters SET gold=gold+$2 WHERE id=$1`, characterID, currency); err != nil {
			return "", err
		}
		currencyLooted = true
		parts = append(parts, FormatCurrency(currency))
	}
	if itemID.Valid && !itemLooted {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(quantity),0) FROM inventory WHERE character_id=$1`, characterID).Scan(&count); err != nil {
			return "", err
		}
		if count < maxInventorySize {
			var itemName string
			if err = tx.QueryRowContext(ctx, `SELECT name FROM items WHERE id=$1`, itemID.String).Scan(&itemName); err != nil {
				return "", err
			}
			if _, err = tx.ExecContext(ctx, `
				INSERT INTO inventory(character_id,item_id,quantity) VALUES($1,$2,1)
				ON CONFLICT(character_id,item_id) DO UPDATE SET quantity=inventory.quantity+1`,
				characterID, itemID.String); err != nil {
				return "", err
			}
			itemLooted = true
			parts = append(parts, itemName)
		}
	}
	if currencyLooted && itemLooted {
		_, err = tx.ExecContext(ctx, `DELETE FROM monster_corpses WHERE id=$1`, chosen.ID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE monster_corpses SET currency_looted=$2,item_looted=$3 WHERE id=$1`, chosen.ID, currencyLooted, itemLooted)
	}
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	if len(parts) == 0 {
		return "Your pack is full; the armor remains on the corpse.", nil
	}
	message := "You loot " + joinLoot(parts) + "."
	if itemID.Valid && !itemLooted {
		message += " Your pack is full; the armor remains."
	}
	return message, nil
}

func joinLoot(parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[0] + " and " + parts[1]
}
