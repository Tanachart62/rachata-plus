-- Publication intent is independent of processing state.
-- Public catalog/media routes require both published AND ready.
ALTER TABLE videos DROP CONSTRAINT IF EXISTS videos_publish_ready;
