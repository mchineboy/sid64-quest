-- A database trigger covers browser recovery and operator password changes alike.
ALTER TABLE users ADD COLUMN auth_version BIGINT NOT NULL DEFAULT 1;
CREATE FUNCTION bump_auth_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.password_hash IS DISTINCT FROM OLD.password_hash
       OR NEW.email IS DISTINCT FROM OLD.email
       OR NEW.is_active IS DISTINCT FROM OLD.is_active THEN
        NEW.auth_version := OLD.auth_version + 1;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER users_credential_change BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION bump_auth_version();
