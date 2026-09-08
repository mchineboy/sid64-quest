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
