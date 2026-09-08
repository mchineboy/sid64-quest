-- A command, its terminal checkpoint and output commit in one transaction.
-- Rows also act as sequence tombstones until the edge's reconnect window ends.
CREATE TABLE terminal_sessions (
    id UUID PRIMARY KEY,
    edge_id UUID NOT NULL,
    character_id UUID REFERENCES characters(id) ON DELETE CASCADE,
    sequence BIGINT NOT NULL DEFAULT 0 CHECK (sequence >= 0),
    checkpoint JSONB NOT NULL,
    last_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed BOOLEAN NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX terminal_character_owner ON terminal_sessions(character_id)
    WHERE character_id IS NOT NULL AND NOT closed;
CREATE INDEX terminal_sessions_last_seen ON terminal_sessions(last_seen);
