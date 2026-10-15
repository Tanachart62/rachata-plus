BEGIN;
ALTER TABLE videos ADD COLUMN processing_generation BIGINT NOT NULL DEFAULT 0 CHECK (processing_generation >= 0);
ALTER TABLE videos ADD COLUMN hls_generation BIGINT NOT NULL DEFAULT 0 CHECK (hls_generation >= 0);
CREATE TABLE playback_leases (
    token_hash BYTEA PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    session_hash BYTEA NOT NULL REFERENCES sessions(token_hash) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_id BIGINT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    generation BIGINT NOT NULL CHECK (generation >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    CHECK (expires_at > created_at)
);
CREATE INDEX playback_leases_user_idx ON playback_leases(user_id, expires_at);
CREATE INDEX playback_leases_expiry_idx ON playback_leases(expires_at);
COMMIT;
