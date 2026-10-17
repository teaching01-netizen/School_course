import proof.Model

namespace AbsencePolicyAudit
open AbsenceSitInLegacySync

/-! Counterexample models for the broader policy audit. These statements describe
the inspected predicates, not compiled Go/TypeScript semantics. Runnable code
reproductions establish a separate correspondence check. -/

def conflictWitness : PolicyOptions :=
  { topLevel := [oct13Candidate]
    priorities := [[oct13Candidate]]
    perMissedSession := [[oct13Candidate]]
    nestedPriorities := [[oct13Candidate]] }

-- Display filtering removes an expected class without removing the student's
-- obligation to attend it. The shared filter remains correct for its supplied
-- list, but the real complete-blocker guarantee fails in every output shape.
theorem incompleteBlockerProjectionBreaksEveryPolicyShape :
    policyOptionsAreSafe [] (filterPolicyOptions [] conflictWitness) = true ∧
    policyOptionsAreSafe [oct13ExpectedA]
      (filterPolicyOptions [] conflictWitness) = false := by
  decide

theorem selectedSubjectStillHidesPickerConflict :
    currentFormAllows oct13Candidate "Reading" [("Reading", oct13ExpectedA)] = true ∧
    formPrecheckAllows oct13Candidate [oct13ExpectedA] = false := by
  decide

-- The absence-scope predicate cannot establish the additional global regular
-- class check used by ValidateCandidate at submission, including make-up dates
-- outside the selected absence date window.
theorem absenceWindowScopeIsWeakerThanGlobalAttendance :
    candidatePickerAllows oct13Candidate true [oct10Missed] = true ∧
    serverAllows oct13Candidate true [oct13ExpectedA] = false := by
  decide

structure Booking where
  sessionID : Nat
  courseID : Nat
  slot : Slot
  deriving Repr

def exactIDAvailable (candidate : Booking) (bookings : List Booking) : Bool :=
  !(bookings.any fun existing => existing.sessionID == candidate.sessionID)

def bookingTimeAvailable (candidate : Booking) (bookings : List Booking) : Bool :=
  !(bookings.any fun existing => overlaps candidate.slot existing.slot)

def firstMakeUp : Booking := ⟨1, 2, oct13Candidate⟩
def otherOverlappingMakeUp : Booking := ⟨2, 2, oct13ExpectedA⟩

theorem differentIDsDoNotPreventOverlappingMakeUps :
    exactIDAvailable otherOverlappingMakeUp [firstMakeUp] = true ∧
    bookingTimeAvailable otherOverlappingMakeUp [firstMakeUp] = false := by
  decide

def genericPhysicalAllows (active eligible withinCutoff : Bool)
    (slot : Slot) (missed : List Slot) : Bool :=
  active && eligible && withinCutoff && !(overlapsAny slot missed)

def enabledPhysicalAllows (enabled active eligible withinCutoff : Bool)
    (slot : Slot) (missed : List Slot) : Bool :=
  enabled && genericPhysicalAllows active eligible withinCutoff slot missed

theorem omissionOfEnableGateStillOffersChoice :
    genericPhysicalAllows true true true oct13Candidate [oct10Missed] = true ∧
    enabledPhysicalAllows false true true true oct13Candidate [oct10Missed] = false := by
  decide

theorem disabledPolicyMustOfferNoPhysicalChoice
    (active eligible withinCutoff : Bool) (slot : Slot) (missed : List Slot) :
    enabledPhysicalAllows false active eligible withinCutoff slot missed = false := by
  rfl

def finalDayPolicyAllows (isFinalDay : Bool) (baseDecision : Bool) : Bool :=
  !isFinalDay && baseDecision

theorem omittedFinalDayGateAllowsForbiddenTarget :
    genericPhysicalAllows true true true oct13Candidate [oct10Missed] = true ∧
    finalDayPolicyAllows true
      (genericPhysicalAllows true true true oct13Candidate [oct10Missed]) = false := by
  decide

def targetEligible (allowedCourseIDs : List Nat) (candidate : Booking) : Bool :=
  allowedCourseIDs.contains candidate.courseID

