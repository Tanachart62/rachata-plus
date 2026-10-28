BEGIN;

ALTER TABLE users ADD COLUMN admin_level TEXT NOT NULL DEFAULT '';
-- Preserve existing administrators' ability to manage the installation.
UPDATE users SET admin_level='owner' WHERE role='admin';
ALTER TABLE users ADD CONSTRAINT users_admin_level_check CHECK (
    (role='user' AND admin_level='') OR
    (role='admin' AND admin_level IN ('admin','owner'))
);
ALTER TABLE users DROP CONSTRAINT users_account_status_check;
ALTER TABLE users ADD CONSTRAINT users_account_status_check
    CHECK (account_status IN ('active','suspended','deleted'));

ALTER TABLE membership_events DROP CONSTRAINT membership_events_action_check;
ALTER TABLE membership_events ADD CONSTRAINT membership_events_action_check CHECK (
    action IN ('grant','revoke','suspend','reactivate','logout_all','logout_session',
               'set_role','reset_password','delete_account')
);
ALTER TABLE membership_events ADD COLUMN admin_level TEXT;

ALTER TABLE videos ADD COLUMN metadata_revision BIGINT NOT NULL DEFAULT 0 CHECK (metadata_revision>=0);
ALTER TABLE videos ADD COLUMN cover_jpeg BYTEA CHECK (octet_length(cover_jpeg)<=5242880);

COMMIT;
