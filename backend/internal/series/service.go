package series

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/schedulelock"
)

type CreateParams struct {
	CourseID        pgtype.UUID
	RoomID          pgtype.UUID
	TeacherID       pgtype.UUID
	Weekdays        []time.Weekday
	StartLocalTime  Clock
	DurationMinutes int
	StartDate       LocalDate
	EndDate         *LocalDate
	Count           *int
	Occurrences     []Occurrence // pre-materialized occurrences; computed from params if nil
	AllowConflicts  bool
}

type CreateResult struct {
	SeriesID      pgtype.UUID
	SessionsAdded int
}

type Service struct {
	db          *pgxpool.Pool
	q           *sqldb.Queries
	instituteTZ string
	loc         *time.Location
}

func NewService(db *pgxpool.Pool, instituteTZ string) (*Service, error) {
	loc, err := time.LoadLocation(instituteTZ)
	if err != nil {
		return nil, err
	}
	return &Service{db: db, q: sqldb.New(db), instituteTZ: instituteTZ, loc: loc}, nil
}

type SplitParams struct {
	SeriesID        pgtype.UUID
	PivotDate       LocalDate
	ExpectedVersion int32

	// Optional overrides for the new series.
	Weekdays        []time.Weekday
	StartLocalTime  *Clock
	DurationMinutes *int
	EndDate         *LocalDate
	Count           *int
	AllowConflicts  bool
}

type SplitResult struct {
	OldSeriesID      pgtype.UUID
	NewSeriesID      pgtype.UUID
	OldSessionsEnded int
	NewSessionsAdded int
}

func isSerializationErr(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40001"
}

func ensureNativeSeries(ctx context.Context, qtx *sqldb.Queries, seriesID pgtype.UUID) error {
	source, err := qtx.SeriesSourceGetByIDForUpdate(ctx, seriesID)
	if err != nil {
		return err
	}
	if source.SourceKind == "legacy" || source.MaterializationMode == "external" {
		return newOperationError("external_series_read_only", "legacy-managed series cannot be changed by native scheduling operations")
	}
	return nil
}

// CreateSeriesAndMaterializeTx performs the series creation and session materialization
// using an existing transaction-bound Queries handle. Does not manage begin/commit/rollback.
func (s *Service) CreateSeriesAndMaterializeTx(ctx context.Context, qtx *sqldb.Queries, p CreateParams) (CreateResult, error) {
	materializeInput := MaterializeInput{
		Weekdays:        p.Weekdays,
		StartDate:       p.StartDate,
		EndDate:         p.EndDate,
		Count:           p.Count,
		StartLocalTime:  p.StartLocalTime,
		DurationMinutes: p.DurationMinutes,
		Location:        s.loc,
	}
	var occ []Occurrence
	if p.Occurrences != nil {
		if err := validateMaterializedOccurrences(ctx, materializeInput, p.Occurrences); err != nil {
			return CreateResult{}, err
		}
		occ = p.Occurrences
	} else {
		var err error
		occ, err = Materialize(ctx, materializeInput)
		if err != nil {
			return CreateResult{}, err
		}
	}
	// Compute the overall time span for student busy range locking.
	minStart, maxEnd := occ[0].StartUTC, occ[0].EndUTC
	for _, o := range occ {
		if o.StartUTC.Before(minStart) {
			minStart = o.StartUTC
		}
		if o.EndUTC.After(maxEnd) {
			maxEnd = o.EndUTC
		}
	}

	weekdays := make([]int16, 0, len(p.Weekdays))
	seen := map[time.Weekday]struct{}{}
	for _, wd := range p.Weekdays {
		if _, ok := seen[wd]; ok {
			continue
		}
		seen[wd] = struct{}{}
		weekdays = append(weekdays, int16(wd))
	}

	startLocal := time.Date(p.StartDate.Year, p.StartDate.Month, p.StartDate.Day, p.StartLocalTime.Hour, p.StartLocalTime.Minute, 0, 0, s.loc)
	startLocalTime := pgtype.Time{Microseconds: int64(startLocal.Hour())*60*60*1_000_000 + int64(startLocal.Minute())*60*1_000_000, Valid: true}

	startDate := pgtype.Date{Time: time.Date(p.StartDate.Year, p.StartDate.Month, p.StartDate.Day, 0, 0, 0, 0, time.UTC), Valid: true}
	var endDate pgtype.Date
	if p.EndDate != nil {
		endDate = pgtype.Date{Time: time.Date(p.EndDate.Year, p.EndDate.Month, p.EndDate.Day, 0, 0, 0, 0, time.UTC), Valid: true}
	}
	var count pgtype.Int4
	if p.Count != nil {
		count = pgtype.Int4{Int32: int32(*p.Count), Valid: true}
	}

	series, err := qtx.SeriesCreate(ctx, sqldb.SeriesCreateParams{
		CourseID:        p.CourseID,
		RoomID:          p.RoomID,
		TeacherID:       p.TeacherID,
		InstituteTz:     s.instituteTZ,
		Weekdays:        weekdays,
		StartLocalTime:  startLocalTime,
		DurationMinutes: int32(p.DurationMinutes),
		StartDate:       startDate,
		EndDate:         endDate,
		Count:           count,
	})
	if err != nil {
		return CreateResult{}, err
	}

	seriesID := series.ID

	// Lock student busy ranges FIRST to prevent deadlock with scheduling.CreateSessionTx
	// (which also locks student_ranges before sessions).
	students, err := qtx.CourseStudentsList(ctx, p.CourseID)
	if err != nil {
		return CreateResult{}, err
	}
	if len(students) > 0 {
		studentIDs := make([]pgtype.UUID, len(students))
		for i, st := range students {
			studentIDs[i] = st.StudentID
		}
		_, err = qtx.StudentBusyRangesLockOverlapping(ctx, sqldb.StudentBusyRangesLockOverlappingParams{
			Column1:     studentIDs,
			Tstzrange:   minStart,
			Tstzrange_2: maxEnd,
		})
		if err != nil {
			return CreateResult{}, err
		}
	}

	added := 0
	for _, o := range occ {
		_, err := qtx.SessionLockOverlappingForInsert(ctx, sqldb.SessionLockOverlappingForInsertParams{
			TeacherID:   p.TeacherID,
			Tstzrange:   o.StartUTC,
			Tstzrange_2: o.EndUTC,
			RoomID:      p.RoomID,
		})
		if err != nil {
			return CreateResult{}, err
		}
		_, err = qtx.SessionCreate(ctx, sqldb.SessionCreateParams{
			SeriesID:         seriesID,
			CourseID:         p.CourseID,
			RoomID:           p.RoomID,
			TeacherID:        p.TeacherID,
			StartAt:          pgtype.Timestamptz{Time: o.StartUTC, Valid: true},
			EndAt:            pgtype.Timestamptz{Time: o.EndUTC, Valid: true},
			ConflictOverride: p.AllowConflicts,
		})
		if err != nil {
			return CreateResult{}, err
		}
		added++
	}

	return CreateResult{SeriesID: seriesID, SessionsAdded: added}, nil
}

