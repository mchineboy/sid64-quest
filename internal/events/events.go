package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// EventType represents different types of game events
type EventType string

const (
	// Player events
	EventPlayerConnect    EventType = "player.connect"
	EventPlayerDisconnect EventType = "player.disconnect"
	EventPlayerMove       EventType = "player.move"
	EventPlayerChat       EventType = "player.chat"
	EventPlayerCommand    EventType = "player.command"
	EventPlayerDeath      EventType = "player.death"
	EventPlayerRespawn    EventType = "player.respawn"

	// Combat events
	EventCombatStart  EventType = "combat.start"
	EventCombatEnd    EventType = "combat.end"
	EventCombatAttack EventType = "combat.attack"
	EventCombatDamage EventType = "combat.damage"

	// Economy events
	EventItemPickup  EventType = "item.pickup"
	EventItemDrop    EventType = "item.drop"
	EventItemUse     EventType = "item.use"
	EventTransaction EventType = "economy.transaction"
	EventAuctionBid  EventType = "auction.bid"
	EventAuctionEnd  EventType = "auction.end"

	// World events
	EventRoomEnter  EventType = "room.enter"
	EventRoomLeave  EventType = "room.leave"
	EventTimeChange EventType = "world.time_change"

	// Admin events
	EventAdminAction EventType = "admin.action"
	EventWorldEdit   EventType = "world.edit"
	EventScriptRun   EventType = "script.run"
)

