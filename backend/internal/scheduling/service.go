package scheduling

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/series"
)

func localDateFromPgDate(d pgtype.Date) LocalDate {
	t := d.Time.UTC()
	return LocalDate{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// SeriesService defines the series operations that scheduling depends on.
// Extracted as an interface to decouple scheduling from series module internals
// and enable unit testing without a database.
type SeriesService interface {
	CreateSeriesAndMaterializeTx(ctx context.Context, qtx *sqldb.Queries, p series.CreateParams) (series.CreateResult, error)
	SplitThisAndFutureTx(ctx context.Context, qtx *sqldb.Queries, p series.SplitParams) (series.SplitResult, error)
	SplitThisAndFuture(ctx context.Context, p series.SplitParams) (series.SplitResult, error)
	CancelTx(ctx context.Context, qtx *sqldb.Queries, p series.CancelParams) (series.CancelResult, error)
	Cancel(ctx context.Context, p series.CancelParams) (series.CancelResult, error)
	EditEntireSeriesFutureOnlyTx(ctx context.Context, qtx *sqldb.Queries, p series.EditEntireFutureParams) (series.EditEntireFutureResult, error)
	EditEntireSeriesFutureOnly(ctx context.Context, p series.EditEntireFutureParams) (series.EditEntireFutureResult, error)
}

type Service struct {
	db          *pgxpool.Pool
	q           *sqldb.Queries
	instituteTZ string
	loc         *time.Location
	seriesSvc   SeriesService
}

type PreflightParams struct {
	SessionID  *pgtype.UUID
	CourseID   pgtype.UUID
	RoomID     pgtype.UUID
	TeacherID  pgtype.UUID
	StartAt    pgtype.Timestamptz
	EndAt      pgtype.Timestamptz
	StudentIDs *[]pgtype.UUID
	Requested  ConflictRequested
}

type PreflightResult struct {
	Status string `json:"status"` // available|provisional
}

type CourseStudentStatus string

const (
	CourseStudentStatusEnrolled CourseStudentStatus = "enrolled"
	CourseStudentStatusDraft    CourseStudentStatus = "draft"
)

func (s *Service) Preflight(ctx context.Context, p PreflightParams) (PreflightResult, *Err, error) {
	if !p.StartAt.Valid || !p.EndAt.Valid || !p.EndAt.Time.After(p.StartAt.Time) {
		return PreflightResult{}, nil, fmt.Errorf("invalid time range")
	}

	in := preflightInput{
		CourseID:   p.CourseID,
		RoomID:     p.RoomID,
		TeacherID:  p.TeacherID,
		StartUTC:   p.StartAt.Time.UTC(),
		EndUTC:     p.EndAt.Time.UTC(),
		StudentIDs: p.StudentIDs,
		Requested:  p.Requested,
	}
	if p.SessionID != nil && p.SessionID.Valid {
		in.IgnoreSession = p.SessionID
	}

	if se := s.preflightSlot(ctx, s.db, s.q, in); se != nil {
		return PreflightResult{}, se, nil
	}
	status := "available"
	if !p.RoomID.Valid {
		status = "provisional"
	}
	return PreflightResult{Status: status}, nil, nil
}

func (s *Service) AddCourseStudentTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, courseID, studentID pgtype.UUID, status CourseStudentStatus) error {
	alreadyRostered, err := courseStudentExists(ctx, tx, courseID, studentID)
	if err != nil {
		return err
	}
	if alreadyRostered {
		return nil
	}

	preflightIn, ok, err := s.courseStudentPreflightInput(ctx, tx, courseID, studentID, nil)
	if err != nil {
		return err
	}
	if ok {
		if se := s.preflightStudentOverlap(ctx, tx, preflightIn); se != nil {
			return se
		}
	}

	switch status {
	case CourseStudentStatusEnrolled:
		err = withSavepoint(ctx, tx, func(qsp *sqldb.Queries) error {
			return qsp.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: courseID, StudentID: studentID})
		})
	case CourseStudentStatusDraft:
		err = withSavepoint(ctx, tx, func(qsp *sqldb.Queries) error {
			return qsp.CourseStudentAddDraft(ctx, sqldb.CourseStudentAddDraftParams{CourseID: courseID, StudentID: studentID})
		})
	default:
		err = fmt.Errorf("unknown course student status %q", status)
	}
	if err != nil {
		if ok {
			if se := s.explainStudentDBErrByRepreflight(ctx, err, tx, preflightIn); se != nil {
				return se
			}
		}
		return err
	}
	return nil
}

func (s *Service) UpsertSessionAttendanceTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, sessionID, studentID pgtype.UUID, status string) error {
	if status == "included" {
		preflightIn, ok, err := s.sessionIncludedStudentPreflightInput(ctx, qtx, sessionID, studentID)
		if err != nil {
			return err
		}
		if ok {
			if se := s.preflightStudentOverlap(ctx, tx, preflightIn); se != nil {
				return se
			}
		}

		if err := withSavepoint(ctx, tx, func(qsp *sqldb.Queries) error {
			return qsp.SessionAttendanceUpsert(ctx, sqldb.SessionAttendanceUpsertParams{SessionID: sessionID, StudentID: studentID, Status: status})
		}); err != nil {
			if ok {
				if se := s.explainStudentDBErrByRepreflight(ctx, err, tx, preflightIn); se != nil {
					return se
				}
			}
			return err
		}
		return nil
	}

	return qtx.SessionAttendanceUpsert(ctx, sqldb.SessionAttendanceUpsertParams{SessionID: sessionID, StudentID: studentID, Status: status})
}

func withSavepoint(ctx context.Context, tx pgx.Tx, fn func(qsp *sqldb.Queries) error) error {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(sqldb.New(sp)); err != nil {
		_ = sp.Rollback(ctx)
		return err
	}
	return sp.Commit(ctx)
}

