import «proof».Batch
import «proof».Policy

namespace Booking

abbrev Key := Nat × Nat × Nat
abbrev Catalog := Row → Bool

structure Batch where
  fingerprint : Nat
  rows : List Row
  deriving DecidableEq, Repr

def Valid (catalog : Catalog) (b : Batch) : Prop :=
  b.rows ≠ [] ∧ ∀ item ∈ b.rows, catalog item = true

structure Attempt where
  id : Nat
  key : Key
  batch : Batch
  validationFacts : Catalog

structure State where
  catalog : Catalog
  staged : List Attempt
  journal : Key → Option Attempt

def init (facts : Catalog) : State := ⟨facts, [], fun _ => none⟩

def forget (s : State) (id : Nat) : State :=
  { s with staged := s.staged.filter (fun p => p.id != id) }

def publish (s : State) (p : Attempt) : State :=
  { forget s p.id with journal := fun k => if k = p.key then some p else s.journal k }

inductive Event where
  | offer (r : Row)
  | edit
  | stage (attemptId : Nat)
  | reject (b : Batch)
  | abort (attemptId : Nat)
  | commit (k : Key)
  | lostAck (k : Key)
  | ack (k : Key) (fingerprint : Nat) (response : Batch)
  | rejectReplay (k : Key) (fingerprint : Nat)

inductive Step : State → Event → State → Prop where
  | offer (s : State) (r : Row) (eligible : s.catalog r = true) :
      Step s (.offer r) s
  | edit (s : State) (facts : Catalog) :
      Step s .edit { s with catalog := facts }
  | stage (s : State) (id : Nat) (key : Key) (b : Batch)
      (fresh : ∀ p ∈ s.staged, p.id ≠ id) (valid : Valid s.catalog b) :
      Step s (.stage id) { s with staged := ⟨id, key, b, s.catalog⟩ :: s.staged }
  | reject (s : State) (b : Batch) (invalid : ¬ Valid s.catalog b) :
      Step s (.reject b) s
  | abort (s : State) (id : Nat) :
      Step s (.abort id) (forget s id)
  | commit (s : State) (p : Attempt) (prepared : p ∈ s.staged) (fresh : s.journal p.key = none) :
      Step s (.commit p.key) (publish s p)
  | lostAck (s : State) (k : Key) :
      Step s (.lostAck k) s
  | ack (s : State) (k : Key) (p : Attempt) (committed : s.journal k = some p) :
      Step s (.ack k p.batch.fingerprint p.batch) s
  | rejectReplay (s : State) (k : Key) (p : Attempt) (fingerprint : Nat)
      (committed : s.journal k = some p) (mismatch : fingerprint ≠ p.batch.fingerprint) :
      Step s (.rejectReplay k fingerprint) s

inductive Evolves : State → State → Prop where
  | refl (s : State) : Evolves s s
  | next {a b c : State} {event : Event} : Evolves a b → Step b event c → Evolves a c

def Invariant (s : State) : Prop :=
  (∀ p ∈ s.staged, Valid p.validationFacts p.batch) ∧
  (∀ k p, s.journal k = some p → p.key = k ∧ Valid p.validationFacts p.batch)

theorem initial_invariant (facts : Catalog) : Invariant (init facts) := by
  simp [Invariant, init]

theorem step_preserves_invariant {s t : State} {e : Event}
    (inv : Invariant s) (step : Step s e t) : Invariant t := by
  rcases inv with ⟨staged, journal⟩
  cases step with
  | offer | edit | reject | lostAck | ack | rejectReplay => exact ⟨staged, journal⟩
  | stage id key b fresh valid =>
    refine ⟨?_, journal⟩
    intro p hp
    rcases List.mem_cons.mp hp with same | old
    · subst p
      exact valid
    · exact staged p old
  | abort id =>
    exact ⟨fun p h => staged p (List.mem_filter.mp h).1, journal⟩
  | commit pending prepared fresh =>
    refine ⟨fun p h => staged p (List.mem_filter.mp h).1, ?_⟩
    intro k p committed
    by_cases same : k = pending.key
    · simp [publish, same] at committed
      subst p
      exact ⟨same.symm, staged pending prepared⟩
    · exact journal k p (by simpa [publish, same] using committed)

theorem reachable_is_validated (facts : Catalog) (s : State) (h : Evolves (init facts) s) :
    Invariant s := by
  induction h with
  | refl => exact initial_invariant _
  | next _ step ih => exact step_preserves_invariant ih step

theorem new_commit_publishes_exact_batch {s t : State} {key : Key}
    (step : Step s (.commit key) t) :
    ∃ p ∈ s.staged, p.key = key ∧ s.journal key = none ∧ t.journal key = some p := by
  cases step with
  | commit p prepared fresh => exact ⟨p, prepared, rfl, fresh, by simp [publish]⟩

