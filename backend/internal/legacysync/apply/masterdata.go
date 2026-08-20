package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync/normalize"
)

var ErrMissingMasterDataIdentity = errors.New("legacy master data: external identity is required")

type TeacherApplyRequest struct {
	Teacher         normalize.LegacyTeacher
	ObservedAt      time.Time
	ShadowMode      bool
	RealtimeEnabled bool
}

type RoomApplyRequest struct {
	Room            normalize.LegacyRoom
	Capacity        int
	ObservedAt      time.Time
	ShadowMode      bool
	RealtimeEnabled bool
}

type SubjectApplyRequest struct {
	Subject         normalize.LegacySubject
	Code            string
	ObservedAt      time.Time
	ShadowMode      bool
	RealtimeEnabled bool
}

type MasterDataApplyResult struct {
	EntityType string
	ExternalID string
	InternalID pgtype.UUID
	SourceHash string
	Changed    bool
	AppliedAt  time.Time
}

type MasterDataService struct {
	pool   *pgxpool.Pool
	q      *sqldb.Queries
	source string
}

func NewMasterDataService(pool *pgxpool.Pool, q *sqldb.Queries, source string) *MasterDataService {
	return &MasterDataService{pool: pool, q: q, source: source}
}

func (s *MasterDataService) ApplyTeacher(ctx context.Context, request TeacherApplyRequest) (MasterDataApplyResult, error) {
	if request.Teacher.LegacyID == "" || request.Teacher.Name == "" {
		return MasterDataApplyResult{}, ErrMissingMasterDataIdentity
	}
	return s.apply(ctx, "teacher", request.Teacher.LegacyID, request, request.ObservedAt, request.ShadowMode, request.RealtimeEnabled, func(ctx context.Context, tx pgx.Tx) (pgtype.UUID, error) {
		return s.applyTeacher(ctx, tx, request.Teacher)
	})
}

func (s *MasterDataService) ApplyRoom(ctx context.Context, request RoomApplyRequest) (MasterDataApplyResult, error) {
	if request.Room.LegacyID == "" || request.Room.Name == "" {
		return MasterDataApplyResult{}, ErrMissingMasterDataIdentity
	}
	return s.apply(ctx, "room", request.Room.LegacyID, request, request.ObservedAt, request.ShadowMode, request.RealtimeEnabled, func(ctx context.Context, tx pgx.Tx) (pgtype.UUID, error) {
		return s.applyRoom(ctx, tx, request.Room, request.Capacity)
	})
}

func (s *MasterDataService) ApplySubject(ctx context.Context, request SubjectApplyRequest) (MasterDataApplyResult, error) {
	if request.Subject.LegacyID == "" || request.Subject.Name == "" {
		return MasterDataApplyResult{}, ErrMissingMasterDataIdentity
	}
	return s.apply(ctx, "subject", request.Subject.LegacyID, request, request.ObservedAt, request.ShadowMode, request.RealtimeEnabled, func(ctx context.Context, tx pgx.Tx) (pgtype.UUID, error) {
		return s.applySubject(ctx, tx, request.Subject, request.Code)
	})
}

