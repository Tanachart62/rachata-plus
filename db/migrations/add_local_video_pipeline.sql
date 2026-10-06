BEGIN;
ALTER TABLE videos ADD COLUMN publication_status TEXT NOT NULL DEFAULT 'draft'
    CHECK (publication_status IN ('draft','published','hidden'));
ALTER TABLE videos ADD COLUMN category TEXT NOT NULL DEFAULT 'อื่น ๆ';
ALTER TABLE videos ADD COLUMN duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0);
ALTER TABLE videos ADD COLUMN storage_key TEXT UNIQUE CHECK (storage_key ~ '^[0-9a-f]{32}$');
ALTER TABLE videos ADD COLUMN processing_error TEXT NOT NULL DEFAULT '';
ALTER TABLE videos ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE videos ADD CONSTRAINT videos_publish_ready CHECK (publication_status <> 'published' OR status = 'ready');
CREATE INDEX videos_pending_idx ON videos(created_at) WHERE status='pending' AND deleted_at IS NULL;
COMMIT;
