package game

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

const (
	attackStaminaCost = 2
	resurrectionDelay = 10 * time.Minute
)

type CombatResult struct {
	Message  string
	RoomID   uuid.UUID
	Defeated bool
}

// Respawns are lazy, use database time, and never restore a living monster's HP.
func respawnNPCs(ctx context.Context, db queries, room uuid.UUID) error {
	_, err := db.ExecContext(ctx, `UPDATE npcs SET health=max_health,respawn_at=NULL WHERE room_id=$1 AND hostile AND health<=0 AND respawn_at<=now()`, room)
	return err
}

func combatEquipment(items []*models.InventoryItem) (int, int) {
	weapon, armor := 0, 0
	for _, entry := range items {
		if !entry.Equipped || entry.Quantity <= 0 || entry.Item == nil {
			continue
		}
		if entry.Item.ItemType == "weapon" {
			value, _ := propertyInt(entry.Item.Properties, "damage")
			weapon = max(weapon, min(100, max(0, value)))
		}
		if entry.Item.ItemType == "armor" {
			value, _ := propertyInt(entry.Item.Properties, "defense")
			armor = max(armor, min(50, max(0, value)))
		}
	}
	return 5 + weapon, armor
}

// Attack resolves one PvE round atomically. Only explicitly hostile NPC rows
// can be selected; character names/IDs are never eligible attack targets.
func (ws *WorldService) Attack(ctx context.Context, characterID, roomID uuid.UUID, query string) (CombatResult, error) {
	result := CombatResult{RoomID: roomID}
	if strings.TrimSpace(query) == "" {
		return result, fmt.Errorf("use attack <monster>; players and friendly NPCs cannot be attacked")
	}
	tx, err := ws.beginTx(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = requireCharacterInRoom(ctx, tx, characterID, roomID); err != nil {
		return result, err
	}
	var hp, stamina int
	var isDead bool
	if err = tx.QueryRowContext(ctx, `SELECT health,stamina,is_dead FROM characters WHERE id=$1`, characterID).Scan(&hp, &stamina, &isDead); err != nil {
		return result, err
	}
	if isDead || hp <= 0 {
		return result, fmt.Errorf("the dead cannot attack; use resurrect in the Hall of Returning")
	}
	if stamina < attackStaminaCost {
		return result, fmt.Errorf("you need 2 stamina to attack; retreat and rest or use a stamina potion")
	}
	room, err := ws.getRoomTx(ctx, tx, roomID)
	if err != nil {
		return result, err
	}
	if room.RoomType != "normal" && room.RoomType != "dungeon" {
		return result, fmt.Errorf("combat is not allowed here")
	}
	if err = respawnNPCs(ctx, tx, roomID); err != nil {
		return result, err
	}
	// Match all living NPCs so a partial name cannot silently bypass a friendly
	// name collision. Lock only the chosen opponent after resolving ambiguity.
	npcs, err := ws.listRoomNPCsTx(ctx, tx, roomID)
	if err != nil {
		return result, err
	}
	target, err := findNamed(npcs, query, func(n *models.NPC) string { return n.Name })
	if err != nil {
		return result, fmt.Errorf("choose one monster here by name; players cannot be attacked: %w", err)
	}
	var enemyHP, attack, defense, gold, xp, respawn, lootChance int
	var lootItem sql.NullString
	var hostile bool
	if err = tx.QueryRowContext(ctx, `SELECT health,hostile,attack_damage,defense,reward_gold,reward_experience,respawn_seconds,loot_item_id,loot_chance FROM npcs WHERE id=$1 AND room_id=$2 FOR UPDATE`, target.ID, roomID).Scan(&enemyHP, &hostile, &attack, &defense, &gold, &xp, &respawn, &lootItem, &lootChance); err != nil {
		return result, err
	}
	if !hostile {
		return result, fmt.Errorf("%s is friendly and cannot be attacked", target.Name)
	}
	if enemyHP <= 0 {
		return result, fmt.Errorf("%s has already been defeated", target.Name)
	}
	inventory, err := ws.listInventoryTx(ctx, tx, characterID)
	if err != nil {
		return result, err
	}
	power, armor := combatEquipment(inventory)
	damage := min(enemyHP, max(1, power-defense))
	enemyHP -= damage
	stamina -= attackStaminaCost
	result.Message = fmt.Sprintf("You hit %s for %d damage.", target.Name, damage)
	earnedXP := 0
	if enemyHP == 0 {
		earnedXP = xp
		if _, err = tx.ExecContext(ctx, `DELETE FROM monster_corpses WHERE expires_at<=now()`); err != nil {
			return result, err
		}
		currency := GoldValue(gold) + int64(secureRandom(99)+1)*CopperPerSilver + int64(secureRandom(99)+1)
		var dropped interface{}
		if lootItem.Valid && secureRandom(10000) < lootChance {
			dropped = lootItem.String
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO monster_corpses(npc_id,room_id,owner_id,monster_name,currency_value,item_id,item_looted)
			VALUES($1,$2,$3,$4,$5,$6::uuid,$6::uuid IS NULL)`,
			target.ID, roomID, characterID, target.Name, currency, dropped); err != nil {
			return result, fmt.Errorf("create corpse: %w", err)
		}
		result.Message += fmt.Sprintf(" Victory! You gain %d experience. The corpse carries coin; use loot %s.", xp, target.Name)
	} else {
		hit := min(hp, max(1, attack-armor))
		hp -= hit
		result.Message += fmt.Sprintf(" %s has %d/%d HP and hits you for %d.", target.Name, enemyHP, target.MaxHealth, hit)
		if hp == 0 {
			if err = tx.QueryRowContext(ctx, `SELECT room_id FROM world_content_rooms WHERE content_key='hall_returning'`).Scan(&result.RoomID); err != nil {
				return result, err
			}
			result.Defeated = true
			result.Message += " You die. Crypt wardens carry you to the Hall of Returning. Pay 100 gold now or wait ten minutes, then use resurrect."
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE npcs SET health=$2,respawn_at=CASE WHEN $2=0 THEN now()+$3*interval '1 second' ELSE NULL END WHERE id=$1`, target.ID, enemyHP, respawn); err != nil {
		return result, err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE characters
		SET health=$2,stamina=$3,experience=experience+$4,current_room_id=$5,
		    is_dead=$6,
		    died_at=CASE WHEN $6 THEN now() ELSE died_at END,
		    resurrection_ready_at=CASE WHEN $6 THEN now()+$7*interval '1 second' ELSE resurrection_ready_at END,
		    death_room_id=CASE WHEN $6 THEN $8 ELSE death_room_id END
		WHERE id=$1`,
		characterID, hp, stamina, earnedXP, result.RoomID, result.Defeated, int(resurrectionDelay/time.Second), roomID); err != nil {
		return result, err
	}
	result.Message += fmt.Sprintf(" HP %d; stamina %d.", hp, stamina)
	if err = tx.Commit(); err != nil {
		return CombatResult{}, err
	}
	return result, nil
}

func secureRandom(limit int) int {
	if limit <= 0 {
		return 0
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0
	}
	return int(value.Int64())
}
