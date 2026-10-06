BEGIN;
ALTER TABLE sessions ADD COLUMN session_id BIGINT GENERATED ALWAYS AS IDENTITY;
CREATE UNIQUE INDEX sessions_id_unique ON sessions(session_id);
ALTER TABLE sessions ADD COLUMN user_agent TEXT NOT NULL DEFAULT '' CHECK (length(user_agent)<=512);
CREATE INDEX sessions_user_session_idx ON sessions(user_id,session_id DESC);
ALTER TABLE membership_events ADD COLUMN session_id BIGINT;
ALTER TABLE membership_events DROP CONSTRAINT membership_events_action_check;
ALTER TABLE membership_events ADD CONSTRAINT membership_events_action_check
    CHECK (action IN ('grant','revoke','suspend','reactivate','logout_all','logout_session'));
COMMIT;