// SplitThisAndFutureTx performs the series split using an existing transaction-bound Queries handle.
func (s *Service) SplitThisAndFutureTx(ctx context.Context, qtx *sqldb.Queries, p SplitParams) (SplitResult, error) {
	discovered, err := qtx.SeriesGetByID(ctx, p.SeriesID)
	if err != nil {
		return SplitResult{}, err
	}
	if discovered.DeletedAt.Valid {
		return SplitResult{}, newValidationError("invalid_series", "series is inactive")
	}
	if err := ensureNativeSeries(ctx, qtx, p.SeriesID); err != nil {
		return SplitResult{}, err
	}
	pivotDate := pgtype.Date{Time: time.Date(p.PivotDate.Year, p.PivotDate.Month, p.PivotDate.Day, 0, 0, 0, 0, time.UTC), Valid: true}
	pivot, err := qtx.SessionFindActiveSeriesPivot(ctx, sqldb.SessionFindActiveSeriesPivotParams{SeriesID: p.SeriesID, Column2: pivotDate})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SplitResult{}, newValidationError("invalid_pivot", "pivot_date must identify an active series occurrence")
		}
		return SplitResult{}, err
	}
	if !pivot.IsFuture {
		return SplitResult{}, newValidationError("invalid_pivot", "pivot occurrence must not have started")
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{CourseIDs: []pgtype.UUID{discovered.CourseID}}); err != nil {
		return SplitResult{}, err
	}
	students, err := qtx.CourseStudentsList(ctx, discovered.CourseID)
	if err != nil {
		return SplitResult{}, err
	}
	studentIDs := make([]pgtype.UUID, len(students))
	for i, student := range students {
		studentIDs[i] = student.StudentID
	}
	futureSessionIDs, err := qtx.SessionListActiveIDsForSeriesFrom(ctx, sqldb.SessionListActiveIDsForSeriesFromParams{
		SeriesID: p.SeriesID,
		StartAt:  pivot.StartAt,
	})
	if err != nil {
		return SplitResult{}, err
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		StudentIDs: studentIDs,
		TeacherIDs: []pgtype.UUID{discovered.TeacherID},
		RoomIDs:    []pgtype.UUID{discovered.RoomID},
		SessionIDs: futureSessionIDs,
		SeriesIDs:  []pgtype.UUID{p.SeriesID},
	}); err != nil {
		return SplitResult{}, err
	}
	old, err := qtx.SeriesGetByIDForUpdate(ctx, p.SeriesID)
	if err != nil {
		return SplitResult{}, err
	}
	lockedPivot, err := qtx.SessionFindActiveSeriesPivot(ctx, sqldb.SessionFindActiveSeriesPivotParams{SeriesID: p.SeriesID, Column2: pivotDate})
	if err != nil || lockedPivot.ID.Bytes != pivot.ID.Bytes || !lockedPivot.IsFuture {
		return SplitResult{}, newValidationError("invalid_pivot", "pivot occurrence changed or has started")
	}
	if p.ExpectedVersion != 0 && old.Version != p.ExpectedVersion {
		return SplitResult{}, fmt.Errorf("stale_edit")
	}

	oldWeekdays := make([]time.Weekday, 0, len(old.Weekdays))
	for _, wd := range old.Weekdays {
		oldWeekdays = append(oldWeekdays, time.Weekday(wd))
	}

	originalClock := ClockFromPgTime(old.StartLocalTime)
	clock := originalClock
	if p.StartLocalTime != nil {
		clock = *p.StartLocalTime
	}
	duration := int(old.DurationMinutes)
	if p.DurationMinutes != nil {
		duration = *p.DurationMinutes
	}
	newWeekdays := oldWeekdays
	if len(p.Weekdays) > 0 {
		newWeekdays = p.Weekdays
	}
	unchangedDefinition := len(p.Weekdays) == 0 && p.StartLocalTime == nil && p.DurationMinutes == nil && p.EndDate == nil && p.Count == nil

	pivotLocalStart := time.Date(p.PivotDate.Year, p.PivotDate.Month, p.PivotDate.Day, clock.Hour, clock.Minute, 0, 0, s.loc)
	pivotUTC := pivot.StartAt.Time.UTC()
	dayBefore := pivotLocalStart.AddDate(0, 0, -1)
	dayBeforeDate := pgtype.Date{Time: time.Date(dayBefore.Year(), dayBefore.Month(), dayBefore.Day(), 0, 0, 0, 0, time.UTC), Valid: true}

	var retainedCount int32
	if old.Count.Valid {
		startDate := LocalDateFromPgDate(old.StartDate)
		retainedCount, err = retainedLegacyCount(ctx, oldWeekdays, startDate, p.PivotDate, old.Count.Int32)
		if err != nil {
			return SplitResult{}, err
		}
	} else {
		retainedCount, err = qtx.SessionCountActiveBeforeSeriesPivot(ctx, sqldb.SessionCountActiveBeforeSeriesPivotParams{
			SeriesID: old.ID,
			StartAt:  pgtype.Timestamptz{Time: pivotUTC, Valid: true},
		})
		if err != nil {
			return SplitResult{}, err
		}
	}
	if !old.EndDate.Valid && !old.Count.Valid {
		return SplitResult{}, fmt.Errorf("series missing end bound")
	}

	var endLD *LocalDate
	var countNew *int
	if unchangedDefinition {
		var oldEnd *LocalDate
		if old.EndDate.Valid {
			tmp := LocalDateFromPgDate(old.EndDate)
			oldEnd = &tmp
		}
		var oldCount *int32
		if old.Count.Valid {
			value := old.Count.Int32
			oldCount = &value
		}
		endLD, countNew, err = inheritedSuccessorBounds(p.PivotDate, oldEnd, oldCount, retainedCount)
		if err != nil {
			return SplitResult{}, err
		}
	} else {
		if p.EndDate != nil {
			tmp := *p.EndDate
			endLD = &tmp
		} else if old.EndDate.Valid {
			tmp := LocalDateFromPgDate(old.EndDate)
			endLD = &tmp
		}
		if p.Count != nil {
			partition, partitionErr := partitionCountBoundedSplit(int(retainedCount), *p.Count)
			if partitionErr != nil {
				return SplitResult{}, partitionErr
			}
			value := partition.Remaining
			countNew = &value
		} else if old.Count.Valid {
			value := int(old.Count.Int32)
			countNew = &value
		}
	}

	newMaterializeInput := MaterializeInput{
		Weekdays:        newWeekdays,
		StartDate:       p.PivotDate,
		EndDate:         endLD,
		Count:           countNew,
		StartLocalTime:  clock,
		DurationMinutes: duration,
		Location:        s.loc,
	}
	var occNew []Occurrence
	if !unchangedDefinition {
		occNew, err = Materialize(ctx, newMaterializeInput)
		if err != nil {
			return SplitResult{}, err
		}
		if err := qtx.SessionHardDeleteFutureBySeries(ctx, sqldb.SessionHardDeleteFutureBySeriesParams{
			SeriesID: old.ID,
			StartAt:  pgtype.Timestamptz{Time: pivotUTC, Valid: true},
		}); err != nil {
			return SplitResult{}, err
		}
	}

	inPlace := retainedCount == 0
	if !inPlace {
		// Clamp the old series to the portion before the pivot. Both-bound series
		// keep their count because the newly earlier end_date remains authoritative.
		if old.EndDate.Valid {
			if err := qtx.SeriesUpdateEndDate(ctx, sqldb.SeriesUpdateEndDateParams{ID: old.ID, EndDate: dayBeforeDate, Version: old.Version}); err != nil {
				return SplitResult{}, err
			}
		} else {
			bound := legacyRetainedBound(retainedCount, LocalDate{Year: dayBefore.Year(), Month: dayBefore.Month(), Day: dayBefore.Day()})
			if bound.Count != nil {
				if err := qtx.SeriesUpdateCount(ctx, sqldb.SeriesUpdateCountParams{ID: old.ID, Count: pgtype.Int4{Int32: *bound.Count, Valid: true}, Version: old.Version}); err != nil {
					return SplitResult{}, err
				}
			} else {
				if err := qtx.SeriesUpdateEndDate(ctx, sqldb.SeriesUpdateEndDateParams{ID: old.ID, EndDate: dayBeforeDate, Version: old.Version}); err != nil {
					return SplitResult{}, err
				}
				if err := qtx.SeriesUpdateCount(ctx, sqldb.SeriesUpdateCountParams{ID: old.ID, Count: pgtype.Int4{}, Version: old.Version + 1}); err != nil {
					return SplitResult{}, err
				}
			}
		}
	}

	var newEndDate pgtype.Date
	if endLD != nil {
		newEndDate = pgtype.Date{Time: time.Date(endLD.Year, endLD.Month, endLD.Day, 0, 0, 0, 0, time.UTC), Valid: true}
	}
	var newCount pgtype.Int4
	if countNew != nil {
		newCount = pgtype.Int4{Int32: int32(*countNew), Valid: true}
	}

	startDateNew := pgtype.Date{Time: time.Date(p.PivotDate.Year, p.PivotDate.Month, p.PivotDate.Day, 0, 0, 0, 0, time.UTC), Valid: true}
	startLocalTime := pgtype.Time{Microseconds: int64(clock.Hour)*60*60*1_000_000 + int64(clock.Minute)*60*1_000_000, Valid: true}
	wds := make([]int16, 0, len(newWeekdays))
	seen := map[time.Weekday]struct{}{}
	for _, wd := range newWeekdays {
		if _, ok := seen[wd]; ok {
			continue
		}
		seen[wd] = struct{}{}
		wds = append(wds, int16(wd))
	}
	if inPlace {
		if _, err := qtx.SeriesReplaceDefinition(ctx, sqldb.SeriesReplaceDefinitionParams{
			ID:              old.ID,
			RoomID:          old.RoomID,
			TeacherID:       old.TeacherID,
			Weekdays:        wds,
			StartLocalTime:  startLocalTime,
			DurationMinutes: int32(duration),
			StartDate:       startDateNew,
			EndDate:         newEndDate,
			Count:           newCount,
			Version:         old.Version,
		}); err != nil {
			return SplitResult{}, err
		}
		added := 0
		if !unchangedDefinition {
			for _, o := range occNew {
				if _, err := qtx.SessionCreate(ctx, sqldb.SessionCreateParams{
					SeriesID:         old.ID,
					CourseID:         old.CourseID,
					RoomID:           old.RoomID,
					TeacherID:        old.TeacherID,
					StartAt:          pgtype.Timestamptz{Time: o.StartUTC, Valid: true},
					EndAt:            pgtype.Timestamptz{Time: o.EndUTC, Valid: true},
					ConflictOverride: p.AllowConflicts,
				}); err != nil {
					return SplitResult{}, err
				}
				added++
			}
		}
		return SplitResult{OldSeriesID: old.ID, NewSeriesID: old.ID, NewSessionsAdded: added}, nil
	}

	newSeries, err := qtx.SeriesCreate(ctx, sqldb.SeriesCreateParams{
		CourseID:        old.CourseID,
		RoomID:          old.RoomID,
		TeacherID:       old.TeacherID,
		InstituteTz:     s.instituteTZ,
		Weekdays:        wds,
		StartLocalTime:  startLocalTime,
		DurationMinutes: int32(duration),
		StartDate:       startDateNew,
		EndDate:         newEndDate,
		Count:           newCount,
	})
	if err != nil {
		return SplitResult{}, err
	}

	added := 0
	if unchangedDefinition {
		moved, err := qtx.SessionReparentFutureBySeries(ctx, sqldb.SessionReparentFutureBySeriesParams{
			NewSeriesID: newSeries.ID,
			OldSeriesID: old.ID,
			StartAt:     pgtype.Timestamptz{Time: pivotUTC, Valid: true},
		})
		if err != nil {
			return SplitResult{}, err
		}
		added = int(moved)
	} else {
		for _, o := range occNew {
			_, err := qtx.SessionCreate(ctx, sqldb.SessionCreateParams{
				SeriesID:         newSeries.ID,
				CourseID:         old.CourseID,
				RoomID:           old.RoomID,
				TeacherID:        old.TeacherID,
				StartAt:          pgtype.Timestamptz{Time: o.StartUTC, Valid: true},
				EndAt:            pgtype.Timestamptz{Time: o.EndUTC, Valid: true},
				ConflictOverride: p.AllowConflicts,
			})
			if err != nil {
				return SplitResult{}, err
			}
			added++
		}
	}

	return SplitResult{
		OldSeriesID:      old.ID,
		NewSeriesID:      newSeries.ID,
		OldSessionsEnded: 0,
		NewSessionsAdded: added,
	}, nil
}