func (s *MasterDataService) apply(ctx context.Context, entityType, externalID string, value any, observedAt time.Time, shadowMode, realtimeEnabled bool, applyDomain func(context.Context, pgx.Tx) (pgtype.UUID, error)) (MasterDataApplyResult, error) {
	if s.pool == nil || s.q == nil {
		return MasterDataApplyResult{}, errors.New("legacy master data: pool and queries are required")
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	canonical, err := normalize.CanonicalJSON(value)
	if err != nil {
		return MasterDataApplyResult{}, fmt.Errorf("canonicalize legacy %s: %w", entityType, err)
	}
	sourceHash, err := normalize.HashCanonical(value)
	if err != nil {
		return MasterDataApplyResult{}, fmt.Errorf("hash legacy %s: %w", entityType, err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MasterDataApplyResult{}, fmt.Errorf("begin legacy %s apply: %w", entityType, err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, s.source+":"+entityType+":"+externalID); err != nil {
		return MasterDataApplyResult{}, fmt.Errorf("lock legacy %s %s: %w", entityType, externalID, err)
	}
	qtx := s.q.WithTx(tx)
	previous, err := qtx.SnapshotGet(ctx, sqldb.SnapshotGetParams{Source: s.source, EntityType: entityType, ExternalID: externalID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return MasterDataApplyResult{}, fmt.Errorf("load legacy %s snapshot: %w", entityType, err)
	}
	if err == nil && previous.SourceHash == sourceHash {
		internalID, _, mappingErr := mappedID(ctx, tx, s.source, entityType, externalID)
		if mappingErr != nil {
			return MasterDataApplyResult{}, fmt.Errorf("load unchanged legacy %s mapping: %w", entityType, mappingErr)
		}
		if err := tx.Commit(ctx); err != nil {
			return MasterDataApplyResult{}, fmt.Errorf("commit unchanged legacy %s: %w", entityType, err)
		}
		return MasterDataApplyResult{EntityType: entityType, ExternalID: externalID, InternalID: internalID, SourceHash: sourceHash, AppliedAt: observedAt}, nil
	}
	if shadowMode {
		if err := tx.Commit(ctx); err != nil {
			return MasterDataApplyResult{}, fmt.Errorf("commit shadow legacy %s: %w", entityType, err)
		}
		return MasterDataApplyResult{EntityType: entityType, ExternalID: externalID, SourceHash: sourceHash, AppliedAt: observedAt}, nil
	}
	internalID, err := applyDomain(ctx, tx)
	if err != nil {
		return MasterDataApplyResult{}, err
	}
	if _, err := qtx.ExternalRefUpsert(ctx, sqldb.ExternalRefUpsertParams{Source: s.source, EntityType: entityType, ExternalID: externalID, InternalID: internalID, SourceHash: pgtype.Text{String: sourceHash, Valid: true}}); err != nil {
		return MasterDataApplyResult{}, fmt.Errorf("upsert legacy %s mapping: %w", entityType, err)
	}
	if _, err := qtx.SnapshotUpsert(ctx, sqldb.SnapshotUpsertParams{Source: s.source, EntityType: entityType, ExternalID: externalID, CanonicalData: string(canonical), SourceHash: sourceHash, ParserVersion: 1, ObservedAt: timestamp(observedAt), Quality: "ok"}); err != nil {
		return MasterDataApplyResult{}, fmt.Errorf("store legacy %s snapshot: %w", entityType, err)
	}
	if realtimeEnabled {
		payload := fmt.Sprintf(`{"synced_at":%q}`, observedAt.UTC().Format(time.RFC3339Nano))
		if _, err := qtx.OutboxInsert(ctx, sqldb.OutboxInsertParams{SourceEventKey: "legacy:" + entityType + ":" + externalID + ":" + sourceHash, EventType: "legacy." + entityType + ".updated", Channel: entityType + ":" + externalID, EntityType: pgtype.Text{String: entityType, Valid: true}, ExternalID: pgtype.Text{String: externalID, Valid: true}, Payload: payload}); err != nil {
			return MasterDataApplyResult{}, fmt.Errorf("write legacy %s outbox: %w", entityType, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return MasterDataApplyResult{}, fmt.Errorf("commit legacy %s: %w", entityType, err)
	}
	return MasterDataApplyResult{EntityType: entityType, ExternalID: externalID, InternalID: internalID, SourceHash: sourceHash, Changed: true, AppliedAt: observedAt}, nil
}

func (s *MasterDataService) applyTeacher(ctx context.Context, tx pgx.Tx, teacher normalize.LegacyTeacher) (pgtype.UUID, error) {
	internalID, found, err := mappedID(ctx, tx, s.source, "teacher", teacher.LegacyID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	username := "legacy:" + teacher.LegacyID
	active := teacher.IsActive
	if found {
		if _, err := tx.Exec(ctx, `UPDATE users SET username=$1, email=NULLIF($2,''), full_name=COALESCE(NULLIF($5,''), full_name), deleted_at=CASE WHEN $3 THEN NULL ELSE COALESCE(deleted_at, now()) END, updated_at=now() WHERE id=$4`, username, teacher.Email, active, internalID, teacher.Name); err != nil {
			return pgtype.UUID{}, fmt.Errorf("update legacy teacher %s: %w", teacher.LegacyID, err)
		}
		return internalID, nil
	}
	passwordHash := disabledPasswordHash(teacher.LegacyID)
	if err := tx.QueryRow(ctx, `INSERT INTO users (username, role, password_hash, email, full_name, deleted_at) VALUES ($1,'Teacher',$2,NULLIF($3,''),NULLIF($4,''),CASE WHEN $5 THEN NULL ELSE now() END) RETURNING id`, username, passwordHash, teacher.Email, teacher.Name, active).Scan(&internalID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("create legacy teacher %s: %w", teacher.LegacyID, err)
	}
	return internalID, nil
}

func (s *MasterDataService) applyRoom(ctx context.Context, tx pgx.Tx, room normalize.LegacyRoom, capacity int) (pgtype.UUID, error) {
	internalID, found, err := mappedID(ctx, tx, s.source, "room", room.LegacyID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if found {
		if _, err := tx.Exec(ctx, `UPDATE rooms SET name=$1, capacity=NULLIF($2,0), updated_at=now() WHERE id=$3`, room.Name, capacity, internalID); err != nil {
			return pgtype.UUID{}, fmt.Errorf("update legacy room %s: %w", room.LegacyID, err)
		}
		return internalID, nil
	}
	if err := tx.QueryRow(ctx, `INSERT INTO rooms (name, capacity) VALUES ($1,NULLIF($2,0)) RETURNING id`, room.Name, capacity).Scan(&internalID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("create legacy room %s: %w", room.LegacyID, err)
	}
	return internalID, nil
}

func (s *MasterDataService) applySubject(ctx context.Context, tx pgx.Tx, subject normalize.LegacySubject, code string) (pgtype.UUID, error) {
	internalID, found, err := mappedID(ctx, tx, s.source, "subject", subject.LegacyID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if code == "" {
		code = "legacy:" + subject.LegacyID
	}
	if found {
		// subjects were hard-deleted (migration 00032), so the update must
		// not reference the soft-delete column that no longer exists.
		if _, err := tx.Exec(ctx, `UPDATE subjects SET code=$1, name=$2, updated_at=now() WHERE id=$3`, code, subject.Name, internalID); err != nil {
			return pgtype.UUID{}, fmt.Errorf("update legacy subject %s: %w", subject.LegacyID, err)
		}
		return internalID, nil
	}
	if err := tx.QueryRow(ctx, `INSERT INTO subjects (code, name) VALUES ($1,$2) RETURNING id`, code, subject.Name).Scan(&internalID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("create legacy subject %s: %w", subject.LegacyID, err)
	}
	return internalID, nil
}

func mappedID(ctx context.Context, tx pgx.Tx, source, entityType, externalID string) (pgtype.UUID, bool, error) {
	var id pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT internal_id FROM external_refs WHERE source=$1 AND entity_type=$2 AND external_id=$3 AND state IN ('active','restored') FOR UPDATE`, source, entityType, externalID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, false, nil
	}
	if err != nil {
		return pgtype.UUID{}, false, fmt.Errorf("load legacy %s mapping: %w", entityType, err)
	}
	return id, true, nil
}

func disabledPasswordHash(externalID string) string {
	sum := sha256.Sum256([]byte("legacy-disabled:" + externalID))
	return "!legacy-disabled:" + hex.EncodeToString(sum[:])
}
