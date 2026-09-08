package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

// WorldService owns the small persistent starter world used by the gateway.
type WorldService struct {
	db    queries
	pool  *sql.DB
	outer *sql.Tx
}

func NewWorldService(db *sql.DB) *WorldService {
	return &WorldService{db: db, pool: db}
}

// EnsureStarterWorld creates the compact initial map and repairs its exits on
// every startup. It is safe to call repeatedly and gives old local databases a
// usable world without asking developers to destroy their data volumes.
func (ws *WorldService) EnsureStarterWorld(ctx context.Context) error {
	tx, err := ws.beginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin starter world setup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(72632002)`); err != nil {
		return err
	}

	rooms := []struct {
		name, description, shortDescription, roomType string
	}{
		{
			name:             "Town Square",
			description:      "The heart of the kingdom, where a fountain murmurs over old cobblestones. Paths lead toward the gate, the market, and a welcome inn.",
			shortDescription: "A bustling square with a fountain",
			roomType:         "safe",
		},
		{
			name:             "North Gate",
			description:      "Weathered stone gates open onto a quiet road. A bored guard watches the town and pretends not to enjoy the gossip.",
			shortDescription: "The northern gate of the town",
			roomType:         "safe",
		},
		{
			name:             "Market Lane",
			description:      "Canvas awnings snap overhead. A baker, a tinker, and a suspiciously cheerful potion seller compete for your attention.",
			shortDescription: "A busy market lane",
			roomType:         "shop",
		},
		{
			name:             "The Prancing Pony Inn",
			description:      "A low fire, a long bar, and a room full of rumors make this the safest place to rest after a very short adventure.",
			shortDescription: "A warm and welcoming inn",
			roomType:         "inn",
		},
		{
			name:             "Moonlit Docks",
			description:      "Black water taps against the pilings. A small boat bobs at the end of the pier, which feels like a promise for another day.",
			shortDescription: "Quiet docks beneath the moon",
			roomType:         "normal",
		},
	}

	ids := make(map[string]uuid.UUID, len(rooms))
	for _, room := range rooms {
		var id uuid.UUID
		err := tx.QueryRowContext(ctx, `SELECT id FROM rooms WHERE name = $1 ORDER BY created_at LIMIT 1`, room.name).Scan(&id)
		if err == sql.ErrNoRows {
			err = tx.QueryRowContext(ctx, `
				INSERT INTO rooms (name, description, short_description, room_type, exits, flags)
				VALUES ($1, $2, $3, $4, '{}'::jsonb, '{}'::jsonb)
				RETURNING id`, room.name, room.description, room.shortDescription, room.roomType).Scan(&id)
		}
		if err != nil {
			return fmt.Errorf("ensure room %q: %w", room.name, err)
		}
		ids[room.name] = id
	}

	exits := map[string]map[string]uuid.UUID{
		"Town Square":           {"north": ids["North Gate"], "east": ids["Market Lane"], "south": ids["The Prancing Pony Inn"]},
		"North Gate":            {"south": ids["Town Square"], "north": ids["Moonlit Docks"]},
		"Market Lane":           {"west": ids["Town Square"]},
		"The Prancing Pony Inn": {"north": ids["Town Square"]},
		"Moonlit Docks":         {"south": ids["North Gate"]},
	}
	for name, roomExits := range exits {
		data, err := json.Marshal(roomExits)
		if err != nil {
			return fmt.Errorf("encode exits for %q: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE rooms SET exits = $1::jsonb WHERE id = $2`, data, ids[name]); err != nil {
			return fmt.Errorf("update exits for %q: %w", name, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE characters SET current_room_id = $1 WHERE current_room_id IS NULL`, ids["Town Square"]); err != nil {
		return fmt.Errorf("place unlocated characters: %w", err)
	}
	if err := ensurePersistentLoop(ctx, tx, ids); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit starter world setup: %w", err)
	}
	return nil
}

func (ws *WorldService) GetRoom(ctx context.Context, roomID uuid.UUID) (*models.Room, error) {
	var room models.Room
	var exitsData []byte
	var flagsData []byte
	if err := ws.db.QueryRowContext(ctx, `
		SELECT id, name, description, short_description, room_type, exits, flags, created_at
		FROM rooms WHERE id = $1`, roomID).Scan(
		&room.ID, &room.Name, &room.Description, &room.ShortDescription, &room.RoomType, &exitsData, &flagsData, &room.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("room not found")
		}
		return nil, fmt.Errorf("get room: %w", err)
	}

	exits, err := decodeExits(exitsData)
	if err != nil {
		return nil, fmt.Errorf("decode room exits: %w", err)
	}
	room.Exits = exits
	if err := json.Unmarshal(flagsData, &room.Flags); err != nil {
		return nil, fmt.Errorf("decode room flags: %w", err)
	}
	return &room, nil
}

func (ws *WorldService) MoveCharacter(ctx context.Context, characterID uuid.UUID, direction string) (*models.Room, *models.Room, int, error) {
	direction = NormalizeDirection(direction)
	if direction == "" {
		return nil, nil, 0, fmt.Errorf("that is not a direction")
	}

	tx, err := ws.beginTx(ctx)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("begin movement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentRoomID uuid.UUID
	var stamina int
	if err := tx.QueryRowContext(ctx, `SELECT current_room_id, stamina FROM characters WHERE id = $1 FOR UPDATE`, characterID).Scan(&currentRoomID, &stamina); err != nil {
		return nil, nil, 0, fmt.Errorf("load character location: %w", err)
	}

	from, err := ws.getRoomTx(ctx, tx, currentRoomID)
	if err != nil {
		return nil, nil, 0, err
	}
	nextRoomID, exists := from.Exits[direction]
	if !exists || nextRoomID == uuid.Nil {
		return from, nil, stamina, fmt.Errorf("there is no exit %s from here", direction)
	}
	to, err := ws.getRoomTx(ctx, tx, nextRoomID)
	if err != nil {
		return nil, nil, 0, err
	}
	stamina -= moveStaminaCost
	if stamina < 0 {
		stamina = 0
	}
	if _, err := tx.ExecContext(ctx, `UPDATE characters SET current_room_id = $1, stamina = $2 WHERE id = $3`, to.ID, stamina, characterID); err != nil {
		return nil, nil, 0, fmt.Errorf("save character location: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, 0, fmt.Errorf("commit movement: %w", err)
	}
	return from, to, stamina, nil
}

func (ws *WorldService) LoadCharacter(ctx context.Context, characterID uuid.UUID) (*models.Character, error) {
	var char models.Character
	var currentRoomID sql.NullString
	if err := ws.db.QueryRowContext(ctx, `
		SELECT c.id, c.user_id, c.name, c.level, c.experience, c.health, c.max_health,
		       c.stamina, c.max_stamina, c.gold, c.alignment_lawful, c.alignment_good,
		       c.current_room_id, c.last_rest, c.is_sleeping, c.created_at,
		       COALESCE(o.deliveries, 0)
		FROM characters c
		LEFT JOIN character_objectives o ON o.character_id = c.id
		WHERE c.id = $1`, characterID).Scan(
		&char.ID, &char.UserID, &char.Name, &char.Level, &char.Experience,
		&char.Health, &char.MaxHealth, &char.Stamina, &char.MaxStamina, &char.Gold,
		&char.AlignmentLawful, &char.AlignmentGood, &currentRoomID, &char.LastRest,
		&char.IsSleeping, &char.CreatedAt, &char.Deliveries,
	); err != nil {
		return nil, fmt.Errorf("load character: %w", err)
	}
	if currentRoomID.Valid {
		roomID, err := uuid.Parse(currentRoomID.String)
		if err == nil {
			char.CurrentRoomID = &roomID
		}
	}
	return &char, nil
}

func (ws *WorldService) getRoomTx(ctx context.Context, tx transaction, roomID uuid.UUID) (*models.Room, error) {
	var room models.Room
	var exitsData []byte
	var flagsData []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT id, name, description, short_description, room_type, exits, flags, created_at
		FROM rooms WHERE id = $1`, roomID).Scan(
		&room.ID, &room.Name, &room.Description, &room.ShortDescription, &room.RoomType, &exitsData, &flagsData, &room.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("get room: %w", err)
	}
	var err error
	room.Exits, err = decodeExits(exitsData)
	if err != nil {
		return nil, fmt.Errorf("decode room exits: %w", err)
	}
	if err := json.Unmarshal(flagsData, &room.Flags); err != nil {
		return nil, fmt.Errorf("decode room flags: %w", err)
	}
	return &room, nil
}

func decodeExits(data []byte) (map[string]uuid.UUID, error) {
	var raw map[string]*string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	exits := make(map[string]uuid.UUID, len(raw))
	for direction, id := range raw {
		if id == nil {
			continue
		}
		parsed, err := uuid.Parse(*id)
		if err != nil {
			return nil, fmt.Errorf("invalid %s exit: %w", direction, err)
		}
		exits[direction] = parsed
	}
	return exits, nil
}

func SortedExitNames(exits map[string]uuid.UUID) []string {
	names := make([]string, 0, len(exits))
	for direction := range exits {
		names = append(names, direction)
	}
	sort.Strings(names)
	return names
}
