package game

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ResurrectionResult struct {
	Message string
	RoomID  uuid.UUID
}

func (ws *WorldService) Resurrect(ctx context.Context, characterID uuid.UUID, pay bool, costGold int) (ResurrectionResult, error) {
	tx, err := ws.beginTx(ctx)
	if err != nil {
		return ResurrectionResult{}, err
	}
	defer tx.Rollback()

	var dead bool
	var health, maxHealth, maxStamina int
	var balance int64
	var ready time.Time
	var roomID uuid.UUID
	if err = tx.QueryRowContext(ctx, `
		SELECT is_dead,health,max_health,max_stamina,gold,resurrection_ready_at,current_room_id
		FROM characters WHERE id=$1 FOR UPDATE`, characterID).
		Scan(&dead, &health, &maxHealth, &maxStamina, &balance, &ready, &roomID); err != nil {
		return ResurrectionResult{}, err
	}
	if !dead {
		return ResurrectionResult{}, fmt.Errorf("you are already alive")
	}
	var hall uuid.UUID
	if err = tx.QueryRowContext(ctx, `SELECT room_id FROM world_content_rooms WHERE content_key='hall_returning'`).Scan(&hall); err != nil {
		return ResurrectionResult{}, err
	}
	if roomID != hall {
		return ResurrectionResult{}, fmt.Errorf("resurrection is available only in the Hall of Returning")
	}

	cost := GoldValue(costGold)
	if pay {
		if balance < cost {
			return ResurrectionResult{}, fmt.Errorf("resurrection costs %s; you carry %s. Wait for the slow bell instead", FormatCurrency(cost), FormatCurrency(balance))
		}
		balance -= cost
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO transactions(from_character_id,gold_amount,transaction_type)
			VALUES($1,$2,'resurrection')`, characterID, cost); err != nil {
			return ResurrectionResult{}, err
		}
	} else if remaining := time.Until(ready); remaining > 0 {
		minutes := int((remaining + time.Minute - 1) / time.Minute)
		return ResurrectionResult{}, fmt.Errorf("the slow bell has not rung; wait %d more minute(s), or use resurrect pay for %s", minutes, FormatCurrency(cost))
	}

	health = max(1, maxHealth/2)
	if _, err = tx.ExecContext(ctx, `
		UPDATE characters
		SET health=$2,stamina=$3,gold=$4,is_dead=false,died_at=NULL,
		    resurrection_ready_at=NULL,death_room_id=NULL
		WHERE id=$1`, characterID, health, maxStamina, balance); err != nil {
		return ResurrectionResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return ResurrectionResult{}, err
	}
	message := fmt.Sprintf("The slow bell rings. You return with %d/%d health and full stamina.", health, maxHealth)
	if pay {
		message = fmt.Sprintf("The warden accepts %s. "+message, FormatCurrency(cost))
	}
	return ResurrectionResult{Message: message, RoomID: roomID}, nil
}
