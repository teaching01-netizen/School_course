package otpdelivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Enqueue(ctx context.Context, tx pgx.Tx, input NewDelivery) (DeliverySummary, error) {
	var out DeliverySummary
	err := tx.QueryRow(ctx, `
		INSERT INTO sms_otp_deliveries (
			id, session_id, status, campaign_id, key_version,
			payload_nonce, encrypted_payload, expires_at
		)
		VALUES ($1, $2, 'queued', $3, $4, $5, $6, $7)
		RETURNING id, status, attempt_count, created_at
	`, input.ID, input.SessionID, input.CampaignID, input.Ciphertext.KeyVersion,
		input.Ciphertext.Nonce, input.Ciphertext.Data, input.ExpiresAt,
	).Scan(&out.ID, &out.Status, &out.AttemptCount, &out.CreatedAt)
	if err != nil {
		return DeliverySummary{}, fmt.Errorf("enqueue otp delivery: %w", err)
	}
	return out, nil
}

func (s *Store) RequeueUncertain(ctx context.Context, tx pgx.Tx, sessionID, newID uuid.UUID, campaignID string, cooldown time.Duration) (DeliverySummary, error) {
	var keyVersion string
	var nonce, ciphertext []byte
	var uncertainAt, expiresAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT key_version, payload_nonce, encrypted_payload,
			COALESCE(uncertain_at, updated_at), expires_at
		FROM sms_otp_deliveries
		WHERE session_id = $1 AND status = 'uncertain'
			AND encrypted_payload IS NOT NULL AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`, sessionID).Scan(&keyVersion, &nonce, &ciphertext, &uncertainAt, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeliverySummary{}, ErrNoUncertainDelivery
	}
	if err != nil {
		return DeliverySummary{}, err
	}
	if time.Now().UTC().Before(uncertainAt.Add(cooldown)) {
		return DeliverySummary{}, ErrDeliveryCooldown
	}
	return s.Enqueue(ctx, tx, NewDelivery{
		ID:         newID,
		SessionID:  sessionID,
		CampaignID: campaignID,
		Ciphertext: Ciphertext{KeyVersion: keyVersion, Nonce: nonce, Data: ciphertext},
		ExpiresAt:  expiresAt,
	})
}

func (s *Store) RecoverStaleSubmitting(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE sms_otp_deliveries
		SET status = 'uncertain',
			uncertain_at = now(),
			error_code = 'worker_lost_during_submit',
			locked_by = NULL,
			locked_until = NULL,
			updated_at = now()
		WHERE status = 'submitting'
			AND locked_until < now()
	`)
	if err != nil {
		return 0, fmt.Errorf("recover stale submitting otp deliveries: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *Store) Claim(ctx context.Context, workerID string, lease time.Duration) (Delivery, bool, error) {
	if _, err := s.db.Exec(ctx, `
		UPDATE sms_otp_deliveries
		SET status = 'expired', key_version = NULL, payload_nonce = NULL,
			encrypted_payload = NULL, locked_by = NULL, locked_until = NULL,
			error_code = 'otp_expired', updated_at = now()
		WHERE status IN ('queued', 'retryable', 'preparing', 'uncertain')
			AND expires_at <= now()
	`); err != nil {
		return Delivery{}, false, fmt.Errorf("expire otp deliveries: %w", err)
	}

	var delivery Delivery
	err := s.db.QueryRow(ctx, `
		WITH eligible AS (
			SELECT id
			FROM sms_otp_deliveries
			WHERE expires_at > now()
				AND (
					(status IN ('queued', 'retryable') AND run_after <= now())
					OR (status = 'preparing' AND locked_until < now())
				)
			ORDER BY run_after, created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE sms_otp_deliveries d
		SET status = 'preparing',
			locked_by = $1,
			locked_until = now() + $2::interval,
			updated_at = now()
		FROM eligible e
		WHERE d.id = e.id
		RETURNING d.id, d.session_id, d.campaign_id, d.key_version,
			d.payload_nonce, d.encrypted_payload, d.attempt_count, d.expires_at
	`, workerID, durationInterval(lease)).Scan(
		&delivery.ID, &delivery.SessionID, &delivery.CampaignID,
		&delivery.Ciphertext.KeyVersion, &delivery.Ciphertext.Nonce,
		&delivery.Ciphertext.Data, &delivery.AttemptCount, &delivery.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, false, nil
	}
	if err != nil {
		return Delivery{}, false, fmt.Errorf("claim otp delivery: %w", err)
	}
	return delivery, true, nil
}

func (s *Store) MarkSubmitting(ctx context.Context, id uuid.UUID, workerID string, lease time.Duration) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE sms_otp_deliveries
		SET status = 'submitting', submitting_at = now(),
			attempt_count = attempt_count + 1,
			locked_until = now() + $3::interval, updated_at = now()
		WHERE id = $1 AND status = 'preparing' AND locked_by = $2
	`, id, workerID, durationInterval(lease))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("otp delivery %s lost preparing lease", id)
	}
	return nil
}

func (s *Store) Complete(ctx context.Context, id uuid.UUID, result SubmitResult) error {
	status := Status(result.Outcome)
	if status != StatusAccepted && status != StatusFailed && status != StatusUncertain {
		return fmt.Errorf("invalid terminal otp delivery outcome %q", result.Outcome)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	clearPayload := status == StatusAccepted || status == StatusFailed
	var sessionID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE sms_otp_deliveries
		SET status = $2,
			accepted_at = CASE WHEN $2 = 'accepted' THEN now() ELSE accepted_at END,
			failed_at = CASE WHEN $2 = 'failed' THEN now() ELSE failed_at END,
			uncertain_at = CASE WHEN $2 = 'uncertain' THEN now() ELSE uncertain_at END,
			error_code = NULLIF($3, ''),
			key_version = CASE WHEN $4 THEN NULL ELSE key_version END,
			payload_nonce = CASE WHEN $4 THEN NULL ELSE payload_nonce END,
			encrypted_payload = CASE WHEN $4 THEN NULL ELSE encrypted_payload END,
			locked_by = NULL, locked_until = NULL, updated_at = now()
		WHERE id = $1
			AND (status = 'submitting' OR ($2 = 'failed' AND status = 'preparing'))
		RETURNING session_id
	`, id, status, result.ErrorCode, clearPayload).Scan(&sessionID)
	if err != nil {
		return fmt.Errorf("complete otp delivery: %w", err)
	}
	if status == StatusAccepted {
		if _, err := tx.Exec(ctx, `
			UPDATE student_parent_verification_sessions
			SET otp_last_sent_at = now(), updated_at = now()
			WHERE id = $1
		`, sessionID); err != nil {
			return fmt.Errorf("mark otp session sent: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) Retry(ctx context.Context, id uuid.UUID, errorCode string, runAfter time.Time) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE sms_otp_deliveries
		SET status = 'retryable', error_code = NULLIF($2, ''), run_after = $3,
			locked_by = NULL, locked_until = NULL, updated_at = now()
		WHERE id = $1 AND status IN ('preparing', 'submitting')
	`, id, errorCode, runAfter.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("otp delivery %s cannot be retried", id)
	}
	return nil
}

func (s *Store) Latest(ctx context.Context, sessionID uuid.UUID) (DeliverySummary, bool, error) {
	var out DeliverySummary
	var acceptedAt, failedAt, uncertainAt *time.Time
	var runAfter time.Time
	err := s.db.QueryRow(ctx, `
		SELECT id, status, attempt_count, run_after, created_at,
			accepted_at, failed_at, uncertain_at
		FROM sms_otp_deliveries
		WHERE session_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, sessionID).Scan(&out.ID, &out.Status, &out.AttemptCount, &runAfter,
		&out.CreatedAt, &acceptedAt, &failedAt, &uncertainAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeliverySummary{}, false, nil
	}
	if err != nil {
		return DeliverySummary{}, false, err
	}
	out.AcceptedAt = acceptedAt
	out.FailedAt = failedAt
	out.UncertainAt = uncertainAt
	if out.Status == StatusRetryable && runAfter.After(time.Now()) {
		remaining := time.Until(runAfter)
		out.RetryAfterSeconds = int((remaining + time.Second - 1) / time.Second)
	}
	return out, true, nil
}

func durationInterval(d time.Duration) string {
	return fmt.Sprintf("%d microseconds", d.Microseconds())
}
