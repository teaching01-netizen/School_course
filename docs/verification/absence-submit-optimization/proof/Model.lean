import proof.Selection

namespace AbsenceSubmit

-- One student's state; actors and retained logical operation keys are arbitrary
-- naturals. A key denotes the full actor/scope/key tuple after fingerprint checks.
structure Request where
  key : Nat
  sessions : List Candidate
  deriving DecidableEq, Repr

def Compatible (a b : Request) : Prop :=
  ∀ x ∈ a.sessions, ∀ y ∈ b.sessions, ¬ Overlap x.slot y.slot

structure State where
  active : List Request
  history : List Request
  owner : Option Nat
  staged : Option Request
  acknowledged : List Nat
  deriving DecidableEq, Repr

def CanStage (policy : Request → Prop) (s : State) (r : Request) : Prop :=
  policy r ∧ r.key ∉ s.history.map Request.key ∧ ∀ old ∈ s.active, Compatible r old

def StagedSafe (policy : Request → Prop) (s : State) : Prop :=
  ∀ r, s.staged = some r → CanStage policy s r ∧ s.owner ≠ none

def Invariant (policy : Request → Prop) (s : State) : Prop :=
  (s.history.map Request.key).Nodup ∧ s.active.Pairwise Compatible ∧
    (∀ r ∈ s.active, r ∈ s.history) ∧ StagedSafe policy s

def Init (s : State) : Prop :=
  (s.history.map Request.key).Nodup ∧ s.active.Pairwise Compatible ∧
    (∀ r ∈ s.active, r ∈ s.history) ∧ s.owner = none ∧ s.staged = none

def released (s : State) : State := { s with owner := none, staged := none }

inductive AbortReason where
  | deadline | canceled | deadlock | crash | validationFailure | connectionLoss
  deriving DecidableEq, Repr

inductive Action where
  | acquire (actor : Nat)
  | validate (actor : Nat) (r : Request)
  | commit (actor : Nat) (r : Request)
  | abort (actor : Nat) (reason : AbortReason)
  | cancel (actor key : Nat)
  | replay (key : Nat)
  | ack (key : Nat)
  | waitOrLoseAck (actor : Nat)
  deriving DecidableEq, Repr

-- Reads and writes are separate transitions. Only the lock owner can validate,
-- commit, or cancel. No other actor may change this student's active rows while
-- the lock is held. Real SQL writers must refine this protocol; it is not a fact
-- about all deployed writers or changing schedules/configuration.
inductive Step (policy : Request → Prop) : State → Action → State → Prop where
  | acquire (s : State) (actor : Nat) (free : s.owner = none) :
      Step policy s (.acquire actor) { s with owner := some actor, staged := none }
  | validate (s : State) (actor : Nat) (r : Request)
      (owns : s.owner = some actor) (checked : CanStage policy s r) :
      Step policy s (.validate actor r) { s with staged := some r }
  | commit (s : State) (actor : Nat) (r : Request)
      (owns : s.owner = some actor) (checked : s.staged = some r) :
      Step policy s (.commit actor r)
        { s with
          active := r :: s.active
          history := r :: s.history
          owner := none
          staged := none }
  | abort (s : State) (actor : Nat) (reason : AbortReason) (owns : s.owner = some actor) :
      Step policy s (.abort actor reason) (released s)
  | cancel (s : State) (actor key : Nat) (owns : s.owner = some actor) :
      Step policy s (.cancel actor key)
        { s with active := s.active.filter (fun r => r.key != key), owner := none, staged := none }
  | replay (s : State) (key : Nat) (retained : key ∈ s.history.map Request.key) :
      Step policy s (.replay key) s
  | ack (s : State) (key : Nat) (retained : key ∈ s.history.map Request.key) :
      Step policy s (.ack key) { s with acknowledged := key :: s.acknowledged }
  | waitOrLoseAck (s : State) (actor : Nat) : Step policy s (.waitOrLoseAck actor) s

inductive Reachable (policy : Request → Prop) : State → Prop where
  | initial {s : State} (h : Init s) : Reachable policy s
  | next {s t : State} {a : Action} (hs : Reachable policy s) (h : Step policy s a t) :
      Reachable policy t

end AbsenceSubmit
