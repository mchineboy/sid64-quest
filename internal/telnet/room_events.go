package telnet

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/tylerhardison/race-condition-kingdom/internal/game"
)

// A per-player view is checkpointed with its output. This reports real changes
// to room contents, including changes made while a core is being replaced.
// It observes net changes between polls, not every intermediate world mutation.
type roomScene struct {
	Room  uuid.UUID
	Items map[string]sceneEntry
	NPCs  map[string]sceneEntry
}

type sceneEntry struct {
	Name     string
	Quantity int
}

func (c *Connection) currentPrompt() string {
	if c.ScriptEditor != nil {
		return "Edit> "
	}
	return c.formatPrompt()
}

func (s *coreSession) observeRoom(world *game.WorldService, notify bool) error {
	c := s.conn
	if c.Room == nil {
		return nil
	}
	next := &roomScene{Room: c.Room.ID, Items: map[string]sceneEntry{}, NPCs: map[string]sceneEntry{}}
	items, err := world.ListRoomItems(c.Context, c.Room.ID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Quantity > 0 {
			next.Items[item.ItemID.String()] = sceneEntry{item.Name, item.Quantity}
		}
	}
	npcs, err := world.ListRoomNPCs(c.Context, c.Room.ID)
	if err != nil {
		return err
	}
	for _, npc := range npcs {
		if npc.Health > 0 {
			next.NPCs[npc.ID.String()] = sceneEntry{npc.Name, 1}
		}
	}
	previous := s.saved.Scene
	if notify && previous != nil && previous.Room == next.Room {
		for _, message := range sceneChanges(previous, next) {
			if err := c.SendMessage("\r\n" + message); err != nil {
				return err
			}
		}
	}
	s.saved.Scene = next
	return nil
}

func sceneChanges(before, after *roomScene) []string {
	var messages []string
	for _, pair := range [][2]map[string]sceneEntry{{before.Items, after.Items}, {before.NPCs, after.NPCs}} {
		for id, entry := range pair[1] {
			if delta := entry.Quantity - pair[0][id].Quantity; delta > 0 {
				messages = append(messages, fmt.Sprintf("%s appears here. (+%d)", entry.Name, delta))
			}
		}
		for id, entry := range pair[0] {
			if delta := entry.Quantity - pair[1][id].Quantity; delta > 0 {
				messages = append(messages, fmt.Sprintf("%s is no longer here. (-%d)", entry.Name, delta))
			}
		}
	}
	sort.Strings(messages)
	return messages
}
