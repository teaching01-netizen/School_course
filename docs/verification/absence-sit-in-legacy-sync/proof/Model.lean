import Std

namespace AbsenceSitInLegacySync

/-! ## Sit-in overlap model

Times are minutes on a single institute-local timeline. Intervals are
half-open, matching the SQL predicates `candidate.start < blocker.end` and
`candidate.end > blocker.start`. -/

structure Slot where
  startMinute : Nat
  endMinute : Nat
  deriving DecidableEq, Repr

def overlaps (a b : Slot) : Bool :=
  (a.startMinute < b.endMinute) && (b.startMinute < a.endMinute)

def overlapsAny (candidate : Slot) (slots : List Slot) : Bool :=
  slots.any (overlaps candidate)

-- Abstracts current candidate generation: it excludes overlap with the
-- missed-session list supplied to the resolver, but that list need not include
-- every expected session in the server's merge-group conflict scope.
def currentPickerAllows
    (candidate : Slot) (active : Bool) (missedSessions : List Slot) : Bool :=
  active && !(overlapsAny candidate missedSessions)

-- Mirrors collectAttendingSessions: classes whose subject is selected for the
-- absence are skipped before this conflict check.
def currentFormAllows
    (candidate : Slot) (selectedSubject : String)
    (attending : List (String × Slot)) : Bool :=
  !(attending.any fun item => item.1 != selectedSubject && overlaps candidate item.2)

def currentFlowAllows
    (candidate : Slot) (active : Bool) (selectedSubject : String)
    (missedSessions : List Slot) (attending : List (String × Slot)) : Bool :=
  currentPickerAllows candidate active missedSessions &&
    currentFormAllows candidate selectedSubject attending

-- Mirrors the final SQL decision once the exact expected course/merge-group
-- sessions inside the absence date window have been selected.
def serverAllows
    (candidate : Slot) (active : Bool) (expectedInScope : List Slot) : Bool :=
  active && !(overlapsAny candidate expectedInScope)

def candidatePickerAllows (candidate : Slot) (active : Bool)
    (expectedInScope : List Slot) : Bool :=
  active && !(overlapsAny candidate expectedInScope)

def formPrecheckAllows (candidate : Slot) (expectedInScope : List Slot) : Bool :=
  !(overlapsAny candidate expectedInScope)

def oct10Missed : Slot := ⟨780, 980⟩
def oct17Missed : Slot := ⟨7 * 1440 + 780, 7 * 1440 + 980⟩
def oct13Candidate : Slot := ⟨3 * 1440 + 780, 3 * 1440 + 980⟩
def oct13ExpectedA : Slot := ⟨3 * 1440 + 780, 3 * 1440 + 880⟩
def oct13ExpectedB : Slot := ⟨3 * 1440 + 880, 3 * 1440 + 980⟩

def oct13CurrentFlow : Bool :=
  currentFlowAllows oct13Candidate true "SAT-Verbal"
    [oct10Missed, oct17Missed]
    [("SAT-Verbal", oct13ExpectedA), ("SAT-Verbal", oct13ExpectedB)]

def oct13ServerDecision : Bool :=
  serverAllows oct13Candidate true
    [oct10Missed, oct17Missed, oct13ExpectedA, oct13ExpectedB]

theorem currentPickerAndFormMissTheConflict :
    oct13CurrentFlow = true ∧ oct13ServerDecision = false := by
  decide

theorem sharedPolicyRejectsAnyScopedOverlap
    (candidate : Slot) (active : Bool) (expectedInScope : List Slot)
    (h : overlapsAny candidate expectedInScope = true) :
    serverAllows candidate active expectedInScope = false := by
  unfold serverAllows
  rw [h]
  cases active <;> rfl

theorem candidatePickerRejectsAnyScopedOverlap
    (candidate : Slot) (active : Bool) (expectedInScope : List Slot)
    (h : overlapsAny candidate expectedInScope = true) :
    candidatePickerAllows candidate active expectedInScope = false := by
  unfold candidatePickerAllows
  rw [h]
  cases active <;> rfl

theorem formPrecheckRejectsAnyScopedOverlap
    (candidate : Slot) (expectedInScope : List Slot)
    (h : overlapsAny candidate expectedInScope = true) :
    formPrecheckAllows candidate expectedInScope = false := by
  unfold formPrecheckAllows
  rw [h]
  rfl

/-! ## Legacy observation/deactivation model

The current set-difference rule deletes whenever an ID is absent from the
single incoming aggregate. The proposed model applies only newer, authenticated,
parser-valid, complete observations; one miss is provisional; two misses plus
the approved grace period may tombstone. A present row restores local state.
The generation token is minted before fetch so completion order cannot reorder
source observations. -/

