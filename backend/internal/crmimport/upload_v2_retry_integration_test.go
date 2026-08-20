package crmimport

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/crmimport/queue"
)

type recordingReconciler struct {
	called bool
}

func (r *recordingReconciler) EnqueueReconcileJobsForSnapshot(context.Context, pgtype.UUID, *queue.QueueWorker) error {
	r.called = true
	return nil
}

func TestImportSnapshotHandlerResumesReadySnapshotWithoutUploadBlob(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snapshotSvc, err := NewSnapshotService(dbpool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	snapshotID, err := snapshotSvc.CreateSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshotSvc.MarkSnapshotReady(ctx, snapshotID, 0); err != nil {
		t.Fatal(err)
	}

	worker := queue.NewQueueWorker(slog.Default(), queue.NewInMemoryQueueStore(), "test-worker")
	reconciler := &recordingReconciler{}
	payload, err := json.Marshal(ImportSnapshotPayload{
		UploadID:   "already-deleted-upload",
		SnapshotID: uuid.MustParse(snapshotID.String()),
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := ImportSnapshotJobHandler(snapshotSvc, NewStudentSyncService(dbpool), reconciler, worker, nil)
	err = handler(ctx, queue.JobRow{Payload: payload})
	if err != nil {
		t.Fatalf("ready snapshot retry returned error: %v", err)
	}
	if !reconciler.called {
		t.Fatal("expected downstream reconcile jobs to be re-enqueued")
	}
}
