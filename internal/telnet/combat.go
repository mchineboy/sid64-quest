package telnet

import (
	"fmt"
	"github.com/tylerhardison/race-condition-kingdom/internal/events"
)

func (s *Server) attackNPC(conn *Connection, query string) error {
	from := conn.Room.ID
	result, err := s.world.Attack(conn.Context, conn.Character.ID, from, query)
	if err != nil {
		return conn.SendError(err.Error())
	}
	character, err := s.world.LoadCharacter(conn.Context, conn.Character.ID)
	if err != nil {
		return err
	}
	conn.Character = character
	if err = conn.SendMessage(result.Message); err != nil {
		return err
	}
	if !result.Defeated {
		return nil
	}
	room, err := s.world.GetRoom(conn.Context, result.RoomID)
	if err != nil {
		return err
	}
	conn.Room = room
	s.hub.update(conn)
	s.broadcastToRoom(from, fmt.Sprintf("%s falls and is carried back to the inn.", character.Name))
	s.broadcastToRoom(room.ID, fmt.Sprintf("%s is brought in, bruised but alive.", character.Name))
	if err = s.publish(events.PlayerMoveEvent(character.ID, from, room.ID, "defeat")); err != nil {
		s.logger.WithError(err).Warn("Failed to publish combat recovery movement")
	}
	if err = s.sendLook(conn); err != nil {
		return err
	}
	_, err = s.runScriptHook(conn, "room", room.ID, "on_enter", "defeat", "")
	return err
}
