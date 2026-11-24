import proof.Model

namespace AbsenceSubmit

theorem init_invariant (policy : Request → Prop) (s : State) (h : Init s) : Invariant policy s := by
  rcases h with ⟨hu, hc, hh, _, hp⟩
  exact ⟨hu, hc, hh, by simp [StagedSafe, hp]⟩

theorem step_preserves (policy : Request → Prop) {s t : State} {a : Action}
    (hi : Invariant policy s) (h : Step policy s a t) : Invariant policy t := by
  rcases hi with ⟨hu, hc, hh, hp⟩
  cases h with
  | acquire actor free =>
      exact ⟨hu, hc, hh, by simp [StagedSafe]⟩
  | validate actor r owns checked =>
      refine ⟨hu, hc, hh, ?_⟩
      intro r' heq
      have heq' : r = r' := Option.some.inj heq
      subst r'
      exact ⟨checked, by simp [owns]⟩
  | commit actor r owns checked =>
      obtain ⟨⟨hrPolicy, hfresh, hcompatible⟩, _⟩ := hp r checked
      refine ⟨?_, ?_, ?_, ?_⟩
      · simpa using List.nodup_cons.mpr ⟨hfresh, hu⟩
      · exact List.pairwise_cons.mpr ⟨hcompatible, hc⟩
      · intro r' hr'
        rcases List.mem_cons.mp hr' with hnew | hold
        · exact List.mem_cons.mpr (Or.inl hnew)
        · exact List.mem_cons.mpr (Or.inr (hh r' hold))
      · simp [StagedSafe]
  | abort actor reason owns =>
      exact ⟨hu, hc, hh, by simp [StagedSafe, released]⟩
  | cancel actor key owns =>
      refine ⟨hu, hc.filter _, ?_, ?_⟩
      · intro r hr
        exact hh r (List.mem_filter.mp hr).1
      · simp [StagedSafe]
  | replay key retained => exact ⟨hu, hc, hh, hp⟩
  | ack key retained => exact ⟨hu, hc, hh, hp⟩
  | waitOrLoseAck actor => exact ⟨hu, hc, hh, hp⟩

theorem all_reachable_safe (policy : Request → Prop) {s : State} (h : Reachable policy s) :
    Invariant policy s := by
  induction h with
  | initial h => exact init_invariant policy _ h
  | next hs step ih => exact step_preserves policy ih step

theorem retained_operations_not_duplicated (policy : Request → Prop) {s : State}
    (h : Reachable policy s) : (s.history.map Request.key).Nodup :=
  (all_reachable_safe policy h).1

theorem active_requests_do_not_conflict (policy : Request → Prop) {s : State}
    (h : Reachable policy s) : s.active.Pairwise Compatible :=
  (all_reachable_safe policy h).2.1

theorem abort_publishes_no_staged_effect (s : State) :
    (released s).active = s.active ∧ (released s).history = s.history := by
  exact ⟨rfl, rfl⟩

theorem replay_does_not_create_effect (policy : Request → Prop) {s t : State} (key : Nat)
    (h : Step policy s (.replay key) t) : t.history = s.history := by
  cases h
  rfl

theorem validate_publishes_no_effect (policy : Request → Prop) {s t : State}
    (actor : Nat) (r : Request) (h : Step policy s (.validate actor r) t) :
    t.active = s.active ∧ t.history = s.history := by
  cases h
  exact ⟨rfl, rfl⟩

theorem committed_request_was_authorized (policy : Request → Prop) {s t : State}
    (actor : Nat) (r : Request) (hi : Invariant policy s)
    (h : Step policy s (.commit actor r) t) : policy r := by
  cases h
  rename_i owns checked
  exact (hi.2.2.2 r checked).1.1

-- Non-vacuity: arbitrary nonempty, valid requests can commit from an empty
-- initial state. Infinite waiting is also allowed; no liveness is claimed.
theorem a_commit_is_reachable (policy : Request → Prop) (r : Request) (actor : Nat)
    (allowed : policy r) :
    Reachable policy { active := [r], history := [r], owner := none, staged := none, acknowledged := [] } := by
  let s : State := ⟨[], [], none, none, []⟩
  have hs : Reachable policy s := .initial (by simp [Init, s])
  have ha := Reachable.next hs (Step.acquire s actor rfl)
  have checked : CanStage policy { s with owner := some actor, staged := none } r := by
    simp [CanStage, s, allowed]
  have hv := Reachable.next ha (Step.validate _ actor r rfl checked)
  exact Reachable.next hv (Step.commit _ actor r rfl rfl)

#print axioms init_invariant
#print axioms step_preserves
#print axioms all_reachable_safe
#print axioms retained_operations_not_duplicated
#print axioms active_requests_do_not_conflict
#print axioms abort_publishes_no_staged_effect
#print axioms replay_does_not_create_effect
#print axioms validate_publishes_no_effect
#print axioms committed_request_was_authorized
#print axioms a_commit_is_reachable

end AbsenceSubmit
