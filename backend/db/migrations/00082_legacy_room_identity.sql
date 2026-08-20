-- +goose Up

-- Legacy room identity is the source mapping, not the display name. Native
-- room names were historically unique, but two source rooms may legitimately
-- share a label and must remain distinct local records.
ALTER TABLE rooms DROP CONSTRAINT IF EXISTS rooms_name_key;

-- +goose Down

ALTER TABLE rooms ADD CONSTRAINT rooms_name_key UNIQUE (name);
