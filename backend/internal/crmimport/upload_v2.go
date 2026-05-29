package crmimport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/crmimport/queue"
	"warwick-institute/internal/crmimport/xlsx"
)

// UploadV2Service manages the snapshot-based XLSX upload pipeline.
type UploadV2Service struct {
	db           *pgxpool.Pool
	worker       *queue.QueueWorker
	instituteLoc *time.Location
}

// NewUploadV2Service creates a new UploadV2Service.
func NewUploadV2Service(db *pgxpool.Pool, worker *queue.QueueWorker, instituteTZ string) (*UploadV2Service, error) {
	loc, err := time.LoadLocation(instituteTZ)
	if err != nil {
		return nil, err
	}
	return &UploadV2Service{db: db, worker: worker, instituteLoc: loc}, nil
}

// UploadResponse is returned from POST /api/v1/crm/upload.
type UploadResponse struct {
	JobID      string `json:"job_id"`
	Status     string `json:"status"`
	SnapshotID string `json:"snapshot_id,omitempty"`
	Message    string `json:"message"`
}

// StartUploadAsync starts an async upload pipeline.
func (s *UploadV2Service) StartUploadAsync(ctx context.Context, file multipart.File, filename string, filesize int64) (*UploadResponse, error) {
	const maxUploadSize = 50 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(file, maxUploadSize))
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	_ = file.Close()

	if len(data) < 4 || string(data[:2]) != "PK" {
		return nil, fmt.Errorf("file is not a valid XLSX (bad signature)")
	}

	snapshotSvc, err := NewSnapshotService(s.db, s.instituteLoc.String())
	if err != nil {
		return nil, fmt.Errorf("create snapshot service: %w", err)
	}

	snapshotID, err := snapshotSvc.CreateSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("create snapshot: %w", err)
	}

	snapshotUUID, err := uuid.FromBytes(snapshotID.Bytes[:])
	if err != nil {
		return nil, fmt.Errorf("parse snapshot uuid: %w", err)
	}

	uploadID := fmt.Sprintf("upload-%d", time.Now().UnixNano())
	if err := storeUploadBlob(ctx, s.db, uploadID, data); err != nil {
		_ = snapshotSvc.MarkSnapshotFailed(ctx, snapshotID, fmt.Sprintf("stage upload blob: %v", err))
		return nil, fmt.Errorf("stage upload blob: %w", err)
	}

	payload := ImportSnapshotPayload{UploadID: uploadID, SnapshotID: snapshotUUID}
	jobID, err := s.worker.EnqueueJob(ctx, queue.JobTypeImportSnapshot, payload, "")
	if err != nil {
		_ = snapshotSvc.MarkSnapshotFailed(ctx, snapshotID, fmt.Sprintf("enqueue import snapshot: %v", err))
		return nil, fmt.Errorf("enqueue import snapshot job: %w", err)
	}

	return &UploadResponse{
		JobID:      jobID.String(),
		Status:     "importing",
		SnapshotID: snapshotUUID.String(),
		Message:    "Upload accepted, processing asynchronously",
	}, nil
}

// storeUploadBlob stores upload bytes for async processing.
func storeUploadBlob(ctx context.Context, db *pgxpool.Pool, uploadID string, data []byte) error {
	_, err := db.Exec(ctx, `
		INSERT INTO crm_upload_blobs (id, data, created_at)
		VALUES ($1, $2, now())
		ON CONFLICT (id) DO NOTHING
	`, uploadID, data)
	return err
}

// GetUploadJobStatus returns a simple status response.
func (s *UploadV2Service) GetUploadJobStatus(ctx context.Context, jobID string) (*UploadResponse, error) {
	var status, jobType string
	var payload []byte
	err := s.db.QueryRow(ctx, `
		SELECT status::text, job_type::text, COALESCE(payload::text, '{}')
		FROM crm_jobs WHERE id = $1
	`, jobID).Scan(&status, &jobType, &payload)
	if err != nil {
		return nil, fmt.Errorf("query job: %w", err)
	}

	return &UploadResponse{
		JobID:   jobID,
		Status:  status,
		Message: fmt.Sprintf("Job %s is %s", jobType, status),
	}, nil
}

// EnqueueReconciler is implemented by reconcile.ReconcileV2Service to enqueue
// reconcile jobs after a snapshot import completes.
type EnqueueReconciler interface {
	EnqueueReconcileJobsForSnapshot(ctx context.Context, snapshotID pgtype.UUID, worker *queue.QueueWorker) error
}