theorem abort_keeps_committed_results {s t : State} {id : Nat} (h : Step s (.abort id) t) :
    t.journal = s.journal := by cases h; rfl

theorem committed_result_survives_step {s t : State} {event : Event} {k : Key} {p : Attempt}
    (step : Step s event t) (committed : s.journal k = some p) : t.journal k = some p := by
  cases step with
  | commit q prepared fresh =>
    have different : k ≠ q.key := by
      intro equal
      subst k
      rw [fresh] at committed
      contradiction
    simpa [publish, different] using committed
  | offer | edit | stage | reject | abort | lostAck | ack | rejectReplay => exact committed

theorem retained_key_never_changes {s t : State} {k : Key} {p : Attempt}
    (trace : Evolves s t) (committed : s.journal k = some p) : t.journal k = some p := by
  induction trace with
  | refl => exact committed
  | next _ step ih => exact committed_result_survives_step step ih

theorem ack_has_matching_durable_result {s t : State} {k : Key} {fingerprint : Nat} {b : Batch}
    (step : Step s (.ack k fingerprint b) t) :
    t = s ∧ ∃ p, s.journal k = some p ∧ p.batch = b ∧ b.fingerprint = fingerprint := by
  cases step with
  | ack _ p committed => exact ⟨rfl, p, committed, rfl, rfl⟩

def demoRow : Row := ⟨1, 57, .physical, some 2, none, [60]⟩
def demoBatch : Batch := ⟨17, [demoRow, ⟨1, 58, .physical, some 4, none, [62]⟩]⟩
def demoKey : Key := (1, 2, 3)
def demoFacts : Catalog := fun _ => true
def closedFacts : Catalog := fun _ => false
def demoAttempt : Attempt := ⟨3, demoKey, demoBatch, demoFacts⟩
def demoStaged : State := { init demoFacts with staged := [demoAttempt] }

theorem offer_can_be_revoked_before_submit :
    let openState := init demoFacts
    let closedState := init closedFacts
    Step openState (.offer demoRow) openState ∧
    Step openState .edit closedState ∧ Step closedState (.reject demoBatch) closedState := by
  change Step (init demoFacts) (.offer demoRow) (init demoFacts) ∧
    Step (init demoFacts) .edit (init closedFacts) ∧
    Step (init closedFacts) (.reject demoBatch) (init closedFacts)
  refine ⟨Step.offer _ _ rfl, Step.edit _ _, Step.reject _ _ ?_⟩
  intro valid
  have rejected := valid.2 demoRow (by simp [demoBatch])
  simp [init, closedFacts] at rejected

theorem stage_commit_lost_ack_then_replay :
    let committed := publish demoStaged demoAttempt
    Evolves (init demoFacts) committed ∧
    Step committed (.lostAck demoKey) committed ∧
    Step committed (.ack demoKey 17 demoBatch) committed := by
  refine ⟨?_, Step.lostAck _ _, Step.ack _ _ demoAttempt ?_⟩
  · apply Evolves.next (Evolves.next (Evolves.refl _) (Step.stage _ 3 demoKey demoBatch ?_ ?_))
    · exact Step.commit _ demoAttempt (by simp [demoStaged, demoAttempt, init]) rfl
    · simp [init]
    · simp [Valid, init, demoBatch, demoFacts]
  · simp [publish, demoAttempt]

theorem validated_before_revocation_can_still_commit :
    ∃ s t, Evolves (init demoFacts) s ∧ Step s (.commit demoKey) t ∧
      t.catalog demoRow = false ∧ t.journal demoKey = some demoAttempt := by
  let stale := { demoStaged with catalog := closedFacts }
  refine ⟨stale, publish stale demoAttempt, ?_, ?_, rfl, by simp [publish, demoAttempt]⟩
  · apply Evolves.next _ (Step.edit demoStaged closedFacts)
    exact Evolves.next (Evolves.refl _) (Step.stage _ 3 demoKey demoBatch (by simp [init])
      (by simp [Valid, init, demoBatch, demoFacts]))
  · exact Step.commit stale demoAttempt (by simp [stale, demoStaged]) rfl

#print axioms initial_invariant
#print axioms step_preserves_invariant
#print axioms reachable_is_validated
#print axioms new_commit_publishes_exact_batch
#print axioms abort_keeps_committed_results
#print axioms retained_key_never_changes
#print axioms ack_has_matching_durable_result
#print axioms offer_can_be_revoked_before_submit
#print axioms stage_commit_lost_ack_then_replay
#print axioms validated_before_revocation_can_still_commit

end Booking
