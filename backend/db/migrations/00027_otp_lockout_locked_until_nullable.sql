-- +goose Up
ALTER TABLE student_otp_lockouts
  ALTER COLUMN locked_until DROP NOT NULL;

-- +goose Down
ALTER TABLE student_otp_lockouts
  ALTER COLUMN locked_until SET NOT NULL;
