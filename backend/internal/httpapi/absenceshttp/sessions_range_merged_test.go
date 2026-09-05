package absenceshttp

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func mergedTestFact(start, end time.Time, group pgtype.UUID) sessionFact {
	id := uuid.New()
	return sessionFact{
		row: sqldb.SessionsRangeFactRow{
			SessionID:    pgtype.UUID{Bytes: id, Valid: true},
			MergeGroupID: group,
		},
		id:      id.String(),
		startAt: start,
		endAt:   end,
	}
}

func TestMergedRangesFromSiblings_BucketsBySourceDay(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	group := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	mon9 := time.Date(2026, 9, 7, 9, 0, 0, 0, loc)
	mon14 := time.Date(2026, 9, 7, 14, 0, 0, 0, loc)
	tue10 := time.Date(2026, 9, 8, 10, 0, 0, 0, loc)
	facts := []sessionFact{
		mergedTestFact(mon9, mon9.Add(time.Hour), group),
		mergedTestFact(mon14, mon14.Add(time.Hour), group),
		mergedTestFact(tue10, tue10.Add(time.Hour), group),
		mergedTestFact(mon9, mon9.Add(time.Hour), pgtype.UUID{}),
	}
	got := mergedRangesFromSiblings(facts, nil, "Asia/Bangkok")
	if len(got) != 3 {
		t.Fatalf("expected 3 merged entries (plain session excluded), got %d", len(got))
	}
	wantMon := [2]string{
		mon9.UTC().Format(time.RFC3339Nano),
		mon14.Add(time.Hour).UTC().Format(time.RFC3339Nano),
	}
	for _, f := range facts[:2] {
		if got[f.id] != wantMon {
			t.Fatalf("monday session %s got %v want %v", f.id, got[f.id], wantMon)
		}
	}
	wantTue := [2]string{
		tue10.UTC().Format(time.RFC3339Nano),
		tue10.Add(time.Hour).UTC().Format(time.RFC3339Nano),
	}
	if got[facts[2].id] != wantTue {
		t.Fatalf("tuesday session got %v want %v", got[facts[2].id], wantTue)
	}
}

func TestMergedRangesFromSiblings_ExtraSiblingsWidenRange(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	group := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	mon9 := time.Date(2026, 9, 7, 9, 0, 0, 0, loc)
	mon16 := time.Date(2026, 9, 7, 16, 0, 0, 0, loc)
	facts := []sessionFact{mergedTestFact(mon9, mon9.Add(time.Hour), group)}
	extra := []sessionFact{mergedTestFact(mon16, mon16.Add(2*time.Hour), group)}
	got := mergedRangesFromSiblings(facts, extra, "Asia/Bangkok")
	want := [2]string{
		mon9.UTC().Format(time.RFC3339Nano),
		mon16.Add(2 * time.Hour).UTC().Format(time.RFC3339Nano),
	}
	if got[facts[0].id] != want {
		t.Fatalf("got %v want %v", got[facts[0].id], want)
	}
}

func TestMergedRangesFromSiblings_DeterministicAcrossRuns(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	group := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	var facts []sessionFact
	for d := 0; d < 3; d++ {
		for h := 0; h < 5; h++ {
			start := time.Date(2026, 9, 7+d, 8+h, 0, 0, 0, loc)
			facts = append(facts, mergedTestFact(start, start.Add(time.Hour), group))
		}
	}
	first := mergedRangesFromSiblings(facts, nil, "Asia/Bangkok")
	for i := 0; i < 50; i++ {
		again := mergedRangesFromSiblings(facts, nil, "Asia/Bangkok")
		if len(again) != len(first) {
			t.Fatalf("iteration %d: got %d entries, want %d", i, len(again), len(first))
		}
		for id, want := range first {
			if again[id] != want {
				t.Fatalf("iteration %d: session %s got %v want %v", i, id, again[id], want)
			}
		}
	}
}