func (s *Service) SplitThisAndFuture(ctx context.Context, p SplitParams) (SplitResult, error) {
	const maxRetries = 2
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*10+rand.Intn(20)) * time.Millisecond)
		}

		tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return SplitResult{}, err
		}

		result, err := s.SplitThisAndFutureTx(ctx, s.q.WithTx(tx), p)
		if err != nil {
			_ = tx.Rollback(ctx)
			lastErr = err
			if isSerializationErr(err) && attempt < maxRetries {
				continue
			}
			return SplitResult{}, err
		}

		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(ctx)
			lastErr = err
			if isSerializationErr(err) && attempt < maxRetries {
				continue
			}
			return SplitResult{}, err
		}

		return result, nil
	}

	return SplitResult{}, fmt.Errorf("too many serialization retries: %w", lastErr)
}

type EditEntireFutureParams struct {
	SeriesID        pgtype.UUID
	ExpectedVersion int32
	NowUTC          time.Time

	// New definition for the series (applies to future occurrences only).
	CourseID        pgtype.UUID
	RoomID          pgtype.UUID
	TeacherID       pgtype.UUID
	Weekdays        []time.Weekday
	StartLocalTime  Clock
	DurationMinutes int
	EndDate         *LocalDate
	Count           *int
	AllowConflicts  bool
}

