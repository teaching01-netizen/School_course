-- +goose Up

ALTER TABLE session_series
  ADD COLUMN IF NOT EXISTS weekdays smallint[] NULL;

UPDATE session_series
SET weekdays = ARRAY[weekday]::smallint[]
WHERE weekdays IS NULL;

ALTER TABLE session_series
  ALTER COLUMN weekdays SET NOT NULL;

ALTER TABLE session_series
  ADD CONSTRAINT session_series_weekdays_valid
  CHECK (
    array_length(weekdays, 1) >= 1
    AND weekdays <@ ARRAY[0,1,2,3,4,5,6]::smallint[]
  );

ALTER TABLE session_series
  DROP COLUMN IF EXISTS weekday;

-- +goose Down

ALTER TABLE session_series
  ADD COLUMN IF NOT EXISTS weekday smallint NULL;

UPDATE session_series
SET weekday = weekdays[1]
WHERE weekday IS NULL;

ALTER TABLE session_series
  DROP CONSTRAINT IF EXISTS session_series_weekdays_valid;

ALTER TABLE session_series
  DROP COLUMN IF EXISTS weekdays;
