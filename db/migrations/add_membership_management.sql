BEGIN;

-- Legacy memberships have no tier; preserve their dates and assign Basic.
ALTER TABLE subscriptions ADD COLUMN plan TEXT NOT NULL DEFAULT 'basic'
    CHECK (plan IN ('basic', 'standard', 'premium'));
ALTER TABLE users ADD COLUMN membership_revision BIGINT NOT NULL DEFAULT 0
    CHECK (membership_revision >= 0);
CREATE INDEX subscriptions_active_expiry_idx
    ON subscriptions (user_id, expires_at DESC) WHERE status = 'active';

CREATE TABLE membership_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    actor_id BIGINT NOT NULL REFERENCES users(id),
    action TEXT NOT NULL CHECK (action IN ('grant', 'revoke')),
    plan TEXT CHECK (plan IN ('basic', 'standard', 'premium')),
    expires_at TIMESTAMPTZ,
    revision BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, revision)
);
CREATE INDEX membership_events_user_idx ON membership_events (user_id, id DESC);
COMMIT;
