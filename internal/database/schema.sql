-- SID64 Quest Database Schema
-- PostgreSQL Schema for critical game data

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_login TIMESTAMP WITH TIME ZONE,
    is_active BOOLEAN DEFAULT true,
    permissions JSONB DEFAULT '{}'::jsonb
);

-- Characters table
CREATE TABLE IF NOT EXISTS characters (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(50) UNIQUE NOT NULL,
    level INTEGER DEFAULT 1 CHECK (level > 0),
    experience BIGINT DEFAULT 0 CHECK (experience >= 0),
    health INTEGER DEFAULT 100 CHECK (health >= 0),
    max_health INTEGER DEFAULT 100 CHECK (max_health > 0),
    stamina INTEGER DEFAULT 100 CHECK (stamina >= 0),
    max_stamina INTEGER DEFAULT 100 CHECK (max_stamina > 0),
    gold BIGINT DEFAULT 0 CHECK (gold >= 0),
    alignment_lawful INTEGER DEFAULT 0 CHECK (alignment_lawful >= -100 AND alignment_lawful <= 100),
    alignment_good INTEGER DEFAULT 0 CHECK (alignment_good >= -100 AND alignment_good <= 100),
    current_room_id UUID,
    last_rest TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    is_sleeping BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Scripts table (for Starlark scripts)
CREATE TABLE IF NOT EXISTS scripts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    content TEXT NOT NULL,
    script_type VARCHAR(50) NOT NULL, -- room, item, npc, combat, etc.
    created_by UUID NOT NULL REFERENCES users(id),
    approved_by UUID REFERENCES users(id),
    is_active BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    approved_at TIMESTAMP WITH TIME ZONE
);

-- Rooms table
CREATE TABLE IF NOT EXISTS rooms (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    short_description VARCHAR(255),
    room_type VARCHAR(50) DEFAULT 'normal', -- safe, pvp, shop, inn, bank, etc.
    exits JSONB DEFAULT '{}'::jsonb,
    flags JSONB DEFAULT '{}'::jsonb,
    script_id UUID REFERENCES scripts(id),
    created_by UUID REFERENCES users(id),
    approved_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    approved_at TIMESTAMP WITH TIME ZONE
);

-- Add foreign key constraint for current_room_id after rooms table is created
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_characters_current_room'
    ) THEN
        ALTER TABLE characters
        ADD CONSTRAINT fk_characters_current_room
        FOREIGN KEY (current_room_id) REFERENCES rooms(id);
    END IF;
END $$;

-- Items table
CREATE TABLE IF NOT EXISTS items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    item_type VARCHAR(50) NOT NULL, -- weapon, armor, consumable, trinket, etc.
    weight DECIMAL(10,2) DEFAULT 0 CHECK (weight >= 0),
    value BIGINT DEFAULT 0 CHECK (value >= 0),
    properties JSONB DEFAULT '{}'::jsonb,
    alignment_required JSONB DEFAULT '{}'::jsonb,
    script_id UUID REFERENCES scripts(id),
    created_by UUID REFERENCES users(id),
    approved_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- NPCs table
CREATE TABLE IF NOT EXISTS npcs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    room_id UUID NOT NULL REFERENCES rooms(id),
    health INTEGER DEFAULT 100 CHECK (health >= 0),
    max_health INTEGER DEFAULT 100 CHECK (max_health > 0),
    level INTEGER DEFAULT 1 CHECK (level > 0),
    properties JSONB DEFAULT '{}'::jsonb,
    script_id UUID REFERENCES scripts(id),
    created_by UUID REFERENCES users(id),
    approved_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    approved_at TIMESTAMP WITH TIME ZONE
);

-- Player inventory
CREATE TABLE IF NOT EXISTS inventory (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    character_id UUID NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES items(id),
    quantity INTEGER DEFAULT 1 CHECK (quantity > 0),
    equipped BOOLEAN DEFAULT false,
    acquired_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE (character_id, item_id)
);

-- Items sitting in a room until someone takes them
CREATE TABLE IF NOT EXISTS room_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES items(id),
    quantity INTEGER DEFAULT 1 CHECK (quantity > 0),
    UNIQUE (room_id, item_id)
);

-- Repeatable fetch-and-return progress
CREATE TABLE IF NOT EXISTS character_objectives (
    character_id UUID PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,
    deliveries INTEGER NOT NULL DEFAULT 0 CHECK (deliveries >= 0),
    last_delivered_at TIMESTAMP WITH TIME ZONE
);

