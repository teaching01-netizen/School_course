import Std

namespace AbsenceSubmit

structure Slot where
  start : Int
  finish : Int
  deriving DecidableEq, Repr

def Overlap (a b : Slot) : Prop := a.start < b.finish ∧ b.start < a.finish

theorem overlap_symmetric (a b : Slot) : Overlap a b ↔ Overlap b a := by
  simp [Overlap, and_comm]

theorem adjacent_not_overlapping (a b : Slot) (h : a.finish = b.start) : ¬ Overlap a b := by
  simp [Overlap, h]

structure Candidate where
  id : Nat
  course : Nat
  group : Option Nat
  slot : Slot
  eligible : Bool
  deriving DecidableEq, Repr

def inScope (target : Nat) (group : Option Nat) (c : Candidate) : Bool :=
  c.course == target || (group.isSome && c.group == group)

def focus (target : Nat) (group : Option Nat) (rows : List Candidate) : List Candidate :=
  rows.filter (inScope target group)

def Authorized (target : Nat) (group : Option Nat) (rows : List Candidate)
    (selected : List Nat) : Prop :=
  selected ≠ [] ∧ (∃ c ∈ rows, c.course = target) ∧
    ∀ id ∈ selected, ∃ c ∈ rows, inScope target group c = true ∧ c.eligible = true ∧ c.id = id

-- This theorem preserves all quantifiers, target presence, and nonempty selection.
-- Coherent metadata and resolver/dedup correspondence are external obligations.
theorem focus_preserves_authorization (target : Nat) (group : Option Nat)
    (rows : List Candidate) (selected : List Nat) :
    Authorized target group (focus target group rows) selected ↔
      Authorized target group rows selected := by
  constructor
  · rintro ⟨hn, ⟨c, hc, ht⟩, hs⟩
    refine ⟨hn, ⟨c, (List.mem_filter.mp hc).1, ht⟩, ?_⟩
    intro id hid
    obtain ⟨c, hc, hscope, heligible, hId⟩ := hs id hid
    exact ⟨c, (List.mem_filter.mp hc).1, hscope, heligible, hId⟩
  · rintro ⟨hn, ⟨c, hc, ht⟩, hs⟩
    refine ⟨hn, ⟨c, ?_, ht⟩, ?_⟩
    · exact List.mem_filter.mpr ⟨hc, by simp [inScope, ht]⟩
    · intro id hid
      obtain ⟨c, hc, hscope, heligible, hId⟩ := hs id hid
      exact ⟨c, List.mem_filter.mpr ⟨hc, hscope⟩, hscope, heligible, hId⟩

theorem focused_rows_are_in_scope (target : Nat) (group : Option Nat)
    (rows : List Candidate) (c : Candidate) (h : c ∈ focus target group rows) :
    inScope target group c = true := (List.mem_filter.mp h).2

def ExpectedConflict (c b : Candidate) : Prop := c.id ≠ b.id ∧ Overlap c.slot b.slot

instance (c b : Candidate) : Decidable (ExpectedConflict c b) :=
  inferInstanceAs (Decidable (c.id ≠ b.id ∧ (c.slot.start < b.slot.finish ∧ b.slot.start < c.slot.finish)))

def selectedBlockers (selected expected : List Candidate) : List Candidate :=
  expected.filter (fun b => selected.any (fun c => decide (ExpectedConflict c b)))

def PassesExpectedCheck (c : Candidate) (selected expected : List Candidate) : Prop :=
  ∀ b ∈ selectedBlockers selected expected, ¬ Overlap c.slot b.slot

theorem selected_query_includes_every_conflict (selected expected : List Candidate)
    (c b : Candidate) (hc : c ∈ selected) (hb : b ∈ expected)
    (h : ExpectedConflict c b) : b ∈ selectedBlockers selected expected := by
  apply List.mem_filter.mpr
  refine ⟨hb, ?_⟩
  exact List.any_eq_true.mpr ⟨c, hc, by simpa using h⟩

theorem selected_conflict_is_rejected (selected expected : List Candidate)
    (c b : Candidate) (hc : c ∈ selected) (hb : b ∈ expected)
    (h : ExpectedConflict c b) : ¬ PassesExpectedCheck c selected expected := by
  intro accepted
  exact accepted b (selected_query_includes_every_conflict selected expected c b hc hb h) h.2

inductive ValidationError where
  | invalid | deadline | canceled | internal
  deriving DecidableEq, Repr

def publicResponse (e : ValidationError) (_privateDetails : String) : Nat × String :=
  match e with
  | .invalid => (400, "A selected make-up session is no longer eligible. Refresh the form and choose another session.")
  | .deadline => (504, "Checking make-up sessions took too long. Please try again.")
  | .canceled => (503, "The request was interrupted. Please try again.")
  | .internal => (500, "Could not check the selected make-up sessions. Please try again.")

theorem response_independent_of_private_details (e : ValidationError) (a b : String) :
    publicResponse e a = publicResponse e b := by cases e <;> rfl

theorem deadline_is_server_timeout (details : String) :
    (publicResponse .deadline details).1 = 504 := rfl

#print axioms focus_preserves_authorization
#print axioms focused_rows_are_in_scope
#print axioms selected_query_includes_every_conflict
#print axioms selected_conflict_is_rejected
#print axioms adjacent_not_overlapping
#print axioms response_independent_of_private_details
#print axioms deadline_is_server_timeout

end AbsenceSubmit
