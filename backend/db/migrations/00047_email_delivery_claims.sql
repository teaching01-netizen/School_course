-- +goose Up

CREATE TABLE IF NOT EXISTS email_delivery_claims (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id     uuid NOT NULL REFERENCES email_workflows(id) ON DELETE CASCADE,
    local_date      date NOT NULL,
    recipient_email text NOT NULL,
    claimed_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workflow_id, local_date, recipient_email)
);

-- +goose Down

DROP TABLE IF EXISTS email_delivery_claims;
