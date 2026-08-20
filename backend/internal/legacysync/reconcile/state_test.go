package reconcile

import (
	"testing"
	"time"
)

func completeGeneration() Generation {
	return Generation{Complete: true, ArchiveFiltersCovered: true, ParserOK: true, AuthOK: true}
}

func TestObserveRequiresTwoCompleteGenerations(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	record := Record{State: Active}
	first := Observe(record, completeGeneration(), false, now, time.Hour)
	if first.State != SuspectedMissing || first.Tombstone {
		t.Fatalf("first missing generation = %+v", first)
	}
	second := Observe(Record{State: first.State, MissingSince: first.MissingSince, MissingGenerations: first.MissingGenerations}, completeGeneration(), false, now.Add(time.Minute), time.Hour)
	if second.State != ConfirmedMissing || second.Tombstone {
		t.Fatalf("second missing generation = %+v", second)
	}
}

func TestObserveTombstonesAfterGracePeriod(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	confirmed := Record{State: ConfirmedMissing, MissingSince: now.Add(-2 * time.Hour), MissingGenerations: 2}
	decision := Observe(confirmed, completeGeneration(), false, now, time.Hour)
	if decision.State != Tombstoned || !decision.Tombstone {
		t.Fatalf("expired missing record = %+v", decision)
	}
}

func TestObservePartialGenerationDoesNotDelete(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	decision := Observe(Record{State: Active}, Generation{ParserOK: true, AuthOK: true}, false, now, 0)
	if decision.State != Active || decision.Tombstone {
		t.Fatalf("partial generation = %+v", decision)
	}
	parserFailure := Observe(Record{State: Active}, Generation{Complete: true, ArchiveFiltersCovered: true, AuthOK: true}, false, now, 0)
	if parserFailure.State != ParserError || parserFailure.Tombstone {
		t.Fatalf("parser failure = %+v", parserFailure)
	}
}

func TestObserveRestoresTombstonedRecord(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	decision := Observe(Record{State: Tombstoned, MissingSince: now.Add(-time.Hour), MissingGenerations: 3}, completeGeneration(), true, now, time.Hour)
	if decision.State != Restored || !decision.MissingSince.IsZero() || decision.MissingGenerations != 0 {
		t.Fatalf("restored record = %+v", decision)
	}
}
func TestObserveAuthenticationFailurePreservesLastGoodState(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	record := Record{State: Active, MissingSince: now.Add(-time.Hour), MissingGenerations: 3}
	decision := Observe(record, Generation{Complete: true, ArchiveFiltersCovered: true, ParserOK: false, AuthOK: false}, false, now, time.Minute)
	if decision != (Decision{State: Active, MissingSince: record.MissingSince, MissingGenerations: record.MissingGenerations}) {
		t.Fatalf("authentication failure decision = %+v, want unchanged active state", decision)
	}
}
func TestObserveSeenRecordClearsMissingProgressBeforeTombstone(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	first := Observe(Record{State: Active}, completeGeneration(), false, now, time.Hour)
	restored := Observe(Record{State: first.State, MissingSince: first.MissingSince, MissingGenerations: first.MissingGenerations}, completeGeneration(), true, now.Add(time.Minute), time.Hour)
	if restored.State != Active || !restored.MissingSince.IsZero() || restored.MissingGenerations != 0 || restored.Tombstone {
		t.Fatalf("reappeared record = %+v, want active with cleared missing progress", restored)
	}
}

func TestObserveGraceBoundaryTombstonesOnlyAfterElapsedGrace(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	confirmed := Record{State: ConfirmedMissing, MissingSince: now.Add(-time.Hour), MissingGenerations: 2}
	beforeGrace := Observe(confirmed, completeGeneration(), false, now.Add(-time.Nanosecond), time.Hour)
	if beforeGrace.State == Tombstoned || beforeGrace.Tombstone {
		t.Fatalf("before grace decision = %+v, want confirmed missing", beforeGrace)
	}
	atGrace := Observe(confirmed, completeGeneration(), false, now, time.Hour)
	if atGrace.State != Tombstoned || !atGrace.Tombstone {
		t.Fatalf("at grace decision = %+v, want tombstoned", atGrace)
	}
}
