-- +goose Up

CREATE TABLE IF NOT EXISTS session_change_batches (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  requested_count integer NOT NULL CHECK (requested_count > 0),
  succeeded_count integer NOT NULL DEFAULT 0 CHECK (succeeded_count >= 0),
  failed_count    integer NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
  status          text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'completed', 'failed')),
  requested_by    uuid NULL REFERENCES users(id),
  idempotency_key text NULL,
  completed_at    timestamptz NULL,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS session_change_batches_idempotency_idx
  ON session_change_batches(idempotency_key)
  WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS session_changes (
  id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id             uuid NOT NULL,
  session_version        integer NOT NULL CHECK (session_version > 0),
  batch_id               uuid NULL REFERENCES session_change_batches(id),
  changed_by             uuid NULL REFERENCES users(id),
  change_source          text NOT NULL DEFAULT 'session_edit',
  changed_fields         jsonb NOT NULL DEFAULT '{}'::jsonb,
  before_snapshot        jsonb NOT NULL,
  after_snapshot         jsonb NOT NULL,
  old_start_at           timestamptz NOT NULL,
  old_end_at             timestamptz NOT NULL,
  new_start_at           timestamptz NOT NULL,
  new_end_at             timestamptz NOT NULL,
  old_course_id          uuid NOT NULL,
  new_course_id          uuid NOT NULL,
  old_room_id            uuid NULL,
  new_room_id            uuid NULL,
  old_teacher_id         uuid NOT NULL,
  new_teacher_id         uuid NOT NULL,
  created_at              timestamptz NOT NULL DEFAULT now(),
  UNIQUE(session_id, session_version)
);

