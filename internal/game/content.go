package game

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tylerhardison/race-condition-kingdom/internal/scripting"
)

//go:embed content/*.star
var worldContent embed.FS

func bundledWorld(ctx context.Context) (*scripting.WorldDefinition, map[string]string, error) {
	source, err := worldContent.ReadFile("content/world.star")
	if err != nil {
		return nil, nil, err
	}
	out, err := scripting.Run(ctx, scripting.Input{Kind: "world", Source: string(source)})
	if err != nil {
		return nil, nil, fmt.Errorf("build world: %w", err)
	}
	scripts := map[string]string{}
	for _, room := range out.World.Rooms {
		if room.Script == "" || scripts[room.Script] != "" {
			continue
		}
		source, err := worldContent.ReadFile("content/" + room.Script + ".star")
		if err != nil {
			return nil, nil, err
		}
		if _, err = scripting.Run(ctx, scripting.Input{Kind: "room", Source: string(source), Validate: true}); err != nil {
			return nil, nil, fmt.Errorf("validate %s: %w", room.Script, err)
		}
		scripts[room.Script] = string(source)
	}
	return out.World, scripts, nil
}

// Content keys and script UUIDs are permanent: player puzzle state survives
// restarts. Only new rooms receive attachments, so admin detaches stay detached.
func installWorldContent(ctx context.Context, tx transaction) (map[string]uuid.UUID, error) {
	world, scripts, err := bundledWorld(ctx)
	if err != nil {
		return nil, err
	}
	scriptIDs := map[string]uuid.UUID{}
	for key, source := range scripts {
		id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("sid64.quest/world/scripts/"+key))
		scriptIDs[key] = id
		result, err := tx.ExecContext(ctx, `INSERT INTO scripts(id,name,description,content,script_type,is_active,published_content,published_revision) VALUES($1,$2,'Bundled world content',$3,'room',true,$3,1) ON CONFLICT(id) DO NOTHING`, id, "world_"+key, source)
		if err != nil {
			return nil, fmt.Errorf("install script %s: %w", key, err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n > 0 {
			if _, err = tx.ExecContext(ctx, `INSERT INTO script_audit(script_id,action,revision,content) VALUES($1,'install',1,$2)`, id, source); err != nil {
				return nil, err
			}
		}
	}
	ids := map[string]uuid.UUID{}
	names := map[string]uuid.UUID{}
	fresh := map[string]bool{}
	legacy := map[string]bool{"square": true, "gate": true, "market": true, "inn": true, "docks": true}
	for _, room := range world.Rooms {
		var id uuid.UUID
		err := tx.QueryRowContext(ctx, `SELECT room_id FROM world_content_rooms WHERE content_key=$1`, room.Key).Scan(&id)
		if err == sql.ErrNoRows {
			fresh[room.Key] = true
			if legacy[room.Key] {
				err = tx.QueryRowContext(ctx, `SELECT id FROM rooms WHERE name=$1 ORDER BY created_at,id LIMIT 1`, room.Name).Scan(&id)
			}
			if err == sql.ErrNoRows {
				err = tx.QueryRowContext(ctx, `INSERT INTO rooms(name,description,short_description,room_type) VALUES($1,$2,$1,$3) RETURNING id`, room.Name, room.Description, room.Kind).Scan(&id)
			}
			if err == nil {
				_, err = tx.ExecContext(ctx, `INSERT INTO world_content_rooms(content_key,room_id) VALUES($1,$2)`, room.Key, id)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("install room %s: %w", room.Key, err)
		}
		ids[room.Key], names[room.Name] = id, id
	}
	for _, room := range world.Rooms {
		exits := map[string]uuid.UUID{}
		for direction, key := range room.Exits {
			// New regions also need an entrance from already-installed rooms.
			if fresh[room.Key] || fresh[key] {
				exits[direction] = ids[key]
			}
		}
		if len(exits) == 0 && !fresh[room.Key] {
			continue
		}
		data, err := json.Marshal(exits)
		if err != nil {
			return nil, err
		}
		if !fresh[room.Key] {
			// Extending a pack must not silently replace an operator's exit.
			var current []byte
			if err = tx.QueryRowContext(ctx, `SELECT exits FROM rooms WHERE id=$1 FOR UPDATE`, ids[room.Key]).Scan(&current); err != nil {
				return nil, err
			}
			existing, err := decodeExits(current)
			if err != nil {
				return nil, err
			}
			for direction, target := range exits {
				if occupied, ok := existing[direction]; ok && occupied != target {
					return nil, fmt.Errorf("new content conflicts with %s exit in %s", direction, room.Key)
				}
			}
			if _, err = tx.ExecContext(ctx, `UPDATE rooms SET exits=COALESCE(exits,'{}'::jsonb) || $2::jsonb WHERE id=$1`, ids[room.Key], data); err != nil {
				return nil, err
			}
			continue
		}
		var script interface{}
		if room.Script != "" {
			script = scriptIDs[room.Script]
		}
		// Preserve custom exits and any existing room attachment during adoption.
		_, err = tx.ExecContext(ctx, `UPDATE rooms SET exits=COALESCE(exits,'{}'::jsonb) || $2::jsonb,description=$3,short_description=COALESCE(short_description,name),script_id=COALESCE(script_id,$4) WHERE id=$1`, ids[room.Key], data, room.Description, script)
		if err != nil {
			return nil, fmt.Errorf("link room %s: %w", room.Key, err)
		}
	}
	for _, m := range world.Monsters {
		id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("sid64.quest/world/monsters/"+m.Key))
		_, err := tx.ExecContext(ctx, `INSERT INTO npcs(id,name,description,room_id,health,max_health,hostile,attack_damage,defense,reward_gold,reward_experience,respawn_seconds) VALUES($1,$2,$3,$4,$5,$5,true,$6,$7,$8,$9,$10) ON CONFLICT(id) DO NOTHING`, id, m.Name, m.Description, ids[m.Room], m.Health, m.Attack, m.Defense, m.Gold, m.Experience, m.Respawn)
		if err != nil {
			return nil, fmt.Errorf("install monster %s: %w", m.Key, err)
		}
	}
	return names, nil
}