func courseStudentExists(ctx context.Context, db sqldb.DBTX, courseID, studentID pgtype.UUID) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM course_students
			WHERE course_id = $1 AND student_id = $2
		)
	`, courseID, studentID).Scan(&exists)
	return exists, err
}

type PreflightSeriesParams struct {
	CourseID        pgtype.UUID
	RoomID          pgtype.UUID
	TeacherID       pgtype.UUID
	Weekdays        []time.Weekday
	StartLocalTime  Clock
	DurationMinutes int
	StartDate       LocalDate
	EndDate         *LocalDate
	Count           *int
	SeriesID        *pgtype.UUID
}

type PreflightSeriesResult struct {
	Status             string `json:"status"` // available|provisional
	OccurrencesPlanned int    `json:"occurrences_planned"`
}

// PreflightSeries checks each occurrence of a proposed recurring series for
// conflicts. It runs outside any transaction (on the connection pool), so it
// is advisory-only: another request can insert a conflicting session between
// checking occurrence N and occurrence N+1. This is acceptable because the
// preflight is a UX hint (showing likely availability to the admin) — the
// actual correctness gate is the serializable transaction used by the write
// path (CreateSeriesAndMaterialize), which will reject real conflicts at
// commit time.
func (s *Service) PreflightSeries(ctx context.Context, p PreflightSeriesParams) (PreflightSeriesResult, *Err, error) {
	occ, err := series.Materialize(series.MaterializeInput{
		Weekdays:        p.Weekdays,
		StartDate:       p.StartDate,
		EndDate:         p.EndDate,
		Count:           p.Count,
		StartLocalTime:  p.StartLocalTime,
		DurationMinutes: p.DurationMinutes,
		Location:        s.loc,
	})
	if err != nil {
		return PreflightSeriesResult{}, nil, err
	}

	ps, err := newPreflightStrings(p.CourseID, p.RoomID, p.TeacherID)
	if err != nil {
		return PreflightSeriesResult{}, nil, err
	}

	for _, o := range occ {
		if se := s.preflightSlot(ctx, s.db, s.q, preflightInput{
			CourseID:     p.CourseID,
			RoomID:       p.RoomID,
			TeacherID:    p.TeacherID,
			StartUTC:     o.StartUTC,
			EndUTC:       o.EndUTC,
			IgnoreSeries: p.SeriesID,
			Requested:    ps.conflictRequested(o.StartUTC, o.EndUTC, nil),
		}); se != nil {
			return PreflightSeriesResult{}, se, nil
		}
	}

	status := "available"
	if !p.RoomID.Valid {
		status = "provisional"
	}
	return PreflightSeriesResult{Status: status, OccurrencesPlanned: len(occ)}, nil, nil
}

func NewService(db *pgxpool.Pool, instituteTZ string, seriesSvc SeriesService) (*Service, error) {
	loc, err := time.LoadLocation(instituteTZ)
	if err != nil {
		return nil, err
	}
	return &Service{
		db:          db,
		q:           sqldb.New(db),
		instituteTZ: instituteTZ,
		loc:         loc,
		seriesSvc:   seriesSvc,
	}, nil
}

type CreateSeriesParams struct {
	CourseID        pgtype.UUID
	RoomID          pgtype.UUID
	TeacherID       pgtype.UUID
	Weekdays        []time.Weekday
	StartLocalTime  Clock
	DurationMinutes int
	StartDate       LocalDate
	EndDate         *LocalDate
	Count           *int
}

type CreateSeriesResult struct {
	SeriesID      pgtype.UUID
	SessionsAdded int
}

// CreateSeriesAndMaterializeTx performs series creation using an existing tx-bound
// handle. The caller owns begin/commit/rollback. Preflight runs inside the tx.
func (s *Service) CreateSeriesAndMaterializeTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, p CreateSeriesParams) (CreateSeriesResult, error) {
	occ, err := series.Materialize(series.MaterializeInput{
		Weekdays:        p.Weekdays,
		StartDate:       p.StartDate,
		EndDate:         p.EndDate,
		Count:           p.Count,
		StartLocalTime:  p.StartLocalTime,
		DurationMinutes: p.DurationMinutes,
		Location:        s.loc,
	})
	if err != nil {
		return CreateSeriesResult{}, err
	}

	ps, err := newPreflightStrings(p.CourseID, p.RoomID, p.TeacherID)
	if err != nil {
		return CreateSeriesResult{}, err
	}

	for _, o := range occ {
		if serr := s.preflightSlot(ctx, tx, qtx, preflightInput{
			CourseID:  p.CourseID,
			RoomID:    p.RoomID,
			TeacherID: p.TeacherID,
			StartUTC:  o.StartUTC,
			EndUTC:    o.EndUTC,
			Requested: ps.conflictRequested(o.StartUTC, o.EndUTC, nil),
		}); serr != nil {
			return CreateSeriesResult{}, serr
		}
	}

	res, err := s.seriesSvc.CreateSeriesAndMaterializeTx(ctx, qtx, series.CreateParams{
		CourseID:        p.CourseID,
		RoomID:          p.RoomID,
		TeacherID:       p.TeacherID,
		Weekdays:        p.Weekdays,
		StartLocalTime:  p.StartLocalTime,
		DurationMinutes: p.DurationMinutes,
		StartDate:       p.StartDate,
		EndDate:         p.EndDate,
		Count:           p.Count,
		Occurrences:     occ,
	})
	if err != nil {
		candidates := make([]preflightInput, 0, len(occ))
		for _, o := range occ {
			candidates = append(candidates, preflightInput{
				CourseID:  p.CourseID,
				RoomID:    p.RoomID,
				TeacherID: p.TeacherID,
				StartUTC:  o.StartUTC,
				EndUTC:    o.EndUTC,
				Requested: ps.conflictRequested(o.StartUTC, o.EndUTC, nil),
			})
		}
		if se := s.explainFromDBErrByRepreflightTx(ctx, err, tx, qtx, candidates); se != nil {
			return CreateSeriesResult{}, se
		}
		return CreateSeriesResult{}, err
	}
	return CreateSeriesResult{SeriesID: res.SeriesID, SessionsAdded: res.SessionsAdded}, nil
}

func (s *Service) CreateSeriesAndMaterialize(ctx context.Context, p CreateSeriesParams) (CreateSeriesResult, error) {
	// Delegate to the Tx variant with a serializable retry loop.
	const maxRetries = 2
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*10+rand.Intn(20)) * time.Millisecond)
		}

		tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return CreateSeriesResult{}, err
		}

		qtx := s.q.WithTx(tx)

		res, err := s.CreateSeriesAndMaterializeTx(ctx, tx, qtx, p)
		if err != nil {
			_ = tx.Rollback(ctx)
			lastErr = err
			if isRetryableSchedulingErr(err) && attempt < maxRetries {
				continue
			}
			return CreateSeriesResult{}, err
		}

		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(ctx)
			lastErr = err
			if isRetryableSchedulingErr(err) && attempt < maxRetries {
				continue
			}
			return CreateSeriesResult{}, err
		}

		return CreateSeriesResult{SeriesID: res.SeriesID, SessionsAdded: res.SessionsAdded}, nil
	}

	return CreateSeriesResult{}, fmt.Errorf("too many scheduling retries: %w", lastErr)
}

type SplitSeriesParams struct {
	SeriesID        pgtype.UUID
	PivotDate       LocalDate
	ExpectedVersion int32

	Weekdays        []time.Weekday
	StartLocalTime  *Clock
	DurationMinutes *int
	EndDate         *LocalDate
	Count           *int
}

type SplitSeriesResult struct {
	OldSeriesID      pgtype.UUID
	NewSeriesID      pgtype.UUID
	NewSessionsAdded int
}

// SplitThisAndFutureTx splits a series using an existing tx-bound handle.
func (s *Service) SplitThisAndFutureTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, p SplitSeriesParams) (SplitSeriesResult, error) {
	res, err := s.seriesSvc.SplitThisAndFutureTx(ctx, qtx, series.SplitParams{
		SeriesID:        p.SeriesID,
		PivotDate:       p.PivotDate,
		ExpectedVersion: p.ExpectedVersion,
		Weekdays:        p.Weekdays,
		StartLocalTime:  p.StartLocalTime,
		DurationMinutes: p.DurationMinutes,
		EndDate:         p.EndDate,
		Count:           p.Count,
	})
	if err != nil {
		if strings.Contains(err.Error(), "stale_edit") {
			return SplitSeriesResult{}, &Err{Code: "stale_edit", Message: err.Error()}
		}
		return SplitSeriesResult{}, err
	}
	return SplitSeriesResult{OldSeriesID: res.OldSeriesID, NewSeriesID: res.NewSeriesID, NewSessionsAdded: res.NewSessionsAdded}, nil
}

func (s *Service) SplitThisAndFuture(ctx context.Context, p SplitSeriesParams) (SplitSeriesResult, error) {
	res, err := s.seriesSvc.SplitThisAndFuture(ctx, series.SplitParams{
		SeriesID:        p.SeriesID,
		PivotDate:       p.PivotDate,
		ExpectedVersion: p.ExpectedVersion,
		Weekdays:        p.Weekdays,
		StartLocalTime:  p.StartLocalTime,
		DurationMinutes: p.DurationMinutes,
		EndDate:         p.EndDate,
		Count:           p.Count,
	})
	if err != nil {
		if strings.Contains(err.Error(), "stale_edit") {
			return SplitSeriesResult{}, &Err{Code: "stale_edit", Message: err.Error()}
		}
		return SplitSeriesResult{}, err
	}
	return SplitSeriesResult{OldSeriesID: res.OldSeriesID, NewSeriesID: res.NewSeriesID, NewSessionsAdded: res.NewSessionsAdded}, nil
}

type CancelSeriesParams struct {
	SeriesID        pgtype.UUID
	Scope           CancelScope
	PivotDate       *LocalDate // required when Scope == this_and_future
	ExpectedVersion int32
	NowUTC          time.Time // injectable for tests; if zero, time.Now().UTC() is used
}

type CancelSeriesResult struct {
	SeriesID         pgtype.UUID
	CanceledFromUTC  time.Time
	SessionsCanceled int
}

// CancelSeriesTx cancels a series using an existing tx-bound handle.
func (s *Service) CancelSeriesTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, p CancelSeriesParams) (CancelSeriesResult, error) {
	res, err := s.seriesSvc.CancelTx(ctx, qtx, series.CancelParams{
		SeriesID:        p.SeriesID,
		Scope:           series.CancelScope(p.Scope),
		PivotDate:       p.PivotDate,
		ExpectedVersion: p.ExpectedVersion,
		NowUTC:          p.NowUTC,
	})
	if err != nil {
		return CancelSeriesResult{}, err
	}
	return CancelSeriesResult{SeriesID: res.SeriesID, CanceledFromUTC: res.CanceledFromUTC, SessionsCanceled: res.SessionsCanceled}, nil
}

func (s *Service) CancelSeries(ctx context.Context, p CancelSeriesParams) (CancelSeriesResult, error) {
	res, err := s.seriesSvc.Cancel(ctx, series.CancelParams{
		SeriesID:        p.SeriesID,
		Scope:           series.CancelScope(p.Scope),
		PivotDate:       p.PivotDate,
		ExpectedVersion: p.ExpectedVersion,
		NowUTC:          p.NowUTC,
	})
	if err != nil {
		return CancelSeriesResult{}, err
	}
	return CancelSeriesResult{SeriesID: res.SeriesID, CanceledFromUTC: res.CanceledFromUTC, SessionsCanceled: res.SessionsCanceled}, nil
}

type CancelScope string

const (
	CancelScopeThisAndFuture          CancelScope = CancelScope(series.CancelScopeThisAndFuture)
	CancelScopeEntireSeriesFutureOnly CancelScope = CancelScope(series.CancelScopeEntireSeriesFutureOnly)
)

type EditEntireSeriesParams struct {
	SeriesID        pgtype.UUID
	ExpectedVersion int32
	NowUTC          time.Time

	CourseID        pgtype.UUID
	RoomID          pgtype.UUID
	TeacherID       pgtype.UUID
	Weekdays        []time.Weekday
	StartLocalTime  Clock
	DurationMinutes int
	EndDate         *LocalDate
	Count           *int
}

type EditEntireSeriesResult struct {
	SeriesID         pgtype.UUID
	SessionsCanceled int
	SessionsAdded    int
}

// EditEntireSeriesFutureOnlyTx edits a series' future occurrences using an existing tx-bound handle.
func (s *Service) EditEntireSeriesFutureOnlyTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, p EditEntireSeriesParams) (EditEntireSeriesResult, error) {
	ser, err := qtx.SeriesGetByIDForUpdate(ctx, p.SeriesID)
	if err != nil {
		return EditEntireSeriesResult{}, err
	}
	startLD := localDateFromPgDate(ser.StartDate)
	occ, err := series.Materialize(series.MaterializeInput{
		Weekdays:        p.Weekdays,
		StartDate:       startLD,
		EndDate:         p.EndDate,
		Count:           p.Count,
		StartLocalTime:  p.StartLocalTime,
		DurationMinutes: p.DurationMinutes,
		Location:        s.loc,
	})
	if err != nil {
		return EditEntireSeriesResult{}, err
	}

	ps, err := newPreflightStrings(p.CourseID, p.RoomID, p.TeacherID)
	if err != nil {
		return EditEntireSeriesResult{}, err
	}
	seriesIDStr, err := uuidString(p.SeriesID)
	if err != nil {
		return EditEntireSeriesResult{}, err
	}

	nowUTC := p.NowUTC
	if nowUTC.IsZero() {
		nowUTC = time.Now().UTC()
	}

	candidates := make([]preflightInput, 0, 64)
	for _, o := range occ {
		if o.StartUTC.Before(nowUTC) {
			continue
		}
		candidates = append(candidates, preflightInput{
			CourseID:  p.CourseID,
			RoomID:    p.RoomID,
			TeacherID: p.TeacherID,
			StartUTC:  o.StartUTC,
			EndUTC:    o.EndUTC,
			Requested: ps.conflictRequested(o.StartUTC, o.EndUTC, &seriesIDStr),
		})
	}

	res, err := s.seriesSvc.EditEntireSeriesFutureOnlyTx(ctx, qtx, series.EditEntireFutureParams{
		SeriesID:        p.SeriesID,
		ExpectedVersion: p.ExpectedVersion,
		NowUTC:          nowUTC,
		CourseID:        p.CourseID,
		RoomID:          p.RoomID,
		TeacherID:       p.TeacherID,
		Weekdays:        p.Weekdays,
		StartLocalTime:  p.StartLocalTime,
		DurationMinutes: p.DurationMinutes,
		EndDate:         p.EndDate,
		Count:           p.Count,
	})
	if err != nil {
		if strings.Contains(err.Error(), "stale_edit") {
			return EditEntireSeriesResult{}, &Err{Code: "stale_edit", Message: err.Error()}
		}
		if se := s.explainFromDBErrByRepreflightTx(ctx, err, tx, qtx, candidates); se != nil {
			return EditEntireSeriesResult{}, se
		}
		return EditEntireSeriesResult{}, err
	}
	return EditEntireSeriesResult{SeriesID: res.SeriesID, SessionsCanceled: res.SessionsCanceled, SessionsAdded: res.SessionsAdded}, nil
}

func (s *Service) EditEntireSeriesFutureOnly(ctx context.Context, p EditEntireSeriesParams) (EditEntireSeriesResult, error) {
	ser, err := s.q.SeriesGetByID(ctx, p.SeriesID)
	if err != nil {
		return EditEntireSeriesResult{}, err
	}
	startLD := localDateFromPgDate(ser.StartDate)
	occ, err := series.Materialize(series.MaterializeInput{
		Weekdays:        p.Weekdays,
		StartDate:       startLD,
		EndDate:         p.EndDate,
		Count:           p.Count,
		StartLocalTime:  p.StartLocalTime,
		DurationMinutes: p.DurationMinutes,
		Location:        s.loc,
	})
	if err != nil {
		return EditEntireSeriesResult{}, err
	}

	ps, err := newPreflightStrings(p.CourseID, p.RoomID, p.TeacherID)
	if err != nil {
		return EditEntireSeriesResult{}, err
	}
	seriesIDStr, err := uuidString(p.SeriesID)
	if err != nil {
		return EditEntireSeriesResult{}, err
	}

	nowUTC := p.NowUTC
	if nowUTC.IsZero() {
		nowUTC = time.Now().UTC()
	}

	candidates := make([]preflightInput, 0, 64)
	for _, o := range occ {
		if o.StartUTC.Before(nowUTC) {
			continue
		}
		candidates = append(candidates, preflightInput{
			CourseID:  p.CourseID,
			RoomID:    p.RoomID,
			TeacherID: p.TeacherID,
			StartUTC:  o.StartUTC,
			EndUTC:    o.EndUTC,
			Requested: ps.conflictRequested(o.StartUTC, o.EndUTC, &seriesIDStr),
		})
	}

	res, err := s.seriesSvc.EditEntireSeriesFutureOnly(ctx, series.EditEntireFutureParams{
		SeriesID:        p.SeriesID,
		ExpectedVersion: p.ExpectedVersion,
		NowUTC:          nowUTC,
		CourseID:        p.CourseID,
		RoomID:          p.RoomID,
		TeacherID:       p.TeacherID,
		Weekdays:        p.Weekdays,
		StartLocalTime:  p.StartLocalTime,
		DurationMinutes: p.DurationMinutes,
		EndDate:         p.EndDate,
		Count:           p.Count,
	})
	if err != nil {
		if strings.Contains(err.Error(), "stale_edit") {
			return EditEntireSeriesResult{}, &Err{Code: "stale_edit", Message: err.Error()}
		}
		if se := s.explainFromDBErrByRepreflight(ctx, err, candidates); se != nil {
			return EditEntireSeriesResult{}, se
		}
		return EditEntireSeriesResult{}, err
	}
	return EditEntireSeriesResult{SeriesID: res.SeriesID, SessionsCanceled: res.SessionsCanceled, SessionsAdded: res.SessionsAdded}, nil
}

type CreateSessionParams struct {
	SeriesID  *pgtype.UUID
	CourseID  pgtype.UUID
	RoomID    pgtype.UUID
	TeacherID pgtype.UUID
	StartAt   pgtype.Timestamptz
	EndAt     pgtype.Timestamptz
	// Required for optimistic concurrency on later edits; create ignores it.
}

type CreateSessionResult struct {
	SessionID pgtype.UUID
}

// CreateSessionTx creates a session using an existing tx-bound handle.
// The caller owns begin/commit/rollback. Runs preflight inside the caller's tx.
func (s *Service) CreateSessionTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, p CreateSessionParams) (CreateSessionResult, error) {
	if !p.StartAt.Valid || !p.EndAt.Valid || !p.EndAt.Time.After(p.StartAt.Time) {
		return CreateSessionResult{}, fmt.Errorf("invalid time range")
	}

	courseIDStr, err := uuidString(p.CourseID)
	if err != nil {
		return CreateSessionResult{}, err
	}
	roomIDStr, err := uuidStringPtr(p.RoomID)
	if err != nil {
		return CreateSessionResult{}, err
	}
	teacherIDStr, err := uuidString(p.TeacherID)
	if err != nil {
		return CreateSessionResult{}, err
	}
	var seriesIDStr *string
	var seriesID pgtype.UUID
	if p.SeriesID != nil && p.SeriesID.Valid {
		seriesID = *p.SeriesID
		v, err := uuidString(seriesID)
		if err != nil {
			return CreateSessionResult{}, err
		}
		seriesIDStr = &v
	}

	preflightIn := preflightInput{
		CourseID:  p.CourseID,
		RoomID:    p.RoomID,
		TeacherID: p.TeacherID,
		StartUTC:  p.StartAt.Time.UTC(),
		EndUTC:    p.EndAt.Time.UTC(),
		Requested: ConflictRequested{
			StartAt:   p.StartAt.Time.UTC().Format(time.RFC3339Nano),
			EndAt:     p.EndAt.Time.UTC().Format(time.RFC3339Nano),
			CourseID:  courseIDStr,
			RoomID:    roomIDStr,
			TeacherID: teacherIDStr,
			SeriesID:  seriesIDStr,
		},
	}

	// Preflight inside the caller's tx.
	if serr := s.preflightSlot(ctx, tx, qtx, preflightIn); serr != nil {
		return CreateSessionResult{}, serr
	}

	// Lock student busy ranges.
	students, err := qtx.CourseStudentsList(ctx, p.CourseID)
	if err != nil {
		return CreateSessionResult{}, err
	}

	if len(students) > 0 {
		studentIDs := make([]pgtype.UUID, len(students))
		for i, st := range students {
			studentIDs[i] = st.StudentID
		}
		_, err = qtx.StudentBusyRangesLockOverlapping(ctx, sqldb.StudentBusyRangesLockOverlappingParams{
			Column1:     studentIDs,
			Tstzrange:   p.StartAt.Time.UTC(),
			Tstzrange_2: p.EndAt.Time.UTC(),
		})
		if err != nil {
			return CreateSessionResult{}, err
		}
	}

	row, err := qtx.SessionCreate(ctx, sqldb.SessionCreateParams{
		SeriesID:  seriesID,
		CourseID:  p.CourseID,
		RoomID:    p.RoomID,
		TeacherID: p.TeacherID,
		StartAt:   p.StartAt,
		EndAt:     p.EndAt,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "23514") {
			if se := s.preflightSlot(ctx, s.db, s.q, preflightIn); se != nil {
				return CreateSessionResult{}, se
			}
		}
		return CreateSessionResult{}, err
	}

	return CreateSessionResult{SessionID: row.ID}, nil
}

func (s *Service) CreateSession(ctx context.Context, p CreateSessionParams) (CreateSessionResult, error) {
	if !p.StartAt.Valid || !p.EndAt.Valid || !p.EndAt.Time.After(p.StartAt.Time) {
		return CreateSessionResult{}, fmt.Errorf("invalid time range")
	}

	courseIDStr, err := uuidString(p.CourseID)
	if err != nil {
		return CreateSessionResult{}, err
	}
	roomIDStr, err := uuidStringPtr(p.RoomID)
	if err != nil {
		return CreateSessionResult{}, err
	}
	teacherIDStr, err := uuidString(p.TeacherID)
	if err != nil {
		return CreateSessionResult{}, err
	}
	var seriesIDStr *string
	var seriesID pgtype.UUID
	if p.SeriesID != nil && p.SeriesID.Valid {
		seriesID = *p.SeriesID
		v, err := uuidString(seriesID)
		if err != nil {
			return CreateSessionResult{}, err
		}
		seriesIDStr = &v
	}

	preflightIn := preflightInput{
		CourseID:  p.CourseID,
		RoomID:    p.RoomID,
		TeacherID: p.TeacherID,
		StartUTC:  p.StartAt.Time.UTC(),
		EndUTC:    p.EndAt.Time.UTC(),
		Requested: ConflictRequested{
			StartAt:   p.StartAt.Time.UTC().Format(time.RFC3339Nano),
			EndAt:     p.EndAt.Time.UTC().Format(time.RFC3339Nano),
			CourseID:  courseIDStr,
			RoomID:    roomIDStr,
			TeacherID: teacherIDStr,
			SeriesID:  seriesIDStr,
		},
	}

	const maxRetries = 2
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*10+rand.Intn(20)) * time.Millisecond)
		}
		tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return CreateSessionResult{}, err
		}

		qtx := s.q.WithTx(tx)

		// Preflight inside the serializable transaction.
		if serr := s.preflightSlot(ctx, tx, qtx, preflightIn); serr != nil {
			_ = tx.Rollback(ctx)
			return CreateSessionResult{}, serr
		}

		// Lock student busy ranges.
		students, err := qtx.CourseStudentsList(ctx, p.CourseID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return CreateSessionResult{}, err
		}

		if len(students) > 0 {
			studentIDs := make([]pgtype.UUID, len(students))
			for i, s := range students {
				studentIDs[i] = s.StudentID
			}
			_, err = qtx.StudentBusyRangesLockOverlapping(ctx, sqldb.StudentBusyRangesLockOverlappingParams{
				Column1:     studentIDs,
				Tstzrange:   p.StartAt.Time.UTC(),
				Tstzrange_2: p.EndAt.Time.UTC(),
			})
			if err != nil {
				_ = tx.Rollback(ctx)
				return CreateSessionResult{}, err
			}
		}

		row, err := qtx.SessionCreate(ctx, sqldb.SessionCreateParams{
			SeriesID:  seriesID,
			CourseID:  p.CourseID,
			RoomID:    p.RoomID,
			TeacherID: p.TeacherID,
			StartAt:   p.StartAt,
			EndAt:     p.EndAt,
		})
		if err != nil {
			lastErr = err
			var pgErr *pgconn.PgError
			if isRetryableSchedulingErr(err) && attempt < maxRetries {
				_ = tx.Rollback(ctx)
				continue
			}
			_ = tx.Rollback(ctx)
			if errors.As(err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "23514") {
				if se := s.preflightSlot(ctx, s.db, s.q, preflightIn); se != nil {
					return CreateSessionResult{}, se
				}
			}
			return CreateSessionResult{}, err
		}

		if err := tx.Commit(ctx); err != nil {
			lastErr = err
			if isRetryableSchedulingErr(err) && attempt < maxRetries {
				_ = tx.Rollback(ctx)
				continue
			}
			_ = tx.Rollback(ctx)
			return CreateSessionResult{}, err
		}

		return CreateSessionResult{SessionID: row.ID}, nil
	}

	return CreateSessionResult{}, fmt.Errorf("too many scheduling retries: %w", lastErr)
}

type EditOccurrenceParams struct {
	SessionID       pgtype.UUID
	StartAt         *pgtype.Timestamptz
	EndAt           *pgtype.Timestamptz
	CourseID        *pgtype.UUID
	RoomID          *pgtype.UUID
	TeacherID       *pgtype.UUID
	ExpectedVersion int32
}

type EditOccurrenceResult struct {
	SessionID pgtype.UUID
}

// EditOccurrenceTimeTx edits a session occurrence using an existing tx-bound handle.
func (s *Service) EditOccurrenceTimeTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, p EditOccurrenceParams) (EditOccurrenceResult, error) {
	// Fetch existing session inside the caller's tx for consistent reads.
	existing, err := qtx.SessionGetByID(ctx, p.SessionID)
	if err != nil {
		return EditOccurrenceResult{}, err
	}

	newCourseID := existing.CourseID
	if p.CourseID != nil && p.CourseID.Valid {
		newCourseID = *p.CourseID
	}
	newRoomID := existing.RoomID
	if p.RoomID != nil {
		newRoomID = *p.RoomID
	}
	newTeacherID := existing.TeacherID
	if p.TeacherID != nil && p.TeacherID.Valid {
		newTeacherID = *p.TeacherID
	}
	newStartAt := existing.StartAt
	if p.StartAt != nil && p.StartAt.Valid {
		newStartAt = *p.StartAt
	}
	newEndAt := existing.EndAt
	if p.EndAt != nil && p.EndAt.Valid {
		newEndAt = *p.EndAt
	}
	if !newStartAt.Valid || !newEndAt.Valid || !newEndAt.Time.After(newStartAt.Time) {
		return EditOccurrenceResult{}, fmt.Errorf("invalid time range")
	}

	courseIDStr, err := uuidString(newCourseID)
	if err != nil {
		return EditOccurrenceResult{}, err
	}
	roomIDStr, err := uuidStringPtr(newRoomID)
	if err != nil {
		return EditOccurrenceResult{}, err
	}
	teacherIDStr, err := uuidString(newTeacherID)
	if err != nil {
		return EditOccurrenceResult{}, err
	}
	var seriesIDStr *string
	if existing.SeriesID.Valid {
		v, err := uuidString(existing.SeriesID)
		if err != nil {
			return EditOccurrenceResult{}, err
		}
		seriesIDStr = &v
	}

	preflightIn := preflightInput{
		CourseID:      newCourseID,
		RoomID:        newRoomID,
		TeacherID:     newTeacherID,
		StartUTC:      newStartAt.Time.UTC(),
		EndUTC:        newEndAt.Time.UTC(),
		IgnoreSession: &p.SessionID,
		Requested: ConflictRequested{
			StartAt:   newStartAt.Time.UTC().Format(time.RFC3339Nano),
			EndAt:     newEndAt.Time.UTC().Format(time.RFC3339Nano),
			CourseID:  courseIDStr,
			RoomID:    roomIDStr,
			TeacherID: teacherIDStr,
			SeriesID:  seriesIDStr,
		},
	}

	courseChanged := existing.CourseID.Valid && newCourseID.Valid && existing.CourseID.Bytes != newCourseID.Bytes

	studentIDsPtr, hasOverrides, err := effectiveStudentIDsForSession(ctx, qtx, p.SessionID, newCourseID, courseChanged)
	if err != nil {
		return EditOccurrenceResult{}, err
	}
	if hasOverrides {
		preflightIn.StudentIDs = studentIDsPtr
	}

	// Preflight inside the caller's tx.
	if serr := s.preflightSlot(ctx, tx, qtx, preflightIn); serr != nil {
		return EditOccurrenceResult{}, serr
	}

	row, err := qtx.SessionUpdateOccurrence(ctx, sqldb.SessionUpdateOccurrenceParams{
		ID:        p.SessionID,
		CourseID:  newCourseID,
		RoomID:    newRoomID,
		TeacherID: newTeacherID,
		StartAt:   newStartAt,
		EndAt:     newEndAt,
		Version:   p.ExpectedVersion,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EditOccurrenceResult{}, fmt.Errorf("stale_edit: session %s has been modified", p.SessionID)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "23514") {
			// Re-run preflight using the same tx view (including any roster overrides).
			if se := s.preflightSlot(ctx, tx, qtx, preflightIn); se != nil {
				return EditOccurrenceResult{}, se
			}
		}
		return EditOccurrenceResult{}, err
	}

	if courseChanged {
		if err := qtx.SessionAttendanceDeleteNotInCourse(ctx, sqldb.SessionAttendanceDeleteNotInCourseParams{
			SessionID: p.SessionID,
			CourseID:  newCourseID,
		}); err != nil {
			return EditOccurrenceResult{}, err
		}
	}

	return EditOccurrenceResult{SessionID: row.ID}, nil
}

func (s *Service) EditOccurrenceTime(ctx context.Context, p EditOccurrenceParams) (EditOccurrenceResult, error) {
	const maxRetries = 2
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*10+rand.Intn(20)) * time.Millisecond)
		}

		tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return EditOccurrenceResult{}, err
		}

		qtx := s.q.WithTx(tx)

		// Fetch existing session inside the serializable tx for consistent reads.
		// The previous implementation read outside the tx, making the data stale
		// by the time the serializable tx opened (TOCTOU race).
		existing, err := qtx.SessionGetByID(ctx, p.SessionID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return EditOccurrenceResult{}, err
		}

		newCourseID := existing.CourseID
		if p.CourseID != nil && p.CourseID.Valid {
			newCourseID = *p.CourseID
		}
		newRoomID := existing.RoomID
		if p.RoomID != nil {
			newRoomID = *p.RoomID
		}
		newTeacherID := existing.TeacherID
		if p.TeacherID != nil && p.TeacherID.Valid {
			newTeacherID = *p.TeacherID
		}
		newStartAt := existing.StartAt
		if p.StartAt != nil && p.StartAt.Valid {
			newStartAt = *p.StartAt
		}
		newEndAt := existing.EndAt
		if p.EndAt != nil && p.EndAt.Valid {
			newEndAt = *p.EndAt
		}
		if !newStartAt.Valid || !newEndAt.Valid || !newEndAt.Time.After(newStartAt.Time) {
			_ = tx.Rollback(ctx)
			return EditOccurrenceResult{}, fmt.Errorf("invalid time range")
		}

		courseIDStr, err := uuidString(newCourseID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return EditOccurrenceResult{}, err
		}
		roomIDStr, err := uuidStringPtr(newRoomID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return EditOccurrenceResult{}, err
		}
		teacherIDStr, err := uuidString(newTeacherID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return EditOccurrenceResult{}, err
		}
		var seriesIDStr *string
		if existing.SeriesID.Valid {
			v, err := uuidString(existing.SeriesID)
			if err != nil {
				_ = tx.Rollback(ctx)
				return EditOccurrenceResult{}, err
			}
			seriesIDStr = &v
		}

		preflightIn := preflightInput{
			CourseID:      newCourseID,
			RoomID:        newRoomID,
			TeacherID:     newTeacherID,
			StartUTC:      newStartAt.Time.UTC(),
			EndUTC:        newEndAt.Time.UTC(),
			IgnoreSession: &p.SessionID,
			Requested: ConflictRequested{
				StartAt:   newStartAt.Time.UTC().Format(time.RFC3339Nano),
				EndAt:     newEndAt.Time.UTC().Format(time.RFC3339Nano),
				CourseID:  courseIDStr,
				RoomID:    roomIDStr,
				TeacherID: teacherIDStr,
				SeriesID:  seriesIDStr,
			},
		}

		courseChanged := existing.CourseID.Valid && newCourseID.Valid && existing.CourseID.Bytes != newCourseID.Bytes

		studentIDsPtr, hasOverrides, err := effectiveStudentIDsForSession(ctx, qtx, p.SessionID, newCourseID, courseChanged)
		if err != nil {
			_ = tx.Rollback(ctx)
			return EditOccurrenceResult{}, err
		}
		if hasOverrides {
			preflightIn.StudentIDs = studentIDsPtr
		}

		if serr := s.preflightSlot(ctx, tx, qtx, preflightIn); serr != nil {
			_ = tx.Rollback(ctx)
			return EditOccurrenceResult{}, serr
		}

		row, err := qtx.SessionUpdateOccurrence(ctx, sqldb.SessionUpdateOccurrenceParams{
			ID:        p.SessionID,
			CourseID:  newCourseID,
			RoomID:    newRoomID,
			TeacherID: newTeacherID,
			StartAt:   newStartAt,
			EndAt:     newEndAt,
			Version:   p.ExpectedVersion,
		})
		if err != nil {
			lastErr = err
			if errors.Is(err, pgx.ErrNoRows) {
				_ = tx.Rollback(ctx)
				return EditOccurrenceResult{}, fmt.Errorf("stale_edit: session %s has been modified", p.SessionID)
			}
			if isRetryableSchedulingErr(err) && attempt < maxRetries {
				_ = tx.Rollback(ctx)
				continue
			}
			_ = tx.Rollback(ctx)
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "23514") {
				// Best-effort: rebuild the effective roster outside the rolled-back tx.
				in2 := preflightIn
				studentIDsPtr2, hasOverrides2, err2 := effectiveStudentIDsForSession(ctx, s.q, p.SessionID, newCourseID, courseChanged)
				if err2 == nil && hasOverrides2 {
					in2.StudentIDs = studentIDsPtr2
				}
				if se := s.preflightSlot(ctx, s.db, s.q, in2); se != nil {
					return EditOccurrenceResult{}, se
				}
			}
			return EditOccurrenceResult{}, err
		}

		if courseChanged {
			if err := qtx.SessionAttendanceDeleteNotInCourse(ctx, sqldb.SessionAttendanceDeleteNotInCourseParams{
				SessionID: p.SessionID,
				CourseID:  newCourseID,
			}); err != nil {
				_ = tx.Rollback(ctx)
				return EditOccurrenceResult{}, err
			}
		}

		if err := tx.Commit(ctx); err != nil {
			lastErr = err
			if isRetryableSchedulingErr(err) && attempt < maxRetries {
				_ = tx.Rollback(ctx)
				continue
			}
			_ = tx.Rollback(ctx)
			return EditOccurrenceResult{}, err
		}

		return EditOccurrenceResult{SessionID: row.ID}, nil
	}

	return EditOccurrenceResult{}, fmt.Errorf("too many scheduling retries: %w", lastErr)
}

type preflightInput struct {
	CourseID      pgtype.UUID
	RoomID        pgtype.UUID
	TeacherID     pgtype.UUID
	StartUTC      time.Time
	EndUTC        time.Time
	IgnoreSession *pgtype.UUID
	IgnoreSeries  *pgtype.UUID
	StudentIDs    *[]pgtype.UUID
	Requested     ConflictRequested
}

func uuidString(u pgtype.UUID) (string, error) {
	if !u.Valid {
		return "", fmt.Errorf("invalid uuid")
	}
	id, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func uuidStringPtr(u pgtype.UUID) (*string, error) {
	if !u.Valid {
		return nil, nil
	}
	s, err := uuidString(u)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// preflightStrings holds the string versions of UUIDs commonly needed for building ConflictRequested.
type preflightStrings struct {
	CourseID  string
	RoomID    *string
	TeacherID string
}

func newPreflightStrings(courseID, roomID, teacherID pgtype.UUID) (preflightStrings, error) {
	courseIDStr, err := uuidString(courseID)
	if err != nil {
		return preflightStrings{}, err
	}
	roomIDStr, err := uuidStringPtr(roomID)
	if err != nil {
		return preflightStrings{}, err
	}
	teacherIDStr, err := uuidString(teacherID)
	if err != nil {
		return preflightStrings{}, err
	}
	return preflightStrings{CourseID: courseIDStr, RoomID: roomIDStr, TeacherID: teacherIDStr}, nil
}

// conflictRequested creates a ConflictRequested for a given occurrence time range.
func (ps preflightStrings) conflictRequested(startUTC, endUTC time.Time, seriesID *string) ConflictRequested {
	return ConflictRequested{
		StartAt:   startUTC.Format(time.RFC3339Nano),
		EndAt:     endUTC.Format(time.RFC3339Nano),
		CourseID:  ps.CourseID,
		RoomID:    ps.RoomID,
		TeacherID: ps.TeacherID,
		SeriesID:  seriesID,
	}
}

func (s *Service) explainFromDBErrByRepreflight(ctx context.Context, err error, candidates []preflightInput) *Err {
	return s._explainFromDBErrByRepreflight(ctx, err, s.db, s.q, candidates)
}

func (s *Service) explainFromDBErrByRepreflightTx(ctx context.Context, err error, tx pgx.Tx, qtx *sqldb.Queries, candidates []preflightInput) *Err {
	return s._explainFromDBErrByRepreflight(ctx, err, tx, qtx, candidates)
}

func (s *Service) _explainFromDBErrByRepreflight(ctx context.Context, err error, db sqldb.DBTX, q *sqldb.Queries, candidates []preflightInput) *Err {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}
	// Exclusion constraint violation or check constraint violation (incl. availability triggers).
	if pgErr.Code != "23P01" && pgErr.Code != "23514" {
		return nil
	}
	for _, in := range candidates {
		if in.StartUTC.IsZero() || in.EndUTC.IsZero() || !in.EndUTC.After(in.StartUTC) {
			continue
		}
		if se := s.preflightSlot(ctx, db, q, in); se != nil {
			return se
		}
	}
	return nil
}

func (s *Service) courseStudentPreflightInput(ctx context.Context, db sqldb.DBTX, courseID, studentID pgtype.UUID, ignoreSession *pgtype.UUID) (preflightInput, bool, error) {
	var minStart, maxEnd pgtype.Timestamptz
	if err := db.QueryRow(ctx, `
		SELECT MIN(start_at), MAX(end_at) FROM sessions
		WHERE course_id = $1 AND deleted_at IS NULL
	`, courseID).Scan(&minStart, &maxEnd); err != nil {
		return preflightInput{}, false, err
	}
	if !minStart.Valid || !maxEnd.Valid {
		return preflightInput{}, false, nil
	}

	courseIDStr, err := uuidString(courseID)
	if err != nil {
		return preflightInput{}, false, err
	}
	studentIDs := []pgtype.UUID{studentID}
	return preflightInput{
		CourseID:      courseID,
		StartUTC:      minStart.Time.UTC(),
		EndUTC:        maxEnd.Time.UTC(),
		StudentIDs:    &studentIDs,
		IgnoreSession: ignoreSession,
		Requested: ConflictRequested{
			StartAt:  minStart.Time.UTC().Format(time.RFC3339Nano),
			EndAt:    maxEnd.Time.UTC().Format(time.RFC3339Nano),
			CourseID: courseIDStr,
		},
	}, true, nil
}

func (s *Service) sessionIncludedStudentPreflightInput(ctx context.Context, qtx *sqldb.Queries, sessionID, studentID pgtype.UUID) (preflightInput, bool, error) {
	session, err := qtx.SessionGetByID(ctx, sessionID)
	if err != nil {
		return preflightInput{}, false, err
	}
	if !session.StartAt.Valid || !session.EndAt.Valid {
		return preflightInput{}, false, nil
	}

	courseIDStr, err := uuidString(session.CourseID)
	if err != nil {
		return preflightInput{}, false, err
	}
	roomIDStr, err := uuidStringPtr(session.RoomID)
	if err != nil {
		return preflightInput{}, false, err
	}
	teacherIDStr, err := uuidString(session.TeacherID)
	if err != nil {
		return preflightInput{}, false, err
	}
	var seriesIDStr *string
	if session.SeriesID.Valid {
		v, err := uuidString(session.SeriesID)
		if err != nil {
			return preflightInput{}, false, err
		}
		seriesIDStr = &v
	}

	studentIDs := []pgtype.UUID{studentID}
	return preflightInput{
		CourseID:      session.CourseID,
		StartUTC:      session.StartAt.Time.UTC(),
		EndUTC:        session.EndAt.Time.UTC(),
		StudentIDs:    &studentIDs,
		IgnoreSession: &sessionID,
		Requested: ConflictRequested{
			StartAt:   session.StartAt.Time.UTC().Format(time.RFC3339Nano),
			EndAt:     session.EndAt.Time.UTC().Format(time.RFC3339Nano),
			CourseID:  courseIDStr,
			RoomID:    roomIDStr,
			TeacherID: teacherIDStr,
			SeriesID:  seriesIDStr,
		},
	}, true, nil
}

func (s *Service) preflightStudentOverlap(ctx context.Context, db sqldb.DBTX, in preflightInput) *Err {
	if in.StudentIDs == nil || len(*in.StudentIDs) == 0 {
		return nil
	}
	conflicts, err := s.overlappingSessionsByStudents(ctx, db, *in.StudentIDs, in.StartUTC, in.EndUTC, in.IgnoreSession, in.IgnoreSeries)
	if err != nil {
		return &Err{Code: "db_error", Message: "Database error", Details: ConflictDetails{Kind: ConflictKindStudentOverlap, Conflicts: nil, Requested: in.Requested}}
	}
	if len(conflicts) == 0 {
		return nil
	}
	sessionIDs := make([]string, len(conflicts))
	for i, c := range conflicts {
		sessionIDs[i] = c.SessionID
	}
	conflictingStudents, _ := s.conflictingStudentsForOverlap(ctx, db, sessionIDs, *in.StudentIDs, in.CourseID)
	return &Err{
		Code:    "schedule_conflict",
		Message: "Schedule conflict",
		Details: ConflictDetails{
			Kind:                ConflictKindStudentOverlap,
			Conflicts:           conflicts,
			ConflictingStudents: conflictingStudents,
			Requested:           in.Requested,
		},
	}
}

func (s *Service) explainStudentDBErrByRepreflight(ctx context.Context, err error, db sqldb.DBTX, candidate preflightInput) *Err {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}
	if pgErr.Code != "23P01" && pgErr.Code != "23514" {
		return nil
	}
	return s.preflightStudentOverlap(ctx, db, candidate)
}

func (s *Service) FindAvailableSlots(ctx context.Context, p FindAvailableSlotsParams) (FindAvailableSlotsResult, error) {
	// Fetch course teacher.
	row := s.db.QueryRow(ctx, `SELECT teacher_id FROM courses WHERE id = $1`, p.CourseID)
	var teacherID pgtype.UUID
	if err := row.Scan(&teacherID); err != nil {
		return FindAvailableSlotsResult{}, fmt.Errorf("course not found: %w", err)
	}

	// Fetch course roster student IDs.
	roster, err := s.q.CourseStudentsList(ctx, p.CourseID)
	if err != nil {
		return FindAvailableSlotsResult{}, err
	}
	studentIDSet := map[[16]byte]pgtype.UUID{}
	for _, r := range roster {
		if r.StudentID.Valid {
			studentIDSet[r.StudentID.Bytes] = r.StudentID
		}
	}
	// Add the specific student we're checking for.
	if p.StudentID.Valid {
		studentIDSet[p.StudentID.Bytes] = p.StudentID
	}
	studentIDs := make([]pgtype.UUID, 0, len(studentIDSet))
	for _, id := range studentIDSet {
		studentIDs = append(studentIDs, id)
	}

	startDate := time.Date(p.StartDate.Year, p.StartDate.Month, p.StartDate.Day, 0, 0, 0, 0, s.loc)
	endDate := time.Date(p.EndDate.Year, p.EndDate.Month, p.EndDate.Day, 0, 0, 0, 0, s.loc)
	if endDate.Before(startDate) {
		return FindAvailableSlotsResult{}, fmt.Errorf("end_date must be on or after start_date")
	}

	// Cap at 14 days to prevent excessive computation.
	maxDays := 14
	if days := int(endDate.Sub(startDate).Hours()/24) + 1; days > maxDays {
		return FindAvailableSlotsResult{}, fmt.Errorf("date range limited to %d days", maxDays)
	}

	slotDur := p.SlotDurationMins
	if slotDur <= 0 {
		slotDur = 60
	}
	dayStart := p.DayStartHour
	if dayStart < 0 || dayStart > 23 {
		dayStart = 8
	}
	dayEnd := p.DayEndHour
	if dayEnd < 1 || dayEnd > 24 || dayEnd <= dayStart {
		dayEnd = 20
	}

	// Build all slots and tstzrange literals for batch checking.
	var slots []AvailableSlot
	var rangeLiterals []string

	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		for h := dayStart; h+slotDur/60 <= dayEnd; h++ {
			slotStart := time.Date(d.Year(), d.Month(), d.Day(), h, 0, 0, 0, s.loc).UTC()
			slotEnd := slotStart.Add(time.Duration(slotDur) * time.Minute).UTC()

			rangeLiterals = append(rangeLiterals, fmt.Sprintf(
				"tstzrange('%s'::timestamptz, '%s'::timestamptz, '[)')",
				slotStart.Format(time.RFC3339Nano),
				slotEnd.Format(time.RFC3339Nano),
			))

			slots = append(slots, AvailableSlot{
				Date:      d.Format("2006-01-02"),
				StartTime: slotStart.In(s.loc).Format("15:04"),
				EndTime:   slotEnd.In(s.loc).Format("15:04"),
			})
		}
	}

	if len(slots) == 0 {
		return FindAvailableSlotsResult{Slots: slots}, nil
	}

	rangeArray := "ARRAY[" + strings.Join(rangeLiterals, ",") + "]::tstzrange[]"
	blocked := make([]bool, len(slots))

	// 1. Batch teacher availability check.
	var hasWindows bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM teacher_availability WHERE teacher_id = $1 AND deleted_at IS NULL)`, teacherID).Scan(&hasWindows); err != nil {
		return FindAvailableSlotsResult{}, fmt.Errorf("teacher availability check: %w", err)
	}
	if hasWindows {
		q := fmt.Sprintf(`
SELECT DISTINCT sr.idx
FROM (SELECT unnest(%s) WITH ORDINALITY AS (r, idx)) sr
JOIN teacher_availability a ON a.time_range @> sr.r
WHERE a.teacher_id = $1 AND a.deleted_at IS NULL`, rangeArray)
		rows, err := s.db.Query(ctx, q, teacherID)
		if err != nil {
			return FindAvailableSlotsResult{}, fmt.Errorf("batch teacher availability: %w", err)
		}
		covered := make(map[int]bool, len(slots))
		for rows.Next() {
			var idx int
			if err := rows.Scan(&idx); err != nil {
				rows.Close()
				return FindAvailableSlotsResult{}, fmt.Errorf("scan teacher availability: %w", err)
			}
			covered[idx] = true
		}
		rows.Close()
		for i := range slots {
			if !covered[i] {
				blocked[i] = true
			}
		}
	}

	// 2. Batch teacher overlap check.
	{
		q := fmt.Sprintf(`
SELECT DISTINCT sr.idx
FROM (SELECT unnest(%s) WITH ORDINALITY AS (r, idx)) sr
JOIN sessions s ON s.time_range && sr.r
WHERE s.teacher_id = $1 AND s.deleted_at IS NULL`, rangeArray)
		rows, err := s.db.Query(ctx, q, teacherID)
		if err != nil {
			return FindAvailableSlotsResult{}, fmt.Errorf("batch teacher overlap: %w", err)
		}
		for rows.Next() {
			var idx int
			if err := rows.Scan(&idx); err != nil {
				rows.Close()
				return FindAvailableSlotsResult{}, fmt.Errorf("scan teacher overlap: %w", err)
			}
			blocked[idx] = true
		}
		rows.Close()
	}

	// 3. Batch student overlap check (if any students).
	if len(studentIDs) > 0 {
		q := fmt.Sprintf(`
SELECT DISTINCT sr.idx
FROM (SELECT unnest(%s) WITH ORDINALITY AS (r, idx)) sr
JOIN student_busy_ranges br ON br.time_range && sr.r
JOIN sessions s ON s.id = br.session_id AND s.deleted_at IS NULL
WHERE br.student_id = ANY($1::uuid[])
  AND br.deleted_at IS NULL`, rangeArray)
		rows, err := s.db.Query(ctx, q, studentIDs)
		if err != nil {
			return FindAvailableSlotsResult{}, fmt.Errorf("batch student overlap: %w", err)
		}
		for rows.Next() {
			var idx int
			if err := rows.Scan(&idx); err != nil {
				rows.Close()
				return FindAvailableSlotsResult{}, fmt.Errorf("scan student overlap: %w", err)
			}
			blocked[idx] = true
		}
		rows.Close()
	}

	for i := range slots {
		if blocked[i] {
			slots[i].Status = "blocked"
		} else {
			slots[i].Status = "provisional"
		}
	}

	return FindAvailableSlotsResult{Slots: slots}, nil
}

// isRetryableSchedulingErr reports whether err is a serialization failure (40001)
// or an exclusion-constraint violation (23P01). Under SERIALIZABLE isolation a
// concurrent insert can cause a false exclusion violation that succeeds on retry.
func isRetryableSchedulingErr(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "23P01")
}

// ensure pgx import stays used in builds where some features are behind tags.
var _ = pgx.ErrNoRows
