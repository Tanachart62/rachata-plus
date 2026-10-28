BEGIN;
ALTER TABLE videos ADD COLUMN cover_s3_key TEXT;
ALTER TABLE users ADD COLUMN avatar_s3_key TEXT;
CREATE TABLE direct_uploads (
 id TEXT PRIMARY KEY CHECK (id ~ '^[0-9a-f]{32}$'),
 actor_id BIGINT NOT NULL REFERENCES users(id),
 session_hash BYTEA NOT NULL,
 upload_id TEXT NOT NULL,
 title TEXT NOT NULL,
 description TEXT NOT NULL,
 category TEXT NOT NULL,
 size_bytes BIGINT NOT NULL CHECK(size_bytes>0),
 part_size BIGINT NOT NULL CHECK(part_size>=5242880),
 expires_at TIMESTAMPTZ NOT NULL,
 video_id BIGINT REFERENCES videos(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX direct_uploads_expiry_idx ON direct_uploads(expires_at);
CREATE TABLE request_limits (
 key_hash BYTEA PRIMARY KEY,
 hits INTEGER NOT NULL,
 resets_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX request_limits_expiry_idx ON request_limits(resets_at);
COMMIT;
