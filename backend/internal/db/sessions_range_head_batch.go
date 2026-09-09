package db

// Step-16 facts+student batch: the two post-validation head reads share
// ONE round trip.
//
// After prelim validation + settings (range cap) + lookup finalize, the
// head of serveSessionsRangeV2 issues two independent reads before any
// dependent work: the window facts (display sessions) and the student row
// (bundle + tail-batch identity). Neither reads the other's output, so
// they share ONE trip via pgx.Batch with narrow shapes kept (no UNION ALL
// padding across heterogeneous widths).
//
// Why settings is NOT in this batch: finalizeSessionsRangeLookup needs
// settings for the range cap (maxRangeDaysForLookup), and the legacy
// order (param validation -> admin gate -> settings -> range cap ->
// filters) is pinned by error-parity cases (admin_over_cap,
// lifetime_range return 400 before any session query runs). Moving the
// settings read below finalize would run facts+student work on requests
// the cap rejects. So settings stays standalone (1 trip) and this batch
// fuses facts+student (1 trip): head goes 3 trips -> 2, endpoint 6+3 ->
// 5+4 with identical statements and identical failure semantics.
//
// Queue order is facts, student; the drain scans in that order (pgx
// poison semantics: each statement fully scanned before the next is
// touched). Per-arm failure mapping preserves standalone semantics:
//   facts: request-fatal (caller runs ClassifyDBErr -> 500; the
//     normalizeSessionFacts length-mismatch 500 stays downstream).
//   student: DEGRADE, never fatal. A missing student (pgx.ErrNoRows on
//     the scan) yields studentMissing=true with a zero row, mirroring
//     `student, studentErr := ...; studentMissing := studentErr != nil`;
//     a transport/scan error also degrades (the standalone call treats
//     ANY error as missing: log + ResolveFailed bundle skip, 200).
// The all-subjects path uses the same batch with
// Mode=SessionsRangeFactsAllSubjects; its post-batch missing-student rule
// (500 iff len(facts) > 0) stays in the service, unchanged.
import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// sessionsRangeStudentSQLText is the StudentGetByWCode SELECT text
// (students.sql.go is sqlc-generated: DO NOT EDIT it). Copied verbatim
// with a comment so the head batch queues identical SQL.
const sessionsRangeStudentSQLText = `
SELECT id, wcode, full_name, notes, nickname, email, student_phone, parent_phone, email_crm, email_system, school, level, year, created_at, updated_at
FROM students
WHERE lower(wcode) = lower($1)
`

// SessionsRangeFactsStudentBatchOut is the drained head-batch result.
type SessionsRangeFactsStudentBatchOut struct {
	Facts          []SessionsRangeFactRow
	Student        StudentGetByWCodeRow
	StudentMissing bool
}

// SessionsRangeFactsStudentBatch loads facts + student in ONE trip.
// factsArg carries the ALREADY-BUILT SessionsRangeFactsParams for the
// request mode (Wcode+window+TZ for enrolled modes; SubjectIDs+window
// for all-subjects). wcode is the student lookup key (same as
// factsArg.Wcode in enrolled modes; lookup.studentWCode() in all-subjects,
// where facts carry no wcode).
func (q *Queries) SessionsRangeFactsStudentBatch(ctx context.Context, factsArg SessionsRangeFactsParams, wcode string) (*SessionsRangeFactsStudentBatchOut, error) {

	out := &SessionsRangeFactsStudentBatchOut{}
	var b pgx.Batch
	switch factsArg.Mode {
	case SessionsRangeFactsStudent:
		tz := factsArg.InstituteTZ
		if tz == "" {
			tz = "Asia/Bangkok"
		}
		b.Queue(fmt.Sprintf(sessionsRangeFactsEnrolledSQLText, sessionsRangeExpectationGate(factsArg.Lifetime), " AND c.absence_form_visible"+
			" AND EXISTS (SELECT 1 FROM subject_active_courses sac"+
			" WHERE sac.subject_id = sub.id AND sac.course_id = c.id)"), factsArg.Wcode, factsArg.FromUTC, factsArg.ToExclusiveUTC, tz)
	case SessionsRangeFactsAllSubjects:
		b.Queue(sessionsRangeFactsAllSubjectsSQLText, strings.Join(factsArg.SubjectIDs, ","), factsArg.FromUTC, factsArg.ToExclusiveUTC)
	default:
		tz := factsArg.InstituteTZ
		if tz == "" {
			tz = "Asia/Bangkok"
		}
		b.Queue(fmt.Sprintf(sessionsRangeFactsEnrolledSQLText, sessionsRangeExpectationGate(factsArg.Lifetime), ""), factsArg.Wcode, factsArg.FromUTC, factsArg.ToExclusiveUTC, tz)
	}
	b.Queue(sessionsRangeStudentSQLText, wcode)
	br := q.db.(interface {
		SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
	}).SendBatch(ctx, &b)
	defer br.Close()
	// Arm 1: facts (request-fatal on any failure).
	factRows, err := br.Query()
	if err != nil {
		return nil, err
	}
	facts, err := scanSessionsRangeFactRows(factRows)
	factRows.Close()
	if err != nil {
		return nil, err
	}
	out.Facts = facts
	// Arm 2: student (degrade, never fatal). Singleton-row drain via
	// Query()+Next(): zero rows (unknown wcode) yields StudentMissing
	// with a zero row — never ErrNoRows, never fatal. A transport
	// failure on br.Query() ALSO degrades: the standalone call maps ANY
	// student error to missing (log + ResolveFailed bundle skip, 200),
	// and the downstream tail batch re-probes fatally where legacy
	// requires it (all-subjects 500 iff len(facts) > 0).
	studentRows, serr := br.Query()
	if serr != nil {
		out.StudentMissing = true
	} else {
		var st StudentGetByWCodeRow
		if studentRows.Next() {
			if serr := studentRows.Scan(&st.ID, &st.Wcode, &st.FullName, &st.Notes, &st.Nickname, &st.Email, &st.StudentPhone, &st.ParentPhone, &st.EmailCrm, &st.EmailSystem, &st.School, &st.Level, &st.Year, &st.CreatedAt, &st.UpdatedAt); serr != nil {
				studentRows.Close()
				out.StudentMissing = true
				if cerr := br.Close(); cerr != nil {
					return nil, cerr
				}
				return out, nil
			}
			out.Student = st
		} else {
			out.StudentMissing = true
		}
		studentRows.Close()
		if serr := studentRows.Err(); serr != nil {
			out.StudentMissing = true
		}
	}
	if err := br.Close(); err != nil {
		if out.StudentMissing {
			return out, nil
		}
		return nil, err
	}
	return out, nil
}
