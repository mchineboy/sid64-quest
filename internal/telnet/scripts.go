package telnet

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/tylerhardison/race-condition-kingdom/internal/game"
	"github.com/tylerhardison/race-condition-kingdom/internal/scripting"
)

type scriptEditor struct {
	Draft game.ScriptDraft
	Lines []string
}

const scriptHelp = `SCRIPT COMMANDS
script list | targets
script new room|item|npc <name>
script show|edit|history <id>
script test <id> <hook> [text]
script publish <id> <revision> (admin)
script disable <id> (admin)
script attach|detach <id> <target-id|here> (admin)
script restore <id> <revision>
Drafts are private to their author and admins.
In editor: type source lines, preserving spaces.
.list, .set N <line>, .insert N <line>, .delete N
.save saves the draft; .abort discards edits.
Hooks: on_enter, on_look, on_say, on_command (room),
on_use (item), on_talk (NPC).
API: tell(text), say(text), get_state(key, default=""),
set_state(key, value), award_gold(amount), heal(amount).
Return True to handle a command.
Use event.player.name, event.room.name, event.target.name,
event.command and event.text. State values are strings.`

func (s *Server) scriptCommand(c *Connection, args []string) error {
	if c.Character == nil || c.Character.UserID == uuid.Nil {
		return c.SendError("builder account required")
	}
	if _, err := s.world.ScriptCommand(c.Context, c.Character.UserID, game.ScriptRequest{Action: "help"}); err != nil {
		return c.SendError(err.Error())
	}
	if len(args) == 0 || args[0] == "help" {
		return c.SendMessage(scriptHelp)
	}
	action := strings.ToLower(args[0])
	if action == "targets" {
		lines := []string{"room " + c.Room.ID.String() + " " + c.Room.Name}
		npcs, err := s.world.ListRoomNPCs(c.Context, c.Room.ID)
		if err != nil {
			return err
		}
		for _, n := range npcs {
			lines = append(lines, "npc "+n.ID.String()+" "+n.Name)
		}
		items, err := s.world.ListInventory(c.Context, c.Character.ID)
		if err != nil {
			return err
		}
		for _, i := range items {
			lines = append(lines, "item "+i.ItemID.String()+" "+i.Item.Name)
		}
		return c.SendMessage(strings.Join(lines, "\n"))
	}
	req := game.ScriptRequest{Action: action}
	if action == "new" {
		if len(args) != 3 {
			return c.SendError("script new room|item|npc <name>")
		}
		req.Kind = args[1]
		req.Name = args[2]
	} else if action != "list" {
		if len(args) < 2 {
			return c.SendError("script ID required; use script list")
		}
		var err error
		req.ID, err = uuid.Parse(args[1])
		if err != nil {
			return c.SendError("invalid script UUID")
		}
	}
	if action == "edit" {
		req.Action = "show"
	}
	if action == "publish" || action == "restore" {
		if len(args) != 3 {
			return c.SendError("include the revision number shown by script show")
		}
		var err error
		req.Revision, err = strconv.Atoi(args[2])
		if err != nil {
			return c.SendError("invalid revision")
		}
	}
	if action == "attach" || action == "detach" {
		if len(args) != 3 {
			return c.SendError("include a target UUID or here")
		}
		if args[2] == "here" {
			req.Target = c.Room.ID
		} else {
			var err error
			req.Target, err = uuid.Parse(args[2])
			if err != nil {
				return c.SendError("invalid target UUID")
			}
		}
	}
	drafts, err := s.world.ScriptCommand(c.Context, c.Character.UserID, req)
	if err != nil {
		return c.SendError(err.Error())
	}
	if action == "list" {
		if len(drafts) == 0 {
			return c.SendMessage("No scripts yet. Use script new room <name>.")
		}
		var lines []string
		for _, d := range drafts {
			lines = append(lines, scriptSummary(d))
		}
		return c.SendMessage(strings.Join(lines, "\n"))
	}
	if action == "history" {
		var lines []string
		for _, d := range drafts {
			lines = append(lines, fmt.Sprintf("r%d %s %s", d.Revision, d.Name, d.Content))
		}
		return c.SendMessage(strings.Join(lines, "\n"))
	}
	d := drafts[0]
	switch action {
	case "edit":
		c.ScriptEditor = &scriptEditor{Draft: d, Lines: strings.Split(strings.TrimSuffix(d.Content, "\n"), "\n")}
		return c.SendMessage("Editing " + d.Name + ". New lines append. .list, .set, .insert, .delete, .save, .abort\n" + numberedSource(c.ScriptEditor.Lines))
	case "show", "new":
		return c.SendMessage(scriptSummary(d) + "\n" + numberedSource(strings.Split(strings.TrimSuffix(d.Content, "\n"), "\n")))
	case "test":
		if len(args) < 3 {
			return c.SendError("script test <id> <hook> [text]")
		}
		if !scripting.ValidHook(d.Kind, args[2]) {
			return c.SendError("hook does not match script type")
		}
		text := strings.Join(args[3:], " ")
		command := ""
		if fields := strings.Fields(text); len(fields) > 0 {
			command = fields[0]
		}
		out, err := scripting.Run(c.Context, scripting.Input{Source: d.Content, Kind: d.Kind, Hook: args[2], Player: map[string]string{"id": c.Character.ID.String(), "name": c.Character.Name}, Room: map[string]string{"id": c.Room.ID.String(), "name": c.Room.Name}, Target: map[string]string{"id": c.Room.ID.String(), "name": "Test target", "kind": d.Kind}, Command: command, Text: text})
		if err != nil {
			return c.SendError(err.Error())
		}
		for _, m := range out.Messages {
			scope := "you"
			if m.Room {
				scope = "room"
			}
			if err = c.SendMessage("[preview " + scope + "] " + m.Text); err != nil {
				return err
			}
		}
		return c.SendMessage(fmt.Sprintf("Test passed. handled=%t, %d state keys, gold=+%d, heal=%d. No live changes.", out.Handled, len(out.State), out.Gold, out.Heal))
	default:
		return c.SendMessage(action + " complete. " + scriptSummary(d))
	}
}
func scriptSummary(d game.ScriptDraft) string {
	return fmt.Sprintf("%s %s (%s) draft=%d published=%d active=%t", d.ID, d.Name, d.Kind, d.Revision, d.PublishedRevision.Int64, d.Active)
}
func numberedSource(lines []string) string {
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "%3d %s\n", i+1, line)
	}
	return b.String()
}

