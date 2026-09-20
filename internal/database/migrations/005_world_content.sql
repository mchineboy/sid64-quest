-- Release-authored scripts have no player owner. Admin permissions still apply.
ALTER TABLE scripts ALTER COLUMN created_by DROP NOT NULL;
CREATE TABLE world_content_rooms (
 content_key TEXT PRIMARY KEY,
 room_id UUID NOT NULL UNIQUE REFERENCES rooms(id)
);
