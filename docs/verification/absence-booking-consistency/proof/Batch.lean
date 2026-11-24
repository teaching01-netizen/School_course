import Std

namespace Booking

inductive Method where
  | physical | zoom | none
  deriving DecidableEq, Repr

structure Row where
  scope : Nat
  missed : Nat
  method : Method
  target : Option Nat
  targetMerge : Option Nat
  sessions : List Nat
  deriving DecidableEq, Repr

def aliasKey (r : Row) : Option (Nat × Nat × List Nat) := do
  let group ← r.targetMerge
  let _ ← r.target
  if r.sessions.isEmpty then none else
    some (r.scope, group, r.sessions.mergeSort)

def sharesChoice (r peer : Row) : Bool :=
  (aliasKey r).isSome && aliasKey r == aliasKey peer

def resolvedTarget (rows : List Row) (r : Row) : Option Nat :=
  (rows.find? (sharesChoice r)).map Row.target |>.getD r.target

theorem changed_alias_has_matching_peer (rows : List Row) (r : Row)
    (changed : resolvedTarget rows r ≠ r.target) :
    ∃ peer ∈ rows, sharesChoice r peer = true ∧ resolvedTarget rows r = peer.target := by
  cases h : rows.find? (sharesChoice r) with
  | none => simp [resolvedTarget, h] at changed
  | some peer =>
    exact ⟨peer, List.mem_of_find?_eq_some h, List.find?_some h, by simp [resolvedTarget, h]⟩

theorem shares_only_same_scope_group_and_sessions (r peer : Row)
    (h : sharesChoice r peer = true) :
    r.scope = peer.scope ∧ r.targetMerge = peer.targetMerge ∧
      r.sessions.mergeSort = peer.sessions.mergeSort := by
  obtain ⟨hSome, heq⟩ := Bool.and_eq_true_iff.mp h
  have hk : aliasKey r = aliasKey peer := beq_iff_eq.mp heq
  cases hg : r.targetMerge <;> cases ht : r.target <;>
    cases hpg : peer.targetMerge <;> cases hpt : peer.target <;>
    cases hrs : r.sessions <;> cases hps : peer.sessions <;>
    simp_all [aliasKey]

def bucketKey (rows : List Row) (r : Row) : Nat × Method × Option Nat :=
  (r.scope, r.method, resolvedTarget rows r)

def bucket (rows : List Row) (key : Nat × Method × Option Nat) : List Row :=
  rows.filter (fun r => bucketKey rows r == key)

theorem row_in_bucket_exact (rows : List Row) (key : Nat × Method × Option Nat) (r : Row) :
    r ∈ bucket rows key ↔ r ∈ rows ∧ bucketKey rows r = key := by
  simp [bucket]

theorem all_inputs_retained (rows : List Row) (r : Row) (h : r ∈ rows) :
    r ∈ bucket rows (bucketKey rows r) :=
  (row_in_bucket_exact _ _ _).mpr ⟨h, rfl⟩

theorem distinct_targets_never_share_item (rows : List Row) (key : Nat × Method × Option Nat)
    (a b : Row) (ha : a ∈ bucket rows key) (hb : b ∈ bucket rows key) :
    resolvedTarget rows a = resolvedTarget rows b := by
  exact congrArg (fun k : Nat × Method × Option Nat => k.2.2)
    (((row_in_bucket_exact _ _ _).mp ha).2.trans ((row_in_bucket_exact _ _ _).mp hb).2.symm)

theorem independent_targets_stay_distinct (rows : List Row) (a b : Row)
    (ha : ∀ p ∈ rows, sharesChoice a p = true → p.target = a.target)
    (hb : ∀ p ∈ rows, sharesChoice b p = true → p.target = b.target)
    (different : a.target ≠ b.target) :
    ¬ ∃ key, a ∈ bucket rows key ∧ b ∈ bucket rows key := by
  have unchanged (r : Row) (h : ∀ p ∈ rows, sharesChoice r p = true → p.target = r.target) :
      resolvedTarget rows r = r.target := by
    by_cases same : resolvedTarget rows r = r.target
    · exact same
    · obtain ⟨p, hmem, hshare, heq⟩ := changed_alias_has_matching_peer rows r same
      exact heq.trans (h p hmem hshare)
  rintro ⟨key, aMem, bMem⟩
  apply different
  simpa [unchanged a ha, unchanged b hb] using distinct_targets_never_share_item rows key a b aMem bMem

def readingChoice : Row := ⟨1, 70, .physical, some 8, some 9, [81, 82]⟩
def writingChoice : Row := ⟨1, 71, .physical, some 10, some 9, [82, 81]⟩

theorem same_merged_choice_uses_first_alias :
    bucketKey [readingChoice, writingChoice] writingChoice = (1, .physical, some 8) := by
  simp [bucketKey, resolvedTarget, sharesChoice, aliasKey, readingChoice, writingChoice, List.mergeSort]

#print axioms changed_alias_has_matching_peer
#print axioms shares_only_same_scope_group_and_sessions
#print axioms row_in_bucket_exact
#print axioms all_inputs_retained
#print axioms distinct_targets_never_share_item
#print axioms independent_targets_stay_distinct
#print axioms same_merged_choice_uses_first_alias

end Booking
