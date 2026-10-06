BEGIN;

ALTER TABLE users ADD COLUMN username TEXT;
-- Existing accounts receive a unique reserved handle; do not overwrite names.
UPDATE users SET username = 'legacy_' || id::text;
ALTER TABLE users ALTER COLUMN username SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_username_format
    CHECK (username ~ '^[a-z0-9_]{3,30}$');
CREATE UNIQUE INDEX users_username_unique ON users (username);

CREATE TABLE sessions (
    token_hash BYTEA PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    CHECK (expires_at > created_at)
);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

COMMIT;