func (s *Server) editScript(c *Connection, input string) error {
	e := c.ScriptEditor
	// Recheck permission even while an editor is open.
	if _, err := s.world.ScriptCommand(c.Context, c.Character.UserID, game.ScriptRequest{Action: "show", ID: e.Draft.ID}); err != nil {
		c.ScriptEditor = nil
		return c.SendError(err.Error())
	}
	trimmed := strings.TrimSpace(input)
	switch trimmed {
	case ".abort":
		c.ScriptEditor = nil
		return c.SendMessage("Draft edits discarded.")
	case ".list":
		return c.SendMessage(numberedSource(e.Lines))
	case ".save":
		_, err := s.world.ScriptCommand(c.Context, c.Character.UserID, game.ScriptRequest{Action: "save", ID: e.Draft.ID, Revision: e.Draft.Revision, Content: strings.Join(e.Lines, "\n") + "\n"})
		if err != nil {
			return c.SendError(err.Error())
		}
		c.ScriptEditor = nil
		return c.SendMessage("Draft saved. Test it, then ask an admin to review and publish the revision.")
	}
	lines := append([]string(nil), e.Lines...)
	if strings.HasPrefix(input, ".") {
		parts := strings.SplitN(input, " ", 3)
		if len(parts) < 2 {
			return c.SendError("use .list, .set N <line>, .insert N <line>, .delete N, .save, .abort")
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil || n < 1 {
			return c.SendError("invalid line number")
		}
		switch parts[0] {
		case ".delete":
			if n > len(lines) {
				return c.SendError("line does not exist")
			}
			lines = append(lines[:n-1], lines[n:]...)
		case ".set":
			if n > len(lines) || len(parts) != 3 {
				return c.SendError(".set N <source line>")
			}
			lines[n-1] = parts[2]
		case ".insert":
			if n > len(lines)+1 || len(parts) != 3 {
				return c.SendError(".insert N <source line>")
			}
			lines = append(lines, "")
			copy(lines[n:], lines[n-1:])
			lines[n-1] = parts[2]
		default:
			return c.SendError("unknown editor command")
		}
	} else {
		lines = append(lines, input)
	}
	if len(lines) > 256 || len(strings.Join(lines, "\n"))+1 > scripting.MaxSource {
		return c.SendError("draft limit: 256 lines, 16 KiB")
	}
	e.Lines = lines
	return c.SendMessage(fmt.Sprintf("%d lines. .save or continue editing.", len(lines)))
}

func (s *Server) runScriptHook(c *Connection, kind string, target uuid.UUID, hook, command, text string) (bool, error) {
	out, err := s.world.RunScript(c.Context, c.Character.ID, c.Room.ID, target, kind, hook, command, text)
	if err != nil {
		if s.logger != nil {
			s.logger.WithError(err).WithField("hook", hook).Warn("Script failed")
		}
		// Failure falls back to ordinary gameplay and never disconnects the player.
		return false, nil
	}
	if out.Gold != 0 || out.Heal != 0 {
		character, err := s.world.LoadCharacter(c.Context, c.Character.ID)
		if err != nil {
			return false, err
		}
		c.Character = character
	}
	for _, m := range out.Messages {
		if m.Room {
			s.broadcastToRoom(c.Room.ID, m.Text)
		} else if err = c.SendMessage(m.Text); err != nil {
			return false, err
		}
	}
	return out.Handled, nil
}