def declaredCourseMatches (declaredCourseID : Nat) (candidate : Booking) : Bool :=
  declaredCourseID == candidate.courseID

def unrelatedTarget : Booking := ⟨3, 999, oct13Candidate⟩

theorem activeNonOverlappingSessionNeedNotBePolicyEligible :
    genericPhysicalAllows true true true unrelatedTarget.slot [oct10Missed] = true ∧
    targetEligible [2] unrelatedTarget = false ∧
    declaredCourseMatches 2 unrelatedTarget = false := by
  decide

-- The preview and runtime have different final-class meanings. Which business
-- meaning is intended is an unresolved requirement, not a proved runtime bug.
theorem missedFinalAndTargetFinalRulesAreDifferent :
    finalDayPolicyAllows true true = false ∧
    finalDayPolicyAllows false true = true := by
  decide

-- Priority numbering may intentionally omit level 2. A cursor compares actual
-- levels, rather than assuming priorityCount equals the number of rows.
def revealAfter (levels : List Nat) (afterLevel : Nat) : List Nat :=
  levels.filter (fun level => level > afterLevel)

theorem gapInRank3PrioritiesStillRevealsLevel3 :
    revealAfter [1, 3] 1 = [3] ∧ revealAfter [1, 3] 3 = [] := by
  decide

structure SchoolCourse where
  id : Nat
  level : Nat
  deriving DecidableEq

def genericCrossSectionTarget (studentLevel : Nat) (courses : List SchoolCourse) :
    Option SchoolCourse :=
  courses.find? fun course => course.level == studentLevel

theorem crossSectionCanSelectMissedCourseItself :
    genericCrossSectionTarget 2 [⟨1, 2⟩, ⟨2, 2⟩] = some ⟨1, 2⟩ := by
  decide

/-! ## Post-fix guard obligations

These predicates state the safety boundary the implementation now enforces.
They remain an abstract model: local Go/TypeScript/PostgreSQL checks are the
separate evidence that the implementation corresponds to these guards. -/

def formAllowsAllExpectedClasses (candidate : Slot) (expected : List Slot) : Bool :=
  !(overlapsAny candidate expected)

theorem formRejectsOverlapRegardlessOfSelectedSubject
    (candidate : Slot) (expected : List Slot)
    (h : overlapsAny candidate expected = true) :
    formAllowsAllExpectedClasses candidate expected = false := by
  unfold formAllowsAllExpectedClasses
  rw [h]
  rfl

def activeAssignmentConflicts (candidate existing : Booking) : Bool :=
  (candidate.sessionID == existing.sessionID) || overlaps candidate.slot existing.slot

def blockedByActiveAssignment (candidate : Booking) (existing : List Booking) : Bool :=
  existing.any (activeAssignmentConflicts candidate)

theorem activeOverlappingAssignmentIsRejected :
    blockedByActiveAssignment otherOverlappingMakeUp [firstMakeUp] = true := by
  decide

def mergedComponentsMayOverlapInOneAbsence
    (scopeA scopeB sessionA sessionB : Nat)
    (mergeA mergeB : Option Nat) (slotA slotB : Slot) : Bool :=
  overlaps slotA slotB &&
    !(scopeA == scopeB && sessionA != sessionB && mergeA.isSome && mergeA == mergeB)

theorem overlappingMergedComponentsInOneAbsenceAreOneChoice :
    mergedComponentsMayOverlapInOneAbsence 1 1 10 11 (some 4) (some 4)
      ⟨780, 840⟩ ⟨810, 870⟩ = false := by
  decide

theorem sameMergedGroupDoesNotHideOverlapAcrossAbsences :
    mergedComponentsMayOverlapInOneAbsence 1 2 10 11 (some 4) (some 4)
      ⟨780, 840⟩ ⟨810, 870⟩ = true := by
  decide

def policyTargetAllowsSubmission
    (allowedTargets : List Nat) (allowedTargetSessions : List (Nat × Nat))
    (declaredTarget candidateSession : Nat) : Bool :=
  allowedTargets.contains declaredTarget &&
    allowedTargetSessions.contains (declaredTarget, candidateSession)

