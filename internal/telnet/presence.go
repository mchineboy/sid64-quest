package telnet

import (
	"github.com/google/uuid"
	"sort"
	"sync"
)

// playerHub is shared by both listeners in the single gateway process.
// Snapshots prevent room broadcasts from reading another player's mutable state.
type playerHub struct {
	mu      sync.RWMutex
	players map[string]playerSnapshot
	owners  map[uuid.UUID]string
}
type playerSnapshot struct {
	conn                *Connection
	characterID, roomID uuid.UUID
	player              OnlinePlayer
	petscii             bool
}

func newPlayerHub() *playerHub {
	return &playerHub{players: map[string]playerSnapshot{}, owners: map[uuid.UUID]string{}}
}
func (s *Server) SharePlayers(other *Server) { s.hub = other.hub }
func (h *playerHub) claim(id uuid.UUID, connID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if owner, ok := h.owners[id]; ok && owner != connID {
		return false
	}
	h.owners[id] = connID
	return true
}
func (h *playerHub) update(c *Connection) {
	if h == nil || c.Character == nil || c.Room == nil || c.State != StateInGame {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.players[c.ID] = playerSnapshot{conn: c, characterID: c.Character.ID, roomID: c.Room.ID, player: OnlinePlayer{Name: c.Character.Name, Level: c.Character.Level, Location: c.Room.Name}, petscii: c.Presentation == PresentationPETSCII}
}
func (h *playerHub) remove(connID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.players, connID)
	for id, owner := range h.owners {
		if owner == connID {
			delete(h.owners, id)
		}
	}
}
func (h *playerHub) snapshots() []playerSnapshot {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]playerSnapshot, 0, len(h.players))
	for _, p := range h.players {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].player.Name < out[j].player.Name })
	return out
}
func (p playerSnapshot) send(message string) error {
	data := message + "\r\n"
	if p.petscii {
		data = petsciiText(data)
	}
	return p.conn.write(data)
}
