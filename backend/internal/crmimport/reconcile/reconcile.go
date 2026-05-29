package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/crmimport/crmtypes"
	"warwick-institute/internal/crmimport/queue"
)

type ReconcileApplyResult struct {
	DesiredStudents      int      `json:"desired_students"`
	Added                int      `json:"added"`
	Removed              int      `json:"removed"`
	SkippedInvalidWcodes []string `json:"skipped_invalid_wcodes,omitempty"`
	SnapshotID           string   `json:"snapshot_id"`
}

type ReconcileDiffResult struct {
	AddCount             int      `json:"add_count"`
	RemoveCount          int      `json:"remove_count"`
	SkippedInvalidWcodes []string `json:"skipped_invalid_wcodes,omitempty"`
}

type reconcileDesiredStudent struct {
	WCode     string
	FirstName string
	LastName  string
}

type ReconcileV2Service struct {
	db *pgxpool.Pool
}

type EnqueueApplyJobError struct {
	Err error
}

func (e *EnqueueApplyJobError) Error() string {
	if e == nil || e.Err == nil {
		return "enqueue reconcile apply"
	}
	return e.Err.Error()
}

func (e *EnqueueApplyJobError) Unwrap() error { return e.Err }

func NewReconcileV2Service(db *pgxpool.Pool) *ReconcileV2Service {
	return &ReconcileV2Service{db: db}
}

func (s *ReconcileV2Service) queryDesiredStudentsV2(ctx context.Context, snapshotID pgtype.UUID, filter crmtypes.CourseFilter) ([]reconcileDesiredStudent, []string, error) {
	filter.Normalize()
	if err := filter.Validate(); err != nil {
		return nil, nil, err
	}

	conds, args := buildSnapshotFilterConditions(filter)
	conds = append(conds, "cr.snapshot_id = $"+fmt.Sprintf("%d", len(args)+1))
	args = append(args, snapshotID)

	sql := `
		SELECT DISTINCT ON (cr.wcode)
			cr.wcode,
			COALESCE(cr.first_name, '') AS first_name,
			COALESCE(cr.last_name, '') AS last_name
		FROM crm_rows cr
		WHERE ` + strings.Join(conds, " AND ") + `
		ORDER BY cr.wcode, cr.order_quote_updated_at DESC NULLS LAST, cr.xlsx_row_number ASC, cr.row_hash ASC
	`

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("query desired students: %w", err)
	}
	defer rows.Close()

	var desired []reconcileDesiredStudent
	var skippedWcodes []string
	for rows.Next() {
		var d reconcileDesiredStudent
		if err := rows.Scan(&d.WCode, &d.FirstName, &d.LastName); err != nil {
			return nil, nil, fmt.Errorf("scan desired student: %w", err)
		}
		if strings.TrimSpace(d.WCode) == "" {
			continue
		}
		d.WCode = strings.TrimSpace(d.WCode)
		desired = append(desired, d)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return desired, skippedWcodes, nil
}

func (s *ReconcileV2Service) reconcileDesiredStudentIDs(ctx context.Context, tx pgx.Tx, desired []reconcileDesiredStudent) (map[string]pgtype.UUID, map[string]string, []string, error) {
	studentIDs := make(map[string]pgtype.UUID)
	studentNames := make(map[string]string)
	var skippedWcodes []string

	for _, d := range desired {
		fullName := strings.TrimSpace(strings.TrimSpace(d.FirstName) + " " + strings.TrimSpace(d.LastName))
		if fullName == "" {
			fullName = d.WCode
		}

		var stID pgtype.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO students (wcode, full_name, notes)
			VALUES ($1, $2, '')
			ON CONFLICT (wcode) DO UPDATE
			SET full_name = EXCLUDED.full_name,
			    updated_at = now()
			RETURNING id
		`, d.WCode, fullName).Scan(&stID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("upsert student %s: %w", d.WCode, err)
		}
		if stID.Valid {
			studentIDs[d.WCode] = stID
			studentNames[d.WCode] = fullName
		}
	}

	return studentIDs, studentNames, skippedWcodes, nil
}

func (s *ReconcileV2Service) CheckFilterVersion(ctx context.Context, courseID pgtype.UUID, expectedVersion int) (bool, error) {
	var currentVersion int
	err := s.db.QueryRow(ctx,
		`SELECT crm_filter_version FROM courses WHERE id = $1`,
		courseID,
	).Scan(&currentVersion)
	if err != nil {
		return false, fmt.Errorf("get filter version: %w", err)
	}
	return currentVersion == expectedVersion, nil
}

func (s *ReconcileV2Service) BumpFilterVersion(ctx context.Context, courseID pgtype.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE courses SET crm_filter_version = crm_filter_version + 1, updated_at = now() WHERE id = $1`,
		courseID,
	)
	return err
}

