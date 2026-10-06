BEGIN;
ALTER TABLE users ADD COLUMN account_status TEXT NOT NULL DEFAULT 'active'
    CHECK (account_status IN ('active', 'suspended'));
ALTER TABLE membership_events DROP CONSTRAINT membership_events_action_check;
ALTER TABLE membership_events ADD CONSTRAINT membership_events_action_check
    CHECK (action IN ('grant', 'revoke', 'suspend', 'reactivate', 'logout_all'));
COMMIT;
