package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user account
type User struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	Username     string                 `json:"username" db:"username"`
	Email        string                 `json:"email" db:"email"`
	PasswordHash string                 `json:"-" db:"password_hash"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
	LastLogin    *time.Time             `json:"last_login" db:"last_login"`
	IsActive     bool                   `json:"is_active" db:"is_active"`
	Permissions  map[string]interface{} `json:"permissions" db:"permissions"`
}

// Character represents a player character
type Character struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	UserID          uuid.UUID  `json:"user_id" db:"user_id"`
	Name            string     `json:"name" db:"name"`
	Level           int        `json:"level" db:"level"`
	Experience      int64      `json:"experience" db:"experience"`
	Health          int        `json:"health" db:"health"`
	MaxHealth       int        `json:"max_health" db:"max_health"`
	Stamina         int        `json:"stamina" db:"stamina"`
	MaxStamina      int        `json:"max_stamina" db:"max_stamina"`
	Gold            int64      `json:"gold" db:"gold"`
	AlignmentLawful int        `json:"alignment_lawful" db:"alignment_lawful"` // -100 to 100
	AlignmentGood   int        `json:"alignment_good" db:"alignment_good"`     // -100 to 100
	CurrentRoomID   *uuid.UUID `json:"current_room_id" db:"current_room_id"`
	LastRest        time.Time  `json:"last_rest" db:"last_rest"`
	IsSleeping      bool       `json:"is_sleeping" db:"is_sleeping"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	Deliveries      int        `json:"deliveries,omitempty"`
}

// Room represents a location in the game world
type Room struct {
	ID               uuid.UUID              `json:"id" db:"id"`
	Name             string                 `json:"name" db:"name"`
	Description      string                 `json:"description" db:"description"`
	ShortDescription string                 `json:"short_description" db:"short_description"`
	RoomType         string                 `json:"room_type" db:"room_type"` // safe, pvp, shop, etc.
	Exits            map[string]uuid.UUID   `json:"exits" db:"exits"`
	Flags            map[string]interface{} `json:"flags" db:"flags"`
	ScriptID         *uuid.UUID             `json:"script_id" db:"script_id"`
	CreatedBy        *uuid.UUID             `json:"created_by" db:"created_by"`
	ApprovedBy       *uuid.UUID             `json:"approved_by" db:"approved_by"`
	CreatedAt        time.Time              `json:"created_at" db:"created_at"`
	ApprovedAt       *time.Time             `json:"approved_at" db:"approved_at"`
}

// Item represents an item in the game
type Item struct {
	ID                uuid.UUID              `json:"id" db:"id"`
	Name              string                 `json:"name" db:"name"`
	Description       string                 `json:"description" db:"description"`
	ItemType          string                 `json:"item_type" db:"item_type"` // weapon, armor, consumable, etc.
	Weight            float64                `json:"weight" db:"weight"`
	Value             int64                  `json:"value" db:"value"`
	Properties        map[string]interface{} `json:"properties" db:"properties"`
	AlignmentRequired map[string]interface{} `json:"alignment_required" db:"alignment_required"`
	ScriptID          *uuid.UUID             `json:"script_id" db:"script_id"`
	CreatedBy         *uuid.UUID             `json:"created_by" db:"created_by"`
	ApprovedBy        *uuid.UUID             `json:"approved_by" db:"approved_by"`
	CreatedAt         time.Time              `json:"created_at" db:"created_at"`
}

// InventoryItem represents an item in a character's inventory
type InventoryItem struct {
	ID          uuid.UUID `json:"id" db:"id"`
	CharacterID uuid.UUID `json:"character_id" db:"character_id"`
	ItemID      uuid.UUID `json:"item_id" db:"item_id"`
	Quantity    int       `json:"quantity" db:"quantity"`
	Equipped    bool      `json:"equipped" db:"equipped"`
	AcquiredAt  time.Time `json:"acquired_at" db:"acquired_at"`
	Item        *Item     `json:"item,omitempty"` // Populated via JOIN
}