type EditEntireFutureResult struct {
	SeriesID         pgtype.UUID
	SessionsCanceled int
	SessionsAdded    int
}

// EditEntireSeriesFutureOnlyTx performs the edit-entire operation using an existing tx-bound Queries handle.
func (s *Service) EditEntireSeriesFutureOnlyTx(ctx context.Context, qtx *sqldb.Queries, p EditEntireFutureParams) (EditEntireFutureResult, error) {
	nowUTC := p.NowUTC
	if nowUTC.IsZero() {
		nowUTC = time.Now().UTC()
	}
	if p.DurationMinutes <= 0 {
		return EditEntireFutureResult{}, fmt.Errorf("bad_duration")
	}
	if len(p.Weekdays) == 0 {
		return EditEntireFutureResult{}, fmt.Errorf("bad_weekdays")
	}
	if p.EndDate == nil && p.Count == nil {
		return EditEntireFutureResult{}, fmt.Errorf("series missing end bound")
	}

	discovered, err := qtx.SeriesGetByID(ctx, p.SeriesID)
	if err != nil {
		return EditEntireFutureResult{}, err
	}
	courseIDs := []pgtype.UUID{discovered.CourseID, p.CourseID}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{CourseIDs: courseIDs}); err != nil {
		return EditEntireFutureResult{}, err
	}
	studentIDs := make([]pgtype.UUID, 0)
	for _, courseID := range courseIDs {
		students, listErr := qtx.CourseStudentsList(ctx, courseID)
		if listErr != nil {
			return EditEntireFutureResult{}, listErr
		}
		for _, student := range students {
			studentIDs = append(studentIDs, student.StudentID)
		}
	}
	sessionIDs, err := qtx.SessionListActiveIDsForSeriesFrom(ctx, sqldb.SessionListActiveIDsForSeriesFromParams{
		SeriesID: p.SeriesID, StartAt: pgtype.Timestamptz{Time: nowUTC, Valid: true},
	})
	if err != nil {
		return EditEntireFutureResult{}, err
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		StudentIDs: studentIDs, TeacherIDs: []pgtype.UUID{discovered.TeacherID, p.TeacherID}, RoomIDs: []pgtype.UUID{discovered.RoomID, p.RoomID},
		SessionIDs: sessionIDs, SeriesIDs: []pgtype.UUID{p.SeriesID},
	}); err != nil {
		return EditEntireFutureResult{}, err
	}
	ser, err := qtx.SeriesGetByIDForUpdate(ctx, p.SeriesID)
	if err != nil {
		return EditEntireFutureResult{}, err
	}
	if err := ensureNativeSeries(ctx, qtx, p.SeriesID); err != nil {
		return EditEntireFutureResult{}, err
	}
	if p.ExpectedVersion != 0 && ser.Version != p.ExpectedVersion {
		return EditEntireFutureResult{}, fmt.Errorf("stale_edit")
	}

	startDate := LocalDateFromPgDate(ser.StartDate)
	occ, err := Materialize(ctx, MaterializeInput{
		Weekdays:        p.Weekdays,
		StartDate:       startDate,
		EndDate:         p.EndDate,
		Count:           p.Count,
		StartLocalTime:  p.StartLocalTime,
		DurationMinutes: p.DurationMinutes,
		Location:        s.loc,
	})
	if err != nil {
		return EditEntireFutureResult{}, err
	}

	// Store weekdays as unique int16.
	wds := make([]int16, 0, len(p.Weekdays))
	seen := map[time.Weekday]struct{}{}
	for _, wd := range p.Weekdays {
		if _, ok := seen[wd]; ok {
			continue
		}
		seen[wd] = struct{}{}
		wds = append(wds, int16(wd))
	}

	startLocalTime := pgtype.Time{Microseconds: int64(p.StartLocalTime.Hour)*60*60*1_000_000 + int64(p.StartLocalTime.Minute)*60*1_000_000, Valid: true}

	var endDate pgtype.Date
	if p.EndDate != nil {
		endDate = pgtype.Date{Time: time.Date(p.EndDate.Year, p.EndDate.Month, p.EndDate.Day, 0, 0, 0, 0, time.UTC), Valid: true}
	}
	var count pgtype.Int4
	if p.Count != nil {
		count = pgtype.Int4{Int32: int32(*p.Count), Valid: true}
	}

	// Update series definition.
	updated, err := qtx.SeriesUpdateFields(ctx, sqldb.SeriesUpdateFieldsParams{
		ID:              ser.ID,
		CourseID:        p.CourseID,
		RoomID:          p.RoomID,
		TeacherID:       p.TeacherID,
		Weekdays:        wds,
		StartLocalTime:  startLocalTime,
		DurationMinutes: int32(p.DurationMinutes),
		EndDate:         endDate,
		Count:           count,
		Version:         ser.Version,
	})
	if err != nil {
		return EditEntireFutureResult{}, err
	}

	// Cancel all future sessions (from now).
	canceled, err := qtx.SessionHardDeleteFutureBySeriesCount(ctx, sqldb.SessionHardDeleteFutureBySeriesCountParams{
		SeriesID: updated.ID,
		StartAt:  pgtype.Timestamptz{Time: nowUTC, Valid: true},
	})
	if err != nil {
		return EditEntireFutureResult{}, err
	}

	added := 0
	for _, o := range occ {
		if !o.StartUTC.After(nowUTC) && !o.StartUTC.Equal(nowUTC) {
			continue
		}
		_, err := qtx.SessionCreate(ctx, sqldb.SessionCreateParams{
			SeriesID:         updated.ID,
			CourseID:         updated.CourseID,
			RoomID:           updated.RoomID,
			TeacherID:        updated.TeacherID,
			StartAt:          pgtype.Timestamptz{Time: o.StartUTC, Valid: true},
			EndAt:            pgtype.Timestamptz{Time: o.EndUTC, Valid: true},
			ConflictOverride: p.AllowConflicts,
		})
		if err != nil {
			return EditEntireFutureResult{}, err
		}
		added++
	}

	return EditEntireFutureResult{SeriesID: updated.ID, SessionsCanceled: int(canceled), SessionsAdded: added}, nil
}