// ImportSnapshotJobHandler returns a handler for the import_snapshot job type.
func ImportSnapshotJobHandler(snapshotSvc *SnapshotService, syncSvc *StudentSyncService, reconcileV2 EnqueueReconciler, worker *queue.QueueWorker) queue.JobHandler {
	return func(ctx context.Context, job queue.JobRow) error {
		var payload ImportSnapshotPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal payload: %w", err)
		}

		snapshotID := pgtype.UUID{Bytes: payload.SnapshotID, Valid: true}

		data, err := getUploadBlob(ctx, snapshotSvc.db, payload.UploadID)
		if err != nil {
			snapshotSvc.MarkSnapshotFailed(ctx, snapshotID, fmt.Sprintf("get upload blob: %v", err))
			return fmt.Errorf("get upload blob: %w", err)
		}

		_, _ = snapshotSvc.db.Exec(ctx, `DELETE FROM crm_upload_blobs WHERE id = $1`, payload.UploadID)

		parsed, err := xlsx.ParseXLSX(data, snapshotSvc.instituteLoc)
		if err != nil {
			snapshotSvc.MarkSnapshotFailed(ctx, snapshotID, fmt.Sprintf("parse error: %v", err))
			return fmt.Errorf("parse xlsx: %w", err)
		}

		sort.SliceStable(parsed.Rows, func(i, j int) bool {
			a, b := parsed.Rows[i].OrderQuoteUpdatedAt, parsed.Rows[j].OrderQuoteUpdatedAt
			if a == nil && b == nil {
				return false
			}
			if a == nil {
				return false
			}
			if b == nil {
				return true
			}
			return a.After(*b)
		})

		seen := map[string]struct{}{}
		deduped := make([]xlsx.Row, 0, len(parsed.Rows))
		for _, r := range parsed.Rows {
			h := r.Hash()
			if _, ok := seen[h]; ok {
				continue
			}
			seen[h] = struct{}{}
			deduped = append(deduped, r)
		}

		if _, err := snapshotSvc.PopulateRows(ctx, snapshotID, deduped, len(parsed.Rows)); err != nil {
			snapshotSvc.MarkSnapshotFailed(ctx, snapshotID, fmt.Sprintf("populate rows: %v", err))
			return fmt.Errorf("populate rows: %w", err)
		}

		if err := snapshotSvc.MarkSnapshotReady(ctx, snapshotID, len(deduped)); err != nil {
			snapshotSvc.MarkSnapshotFailed(ctx, snapshotID, fmt.Sprintf("mark ready: %v", err))
			return fmt.Errorf("mark snapshot ready: %w", err)
		}

		snapshotUUID, err := uuid.FromBytes(snapshotID.Bytes[:])
		if err != nil {
			snapshotSvc.MarkSnapshotFailed(ctx, snapshotID, fmt.Sprintf("parse snapshot uuid: %v", err))
			return fmt.Errorf("parse snapshot uuid: %w", err)
		}

		syncPayload := StudentSyncPayload{SnapshotID: snapshotUUID}
		if _, err := worker.EnqueueJob(ctx, queue.JobTypeStudentSync, syncPayload, fmt.Sprintf("student-sync-%s", snapshotUUID.String())); err != nil {
			return fmt.Errorf("enqueue student sync: %w", err)
		}

		if err := reconcileV2.EnqueueReconcileJobsForSnapshot(ctx, snapshotID, worker); err != nil {
			return fmt.Errorf("enqueue reconcile jobs: %w", err)
		}

		return nil
	}
}

// StudentSyncJobHandler returns a handler for the student_sync job type.
func StudentSyncJobHandler(syncSvc *StudentSyncService) queue.JobHandler {
	return func(ctx context.Context, job queue.JobRow) error {
		var payload StudentSyncPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal payload: %w", err)
		}

		snapshotID := pgtype.UUID{Bytes: payload.SnapshotID, Valid: true}
		_, err := syncSvc.SyncFromSnapshot(ctx, snapshotID)
		return err
	}
}

func getUploadBlob(ctx context.Context, db *pgxpool.Pool, uploadID string) ([]byte, error) {
	var data []byte
	err := db.QueryRow(ctx, `SELECT data FROM crm_upload_blobs WHERE id = $1`, uploadID).Scan(&data)
	if err != nil {
		return nil, err
	}
	return data, nil
}
