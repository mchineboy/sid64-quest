-- Currency is stored in copper units: 100 copper = 1 silver, 100 silver = 1 gold.
-- Preserve the purchasing power of balances written by earlier releases.
UPDATE characters SET gold = gold * 10000;
UPDATE transactions SET gold_amount = gold_amount * 10000;

ALTER TABLE characters
 ADD COLUMN is_dead BOOLEAN NOT NULL DEFAULT false,
 ADD COLUMN died_at TIMESTAMPTZ,
 ADD COLUMN resurrection_ready_at TIMESTAMPTZ,
 ADD COLUMN death_room_id UUID REFERENCES rooms(id);

ALTER TABLE npcs
 ADD COLUMN loot_item_id UUID REFERENCES items(id),
 ADD COLUMN loot_chance INTEGER NOT NULL DEFAULT 0 CHECK (loot_chance BETWEEN 0 AND 10000);

CREATE TABLE monster_corpses (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    npc_id UUID NOT NULL REFERENCES npcs(id) ON DELETE CASCADE,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    owner_id UUID NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    monster_name VARCHAR(100) NOT NULL,
    currency_value BIGINT NOT NULL CHECK (currency_value >= 0),
    item_id UUID REFERENCES items(id),
    currency_looted BOOLEAN NOT NULL DEFAULT false,
    item_looted BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '30 minutes'
);

CREATE UNIQUE INDEX monster_corpses_active_npc
 ON monster_corpses(npc_id) WHERE NOT (currency_looted AND item_looted);
CREATE INDEX monster_corpses_room ON monster_corpses(room_id, expires_at);
CREATE INDEX characters_resurrection_ready ON characters(resurrection_ready_at) WHERE is_dead;

INSERT INTO items(id, name, description, item_type, weight, value, properties)
VALUES
 (uuid_generate_v5(uuid_ns_url(), 'sid64.quest/items/reinforced-hide'),
  'Reinforced Hide', 'Layered hide bound with bronze rings. Uncommon, but dependable.',
  'armor', 9, 20000, '{"defense":5,"armor_type":"medium","rarity":"uncommon"}'),
 (uuid_generate_v5(uuid_ns_url(), 'sid64.quest/items/warden-mail'),
  'Warden Mail', 'Close-linked silvered mail once worn by dungeon wardens.',
  'armor', 12, 60000, '{"defense":7,"armor_type":"medium","rarity":"rare"}'),
 (uuid_generate_v5(uuid_ns_url(), 'sid64.quest/items/runed-plate'),
  'Runed Plate', 'Heavy plate traced with protective gold runes.',
  'armor', 18, 150000, '{"defense":10,"armor_type":"heavy","rarity":"epic"}'),
 (uuid_generate_v5(uuid_ns_url(), 'sid64.quest/items/crownward-aegis'),
  'Crownward Aegis', 'Legendary armor made for guardians of the old crown.',
  'armor', 15, 500000, '{"defense":14,"armor_type":"heavy","rarity":"legendary"}')
ON CONFLICT (name) DO NOTHING;

UPDATE npcs SET loot_item_id=(SELECT id FROM items WHERE name='Warden Mail'), loot_chance=300
 WHERE id IN (
  uuid_generate_v5(uuid_ns_url(), 'sid64.quest/world/monsters/crypt_sentinel'),
  uuid_generate_v5(uuid_ns_url(), 'sid64.quest/world/monsters/crypt_wraith'),
  uuid_generate_v5(uuid_ns_url(), 'sid64.quest/world/monsters/grotto_eel')
 );
UPDATE npcs SET loot_item_id=(SELECT id FROM items WHERE name='Reinforced Hide'), loot_chance=800
 WHERE id IN (
  uuid_generate_v5(uuid_ns_url(), 'sid64.quest/world/monsters/mine_rat'),
  uuid_generate_v5(uuid_ns_url(), 'sid64.quest/world/monsters/grotto_crab')
 );
UPDATE npcs SET loot_item_id=(SELECT id FROM items WHERE name='Runed Plate'), loot_chance=100
 WHERE id=uuid_generate_v5(uuid_ns_url(), 'sid64.quest/world/monsters/mine_golem');