func (s *Service) EditEntireSeriesFutureOnly(ctx context.Context, p EditEntireFutureParams) (EditEntireFutureResult, error) {
	const maxRetries = 2
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*10+rand.Intn(20)) * time.Millisecond)
		}

		tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return EditEntireFutureResult{}, err
		}

		result, err := s.EditEntireSeriesFutureOnlyTx(ctx, s.q.WithTx(tx), p)
		if err != nil {
			_ = tx.Rollback(ctx)
			lastErr = err
			if isSerializationErr(err) && attempt < maxRetries {
				continue
			}
			return EditEntireFutureResult{}, err
		}

		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(ctx)
			lastErr = err
			if isSerializationErr(err) && attempt < maxRetries {
				continue
			}
			return EditEntireFutureResult{}, err
		}

		return result, nil
	}

	return EditEntireFutureResult{}, fmt.Errorf("too many serialization retries: %w", lastErr)
}

type CancelScope string

const (
	CancelScopeThisAndFuture          CancelScope = "this_and_future"
	CancelScopeEntireSeriesFutureOnly CancelScope = "entire_series_future_only"
)

type CancelParams struct {
	SeriesID        pgtype.UUID
	Scope           CancelScope
	PivotDate       *LocalDate // required for this_and_future
	ExpectedVersion int32
	NowUTC          time.Time // injectable for tests; if zero, time.Now().UTC() is used
}

