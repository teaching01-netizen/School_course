package absenceshttp

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func makeUUID(s string) pgtype.UUID {
	raw := make([]byte, 0, 16)
	j := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			continue
		}
		var b byte
		if s[i] >= 'a' {
			b = s[i] - 'a' + 10
		} else if s[i] >= 'A' {
			b = s[i] - 'A' + 10
		} else {
			b = s[i] - '0'
		}
		if j%2 == 0 {
			raw = append(raw, b<<4)
		} else {
			raw[len(raw)-1] |= b
		}
		j++
	}
	var u pgtype.UUID
	u.Valid = true
	copy(u.Bytes[:], raw[:16])
	return u
}

func makeTS(s string) pgtype.Timestamptz {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func course(id string, level int16) sqldb.SubjectCourseV2 {
	return sqldb.SubjectCourseV2{
		ID:    makeUUID(id),
		Code:  "C-" + id[:8],
		Name:  "Course " + id[:8],
		Level: pgtype.Int2{Int16: level, Valid: true},
	}
}

func session(id, cid, start, end string) sqldb.SessionInRange {
	return sqldb.SessionInRange{
		ID:       makeUUID(id),
		CourseID: makeUUID(cid),
		StartAt:  makeTS(start),
		EndAt:    makeTS(end),
	}
}

func TestResolveV2_TimesOverlap(t *testing.T) {
	ts := func(s string) pgtype.Timestamptz {
		return makeTS(s)
	}

	t.Run("overlap_a_contains_b", func(t *testing.T) {
		if !timesOverlap(ts("2025-01-10T09:00:00Z"), ts("2025-01-10T11:00:00Z"), ts("2025-01-10T09:30:00Z"), ts("2025-01-10T10:30:00Z")) {
			t.Error("expected overlap when a contains b")
		}
	})

	t.Run("overlap_b_contains_a", func(t *testing.T) {
		if !timesOverlap(ts("2025-01-10T09:30:00Z"), ts("2025-01-10T10:30:00Z"), ts("2025-01-10T09:00:00Z"), ts("2025-01-10T11:00:00Z")) {
			t.Error("expected overlap when b contains a")
		}
	})

	t.Run("no_overlap_a_before_b", func(t *testing.T) {
		if timesOverlap(ts("2025-01-10T09:00:00Z"), ts("2025-01-10T10:00:00Z"), ts("2025-01-10T10:00:00Z"), ts("2025-01-10T11:00:00Z")) {
			t.Error("expected no overlap when a ends exactly when b starts")
		}
	})

	t.Run("no_overlap_b_before_a", func(t *testing.T) {
		if timesOverlap(ts("2025-01-10T10:00:00Z"), ts("2025-01-10T11:00:00Z"), ts("2025-01-10T09:00:00Z"), ts("2025-01-10T10:00:00Z")) {
			t.Error("expected no overlap when b ends exactly when a starts")
		}
	})

	t.Run("no_overlap_a_entirely_before_b", func(t *testing.T) {
		if timesOverlap(ts("2025-01-10T09:00:00Z"), ts("2025-01-10T10:00:00Z"), ts("2025-01-10T11:00:00Z"), ts("2025-01-10T12:00:00Z")) {
			t.Error("expected no overlap when a is entirely before b")
		}
	})

	t.Run("invalid_timestamps_no_overlap", func(t *testing.T) {
		invalid := pgtype.Timestamptz{Valid: false}
		if timesOverlap(invalid, invalid, invalid, invalid) {
			t.Error("expected no overlap with invalid timestamps")
		}
	})
}
