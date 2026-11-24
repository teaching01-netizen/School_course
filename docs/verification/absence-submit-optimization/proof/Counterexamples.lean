import proof.Safety

namespace AbsenceSubmit

-- Without a common lock or fresh recheck, both workers may read [] and then
-- append conflicting requests from those stale successful checks.
def witnessCandidate : Candidate := ⟨1, 1, none, ⟨10, 20⟩, true⟩
def witnessA : Request := ⟨1, [witnessCandidate]⟩
def witnessB : Request := ⟨2, [witnessCandidate]⟩

theorem naive_read_check_write_counterexample :
    (∀ old ∈ ([] : List Request), Compatible witnessA old) ∧
    (∀ old ∈ ([] : List Request), Compatible witnessB old) ∧
    ¬ [witnessA, witnessB].Pairwise Compatible := by
  simp [Compatible, witnessA, witnessB, witnessCandidate, Overlap]

-- Two retained keys are two operations; no theorem promises deduplication
-- across keys. Likewise, deleting an expired key permits a later new acquire.
theorem fresh_key_is_not_same_operation :
    ([witnessA, witnessB].map Request.key).Nodup ∧
    [witnessA, witnessB].length = 2 := by decide

theorem acknowledgement_is_not_commit :
    ∃ s : State, Reachable (fun _ => True) s ∧ s.history ≠ [] ∧ s.acknowledged = [] := by
  exact ⟨⟨[witnessA], [witnessA], none, none, []⟩,
    a_commit_is_reachable _ witnessA 123 True.intro, by decide, rfl⟩

theorem authorization_is_nonvacuous :
    Authorized 1 none (focus 1 none [witnessCandidate]) [1] ∧
    ¬ Authorized 2 none [witnessCandidate] [1] := by
  simp [Authorized, focus, inScope, witnessCandidate]

structure RetentionExample where
  retained : List Nat
  effects : List Nat
  deriving DecidableEq

def acquireExample (key : Nat) (s : RetentionExample) : RetentionExample :=
  if key ∈ s.retained then s else ⟨key :: s.retained, key :: s.effects⟩

def expireExample (key : Nat) (s : RetentionExample) : RetentionExample :=
  { s with retained := s.retained.filter (fun k => k != key) }

theorem expiry_ends_deduplication_guarantee :
    let once := acquireExample 1 ⟨[], []⟩
    let retried := acquireExample 1 (expireExample 1 once)
    retried.effects = [1, 1] := by decide

#print axioms naive_read_check_write_counterexample
#print axioms fresh_key_is_not_same_operation
#print axioms acknowledgement_is_not_commit
#print axioms authorization_is_nonvacuous
#print axioms expiry_ends_deduplication_guarantee

end AbsenceSubmit