// Event represents a game event
type Event struct {
	ID        uuid.UUID              `json:"id"`
	Type      EventType              `json:"type"`
	PlayerID  *uuid.UUID             `json:"player_id,omitempty"`
	RoomID    *uuid.UUID             `json:"room_id,omitempty"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// EventBus handles event publishing and subscription
type EventBus struct {
	redis  *redis.Client
	logger *logrus.Logger
	ctx    context.Context
	cancel context.CancelFunc
}

// NewEventBus creates a new event bus
func NewEventBus(redisClient *redis.Client, logger *logrus.Logger) *EventBus {
	ctx, cancel := context.WithCancel(context.Background())

	return &EventBus{
		redis:  redisClient,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Publish publishes an event to the event bus
func (eb *EventBus) Publish(event *Event) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Publish to global events channel
	if err := eb.redis.Publish(eb.ctx, "game_events", data).Err(); err != nil {
		return fmt.Errorf("failed to publish to game_events: %w", err)
	}

	// Publish to room-specific channel if room is specified
	if event.RoomID != nil {
		roomChannel := fmt.Sprintf("room:%s:events", event.RoomID.String())
		if err := eb.redis.Publish(eb.ctx, roomChannel, data).Err(); err != nil {
			eb.logger.WithError(err).Warn("Failed to publish to room channel")
		}
	}

	// Publish to player-specific channel if player is specified
	if event.PlayerID != nil {
		playerChannel := fmt.Sprintf("player:%s:events", event.PlayerID.String())
		if err := eb.redis.Publish(eb.ctx, playerChannel, data).Err(); err != nil {
			eb.logger.WithError(err).Warn("Failed to publish to player channel")
		}
	}

	eb.logger.WithFields(logrus.Fields{
		"event_id":   event.ID,
		"event_type": event.Type,
		"player_id":  event.PlayerID,
		"room_id":    event.RoomID,
	}).Debug("Event published")

	return nil
}

// Subscribe subscribes to events on specified channels
func (eb *EventBus) Subscribe(channels []string, handler func(*Event)) error {
	pubsub := eb.redis.Subscribe(eb.ctx, channels...)
	defer pubsub.Close()

	eb.logger.WithField("channels", channels).Info("Subscribed to event channels")

	for {
		select {
		case <-eb.ctx.Done():
			return eb.ctx.Err()
		default:
			msg, err := pubsub.ReceiveMessage(eb.ctx)
			if err != nil {
				if err == context.Canceled {
					return nil
				}
				eb.logger.WithError(err).Error("Failed to receive message")
				continue
			}

			var event Event
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				eb.logger.WithError(err).Error("Failed to unmarshal event")
				continue
			}

			go handler(&event)
		}
	}
}

// SubscribeToRoom subscribes to events for a specific room
func (eb *EventBus) SubscribeToRoom(roomID uuid.UUID, handler func(*Event)) error {
	channel := fmt.Sprintf("room:%s:events", roomID.String())
	return eb.Subscribe([]string{channel}, handler)
}

// SubscribeToPlayer subscribes to events for a specific player
func (eb *EventBus) SubscribeToPlayer(playerID uuid.UUID, handler func(*Event)) error {
	channel := fmt.Sprintf("player:%s:events", playerID.String())
	return eb.Subscribe([]string{channel}, handler)
}

// SubscribeToGlobal subscribes to all global game events
func (eb *EventBus) SubscribeToGlobal(handler func(*Event)) error {
	return eb.Subscribe([]string{"game_events"}, handler)
}

// Close closes the event bus
func (eb *EventBus) Close() {
	eb.cancel()
}

// EventBuilder helps build events with a fluent interface
type EventBuilder struct {
	event *Event
}

// NewEvent creates a new event builder
func NewEvent(eventType EventType) *EventBuilder {
	return &EventBuilder{
		event: &Event{
			ID:        uuid.New(),
			Type:      eventType,
			Data:      make(map[string]interface{}),
			Timestamp: time.Now(),
		},
	}
}

// WithPlayer sets the player ID for the event
func (eb *EventBuilder) WithPlayer(playerID uuid.UUID) *EventBuilder {
	eb.event.PlayerID = &playerID
	return eb
}

// WithRoom sets the room ID for the event
func (eb *EventBuilder) WithRoom(roomID uuid.UUID) *EventBuilder {
	eb.event.RoomID = &roomID
	return eb
}

// WithData adds data to the event
func (eb *EventBuilder) WithData(key string, value interface{}) *EventBuilder {
	eb.event.Data[key] = value
	return eb
}

// WithDataMap adds multiple data fields to the event
func (eb *EventBuilder) WithDataMap(data map[string]interface{}) *EventBuilder {
	for k, v := range data {
		eb.event.Data[k] = v
	}
	return eb
}

// Build returns the constructed event
func (eb *EventBuilder) Build() *Event {
	return eb.event
}

// Common event builders for convenience

// PlayerConnectEvent creates a player connect event
func PlayerConnectEvent(playerID uuid.UUID, characterName string) *Event {
	return NewEvent(EventPlayerConnect).
		WithPlayer(playerID).
		WithData("character_name", characterName).
		Build()
}

// PlayerDisconnectEvent creates a player disconnect event
func PlayerDisconnectEvent(playerID uuid.UUID, reason string) *Event {
	return NewEvent(EventPlayerDisconnect).
		WithPlayer(playerID).
		WithData("reason", reason).
		Build()
}

// PlayerMoveEvent creates a player movement event
func PlayerMoveEvent(playerID uuid.UUID, fromRoom, toRoom uuid.UUID, direction string) *Event {
	return NewEvent(EventPlayerMove).
		WithPlayer(playerID).
		WithRoom(toRoom).
		WithData("from_room", fromRoom).
		WithData("to_room", toRoom).
		WithData("direction", direction).
		Build()
}

// PlayerChatEvent creates a player chat event
func PlayerChatEvent(playerID uuid.UUID, roomID uuid.UUID, message string, chatType string) *Event {
	return NewEvent(EventPlayerChat).
		WithPlayer(playerID).
		WithRoom(roomID).
		WithData("message", message).
		WithData("chat_type", chatType).
		Build()
}

// CombatAttackEvent creates a combat attack event
func CombatAttackEvent(attackerID, defenderID uuid.UUID, roomID uuid.UUID, damage int, weapon string) *Event {
	return NewEvent(EventCombatAttack).
		WithPlayer(attackerID).
		WithRoom(roomID).
		WithData("attacker_id", attackerID).
		WithData("defender_id", defenderID).
		WithData("damage", damage).
		WithData("weapon", weapon).
		Build()
}

// ItemPickupEvent creates an item pickup event
func ItemPickupEvent(playerID uuid.UUID, roomID uuid.UUID, itemID uuid.UUID, itemName string) *Event {
	return NewEvent(EventItemPickup).
		WithPlayer(playerID).
		WithRoom(roomID).
		WithData("item_id", itemID).
		WithData("item_name", itemName).
		Build()
}

// TransactionEvent creates a transaction event
func TransactionEvent(fromPlayerID, toPlayerID *uuid.UUID, itemID *uuid.UUID, goldAmount int64, transactionType string) *Event {
	event := NewEvent(EventTransaction).
		WithData("gold_amount", goldAmount).
		WithData("transaction_type", transactionType)

	if fromPlayerID != nil {
		event.WithData("from_player_id", *fromPlayerID)
	}
	if toPlayerID != nil {
		event.WithData("to_player_id", *toPlayerID)
	}
	if itemID != nil {
		event.WithData("item_id", *itemID)
	}

	return event.Build()
}

// AdminActionEvent creates an admin action event
func AdminActionEvent(adminID uuid.UUID, action string, target interface{}) *Event {
	return NewEvent(EventAdminAction).
		WithPlayer(adminID).
		WithData("action", action).
		WithData("target", target).
		Build()
}

// WorldEditEvent creates a world edit event
func WorldEditEvent(builderID uuid.UUID, editType string, objectID uuid.UUID, changes map[string]interface{}) *Event {
	return NewEvent(EventWorldEdit).
		WithPlayer(builderID).
		WithData("edit_type", editType).
		WithData("object_id", objectID).
		WithData("changes", changes).
		Build()
}
