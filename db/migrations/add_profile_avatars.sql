ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_jpeg BYTEA
    CHECK (avatar_jpeg IS NULL OR octet_length(avatar_jpeg) <= 262144);
