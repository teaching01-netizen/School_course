package reconcile

import "time"

type State string

const (
	Active           State = "active"
	SuspectedMissing State = "suspected_missing"
	ConfirmedMissing State = "confirmed_missing"
	Tombstoned       State = "tombstoned"
	Restored         State = "restored"
	Conflict         State = "conflict"
	ParserError      State = "parser_error"
)

type Generation struct {
	Complete              bool
	ArchiveFiltersCovered bool
	ParserOK              bool
	AuthOK                bool
}

type Record struct {
	State              State
	MissingSince       time.Time
	MissingGenerations int
}

type Decision struct {
	State              State
	MissingSince       time.Time
	MissingGenerations int
	Tombstone          bool
}

func Observe(record Record, generation Generation, seen bool, now time.Time, gracePeriod time.Duration) Decision {
	decision := Decision{State: record.State, MissingSince: record.MissingSince, MissingGenerations: record.MissingGenerations}
	if !generation.AuthOK {
		return decision
	}
	if !generation.ParserOK {
		decision.State = ParserError
		return decision
	}
	if !generation.Complete || !generation.ArchiveFiltersCovered {
		return decision
	}
	if seen {
		if record.State == Tombstoned {
			decision.State = Restored
		} else {
			decision.State = Active
		}
		decision.MissingSince = time.Time{}
		decision.MissingGenerations = 0
		return decision
	}
	if decision.MissingSince.IsZero() {
		decision.MissingSince = now
	}
	decision.MissingGenerations++
	switch {
	case decision.MissingGenerations == 1:
		decision.State = SuspectedMissing
	case decision.MissingGenerations == 2:
		decision.State = ConfirmedMissing
	case decision.State != Tombstoned:
		decision.State = ConfirmedMissing
	}
	if decision.State == ConfirmedMissing && !decision.MissingSince.After(now.Add(-gracePeriod)) {
		decision.State = Tombstoned
		decision.Tombstone = true
	}
	return decision
}
