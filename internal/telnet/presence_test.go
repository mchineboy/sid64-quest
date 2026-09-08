package telnet

import (
	"github.com/google/uuid"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCharacterOwnershipAndReconnect(t *testing.T) {
	h := newPlayerHub()
	id := uuid.New()
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if h.claim(id, uuid.NewString()) {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d simultaneous owners", wins.Load())
	}
	h.mu.RLock()
	owner := h.owners[id]
	h.mu.RUnlock()
	h.remove("unrelated")
	if h.claim(id, "replacement") {
		t.Fatal("unrelated disconnect released character")
	}
	h.remove(owner)
	if !h.claim(id, "replacement") {
		t.Fatal("disconnect did not release character")
	}
}
func TestSharedListenerPresenceAndBroadcast(t *testing.T) {
	h := newPlayerHub()
	ansi, collectA := drainedConnection(t)
	pet, collectP := drainedConnection(t)
	room := &models.Room{ID: uuid.New(), Name: "Square"}
	for i, c := range []*Connection{ansi, pet} {
		c.ID = uuid.NewString()
		c.State = StateInGame
		c.Room = room
		c.Character = &models.Character{ID: uuid.New(), Name: []string{"Alice", "Bob"}[i], Level: 1}
	}
	pet.SetTerminalType("petscii")
	h.update(ansi)
	h.update(pet)
	a := &Server{hub: h}
	b := &Server{}
	b.SharePlayers(a)
	if len(b.hub.snapshots()) != 2 {
		t.Fatal("listeners do not share presence")
	}
	b.broadcastToRoom(room.ID, "Alice says: hello")
	if !strings.Contains(collectA(), "Alice says: hello") {
		t.Fatal("ANSI broadcast missing")
	}
	if !strings.Contains(collectP(), encodePETSCII("Alice says: hello")) {
		t.Fatal("PETSCII broadcast missing")
	}
	h.remove(ansi.ID)
	if len(h.snapshots()) != 1 {
		t.Fatal("disconnected player still visible")
	}
}