// Transaction represents an economic transaction
type Transaction struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	FromCharacterID *uuid.UUID `json:"from_character_id" db:"from_character_id"`
	ToCharacterID   *uuid.UUID `json:"to_character_id" db:"to_character_id"`
	ItemID          *uuid.UUID `json:"item_id" db:"item_id"`
	GoldAmount      int64      `json:"gold_amount" db:"gold_amount"`
	TransactionType string     `json:"transaction_type" db:"transaction_type"` // sale, gift, auction, etc.
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	FromCharacter   *Character `json:"from_character,omitempty"` // Populated via JOIN
	ToCharacter     *Character `json:"to_character,omitempty"`   // Populated via JOIN
	Item            *Item      `json:"item,omitempty"`           // Populated via JOIN
}

// Auction represents an auction house listing
type Auction struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	SellerID        uuid.UUID  `json:"seller_id" db:"seller_id"`
	ItemID          uuid.UUID  `json:"item_id" db:"item_id"`
	StartingBid     int64      `json:"starting_bid" db:"starting_bid"`
	CurrentBid      *int64     `json:"current_bid" db:"current_bid"`
	CurrentBidderID *uuid.UUID `json:"current_bidder_id" db:"current_bidder_id"`
	EndsAt          time.Time  `json:"ends_at" db:"ends_at"`
	Status          string     `json:"status" db:"status"` // active, completed, cancelled
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	Seller          *Character `json:"seller,omitempty"`         // Populated via JOIN
	Item            *Item      `json:"item,omitempty"`           // Populated via JOIN
	CurrentBidder   *Character `json:"current_bidder,omitempty"` // Populated via JOIN
}

// Session represents a player session
type Session struct {
	ID           string     `json:"id"`
	CharacterID  uuid.UUID  `json:"character_id"`
	UserID       uuid.UUID  `json:"user_id"`
	ConnectionID string     `json:"connection_id"`
	LastActivity time.Time  `json:"last_activity"`
	CurrentRoom  *uuid.UUID `json:"current_room"`
	AuthToken    string     `json:"auth_token"`
}

