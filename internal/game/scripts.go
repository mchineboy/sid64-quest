package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tylerhardison/race-condition-kingdom/internal/scripting"
)

type ScriptDraft struct {
	ID                  uuid.UUID
	Name, Kind, Content string
	Revision            int
	Active              bool
	PublishedRevision   sql.NullInt64
}

type ScriptRequest struct {
	Action              string
	ID                  uuid.UUID
	Name, Kind, Content string
	Revision            int
	Target              uuid.UUID
}

var scriptName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`)
var scriptTables = map[string]string{"room": "rooms", "item": "items", "npc": "npcs"}

// ScriptCommand checks current database permissions for every operation, including
// save after editing. A stale authenticated session cannot retain builder access.
func (ws *WorldService) ScriptCommand(ctx context.Context, actor uuid.UUID, req ScriptRequest) ([]ScriptDraft, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := ws.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var raw []byte
	if err = tx.QueryRowContext(ctx, `SELECT permissions FROM users WHERE id=$1 AND is_active FOR SHARE`, actor).Scan(&raw); err != nil {
		return nil, fmt.Errorf("active builder account required")
	}
	var permissions map[string]interface{}
	if err = json.Unmarshal(raw, &permissions); err != nil {
		return nil, err
	}
	admin := permissions["admin"] == true
	if !admin && permissions["builder"] != true {
		return nil, fmt.Errorf("builder permission required")
	}
	if req.Action == "help" {
		return nil, nil
	}
	if req.Action == "list" {
		rows, err := tx.QueryContext(ctx, `SELECT id,name,script_type,revision,is_active,published_revision FROM scripts WHERE created_by=$1 OR $2 ORDER BY created_at DESC LIMIT 100`, actor, admin)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var result []ScriptDraft
		for rows.Next() {
			var d ScriptDraft
			if err = rows.Scan(&d.ID, &d.Name, &d.Kind, &d.Revision, &d.Active, &d.PublishedRevision); err != nil {
				return nil, err
			}
			result = append(result, d)
		}
		return result, rows.Err()
	}
	if req.Action == "new" {
		if !scriptName.MatchString(req.Name) || scriptTables[req.Kind] == "" {
			return nil, fmt.Errorf("use: script new <room|item|npc> <lowercase-name>")
		}
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM scripts WHERE created_by=$1`, actor).Scan(&count); err != nil {
			return nil, err
		}
		if count >= 100 {
			return nil, fmt.Errorf("100 scripts per author limit reached")
		}
		hook := "on_enter"
		if req.Kind == "npc" {
			hook = "on_talk"
		}
		if req.Kind == "item" {
			hook = "on_use"
		}
		content := "def " + hook + "(event):\n    tell(\"Hello, \" + event.player.name + \"!\")\n"
		var d ScriptDraft
		err = tx.QueryRowContext(ctx, `INSERT INTO scripts(name,script_type,content,created_by) VALUES($1,$2,$3,$4) RETURNING id,name,script_type,content,revision,is_active,published_revision`, req.Name, req.Kind, content, actor).Scan(&d.ID, &d.Name, &d.Kind, &d.Content, &d.Revision, &d.Active, &d.PublishedRevision)
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO script_audit(script_id,actor_id,action,revision,content) VALUES($1,$2,'create',1,$3)`, d.ID, actor, content); err != nil {
			return nil, err
		}
		return []ScriptDraft{d}, tx.Commit()
	}
	var d ScriptDraft
	var owner uuid.UUID
	err = tx.QueryRowContext(ctx, `SELECT id,name,script_type,content,revision,is_active,published_revision,created_by FROM scripts WHERE id=$1 FOR UPDATE`, req.ID).Scan(&d.ID, &d.Name, &d.Kind, &d.Content, &d.Revision, &d.Active, &d.PublishedRevision, &owner)
	if err != nil {
		return nil, fmt.Errorf("script not found")
	}
	if !admin && owner != actor {
		return nil, fmt.Errorf("only the author or an admin can access this script")
	}
	action := req.Action
	if action == "history" {
		rows, err := tx.QueryContext(ctx, `SELECT revision,action,created_at::text FROM script_audit WHERE script_id=$1 ORDER BY id DESC LIMIT 30`, d.ID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var result []ScriptDraft
		for rows.Next() {
			entry := ScriptDraft{ID: d.ID}
			if err := rows.Scan(&entry.Revision, &entry.Name, &entry.Content); err != nil {
				return nil, err
			}
			result = append(result, entry)
		}
		return result, rows.Err()
	}
	switch action {
	case "show", "test":
		return []ScriptDraft{d}, nil
	case "save":
		if len(req.Content) > scripting.MaxSource {
			return nil, fmt.Errorf("source exceeds 16 KiB")
		}
		if req.Revision != d.Revision {
			return nil, fmt.Errorf("draft changed since editing began; abort and reopen to avoid overwriting it")
		}
		d.Content = req.Content
		d.Revision++
		_, err = tx.ExecContext(ctx, `UPDATE scripts SET content=$2,revision=$3 WHERE id=$1`, d.ID, d.Content, d.Revision)
	case "publish":
		if !admin {
			return nil, fmt.Errorf("admin permission required to publish")
		}
		if req.Revision != d.Revision {
			return nil, fmt.Errorf("revision changed; review with script show, then publish that revision")
		}
		if _, err = scripting.Run(ctx, scripting.Input{Source: d.Content, Kind: d.Kind, Validate: true}); err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE scripts SET published_content=content,published_revision=revision,is_active=true,approved_by=$2,approved_at=now() WHERE id=$1`, d.ID, actor)
		d.Active = true
		d.PublishedRevision = sql.NullInt64{Int64: int64(d.Revision), Valid: true}
	case "disable":
		if !admin {
			return nil, fmt.Errorf("admin permission required to disable")
		}
		_, err = tx.ExecContext(ctx, `UPDATE scripts SET is_active=false WHERE id=$1`, d.ID)
		d.Active = false
	case "attach", "detach":
		if !admin {
			return nil, fmt.Errorf("admin permission required to attach or detach")
		}
		table := scriptTables[d.Kind]
		if table == "" {
			return nil, fmt.Errorf("unsupported script type")
		}
		if req.Target == uuid.Nil {
			return nil, fmt.Errorf("target UUID required")
		}
		var result sql.Result
		if action == "attach" {
			if !d.Active {
				return nil, fmt.Errorf("publish the script first")
			}
			result, err = tx.ExecContext(ctx, `UPDATE `+table+` SET script_id=$1 WHERE id=$2`, d.ID, req.Target)
		} else {
			result, err = tx.ExecContext(ctx, `UPDATE `+table+` SET script_id=NULL WHERE id=$2 AND script_id=$1`, d.ID, req.Target)
		}
		if err == nil {
			n, _ := result.RowsAffected()
			if n != 1 {
				return nil, fmt.Errorf("target not found or script not attached")
			}
		}
		action += ":" + req.Target.String()
	case "restore":
		var content string
		err = tx.QueryRowContext(ctx, `SELECT content FROM script_audit WHERE script_id=$1 AND revision=$2 AND content IS NOT NULL ORDER BY id DESC LIMIT 1`, d.ID, req.Revision).Scan(&content)
		if err != nil {
			return nil, fmt.Errorf("saved revision not found")
		}
		d.Content = content
		d.Revision++
		_, err = tx.ExecContext(ctx, `UPDATE scripts SET content=$2,revision=$3 WHERE id=$1`, d.ID, d.Content, d.Revision)
	default:
		return nil, fmt.Errorf("unknown script operation")
	}
	if err != nil {
		return nil, err
	}
	var content interface{}
	if req.Action == "save" || req.Action == "restore" {
		content = d.Content
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO script_audit(script_id,actor_id,action,revision,content) VALUES($1,$2,$3,$4,$5)`, d.ID, actor, action, d.Revision, content)
	if err != nil {
		return nil, err
	}
	return []ScriptDraft{d}, tx.Commit()
}

// RunScript serializes state for this script/target/player and publishes no
// effects until both evaluation and the state transaction succeed.
func (ws *WorldService) RunScript(ctx context.Context, character, room, target uuid.UUID, kind, hook, command, text string) (scripting.Output, error) {
	table := scriptTables[kind]
	if table == "" {
		return scripting.Output{}, fmt.Errorf("invalid script target")
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	tx, err := ws.db.BeginTx(ctx, nil)
	if err != nil {
		return scripting.Output{}, err
	}
	defer tx.Rollback()
	var id uuid.UUID
	var source, targetName string
	err = tx.QueryRowContext(ctx, `SELECT s.id,s.published_content,t.name FROM `+table+` t JOIN scripts s ON s.id=t.script_id WHERE t.id=$1 AND s.script_type=$2 AND s.is_active AND s.published_content IS NOT NULL FOR SHARE OF s,t`, target, kind).Scan(&id, &source, &targetName)
	if err == sql.ErrNoRows {
		return scripting.Output{}, nil
	}
	if err != nil {
		return scripting.Output{}, err
	}
	var name, roomName string
	// Recheck location and target access at execution, rather than trusting client IDs.
	if err = tx.QueryRowContext(ctx, `SELECT c.name,r.name FROM characters c JOIN rooms r ON r.id=c.current_room_id WHERE c.id=$1 AND r.id=$2`, character, room).Scan(&name, &roomName); err != nil {
		return scripting.Output{}, err
	}
	if kind == "room" && target != room {
		return scripting.Output{}, fmt.Errorf("room is not accessible")
	}
	if kind == "npc" {
		var exists bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM npcs WHERE id=$1 AND room_id=$2)`, target, room).Scan(&exists)
		if err != nil || !exists {
			return scripting.Output{}, fmt.Errorf("npc is not here")
		}
	}
	if kind == "item" {
		var exists bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inventory WHERE item_id=$1 AND character_id=$2 AND quantity>0)`, target, character).Scan(&exists)
		if err != nil || !exists {
			return scripting.Output{}, fmt.Errorf("item is not carried")
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO script_state(script_id,target_id,character_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, id, target, character)
	if err != nil {
		return scripting.Output{}, err
	}
	var stateJSON []byte
	err = tx.QueryRowContext(ctx, `SELECT data FROM script_state WHERE script_id=$1 AND target_id=$2 AND character_id=$3 FOR UPDATE`, id, target, character).Scan(&stateJSON)
	if err != nil {
		return scripting.Output{}, err
	}
	var state map[string]string
	if err = json.Unmarshal(stateJSON, &state); err != nil {
		return scripting.Output{}, err
	}
	out, err := scripting.Run(ctx, scripting.Input{Source: source, Kind: kind, Hook: hook, Player: map[string]string{"id": character.String(), "name": name}, Room: map[string]string{"id": room.String(), "name": roomName}, Target: map[string]string{"id": target.String(), "name": targetName, "kind": kind}, Command: command, Text: text, State: state})
	if err != nil {
		return scripting.Output{}, fmt.Errorf("script %s: %w", id, err)
	}
	if out.Gold != 0 || out.Heal != 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE characters SET gold=gold+$2,health=LEAST(max_health,health+$3) WHERE id=$1`, character, out.Gold, out.Heal); err != nil {
			return scripting.Output{}, err
		}
	}
	stateJSON, err = json.Marshal(out.State)
	if err != nil {
		return scripting.Output{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE script_state SET data=$4 WHERE script_id=$1 AND target_id=$2 AND character_id=$3`, id, target, character, stateJSON)
	if err != nil {
		return scripting.Output{}, err
	}
	return out, tx.Commit()
}

// ScriptTarget uses the same unambiguous name matching as normal gameplay.
func (ws *WorldService) ScriptTarget(ctx context.Context, character, room uuid.UUID, kind, query string) (uuid.UUID, error) {
	if kind == "npc" {
		items, err := ws.ListRoomNPCs(ctx, room)
		if err != nil {
			return uuid.Nil, err
		}
		if strings.TrimSpace(query) == "" && len(items) == 1 {
			return items[0].ID, nil
		}
		var ids []uuid.UUID
		for _, item := range items {
			if MatchesName(item.Name, query) {
				ids = append(ids, item.ID)
			}
		}
		if len(ids) == 1 {
			return ids[0], nil
		}
	} else if kind == "item" {
		items, err := ws.ListInventory(ctx, character)
		if err != nil {
			return uuid.Nil, err
		}
		var ids []uuid.UUID
		for _, item := range items {
			if MatchesName(item.Item.Name, query) {
				ids = append(ids, item.ItemID)
			}
		}
		if len(ids) == 1 {
			return ids[0], nil
		}
	}
	return uuid.Nil, fmt.Errorf("no unique target")
}