type CancelResult struct {
	SeriesID         pgtype.UUID
	CanceledFromUTC  time.Time
	SessionsCanceled int
}

// CancelTx performs the series cancel using an existing transaction-bound Queries handle.
func (s *Service) CancelTx(ctx context.Context, qtx *sqldb.Queries, p CancelParams) (CancelResult, error) {
	nowUTC := p.NowUTC
	if nowUTC.IsZero() {
		nowUTC = time.Now().UTC()
	}

	discovered, err := qtx.SeriesGetByID(ctx, p.SeriesID)
	if err != nil {
		return CancelResult{}, err
	}
	discoveredClock := ClockFromPgTime(discovered.StartLocalTime)
	lockFrom := nowUTC
	if p.Scope == CancelScopeThisAndFuture {
		if p.PivotDate == nil {
			return CancelResult{}, fmt.Errorf("pivot_date required")
		}
		pivot := *p.PivotDate
		lockFrom = time.Date(pivot.Year, pivot.Month, pivot.Day, discoveredClock.Hour, discoveredClock.Minute, 0, 0, s.loc).UTC()
	} else if p.Scope != CancelScopeEntireSeriesFutureOnly {
		return CancelResult{}, fmt.Errorf("bad_scope")
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{CourseIDs: []pgtype.UUID{discovered.CourseID}}); err != nil {
		return CancelResult{}, err
	}
	students, err := qtx.CourseStudentsList(ctx, discovered.CourseID)
	if err != nil {
		return CancelResult{}, err
	}
	studentIDs := make([]pgtype.UUID, len(students))
	for i, student := range students {
		studentIDs[i] = student.StudentID
	}
	sessionIDs, err := qtx.SessionListActiveIDsForSeriesFrom(ctx, sqldb.SessionListActiveIDsForSeriesFromParams{
		SeriesID: p.SeriesID, StartAt: pgtype.Timestamptz{Time: lockFrom, Valid: true},
	})
	if err != nil {
		return CancelResult{}, err
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		StudentIDs: studentIDs, TeacherIDs: []pgtype.UUID{discovered.TeacherID}, RoomIDs: []pgtype.UUID{discovered.RoomID},
		SessionIDs: sessionIDs, SeriesIDs: []pgtype.UUID{p.SeriesID},
	}); err != nil {
		return CancelResult{}, err
	}

	ser, err := qtx.SeriesGetByIDForUpdate(ctx, p.SeriesID)
	if err != nil {
		return CancelResult{}, err
	}
	if err := ensureNativeSeries(ctx, qtx, p.SeriesID); err != nil {
		return CancelResult{}, err
	}
	if p.ExpectedVersion != 0 && ser.Version != p.ExpectedVersion {
		return CancelResult{}, fmt.Errorf("stale_edit")
	}

	startClock := ClockFromPgTime(ser.StartLocalTime)

	var canceledFromUTC time.Time
	var pivotLocalDate LocalDate
	switch p.Scope {
	case CancelScopeThisAndFuture:
		if p.PivotDate == nil {
			return CancelResult{}, fmt.Errorf("pivot_date required")
		}
		pivotLocalDate = *p.PivotDate
		pivotLocalStart := time.Date(pivotLocalDate.Year, pivotLocalDate.Month, pivotLocalDate.Day, startClock.Hour, startClock.Minute, 0, 0, s.loc)
		canceledFromUTC = pivotLocalStart.UTC()
		if !canceledFromUTC.After(nowUTC) {
			return CancelResult{}, fmt.Errorf("cannot_cancel_started")
		}
	case CancelScopeEntireSeriesFutureOnly:
		canceledFromUTC = nowUTC
		localNow := nowUTC.In(s.loc)
		pivotLocalDate = LocalDate{Year: localNow.Year(), Month: localNow.Month(), Day: localNow.Day()}
	default:
		return CancelResult{}, fmt.Errorf("bad_scope")
	}

	canceled, err := qtx.SessionSoftDeleteFutureBySeriesCount(ctx, sqldb.SessionSoftDeleteFutureBySeriesCountParams{
		SeriesID: ser.ID,
		StartAt:  pgtype.Timestamptz{Time: canceledFromUTC, Valid: true},
	})
	if err != nil {
		return CancelResult{}, err
	}

	// Clamp series end bound so it doesn't "promise" future occurrences.
	pivotLocalStart := time.Date(pivotLocalDate.Year, pivotLocalDate.Month, pivotLocalDate.Day, startClock.Hour, startClock.Minute, 0, 0, s.loc)
	dayBefore := pivotLocalStart.AddDate(0, 0, -1)
	dayBeforeDate := pgtype.Date{Time: time.Date(dayBefore.Year(), dayBefore.Month(), dayBefore.Day(), 0, 0, 0, 0, time.UTC), Valid: true}

	if ser.EndDate.Valid {
		if err := qtx.SeriesUpdateEndDate(ctx, sqldb.SeriesUpdateEndDateParams{ID: ser.ID, EndDate: dayBeforeDate, Version: ser.Version}); err != nil {
			return CancelResult{}, err
		}
	} else if ser.Count.Valid {
		oldWeekdays := make([]time.Weekday, 0, len(ser.Weekdays))
		for _, wd := range ser.Weekdays {
			oldWeekdays = append(oldWeekdays, time.Weekday(wd))
		}
		startDate := LocalDateFromPgDate(ser.StartDate)
		before, err := retainedLegacyCount(ctx, oldWeekdays, startDate, pivotLocalDate, ser.Count.Int32)
		if err != nil {
			return CancelResult{}, err
		}
		bound := legacyRetainedBound(before, LocalDate{Year: dayBefore.Year(), Month: dayBefore.Month(), Day: dayBefore.Day()})
		if bound.Count != nil {
			if err := qtx.SeriesUpdateCount(ctx, sqldb.SeriesUpdateCountParams{ID: ser.ID, Count: pgtype.Int4{Int32: *bound.Count, Valid: true}, Version: ser.Version}); err != nil {
				return CancelResult{}, err
			}
		} else {
			if err := qtx.SeriesUpdateEndDate(ctx, sqldb.SeriesUpdateEndDateParams{ID: ser.ID, EndDate: dayBeforeDate, Version: ser.Version}); err != nil {
				return CancelResult{}, err
			}
			if err := qtx.SeriesUpdateCount(ctx, sqldb.SeriesUpdateCountParams{ID: ser.ID, Count: pgtype.Int4{}, Version: ser.Version + 1}); err != nil {
				return CancelResult{}, err
			}
		}
	} else {
		return CancelResult{}, fmt.Errorf("series missing end bound")
	}

	return CancelResult{SeriesID: ser.ID, CanceledFromUTC: canceledFromUTC, SessionsCanceled: int(canceled)}, nil
}