func (s *ReconcileV2Service) ApplyCourseReconcile(ctx context.Context, snapshotID, courseID pgtype.UUID, filter crmtypes.CourseFilter) (*ReconcileApplyResult, error) {
	desired, skippedWcodes, err := s.queryDesiredStudentsV2(ctx, snapshotID, filter)
	if err != nil {
		return nil, fmt.Errorf("query desired students: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	desiredIDs, _, syncSkipped, err := s.reconcileDesiredStudentIDs(ctx, tx, desired)
	if err != nil {
		return nil, err
	}
	skippedWcodes = append(skippedWcodes, syncSkipped...)

	overrideExcludes := make(map[string]bool)
	overrideIncludes := make(map[string]bool)

	overrideRows, err := tx.Query(ctx, `
		SELECT s.wcode, cro.action
		FROM course_roster_overrides cro
		JOIN students s ON s.id = cro.student_id
		WHERE cro.course_id = $1 AND cro.deleted_at IS NULL
	`, courseID)
	if err != nil {
		return nil, fmt.Errorf("load overrides: %w", err)
	}
	for overrideRows.Next() {
		var wcode string
		var action string
		if err := overrideRows.Scan(&wcode, &action); err != nil {
			overrideRows.Close()
			return nil, fmt.Errorf("scan override: %w", err)
		}
		switch action {
		case "exclude":
			overrideExcludes[wcode] = true
		case "include":
			overrideIncludes[wcode] = true
		}
	}
	overrideRows.Close()

	finalDesired := make(map[string]pgtype.UUID)
	for wcode, id := range desiredIDs {
		if overrideExcludes[wcode] {
			continue
		}
		finalDesired[wcode] = id
	}

	includeRows, err := tx.Query(ctx, `
		SELECT s.id, s.wcode
		FROM course_roster_overrides cro
		JOIN students s ON s.id = cro.student_id
		WHERE cro.course_id = $1 AND cro.action = 'include' AND cro.deleted_at IS NULL
	`, courseID)
	if err != nil {
		return nil, fmt.Errorf("load includes: %w", err)
	}
	for includeRows.Next() {
		var stID pgtype.UUID
		var wcode string
		if err := includeRows.Scan(&stID, &wcode); err != nil {
			includeRows.Close()
			return nil, fmt.Errorf("scan include: %w", err)
		}
		if overrideIncludes[wcode] {
			finalDesired[wcode] = stID
		}
	}
	includeRows.Close()

	curRows, err := tx.Query(ctx, `SELECT student_id FROM course_students WHERE course_id = $1`, courseID)
	if err != nil {
		return nil, fmt.Errorf("query current students: %w", err)
	}
	currentSet := make(map[uuid.UUID]pgtype.UUID)
	for curRows.Next() {
		var id pgtype.UUID
		if err := curRows.Scan(&id); err != nil {
			curRows.Close()
			return nil, fmt.Errorf("scan current student: %w", err)
		}
		if id.Valid {
			uid, _ := uuid.FromBytes(id.Bytes[:])
			currentSet[uid] = id
		}
	}
	curRows.Close()

	added := 0
	removed := 0

	desiredUUIDSet := make(map[uuid.UUID]bool, len(finalDesired))
	for _, pgid := range finalDesired {
		if pgid.Valid {
			uid, _ := uuid.FromBytes(pgid.Bytes[:])
			desiredUUIDSet[uid] = true
		}
	}

	for wcode, pgid := range finalDesired {
		if !pgid.Valid {
			continue
		}
		uid, _ := uuid.FromBytes(pgid.Bytes[:])
		if _, ok := currentSet[uid]; ok {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO course_students (course_id, student_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			courseID, pgid,
		); err != nil {
			return nil, fmt.Errorf("add student %s: %w", wcode, err)
		}
		added++
	}

	for uid, pgid := range currentSet {
		if desiredUUIDSet[uid] {
			continue
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM course_students WHERE course_id = $1 AND student_id = $2`,
			courseID, pgid,
		); err != nil {
			return nil, fmt.Errorf("remove student: %w", err)
		}
		removed++
	}

	if _, err := tx.Exec(ctx,
		`UPDATE courses SET crm_last_applied_snapshot_id = $2, updated_at = now() WHERE id = $1`,
		courseID, snapshotID,
	); err != nil {
		return nil, fmt.Errorf("update last applied snapshot: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	snapshotIDStr, _ := uuid.FromBytes(snapshotID.Bytes[:])
	return &ReconcileApplyResult{
		DesiredStudents:      len(desired),
		Added:                added,
		Removed:              removed,
		SkippedInvalidWcodes: skippedWcodes,
		SnapshotID:           snapshotIDStr.String(),
	}, nil
}

func (s *ReconcileV2Service) DiffCourseReconcile(ctx context.Context, snapshotID, courseID pgtype.UUID, filter crmtypes.CourseFilter) (*ReconcileDiffResult, error) {
	desired, skippedWcodes, err := s.queryDesiredStudentsV2(ctx, snapshotID, filter)
	if err != nil {
		return nil, fmt.Errorf("query desired students: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	desiredIDs, _, _, err := s.reconcileDesiredStudentIDs(ctx, tx, desired)
	if err != nil {
		return nil, err
	}

	curRows, err := tx.Query(ctx, `
		SELECT cs.student_id, s.wcode, s.full_name
		FROM course_students cs
		JOIN students s ON s.id = cs.student_id
		WHERE cs.course_id = $1
	`, courseID)
	if err != nil {
		return nil, fmt.Errorf("query current students: %w", err)
	}
	currentMap := make(map[string]struct {
		ID       pgtype.UUID
		WCode    string
		FullName string
	})
	for curRows.Next() {
		var id pgtype.UUID
		var wcode, fullName string
		if err := curRows.Scan(&id, &wcode, &fullName); err != nil {
			curRows.Close()
			return nil, fmt.Errorf("scan current: %w", err)
		}
		currentMap[wcode] = struct {
			ID       pgtype.UUID
			WCode    string
			FullName string
		}{ID: id, WCode: wcode, FullName: fullName}
	}
	curRows.Close()

	overrideExcludes := make(map[string]bool)
	orRows, err := tx.Query(ctx, `
		SELECT s.wcode FROM course_roster_overrides cro
		JOIN students s ON s.id = cro.student_id
		WHERE cro.course_id = $1 AND cro.action = 'exclude' AND cro.deleted_at IS NULL
	`, courseID)
	if err != nil {
		return nil, fmt.Errorf("load excludes: %w", err)
	}
	for orRows.Next() {
		var wcode string
		if err := orRows.Scan(&wcode); err != nil {
			orRows.Close()
			return nil, fmt.Errorf("scan exclude: %w", err)
		}
		overrideExcludes[wcode] = true
	}
	orRows.Close()

	var addSet []reconcileDesiredStudent
	for _, d := range desired {
		if overrideExcludes[d.WCode] {
			continue
		}
		if _, exists := currentMap[d.WCode]; !exists {
			addSet = append(addSet, d)
		}
	}

	var removeSet []struct {
		WCode     string
		FullName  string
		StudentID pgtype.UUID
	}
	for wcode, cur := range currentMap {
		if _, inDesired := desiredIDs[wcode]; !inDesired {
			removeSet = append(removeSet, struct {
				WCode     string
				FullName  string
				StudentID pgtype.UUID
			}{WCode: wcode, FullName: cur.FullName, StudentID: cur.ID})
		}
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM crm_pending_diffs WHERE course_id = $1 AND snapshot_id = $2`,
		courseID, snapshotID,
	); err != nil {
		return nil, fmt.Errorf("clear pending diffs: %w", err)
	}

	for seq, d := range addSet {
		fullName := strings.TrimSpace(strings.TrimSpace(d.FirstName) + " " + strings.TrimSpace(d.LastName))
		if fullName == "" {
			fullName = d.WCode
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO crm_pending_diffs (course_id, snapshot_id, diff_action, seq, student_id, wcode, full_name)
			VALUES ($1, $2, 'add', $3, NULL, $4, $5)
			ON CONFLICT (course_id, snapshot_id, diff_action, seq) DO UPDATE
			SET wcode = EXCLUDED.wcode, full_name = EXCLUDED.full_name
		`, courseID, snapshotID, seq+1, d.WCode, fullName); err != nil {
			return nil, fmt.Errorf("insert add diff: %w", err)
		}
	}

	for seq, r := range removeSet {
		if _, err := tx.Exec(ctx, `
			INSERT INTO crm_pending_diffs (course_id, snapshot_id, diff_action, seq, student_id, wcode, full_name)
			VALUES ($1, $2, 'remove', $3, $4, $5, $6)
			ON CONFLICT (course_id, snapshot_id, diff_action, seq) DO UPDATE
			SET wcode = EXCLUDED.wcode, full_name = EXCLUDED.full_name, student_id = EXCLUDED.student_id
		`, courseID, snapshotID, seq+1, r.StudentID, r.WCode, r.FullName); err != nil {
			return nil, fmt.Errorf("insert remove diff: %w", err)
		}
	}

	summary := crmtypes.ReviewSummary{
		AddCount:    len(addSet),
		RemoveCount: len(removeSet),
	}

	var firstPage []crmtypes.PendingDiffRow
	for _, d := range addSet {
		if len(firstPage) >= 10 {
			break
		}
		fullName := strings.TrimSpace(strings.TrimSpace(d.FirstName) + " " + strings.TrimSpace(d.LastName))
		if fullName == "" {
			fullName = d.WCode
		}
		firstPage = append(firstPage, crmtypes.PendingDiffRow{
			Action:   crmtypes.DiffAdd,
			WCode:    d.WCode,
			FullName: fullName,
		})
	}
	for _, r := range removeSet {
		if len(firstPage) >= 10 {
			break
		}
		firstPage = append(firstPage, crmtypes.PendingDiffRow{
			Action:   crmtypes.DiffRemove,
			WCode:    r.WCode,
			FullName: r.FullName,
		})
	}
	summary.FirstPage = firstPage

	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return nil, fmt.Errorf("marshal summary: %w", err)
	}

	snapshotIDStr := pgtype.UUID{Bytes: snapshotID.Bytes, Valid: snapshotID.Valid}
	if _, err := tx.Exec(ctx, `
		UPDATE courses
		SET crm_pending_review_snapshot_id = $2,
		    crm_pending_review_summary = $3::jsonb,
		    updated_at = now()
		WHERE id = $1
	`, courseID, snapshotIDStr, string(summaryJSON)); err != nil {
		return nil, fmt.Errorf("update course review summary: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &ReconcileDiffResult{
		AddCount:             len(addSet),
		RemoveCount:          len(removeSet),
		SkippedInvalidWcodes: skippedWcodes,
	}, nil
}

func (s *ReconcileV2Service) ApproveReview(ctx context.Context, courseID pgtype.UUID, worker *queue.QueueWorker) error {
	var pendingSnapshotID pgtype.UUID
	var currentFilter []byte
	var filterVersion int

	err := s.db.QueryRow(ctx, `
		SELECT crm_pending_review_snapshot_id, crm_filter, crm_filter_version
		FROM courses WHERE id = $1
	`, courseID).Scan(&pendingSnapshotID, &currentFilter, &filterVersion)
	if err != nil {
		return fmt.Errorf("get pending review: %w", err)
	}
	if !pendingSnapshotID.Valid {
		return fmt.Errorf("no pending review for course")
	}

	snapshotUUID, _ := uuid.FromBytes(pendingSnapshotID.Bytes[:])

	var filter crmtypes.CourseFilter
	if len(currentFilter) > 0 {
		if err := json.Unmarshal(currentFilter, &filter); err != nil {
			return fmt.Errorf("unmarshal filter: %w", err)
		}
	}

	uniqueKey := fmt.Sprintf("reconcile-apply-%s-%s", snapshotUUID.String(), courseID)
	payload := crmtypes.CourseReconcilePayload{
		SnapshotID:            snapshotUUID,
		CourseID:              uuid.Must(uuid.FromBytes(courseID.Bytes[:])),
		ExpectedFilterVersion: filterVersion,
	}

	_, err = worker.EnqueueJob(ctx, queue.JobTypeCourseReconcileApply, payload, uniqueKey)
	return err
}

func (s *ReconcileV2Service) RejectReview(ctx context.Context, courseID pgtype.UUID) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var pendingSnapshotID pgtype.UUID
	err = tx.QueryRow(ctx,
		`SELECT crm_pending_review_snapshot_id FROM courses WHERE id = $1 FOR UPDATE`,
		courseID,
	).Scan(&pendingSnapshotID)
	if err != nil {
		return fmt.Errorf("get pending snapshot: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM crm_pending_diffs WHERE course_id = $1 AND snapshot_id = $2`,
		courseID, pendingSnapshotID,
	); err != nil {
		return fmt.Errorf("delete pending diffs: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE courses
		SET crm_pending_review_snapshot_id = NULL,
		    crm_pending_review_summary = NULL,
		    updated_at = now()
		WHERE id = $1
	`, courseID); err != nil {
		return fmt.Errorf("clear pending review: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *ReconcileV2Service) EnqueueReconcileJobsForSnapshot(ctx context.Context, snapshotID pgtype.UUID, worker *queue.QueueWorker) error {
	type courseRow struct {
		ID            pgtype.UUID
		FilterJSON    []byte
		FilterVersion int
		RosterLocked  bool
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, crm_filter, crm_filter_version, crm_roster_locked
		FROM courses
		WHERE crm_filter_enabled = true AND deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("query courses: %w", err)
	}

	var courses []courseRow
	for rows.Next() {
		var c courseRow
		if err := rows.Scan(&c.ID, &c.FilterJSON, &c.FilterVersion, &c.RosterLocked); err != nil {
			rows.Close()
			return fmt.Errorf("scan course: %w", err)
		}
		if c.FilterJSON != nil {
			courses = append(courses, c)
		}
	}
	rows.Close()

	snapshotUUID, err := uuid.FromBytes(snapshotID.Bytes[:])
	if err != nil {
		return fmt.Errorf("parse snapshot uuid: %w", err)
	}

	var enqueueFailures []string
	for _, c := range courses {
		var filter crmtypes.CourseFilter
		if err := json.Unmarshal(c.FilterJSON, &filter); err != nil {
			continue
		}
		_ = filter

		courseUUID, err := uuid.FromBytes(c.ID.Bytes[:])
		if err != nil {
			continue
		}

		jobType := queue.JobTypeCourseReconcileApply
		if c.RosterLocked {
			jobType = queue.JobTypeCourseReconcileDiff
		}

		uniqueKey := fmt.Sprintf("%s-%s-%s", string(jobType), snapshotUUID.String(), courseUUID.String())
		reconcilePayload := crmtypes.CourseReconcilePayload{
			SnapshotID:            snapshotUUID,
			CourseID:              courseUUID,
			ExpectedFilterVersion: c.FilterVersion,
		}

		if _, err := worker.EnqueueJob(ctx, jobType, reconcilePayload, uniqueKey); err != nil {
			enqueueFailures = append(enqueueFailures, fmt.Sprintf("%s: %v", courseUUID.String(), err))
		}
	}
	if len(enqueueFailures) > 0 {
		return fmt.Errorf("enqueue course reconcile jobs failed for %d/%d courses: %s", len(enqueueFailures), len(courses), strings.Join(enqueueFailures, "; "))
	}

	return nil
}

func (s *ReconcileV2Service) GetPendingDiffPage(ctx context.Context, courseID pgtype.UUID, action crmtypes.DiffAction, cursor int, limit int) ([]crmtypes.PendingDiffRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.db.Query(ctx, `
		SELECT course_id, snapshot_id, diff_action::text, seq,
		       COALESCE(student_id, '00000000-0000-0000-0000-000000000000'::uuid), wcode, full_name
		FROM crm_pending_diffs
		WHERE course_id = $1 AND diff_action = $2::diff_action AND seq > $3
		ORDER BY seq ASC
		LIMIT $4
	`, courseID, string(action), cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("query pending diffs: %w", err)
	}
	defer rows.Close()

	var out []crmtypes.PendingDiffRow
	for rows.Next() {
		var d crmtypes.PendingDiffRow
		if err := rows.Scan(
			&d.CourseID, &d.SnapshotID, (*string)(&d.Action), &d.Seq,
			&d.StudentID, &d.WCode, &d.FullName,
		); err != nil {
			return nil, fmt.Errorf("scan diff: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *ReconcileV2Service) PreviewCountForFilter(ctx context.Context, filter crmtypes.CourseFilter) (int, error) {
	filter.Normalize()
	if err := filter.Validate(); err != nil {
		return 0, err
	}

	var snapshotID pgtype.UUID
	err := s.db.QueryRow(ctx,
		`SELECT COALESCE(active_snapshot_id, '00000000-0000-0000-0000-000000000000'::uuid) FROM crm_state WHERE singleton = true`,
	).Scan(&snapshotID)
	if err != nil || !snapshotID.Valid {
		return 0, nil
	}

	conds, args := buildSnapshotFilterConditions(filter)
	conds = append(conds, fmt.Sprintf("cr.snapshot_id = $%d", len(args)+1))
	args = append(args, snapshotID)

	sql := `SELECT COUNT(DISTINCT cr.wcode) FROM crm_rows cr WHERE ` + strings.Join(conds, " AND ")

	var count int
	err = s.db.QueryRow(ctx, sql, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("preview count: %w", err)
	}
	return count, nil
}

func (s *ReconcileV2Service) GetCourseFilterState(ctx context.Context, courseID pgtype.UUID) (enabled bool, locked bool, filterJSON []byte, err error) {
	err = s.db.QueryRow(ctx,
		`SELECT crm_filter_enabled, crm_roster_locked, COALESCE(crm_filter,'{}'::jsonb) FROM courses WHERE id=$1`,
		courseID,
	).Scan(&enabled, &locked, &filterJSON)
	if err != nil {
		return false, false, nil, fmt.Errorf("get course filter state: %w", err)
	}
	return enabled, locked, filterJSON, nil
}

func (s *ReconcileV2Service) UpdateCourseFilter(ctx context.Context, courseID pgtype.UUID, enabled bool, filter crmtypes.CourseFilter) error {
	filterJSON, err := json.Marshal(filter)
	if err != nil {
		return fmt.Errorf("marshal filter: %w", err)
	}

	_, err = s.db.Exec(ctx, `
		UPDATE courses
		SET crm_filter_enabled = $2,
		    crm_filter = $3::jsonb,
		    crm_filter_updated_at = now(),
		    crm_filter_version = crm_filter_version + 1,
		    updated_at = now()
		WHERE id = $1
	`, courseID, enabled, string(filterJSON))
	return err
}

func (s *ReconcileV2Service) SetCourseFilterAndEnqueueApply(ctx context.Context, worker *queue.QueueWorker, courseID pgtype.UUID, enabled bool, filterJSON []byte) error {
	_, err := s.db.Exec(ctx, `
		UPDATE courses
		SET crm_filter_enabled = $2,
		    crm_filter = $3::jsonb,
		    crm_filter_updated_at = now(),
		    crm_filter_version = crm_filter_version + 1,
		    updated_at = now()
		WHERE id = $1
	`, courseID, enabled, filterJSON)
	if err != nil {
		return fmt.Errorf("update course filter: %w", err)
	}

	if !enabled {
		return nil
	}

	return s.enqueueApplyIfEnabledAndUnlocked(ctx, worker, courseID)
}

func (s *ReconcileV2Service) SetRosterLockAndEnqueueApply(ctx context.Context, worker *queue.QueueWorker, courseID pgtype.UUID, locked bool) error {
	_, err := s.db.Exec(ctx, `UPDATE courses SET crm_roster_locked = $2 WHERE id = $1`, courseID, locked)
	if err != nil {
		return fmt.Errorf("update roster lock: %w", err)
	}

	if locked {
		return nil
	}

	return s.enqueueApplyIfEnabledAndUnlocked(ctx, worker, courseID)
}

func (s *ReconcileV2Service) enqueueApplyIfEnabledAndUnlocked(ctx context.Context, worker *queue.QueueWorker, courseID pgtype.UUID) error {
	if worker == nil {
		return nil
	}

	var enabled bool
	var locked bool
	var filterVersion int
	err := s.db.QueryRow(ctx, `
		SELECT crm_filter_enabled, crm_roster_locked, crm_filter_version
		FROM courses
		WHERE id = $1
	`, courseID).Scan(&enabled, &locked, &filterVersion)
	if err != nil {
		return fmt.Errorf("load course reconcile state: %w", err)
	}

	if !enabled || locked {
		return nil
	}

	var snapshotID pgtype.UUID
	if err := s.db.QueryRow(ctx, `
		SELECT COALESCE(active_snapshot_id, '00000000-0000-0000-0000-000000000000'::uuid)
		FROM crm_state
		WHERE singleton = true
	`).Scan(&snapshotID); err != nil {
		return fmt.Errorf("load active snapshot: %w", err)
	}
	if !snapshotID.Valid {
		return nil
	}

	snapshotUUID, err := uuid.FromBytes(snapshotID.Bytes[:])
	if err != nil {
		return fmt.Errorf("parse snapshot id: %w", err)
	}
	courseUUID, err := uuid.FromBytes(courseID.Bytes[:])
	if err != nil {
		return fmt.Errorf("parse course id: %w", err)
	}

	payload := crmtypes.CourseReconcilePayload{
		SnapshotID:            snapshotUUID,
		CourseID:              courseUUID,
		ExpectedFilterVersion: filterVersion,
	}
	uniqueKey := fmt.Sprintf("reconcile-apply-%s-%s", snapshotUUID.String(), courseUUID.String())
	if _, err := worker.EnqueueJob(ctx, queue.JobTypeCourseReconcileApply, payload, uniqueKey); err != nil {
		return &EnqueueApplyJobError{Err: err}
	}
	return nil
}

// buildSnapshotFilterConditions builds SQL WHERE conditions and args from a CourseFilter.
func buildSnapshotFilterConditions(filter crmtypes.CourseFilter) ([]string, []any) {
	conds := []string{"1=1"}
	args := []any{}
	argN := 1

	addIn := func(col string, values []string) {
		if len(values) == 0 {
			return
		}
		conds = append(conds, fmt.Sprintf("cr.%s = ANY($%d)", col, argN))
		args = append(args, values)
		argN++
	}

	addBlankMode := func(col string, mode crmtypes.BlankMode) {
		switch mode {
		case crmtypes.BlankModeAny:
		case crmtypes.BlankModeOnlyBlank:
			conds = append(conds, fmt.Sprintf("(cr.%s IS NULL OR btrim(cr.%s) = '')", col, col))
		case crmtypes.BlankModeOnlyValue:
			conds = append(conds, fmt.Sprintf("(cr.%s IS NOT NULL AND btrim(cr.%s) <> '')", col, col))
		}
	}

	addIn("cycle_label", filter.CycleLabels)
	addBlankMode("cycle_label", filter.CycleBlankMode)
	addIn("course_name", filter.CourseNameValues)
	addBlankMode("course_name", filter.CourseNameBlankMode)
	addIn("academic_level", filter.AcademicLevelValues)
	addBlankMode("academic_level", filter.AcademicLevelBlankMode)
	addIn("secondary_school", filter.SecondarySchoolValues)
	addBlankMode("secondary_school", filter.SecondarySchoolBlankMode)

	if filter.TeachersContains != "" {
		conds = append(conds, fmt.Sprintf("cr.teachers_raw ILIKE $%d", argN))
		args = append(args, "%"+filter.TeachersContains+"%")
		argN++
	}
	addBlankMode("teachers_raw", filter.TeachersBlankMode)

	return conds, args
}

// CourseReconcileJobHandler returns a handler for reconcile job types.
func CourseReconcileJobHandler(reconcileV2 *ReconcileV2Service, worker *queue.QueueWorker) queue.JobHandler {
	return func(ctx context.Context, job queue.JobRow) error {
		var payload crmtypes.CourseReconcilePayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal payload: %w", err)
		}

		courseID := pgtype.UUID{Bytes: payload.CourseID, Valid: true}
		snapshotID := pgtype.UUID{Bytes: payload.SnapshotID, Valid: true}

		valid, err := reconcileV2.CheckFilterVersion(ctx, courseID, payload.ExpectedFilterVersion)
		if err != nil {
			return fmt.Errorf("check filter version: %w", err)
		}

		if !valid {
			if payload.ReenqueueCount >= 3 {
				return fmt.Errorf("filter version mismatch exceeded re-enqueue limit for course %s", courseID)
			}

			var currentVersion int
			_ = reconcileV2.db.QueryRow(ctx,
				`SELECT crm_filter_version FROM courses WHERE id = $1`,
				courseID,
			).Scan(&currentVersion)

			newPayload := crmtypes.CourseReconcilePayload{
				SnapshotID:            payload.SnapshotID,
				CourseID:              payload.CourseID,
				ExpectedFilterVersion: currentVersion,
				ReenqueueCount:        payload.ReenqueueCount + 1,
			}

			uniqueKey := fmt.Sprintf("%s-%s-%s", job.JobType, payload.SnapshotID.String(), payload.CourseID.String())
			_, err := worker.EnqueueJob(ctx, queue.JobType(job.JobType), newPayload, uniqueKey)
			return err
		}

		var filterJSON []byte
		err = reconcileV2.db.QueryRow(ctx,
			`SELECT crm_filter FROM courses WHERE id = $1`,
			courseID,
		).Scan(&filterJSON)
		if err != nil {
			return fmt.Errorf("get course filter: %w", err)
		}

		var filter crmtypes.CourseFilter
		if len(filterJSON) > 0 {
			if err := json.Unmarshal(filterJSON, &filter); err != nil {
				return fmt.Errorf("unmarshal filter: %w", err)
			}
		}

		switch queue.JobType(job.JobType) {
		case queue.JobTypeCourseReconcileApply:
			result, err := reconcileV2.ApplyCourseReconcile(ctx, snapshotID, courseID, filter)
			if err != nil {
				return fmt.Errorf("apply reconcile: %w", err)
			}
			resultJSON, _ := json.Marshal(result)
			_, _ = reconcileV2.db.Exec(ctx,
				`UPDATE crm_jobs SET result = $1::jsonb WHERE id = $2`,
				string(resultJSON), job.ID,
			)

			_, _ = reconcileV2.db.Exec(ctx, `
				UPDATE courses
				SET crm_pending_review_snapshot_id = NULL,
				    crm_pending_review_summary = NULL,
				    updated_at = now()
				WHERE id = $1
			`, courseID)

		case queue.JobTypeCourseReconcileDiff:
			result, err := reconcileV2.DiffCourseReconcile(ctx, snapshotID, courseID, filter)
			if err != nil {
				return fmt.Errorf("diff reconcile: %w", err)
			}
			resultJSON, _ := json.Marshal(result)
			_, _ = reconcileV2.db.Exec(ctx,
				`UPDATE crm_jobs SET result = $1::jsonb WHERE id = $2`,
				string(resultJSON), job.ID,
			)
		}

		return nil
	}
}
