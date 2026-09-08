ALTER TABLE scripts ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE scripts ADD COLUMN published_content TEXT;
ALTER TABLE scripts ADD COLUMN published_revision INTEGER;
-- A legacy active flag without an approved immutable source is not executable.
UPDATE scripts SET is_active=false;
CREATE TABLE script_state (
 script_id UUID NOT NULL REFERENCES scripts(id) ON DELETE CASCADE,
 target_id UUID NOT NULL,
 character_id UUID NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
 data JSONB NOT NULL DEFAULT '{}',
 PRIMARY KEY (script_id,target_id,character_id)
);
CREATE TABLE script_audit (
 id BIGSERIAL PRIMARY KEY,
 script_id UUID NOT NULL REFERENCES scripts(id) ON DELETE CASCADE,
 actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
 action TEXT NOT NULL,
 revision INTEGER NOT NULL,
 content TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX script_audit_script ON script_audit(script_id,id DESC);
