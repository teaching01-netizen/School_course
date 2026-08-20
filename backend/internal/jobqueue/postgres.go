package jobqueue

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"time"

	sqldb "warwick-institute/internal/db"
)

type PostgresStore struct{ q *sqldb.Queries }

func NewPostgresStore(q *sqldb.Queries) *PostgresStore { return &PostgresStore{q: q} }

func (s *PostgresStore) Enqueue(ctx context.Context, request EnqueueRequest) (Job, error) {
	payload := strings.TrimSpace(string(request.Payload))
	if payload == "" {
		payload = "{}"
	}
	row, err := s.q.LegacyJobEnqueue(ctx, sqldb.LegacyJobEnqueueParams{
		JobType: request.JobType, EntityType: nullableText(request.EntityType), ExternalID: nullableText(request.ExternalID),
		Payload: payload, UniqueKey: nullableText(request.UniqueKey), Priority: int32(request.Priority),
		DeadlineAt: pgtype.Timestamptz{}, MaxAttempts: int32(maxAttempts(request.MaxAttempts)), RunAfter: timestamp(request.RunAfter),
	})
	if err != nil {
		return Job{}, err
	}
	return fromDBJob(row), nil
}

func (s *PostgresStore) Claim(ctx context.Context, workerID string, _ time.Time, lease time.Duration) (Job, error) {
	row, err := s.q.LegacyJobClaim(ctx, sqldb.LegacyJobClaimParams{WorkerID: pgtype.Text{String: workerID, Valid: true}, LeaseSeconds: leaseSeconds(lease)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNoJobs
	}
	if err != nil {
		return Job{}, err
	}
	return fromDBJob(row), nil
}

func (s *PostgresStore) Heartbeat(ctx context.Context, id, workerID string, _ time.Time, lease time.Duration) error {
	jobID, err := parseUUID(id)
	if err != nil {
		return err
	}
	owned, err := s.q.LegacyJobHeartbeatOwned(ctx, jobID, pgtype.Text{String: workerID, Valid: true}, leaseSeconds(lease))
	if err != nil {
		return err
	}
	if !owned {
		return errors.New("job queue: lease is not owned by worker")
	}
	return nil
}

func (s *PostgresStore) Complete(ctx context.Context, id, workerID string) error {
	jobID, err := parseUUID(id)
	if err != nil {
		return err
	}
	owned, err := s.q.LegacyJobCompleteOwned(ctx, jobID, pgtype.Text{String: workerID, Valid: true})
	if err != nil {
		return err
	}
	if !owned {
		return errors.New("job queue: lease is not owned by worker")
	}
	return nil
}

func (s *PostgresStore) Retry(ctx context.Context, id, workerID string, _ time.Time, cause error) error {
	jobID, err := parseUUID(id)
	if err != nil {
		return err
	}
	owned, err := s.q.LegacyJobRetryOwned(ctx, jobID, pgtype.Text{String: workerID, Valid: true}, cause.Error())
	if err != nil {
		return err
	}
	if !owned {
		return errors.New("job queue: lease is not owned by worker")
	}
	return nil
}

func nullableText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func timestamp(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func maxAttempts(value int) int {
	if value <= 0 {
		return 5
	}
	return value
}

func leaseSeconds(lease time.Duration) int64 {
	seconds := int64(lease / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

func fromDBJob(row sqldb.LegacySyncJob) Job {
	return Job{ID: row.ID.String(), JobType: row.JobType, EntityType: row.EntityType.String, ExternalID: row.ExternalID.String, Payload: append([]byte(nil), row.Payload...), UniqueKey: row.UniqueKey.String, Priority: int(row.Priority), Attempt: int(row.Attempt), MaxAttempts: int(row.MaxAttempts), Status: row.Status, LastError: row.LastError.String, CreatedAt: row.CreatedAt.Time, RunAfter: row.RunAfter.Time, LockedBy: row.LockedBy.String, LockedUntil: row.LockedUntil.Time, HeartbeatAt: row.HeartbeatAt.Time}
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}