structure SyncState where
  lastGeneration : Nat
  consecutiveMissing : Nat
  tombstoned : Bool
  deriving DecidableEq, Repr

structure Observation where
  generation : Nat
  transportOK : Bool
  authOK : Bool
  parserOK : Bool
  complete : Bool
  sourceContainsSchedule : Bool
  graceElapsed : Bool
  deriving DecidableEq, Repr

def observationUsable (o : Observation) : Bool :=
  o.transportOK && o.authOK && o.parserOK && o.complete

def observe (state : SyncState) (o : Observation) : SyncState :=
  if o.generation <= state.lastGeneration then
    state
  else if !observationUsable o then
    state
  else if o.sourceContainsSchedule then
    { lastGeneration := o.generation, consecutiveMissing := 0, tombstoned := false }
  else
    let misses := state.consecutiveMissing + 1
    { lastGeneration := o.generation
      consecutiveMissing := misses
      tombstoned := state.tombstoned || (misses >= 2 && o.graceElapsed) }

def initiallyActive : SyncState :=
  { lastGeneration := 0, consecutiveMissing := 0, tombstoned := false }

def missingGraceHours : Nat := 24

def fullMissing (generation : Nat) (graceElapsed : Bool := true) : Observation :=
  { generation := generation
    transportOK := true
    authOK := true
    parserOK := true
    complete := true
    sourceContainsSchedule := false
    graceElapsed := graceElapsed }

def fullPresent (generation : Nat) : Observation :=
  { generation := generation
    transportOK := true
    authOK := true
    parserOK := true
    complete := true
    sourceContainsSchedule := true
    graceElapsed := false }

def currentSetDifferenceDeletes (incomingContainsSchedule : Bool) : Bool :=
  !incomingContainsSchedule

theorem oneOmissionCanDeleteInCurrentRule :
    currentSetDifferenceDeletes false = true := by
  decide

theorem oneCompleteMissingObservationDoesNotTombstone :
    (observe initiallyActive (fullMissing 1)).tombstoned = false := by
  decide

theorem duplicateGenerationDoesNotAdvanceMissingCount :
    observe (observe initiallyActive (fullMissing 1)) (fullMissing 1) =
      observe initiallyActive (fullMissing 1) := by
  decide

theorem incompleteObservationDoesNotAdvanceState :
    observe initiallyActive
      { generation := 1
        transportOK := true
        authOK := true
        parserOK := true
        complete := false
        sourceContainsSchedule := false
        graceElapsed := true } = initiallyActive := by
  decide

theorem twoMissingObservationsAfterGraceMayTombstone :
    (observe (observe initiallyActive (fullMissing 1)) (fullMissing 2)).tombstoned = true := by
  decide

theorem twoMissingObservationsBeforeGraceDoNotTombstone :
    (observe (observe initiallyActive (fullMissing 1 false)) (fullMissing 2 false)).tombstoned = false := by
  decide

theorem newerPresentObservationClearsMissingAndTombstone :
    observe
      { lastGeneration := 4, consecutiveMissing := 2, tombstoned := true }
      (fullPresent 5) =
      { lastGeneration := 5, consecutiveMissing := 0, tombstoned := false } := by
  decide

theorem staleMissingObservationCannotUndoNewerPresent :
    observe (observe initiallyActive (fullPresent 2)) (fullMissing 1) =
      observe initiallyActive (fullPresent 2) := by
  decide

#eval oct13CurrentFlow
#eval oct13ServerDecision
#eval currentSetDifferenceDeletes false
#eval (observe (observe initiallyActive (fullMissing 1)) (fullMissing 2)).tombstoned

#print axioms currentPickerAndFormMissTheConflict
#print axioms sharedPolicyRejectsAnyScopedOverlap
#print axioms candidatePickerRejectsAnyScopedOverlap
#print axioms formPrecheckRejectsAnyScopedOverlap
#print axioms oneOmissionCanDeleteInCurrentRule
#print axioms oneCompleteMissingObservationDoesNotTombstone
#print axioms duplicateGenerationDoesNotAdvanceMissingCount
#print axioms incompleteObservationDoesNotAdvanceState
#print axioms twoMissingObservationsAfterGraceMayTombstone
#print axioms twoMissingObservationsBeforeGraceDoNotTombstone
#print axioms newerPresentObservationClearsMissingAndTombstone
#print axioms staleMissingObservationCannotUndoNewerPresent

end AbsenceSitInLegacySync
