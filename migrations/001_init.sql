CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    telegram_id BIGINT UNIQUE NOT NULL,
    telegram_username VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    hy2_username VARCHAR(255) UNIQUE NOT NULL,
    hy2_password VARCHAR(255) NOT NULL,
    socks_username VARCHAR(255) UNIQUE NOT NULL,
    socks_password VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_at TIMESTAMPTZ,
    approved_by BIGINT,
    expires_at TIMESTAMPTZ,
    referral_source TEXT,
    intended_use TEXT,
    accepted_terms BOOLEAN NOT NULL DEFAULT FALSE,
    last_connected_at TIMESTAMPTZ,
    last_connected_ip VARCHAR(45),
    current_connections INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS admin_logs (
    id BIGSERIAL PRIMARY KEY,
    admin_id BIGINT NOT NULL,
    target_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(50) NOT NULL,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_expires_at ON users(expires_at);
CREATE INDEX IF NOT EXISTS idx_users_hy2_username ON users(hy2_username);
CREATE INDEX IF NOT EXISTS idx_users_socks_username ON users(socks_username);

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
