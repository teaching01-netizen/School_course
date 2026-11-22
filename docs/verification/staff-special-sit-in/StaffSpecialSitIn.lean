namespace StaffSpecialSitIn

inductive Approval where
  | pending
  | specialApproved
  deriving DecidableEq

structure Selection where
  special : Bool
  active : Bool
  targetCourseMatches : Bool
  missedClassClear : Bool
  expectedClassClear : Bool
  bookingClear : Bool
  suggestedEligible : Bool

def specialSafe (s : Selection) : Bool :=
  s.active && s.targetCourseMatches && s.missedClassClear &&
    s.expectedClassClear && s.bookingClear

-- Explicit selection contract after the request parser establishes membership.
-- suggestedEligible includes the normal selected-course and scheduling rules.
def accepts (s : Selection) : Bool :=
  if s.special then specialSafe s else s.bookingClear && s.suggestedEligible

def submit (authorized : Bool) (approval : Approval) (selections : List Selection) :
    Option Approval :=
  if authorized && selections.all accepts then some approval else none

theorem authorized_batch_only (authorized : Bool) (approval committed : Approval)
    (selections : List Selection) (h : submit authorized approval selections = some committed) :
    authorized = true ∧ committed = approval ∧
      ∀ s ∈ selections, accepts s = true := by
  unfold submit at h
  split at h
  next accepted =>
    have conditions := Bool.and_eq_true_iff.mp accepted
    exact ⟨conditions.1, (Option.some.inj h).symm, List.all_eq_true.mp conditions.2⟩
  next => contradiction

theorem special_requires_safety (s : Selection) (mode : s.special = true)
    (accepted : accepts s = true) :
    s.active = true ∧ s.targetCourseMatches = true ∧ s.missedClassClear = true ∧
      s.expectedClassClear = true ∧ s.bookingClear = true := by
  simpa [accepts, mode, specialSafe, Bool.and_eq_true_iff, and_assoc] using accepted

theorem suggested_in_mixed_batch_requires_eligibility
    (authorized : Bool) (approval committed : Approval) (selections : List Selection)
    (h : submit authorized approval selections = some committed)
    (s : Selection) (member : s ∈ selections) (mode : s.special = false) :
    s.bookingClear = true ∧ s.suggestedEligible = true := by
  have accepted := (authorized_batch_only authorized approval committed selections h).2.2 s member
  simpa [accepts, mode, Bool.and_eq_true_iff] using accepted

theorem safe_special_can_override (approval : Approval) (s : Selection)
    (mode : s.special = true) (safe : specialSafe s = true) :
    submit true approval [{s with suggestedEligible := false}] = some approval := by
  cases s
  simp_all [submit, accepts, specialSafe]

#print axioms authorized_batch_only
#print axioms special_requires_safety
#print axioms suggested_in_mixed_batch_requires_eligibility
#print axioms safe_special_can_override

end StaffSpecialSitIn
