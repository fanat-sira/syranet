UPDATE users SET status='active' WHERE status='approved';
ALTER TABLE users ADD COLUMN IF NOT EXISTS node_id TEXT NOT NULL DEFAULT '';
UPDATE users SET telegram_username=COALESCE(telegram_username,''), first_name=COALESCE(first_name,''), last_name=COALESCE(last_name,''), referral_source=COALESCE(referral_source,''), intended_use=COALESCE(intended_use,''), last_connected_ip=COALESCE(last_connected_ip,'');
ALTER TABLE users ADD CONSTRAINT users_status_valid CHECK (status IN ('pending','active','suspended','expired','rejected','revoked'));
CREATE TABLE desired_state (id INTEGER PRIMARY KEY CHECK(id=1), revision BIGINT NOT NULL DEFAULT 0);
INSERT INTO desired_state(id) VALUES(1);
CREATE TABLE node_sync (node_id TEXT PRIMARY KEY, desired_hash TEXT NOT NULL, applied_hash TEXT NOT NULL DEFAULT '', revision BIGINT NOT NULL DEFAULT 0, last_success TIMESTAMPTZ, healthy BOOLEAN NOT NULL DEFAULT false, error TEXT NOT NULL DEFAULT '');
CREATE TABLE admin_requests (request_id TEXT PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE OR REPLACE FUNCTION bump_desired_revision() RETURNS TRIGGER AS $$
BEGIN
 UPDATE desired_state SET revision=revision+1 WHERE id=1;
 RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER users_desired_revision AFTER INSERT OR UPDATE OR DELETE ON users FOR EACH STATEMENT EXECUTE FUNCTION bump_desired_revision();

ALTER TABLE users ALTER COLUMN telegram_username SET DEFAULT '';
ALTER TABLE users ALTER COLUMN telegram_username SET NOT NULL;
ALTER TABLE users ALTER COLUMN first_name SET DEFAULT '';
ALTER TABLE users ALTER COLUMN first_name SET NOT NULL;
ALTER TABLE users ALTER COLUMN last_name SET DEFAULT '';
ALTER TABLE users ALTER COLUMN last_name SET NOT NULL;
ALTER TABLE users ALTER COLUMN referral_source SET DEFAULT '';
ALTER TABLE users ALTER COLUMN referral_source SET NOT NULL;
ALTER TABLE users ALTER COLUMN intended_use SET DEFAULT '';
ALTER TABLE users ALTER COLUMN intended_use SET NOT NULL;
ALTER TABLE users ALTER COLUMN last_connected_ip SET DEFAULT '';
ALTER TABLE users ALTER COLUMN last_connected_ip SET NOT NULL;