CREATE INDEX IF NOT EXISTS session_changes_session_created_idx
  ON session_changes(session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS session_changes_created_idx
  ON session_changes(created_at DESC);

CREATE TABLE IF NOT EXISTS outbox_events (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type        text NOT NULL,
  aggregate_id      uuid NOT NULL,
  aggregate_version integer NOT NULL,
  payload           jsonb NOT NULL,
  status            text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
  attempts          integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  available_at      timestamptz NOT NULL DEFAULT now(),
  locked_until      timestamptz NULL,
  last_error        text NULL,
  processed_at      timestamptz NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  UNIQUE(event_type, aggregate_id, aggregate_version)
);

CREATE INDEX IF NOT EXISTS outbox_events_claim_idx
  ON outbox_events(status, available_at, created_at);

CREATE TABLE IF NOT EXISTS absence_schedule_issues (
  id                        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  absence_id                uuid NOT NULL REFERENCES student_absences(id) ON DELETE CASCADE,
  issue_type                text NOT NULL,
  severity                  text NOT NULL CHECK (severity IN ('warning', 'critical')),
  status                    text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved', 'dismissed', 'superseded')),
  source_session_id         uuid NULL REFERENCES sessions(id),
  sit_in_session_id         uuid NULL REFERENCES sessions(id),
  missed_session_id         uuid NULL REFERENCES sessions(id),
  first_session_change_id   uuid NOT NULL REFERENCES session_changes(id),
  latest_session_change_id  uuid NOT NULL REFERENCES session_changes(id),
  details_json              jsonb NOT NULL DEFAULT '{}'::jsonb,
  suggested_resolution_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  detected_at               timestamptz NOT NULL DEFAULT now(),
  updated_at                timestamptz NOT NULL DEFAULT now(),
  resolved_at               timestamptz NULL,
  resolved_by               uuid NULL REFERENCES users(id),
  resolution_action         text NULL,
  fingerprint               text NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS absence_schedule_issues_open_fingerprint_idx
  ON absence_schedule_issues(fingerprint)
  WHERE status = 'open';
CREATE INDEX IF NOT EXISTS absence_schedule_issues_queue_idx
  ON absence_schedule_issues(status, severity, updated_at DESC);
CREATE INDEX IF NOT EXISTS absence_schedule_issues_change_idx
  ON absence_schedule_issues(latest_session_change_id);
CREATE INDEX IF NOT EXISTS absence_schedule_issues_absence_idx
  ON absence_schedule_issues(absence_id, status);

ALTER TABLE absence_sit_ins
  ADD COLUMN IF NOT EXISTS session_version_at_assignment integer NULL,
  ADD COLUMN IF NOT EXISTS assigned_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS assigned_by uuid NULL REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS assignment_source text NOT NULL DEFAULT 'absence_submission';

ALTER TABLE absence_missed_sessions
  ADD COLUMN IF NOT EXISTS session_version_at_submission integer NULL,
  ADD COLUMN IF NOT EXISTS original_start_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS original_end_at timestamptz NULL;

CREATE TABLE IF NOT EXISTS absence_sit_in_assignment_events (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  absence_id          uuid NOT NULL REFERENCES student_absences(id) ON DELETE CASCADE,
  previous_session_id uuid NULL REFERENCES sessions(id),
  new_session_id      uuid NULL REFERENCES sessions(id),
  action              text NOT NULL CHECK (action IN ('assigned', 'reassigned', 'cancelled')),
  reason              text NOT NULL DEFAULT '',
  session_change_id   uuid NULL REFERENCES session_changes(id),
  actor_id            uuid NULL REFERENCES users(id),
  created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS absence_sit_in_assignment_events_absence_idx
  ON absence_sit_in_assignment_events(absence_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notification_outbox (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  absence_id          uuid NOT NULL REFERENCES student_absences(id) ON DELETE CASCADE,
  assignment_id       uuid NULL REFERENCES absence_sit_ins(id) ON DELETE SET NULL,
  session_version     integer NOT NULL,
  message_type        text NOT NULL,
  recipient           text NOT NULL,
  channel             text NOT NULL CHECK (channel IN ('sms', 'email')),
  payload             jsonb NOT NULL DEFAULT '{}'::jsonb,
  status              text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sending', 'delivered', 'failed', 'dead_letter', 'cancelled')),
  provider_message_id text NULL,
  failure_reason      text NULL,
  attempt_count       integer NOT NULL DEFAULT 0,
  available_at        timestamptz NOT NULL DEFAULT now(),
  sent_at             timestamptz NULL,
  created_at           timestamptz NOT NULL DEFAULT now(),
  idempotency_key     text NOT NULL UNIQUE
);

CREATE INDEX IF NOT EXISTS notification_outbox_claim_idx
  ON notification_outbox(status, available_at, created_at);

ALTER TABLE app_settings
  ADD COLUMN IF NOT EXISTS sit_in_change_sms_enabled boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS sit_in_change_sms_template text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS sit_in_change_email_enabled boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS sit_in_change_email_subject text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS sit_in_change_email_body text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS sit_in_change_auto_notify_safe_moves boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS sit_in_change_warning_hours integer NOT NULL DEFAULT 24 CHECK (sit_in_change_warning_hours >= 0),
  ADD COLUMN IF NOT EXISTS sit_in_change_critical_hours integer NOT NULL DEFAULT 4 CHECK (sit_in_change_critical_hours >= 0),
  ADD COLUMN IF NOT EXISTS allow_move_into_past boolean NOT NULL DEFAULT false;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION reject_session_change_mutation()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'session change history is append-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS session_changes_append_only ON session_changes;
CREATE TRIGGER session_changes_append_only
  BEFORE UPDATE OR DELETE ON session_changes
  FOR EACH ROW EXECUTE FUNCTION reject_session_change_mutation();

DROP TRIGGER IF EXISTS absence_sit_in_assignment_events_append_only ON absence_sit_in_assignment_events;
CREATE TRIGGER absence_sit_in_assignment_events_append_only
  BEFORE UPDATE OR DELETE ON absence_sit_in_assignment_events
  FOR EACH ROW EXECUTE FUNCTION reject_session_change_mutation();

-- +goose Down

DROP TRIGGER IF EXISTS absence_sit_in_assignment_events_append_only ON absence_sit_in_assignment_events;
DROP TRIGGER IF EXISTS session_changes_append_only ON session_changes;
DROP FUNCTION IF EXISTS reject_session_change_mutation();

ALTER TABLE app_settings
  DROP COLUMN IF EXISTS allow_move_into_past,
  DROP COLUMN IF EXISTS sit_in_change_critical_hours,
  DROP COLUMN IF EXISTS sit_in_change_warning_hours,
  DROP COLUMN IF EXISTS sit_in_change_auto_notify_safe_moves,
  DROP COLUMN IF EXISTS sit_in_change_email_body,
  DROP COLUMN IF EXISTS sit_in_change_email_subject,
  DROP COLUMN IF EXISTS sit_in_change_email_enabled,
  DROP COLUMN IF EXISTS sit_in_change_sms_template,
  DROP COLUMN IF EXISTS sit_in_change_sms_enabled;

DROP TABLE IF EXISTS notification_outbox;
DROP TABLE IF EXISTS absence_sit_in_assignment_events;

ALTER TABLE absence_missed_sessions
  DROP COLUMN IF EXISTS original_end_at,
  DROP COLUMN IF EXISTS original_start_at,
  DROP COLUMN IF EXISTS session_version_at_submission;
ALTER TABLE absence_sit_ins
  DROP COLUMN IF EXISTS assignment_source,
  DROP COLUMN IF EXISTS assigned_by,
  DROP COLUMN IF EXISTS assigned_at,
  DROP COLUMN IF EXISTS session_version_at_assignment;

DROP TABLE IF EXISTS absence_schedule_issues;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS session_changes;
DROP TABLE IF EXISTS session_change_batches;