-- Economic transactions
CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    from_character_id UUID REFERENCES characters(id),
    to_character_id UUID REFERENCES characters(id),
    item_id UUID REFERENCES items(id),
    gold_amount BIGINT DEFAULT 0 CHECK (gold_amount >= 0),
    transaction_type VARCHAR(50) NOT NULL, -- sale, gift, auction, death_penalty, etc.
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Auction house
CREATE TABLE IF NOT EXISTS auctions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    seller_id UUID NOT NULL REFERENCES characters(id),
    item_id UUID NOT NULL REFERENCES items(id),
    starting_bid BIGINT NOT NULL CHECK (starting_bid > 0),
    current_bid BIGINT CHECK (current_bid >= starting_bid),
    current_bidder_id UUID REFERENCES characters(id),
    ends_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'completed', 'cancelled')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_characters_user_id ON characters(user_id);
CREATE INDEX IF NOT EXISTS idx_characters_name ON characters(name);
CREATE INDEX IF NOT EXISTS idx_characters_current_room ON characters(current_room_id);
CREATE INDEX IF NOT EXISTS idx_rooms_type ON rooms(room_type);
CREATE INDEX IF NOT EXISTS idx_inventory_character ON inventory(character_id);
CREATE INDEX IF NOT EXISTS idx_inventory_item ON inventory(item_id);
CREATE INDEX IF NOT EXISTS idx_room_items_room ON room_items(room_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_items_name ON items(name);
CREATE INDEX IF NOT EXISTS idx_transactions_from_char ON transactions(from_character_id);
CREATE INDEX IF NOT EXISTS idx_transactions_to_char ON transactions(to_character_id);
CREATE INDEX IF NOT EXISTS idx_transactions_created ON transactions(created_at);
CREATE INDEX IF NOT EXISTS idx_auctions_seller ON auctions(seller_id);
CREATE INDEX IF NOT EXISTS idx_auctions_status ON auctions(status);
CREATE INDEX IF NOT EXISTS idx_auctions_ends_at ON auctions(ends_at);
CREATE INDEX IF NOT EXISTS idx_scripts_type ON scripts(script_type);
CREATE INDEX IF NOT EXISTS idx_scripts_active ON scripts(is_active);

-- Initial data: Create the town square and basic rooms
INSERT INTO rooms (id, name, description, short_description, room_type, exits, flags) VALUES
(
    uuid_generate_v4(),
    'Town Square',
    'The heart of the kingdom, a bustling square where adventurers gather. Cobblestone paths lead in all directions, and a magnificent fountain sits in the center. This is a safe haven where no violence is permitted.',
    'A bustling town square with a central fountain',
    'safe',
    '{"north": null, "south": null, "east": null, "west": null}'::jsonb,
    '{"safe_zone": true, "starting_location": true}'::jsonb
),
(
    uuid_generate_v4(),
    'Town Infirmary',
    'A clean, white-walled medical facility where the recently deceased are brought back to life. The smell of antiseptic fills the air, and a large sign reads: "RESURRECTION SERVICES - Insurance Not Accepted. Payment Required Before Discharge."',
    'A medical facility for resurrection services',
    'safe',
    '{"south": null}'::jsonb,
    '{"safe_zone": true, "infirmary": true}'::jsonb
),
(
    uuid_generate_v4(),
    'First National Bank of the Kingdom',
    'A grand marble building with imposing columns. Tellers work behind reinforced glass, and armed guards patrol the premises. A sign advertises "Low Interest Rates! High Security! FDIC Insured*" with tiny text: "*Fantasy Deposit Insurance Corporation"',
    'A secure banking establishment',
    'safe',
    '{"out": null}'::jsonb,
    '{"safe_zone": true, "bank": true, "bank_type": "traditional"}'::jsonb
),
(
    uuid_generate_v4(),
    'Goblin''s Discount Bank & Pawn',
    'A ramshackle building with a crooked sign. The goblin proprietor grins toothily behind a counter piled high with questionable collateral. A banner reads: "No Questions Asked! High Interest Rates! Your Stuff is Safe... Probably!"',
    'A questionable banking establishment run by goblins',
    'safe',
    '{"out": null}'::jsonb,
    '{"safe_zone": true, "bank": true, "bank_type": "goblin"}'::jsonb
),
(
    uuid_generate_v4(),
    'Mystical Emporium',
    'Shelves lined with glowing potions, mysterious scrolls, and arcane artifacts. The air shimmers with magical energy, and strange whispers can be heard from the back room. A wizard in star-spangled robes tends the shop.',
    'A shop specializing in magical items',
    'shop',
    '{"out": null}'::jsonb,
    '{"safe_zone": true, "shop": true, "shop_type": "magic"}'::jsonb
),
(
    uuid_generate_v4(),
    'The Armory',
    'Weapons and armor line the walls in gleaming displays. The sound of hammering echoes from the forge in the back. A burly blacksmith examines each piece with a critical eye.',
    'A shop for weapons and armor',
    'shop',
    '{"out": null}'::jsonb,
    '{"safe_zone": true, "shop": true, "shop_type": "weapons_armor"}'::jsonb
),
(
    uuid_generate_v4(),
    'Curiosities & Trinkets',
    'A cluttered shop filled with odd baubles, jewelry, and mysterious artifacts. Every surface is covered with items of questionable origin and purpose. The elderly shopkeeper peers at you through thick spectacles.',
    'A shop for trinkets and curiosities',
    'shop',
    '{"out": null}'::jsonb,
    '{"safe_zone": true, "shop": true, "shop_type": "trinkets"}'::jsonb
),
(
    uuid_generate_v4(),
    'The Prancing Pony Inn',
    'A cozy inn with a roaring fireplace and the smell of fresh bread. Adventurers sit at wooden tables sharing tales of their exploits. A sign by the stairs reads: "Rooms Available - Infinite Storage Included!"',
    'A comfortable inn for rest and storage',
    'inn',
    '{"out": null}'::jsonb,
    '{"safe_zone": true, "inn": true, "storage": true}'::jsonb
),
(
    uuid_generate_v4(),
    'Auction House',
    'A large hall with a raised platform where an auctioneer calls out bids. Display cases show items up for auction, and excited bidders wave their paddles. A banner reads: "Player-to-Player Trading Made Easy!"',
    'The kingdom''s auction house for player trading',
    'safe',
    '{"out": null}'::jsonb,
    '{"safe_zone": true, "auction_house": true}'::jsonb
);

-- Create some basic items
INSERT INTO items (id, name, description, item_type, weight, value, properties) VALUES
(
    uuid_generate_v4(),
    'Rusty Sword',
    'A well-worn blade with spots of rust. It has seen better days, but it''s still sharp enough to be dangerous.',
    'weapon',
    3.5,
    25,
    '{"damage": 5, "durability": 50, "weapon_type": "sword"}'::jsonb
),
(
    uuid_generate_v4(),
    'Leather Armor',
    'Basic protection made from tanned hide. It''s not much, but it''s better than nothing.',
    'armor',
    8.0,
    50,
    '{"defense": 3, "durability": 75, "armor_type": "light"}'::jsonb
),
(
    uuid_generate_v4(),
    'Health Potion',
    'A small vial filled with red liquid that glows faintly. It smells of herbs and magic.',
    'consumable',
    0.5,
    15,
    '{"healing": 25, "consumable_type": "potion"}'::jsonb
),
(
    uuid_generate_v4(),
    'Stamina Potion',
    'A blue liquid that seems to bubble with energy. Just looking at it makes you feel more alert.',
    'consumable',
    0.5,
    12,
    '{"stamina_restore": 30, "consumable_type": "potion"}'::jsonb
),
(
    uuid_generate_v4(),
    'Lucky Rabbit''s Foot',
    'A small charm that supposedly brings good fortune. The rabbit probably disagrees.',
    'trinket',
    0.1,
    5,
    '{"luck_bonus": 1, "trinket_type": "charm"}'::jsonb
),
(
    uuid_generate_v4(),
    'Misplaced Manifest',
    'A water-stained harbor ledger listing which boats were supposed to arrive. The Town Crier will want this back.',
    'quest',
    0.2,
    0,
    '{"quest": true}'::jsonb
);

-- Create a basic NPC for the town square
INSERT INTO npcs (id, name, description, room_id, health, max_health, level, properties) VALUES
(
    uuid_generate_v4(),
    'Town Crier',
    'An enthusiastic man in colorful robes who shouts the latest news and gossip. He seems to know everything that happens in the kingdom.',
    (SELECT id FROM rooms WHERE name = 'Town Square' LIMIT 1),
    100,
    100,
    1,
    '{"friendly": true, "provides_info": true, "respawn": true}'::jsonb
);

INSERT INTO room_items (room_id, item_id, quantity)
SELECT r.id, i.id, 1
FROM rooms r
JOIN items i ON i.name = 'Misplaced Manifest'
WHERE r.name = 'Moonlit Docks'
ON CONFLICT (room_id, item_id) DO NOTHING;

INSERT INTO room_items (room_id, item_id, quantity)
SELECT r.id, i.id, 1
FROM rooms r
JOIN items i ON i.name = 'Health Potion'
WHERE r.name = 'The Prancing Pony Inn'
ON CONFLICT (room_id, item_id) DO NOTHING;

INSERT INTO room_items (room_id, item_id, quantity)
SELECT r.id, i.id, 1
FROM rooms r
JOIN items i ON i.name = 'Rusty Sword'
WHERE r.name = 'Market Lane'
ON CONFLICT (room_id, item_id) DO NOTHING;

INSERT INTO room_items (room_id, item_id, quantity)
SELECT r.id, i.id, 1
FROM rooms r
JOIN items i ON i.name = 'Leather Armor'
WHERE r.name = 'Market Lane'
ON CONFLICT (room_id, item_id) DO NOTHING;