// AuthToken represents a temporary authentication token
type AuthToken struct {
	Token     string    `json:"token"`
	SessionID string    `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

// GameEvent represents an event in the game world
type GameEvent struct {
	ID        uuid.UUID              `json:"id"`
	Type      string                 `json:"type"`
	RoomID    *uuid.UUID             `json:"room_id,omitempty"`
	PlayerID  *uuid.UUID             `json:"player_id,omitempty"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// Command represents a player command
type Command struct {
	Type      string                 `json:"type"`
	Args      []string               `json:"args"`
	PlayerID  uuid.UUID              `json:"player_id"`
	RoomID    uuid.UUID              `json:"room_id"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// WorldBuilderSession represents a world building session
type WorldBuilderSession struct {
	UserID       uuid.UUID `json:"user_id"`
	Mode         string    `json:"mode"`
	SandboxWorld string    `json:"sandbox_world"`
	ActiveSince  time.Time `json:"active_since"`
}

// Script represents a Starlark script
type Script struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	Description string     `json:"description" db:"description"`
	Content     string     `json:"content" db:"content"`
	ScriptType  string     `json:"script_type" db:"script_type"` // room, item, npc, combat, etc.
	CreatedBy   uuid.UUID  `json:"created_by" db:"created_by"`
	ApprovedBy  *uuid.UUID `json:"approved_by" db:"approved_by"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	ApprovedAt  *time.Time `json:"approved_at" db:"approved_at"`
}

// NPC represents a non-player character
type NPC struct {
	ID          uuid.UUID              `json:"id" db:"id"`
	Name        string                 `json:"name" db:"name"`
	Description string                 `json:"description" db:"description"`
	RoomID      uuid.UUID              `json:"room_id" db:"room_id"`
	Health      int                    `json:"health" db:"health"`
	MaxHealth   int                    `json:"max_health" db:"max_health"`
	Level       int                    `json:"level" db:"level"`
	Properties  map[string]interface{} `json:"properties" db:"properties"`
	ScriptID    *uuid.UUID             `json:"script_id" db:"script_id"`
	CreatedBy   *uuid.UUID             `json:"created_by" db:"created_by"`
	ApprovedBy  *uuid.UUID             `json:"approved_by" db:"approved_by"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	ApprovedAt  *time.Time             `json:"approved_at" db:"approved_at"`
}

// Alignment represents character alignment
type Alignment struct {
	Lawful int `json:"lawful"` // -100 (Chaotic) to 100 (Lawful)
	Good   int `json:"good"`   // -100 (Evil) to 100 (Good)
}

// GetAlignment returns the character's alignment
func (c *Character) GetAlignment() Alignment {
	return Alignment{
		Lawful: c.AlignmentLawful,
		Good:   c.AlignmentGood,
	}
}

// IsAlive returns true if the character is alive
func (c *Character) IsAlive() bool {
	return c.Health > 0
}

// CanCarryWeight returns true if the character can carry the additional weight
func (c *Character) CanCarryWeight(additionalWeight float64, maxWeight float64) bool {
	// This would need to calculate current inventory weight
	// For now, just return true - implement proper weight calculation later
	return true
}

// NeedsRest returns true if the character needs to rest
func (c *Character) NeedsRest(restInterval time.Duration) bool {
	return time.Since(c.LastRest) > restInterval
}

// IsSafe returns true if the room is a safe zone
func (r *Room) IsSafe() bool {
	return r.RoomType == "safe"
}

// IsPvP returns true if the room allows PvP
func (r *Room) IsPvP() bool {
	return r.RoomType == "pvp"
}

// IsShop returns true if the room is a shop
func (r *Room) IsShop() bool {
	return r.RoomType == "shop"
}

// GetExitDirection returns the room ID for a given direction, if it exists
func (r *Room) GetExitDirection(direction string) (uuid.UUID, bool) {
	roomID, exists := r.Exits[direction]
	return roomID, exists
}

// IsWeapon returns true if the item is a weapon
func (i *Item) IsWeapon() bool {
	return i.ItemType == "weapon"
}

// IsArmor returns true if the item is armor
func (i *Item) IsArmor() bool {
	return i.ItemType == "armor"
}

// IsConsumable returns true if the item is consumable
func (i *Item) IsConsumable() bool {
	return i.ItemType == "consumable"
}

// CanUse returns true if the character can use this item based on alignment
func (i *Item) CanUse(character *Character) bool {
	// Check alignment requirements
	if lawfulReq, exists := i.AlignmentRequired["lawful"]; exists {
		if lawfulMin, ok := lawfulReq.(map[string]interface{})["min"]; ok {
			if character.AlignmentLawful < int(lawfulMin.(float64)) {
				return false
			}
		}
		if lawfulMax, ok := lawfulReq.(map[string]interface{})["max"]; ok {
			if character.AlignmentLawful > int(lawfulMax.(float64)) {
				return false
			}
		}
	}

	if goodReq, exists := i.AlignmentRequired["good"]; exists {
		if goodMin, ok := goodReq.(map[string]interface{})["min"]; ok {
			if character.AlignmentGood < int(goodMin.(float64)) {
				return false
			}
		}
		if goodMax, ok := goodReq.(map[string]interface{})["max"]; ok {
			if character.AlignmentGood > int(goodMax.(float64)) {
				return false
			}
		}
	}

	return true
}

// IsActive returns true if the auction is still active
func (a *Auction) IsActive() bool {
	return a.Status == "active" && time.Now().Before(a.EndsAt)
}

// IsExpired returns true if the auction has expired
func (a *Auction) IsExpired() bool {
	return time.Now().After(a.EndsAt)
}