func (s *Service) Cancel(ctx context.Context, p CancelParams) (CancelResult, error) {
	const maxRetries = 2
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*10+rand.Intn(20)) * time.Millisecond)
		}

		tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return CancelResult{}, err
		}

		result, err := s.CancelTx(ctx, s.q.WithTx(tx), p)
		if err != nil {
			_ = tx.Rollback(ctx)
			lastErr = err
			if isSerializationErr(err) && attempt < maxRetries {
				continue
			}
			return CancelResult{}, err
		}

		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(ctx)
			lastErr = err
			if isSerializationErr(err) && attempt < maxRetries {
				continue
			}
			return CancelResult{}, err
		}

		return result, nil
	}

	return CancelResult{}, fmt.Errorf("too many serialization retries: %w", lastErr)
}

func ClockFromPgTime(t pgtype.Time) Clock {
	// Microseconds since midnight.
	us := t.Microseconds
	h := int(us / (60 * 60 * 1_000_000))
	us -= int64(h) * 60 * 60 * 1_000_000
	m := int(us / (60 * 1_000_000))
	return Clock{Hour: h, Minute: m}
}

func LocalDateFromPgDate(d pgtype.Date) LocalDate {
	t := d.Time.UTC()
	return LocalDate{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

func (d LocalDate) Before(other LocalDate) bool {
	if d.Year != other.Year {
		return d.Year < other.Year
	}
	if d.Month != other.Month {
		return d.Month < other.Month
	}
	return d.Day < other.Day
}