theorem submittedSessionMustBelongToDeclaredAllowedTarget :
    policyTargetAllowsSubmission [2, 3] [(2, 20), (3, 30)] 2 30 = false ∧
    policyTargetAllowsSubmission [2, 3] [(2, 20), (3, 30)] 3 30 = true := by
  decide

def automaticEnabledOrDefault (setting : Option Bool) : Bool := setting.getD true

def automaticPolicyEnabled
    (global subject root merge : Option Bool) : Bool :=
  automaticEnabledOrDefault global && automaticEnabledOrDefault subject &&
    automaticEnabledOrDefault root && automaticEnabledOrDefault merge

theorem scopedDisableVetoesAutomaticResolution :
    automaticPolicyEnabled (some true) (some false) (some true) (some true) = false ∧
    automaticPolicyEnabled (some true) (some true) (some false) (some true) = false ∧
    automaticPolicyEnabled (some true) (some true) (some true) (some false) = false ∧
    automaticPolicyEnabled none none none none = true := by
  decide

def genericFinalTargetAllows (lastClassExcluded isFinalTarget baseAllows : Bool) : Bool :=
  baseAllows && !(lastClassExcluded && isFinalTarget)

theorem genericFinalDayPredicateBlocksEveryFinalTarget :
    genericFinalTargetAllows true true true = false ∧
    genericFinalTargetAllows true false true = true ∧
    genericFinalTargetAllows false true true = true := by
  decide

def genericCrossSectionTargetExcluding
    (studentLevel missedCourseID : Nat) (courses : List SchoolCourse) :
    Option SchoolCourse :=
  courses.find? fun course => course.id != missedCourseID && course.level == studentLevel

theorem crossSectionChoosesEligibleSiblingAfterExcludingMissedCourse :
    genericCrossSectionTargetExcluding 2 1 [⟨1, 2⟩, ⟨2, 2⟩, ⟨3, 1⟩] = some ⟨2, 2⟩ := by
  decide

theorem allPolicyShapesRemainSafeWithCompleteExpectedBlockers :
    policyOptionsAreSafe [oct13ExpectedA, oct13ExpectedB]
      (filterPolicyOptions [oct13ExpectedA, oct13ExpectedB] policyPriorityOverlapWitness) = true := by
  exact sharedFilterMakesEveryPolicyAndPriorityChoiceSafe _ _

#print incompleteBlockerProjectionBreaksEveryPolicyShape
#print differentIDsDoNotPreventOverlappingMakeUps
#print activeNonOverlappingSessionNeedNotBePolicyEligible
#print axioms incompleteBlockerProjectionBreaksEveryPolicyShape
#print axioms selectedSubjectStillHidesPickerConflict
#print axioms absenceWindowScopeIsWeakerThanGlobalAttendance
#print axioms differentIDsDoNotPreventOverlappingMakeUps
#print axioms omissionOfEnableGateStillOffersChoice
#print axioms disabledPolicyMustOfferNoPhysicalChoice
#print axioms omittedFinalDayGateAllowsForbiddenTarget
#print axioms activeNonOverlappingSessionNeedNotBePolicyEligible
#print axioms missedFinalAndTargetFinalRulesAreDifferent
#print axioms gapInRank3PrioritiesStillRevealsLevel3
#print axioms crossSectionCanSelectMissedCourseItself
#print axioms formRejectsOverlapRegardlessOfSelectedSubject
#print axioms activeOverlappingAssignmentIsRejected
#print axioms overlappingMergedComponentsInOneAbsenceAreOneChoice
#print axioms sameMergedGroupDoesNotHideOverlapAcrossAbsences
#print axioms submittedSessionMustBelongToDeclaredAllowedTarget
#print axioms scopedDisableVetoesAutomaticResolution
#print axioms genericFinalDayPredicateBlocksEveryFinalTarget
#print axioms crossSectionChoosesEligibleSiblingAfterExcludingMissedCourse
#print axioms allPolicyShapesRemainSafeWithCompleteExpectedBlockers

end AbsencePolicyAudit
