BEGIN;
ALTER TABLE watch_history ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0);
CREATE INDEX IF NOT EXISTS watchlist_recent_idx ON watchlist(user_id,created_at DESC,video_id DESC);
CREATE INDEX IF NOT EXISTS watch_history_page_idx ON watch_history(user_id,watched_at DESC,video_id DESC);
DROP INDEX IF EXISTS watch_history_recent_idx;
CREATE TABLE IF NOT EXISTS media_reservations (
 storage_key VARCHAR(32) PRIMARY KEY CHECK (storage_key ~ '^[0-9a-f]{32}$'),
 reserved_bytes BIGINT NOT NULL CHECK (reserved_bytes > 0),
 lease_until TIMESTAMPTZ
);
COMMIT;
