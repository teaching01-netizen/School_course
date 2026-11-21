namespace LegacyCourseRefresh

inductive Intent where
  | background | manual
  deriving DecidableEq

def key (intent : Intent) (course : String) : String :=
  match intent with
  | .background => "legacy:course:" ++ course
  | .manual => "legacy:manual-course:" ++ course

theorem manualKeyDiffersFromBackground (manualCourse backgroundCourse : String) :
    key .manual manualCourse ≠ key .background backgroundCourse := by
  intro equal
  have distinctCharacter := congrArg (fun value : String => value.toList[7]?) equal
  change some 'm' = some 'c' at distinctCharacter
  cases distinctCharacter

def sweepEligible (linked frozen due sourceOpen : Bool) : Bool :=
  linked && (if frozen then sourceOpen else due)

def shouldFetch (intent : Intent) (linked frozen sourceOpen cooldown snapshotOk : Bool) : Bool :=
  linked && (intent == .manual || (if frozen then sourceOpen else !(cooldown && snapshotOk)))

theorem linkedManualFetches (frozen sourceOpen cooldown snapshotOk : Bool) :
    shouldFetch .manual true frozen sourceOpen cooldown snapshotOk = true := by
  rfl

theorem syncedArchiveSkipsInBackground (due cooldown snapshotOk : Bool) :
    sweepEligible true true due false = false ∧
    shouldFetch .background true true false cooldown snapshotOk = false := by
  exact ⟨rfl, rfl⟩

theorem confirmedReactivationResumes (due cooldown snapshotOk : Bool) :
    sweepEligible true true due true = true ∧
    shouldFetch .background true true true cooldown snapshotOk = true := by
  exact ⟨rfl, rfl⟩

theorem missingLinkSkips (intent : Intent) (frozen due sourceOpen cooldown snapshotOk : Bool) :
    sweepEligible false frozen due sourceOpen = false ∧
    shouldFetch intent false frozen sourceOpen cooldown snapshotOk = false := by
  exact ⟨rfl, rfl⟩

inductive Observation where
  | refused | sourceFailed | shadow | applied (stamp : Nat)

def recordSync (previous : Nat) (observation : Observation) : Nat :=
  match observation with
  | .applied stamp => stamp
  | _ => previous

theorem noCompletedApplyKeepsStamp (previous : Nat) :
    recordSync previous .refused = previous ∧
    recordSync previous .sourceFailed = previous ∧
    recordSync previous .shadow = previous := by
  exact ⟨rfl, rfl, rfl⟩

theorem archivedManualSuccessCanAdvance (previous stamp : Nat) (newer : previous < stamp) :
    shouldFetch .manual true true true true true = true ∧
    previous < recordSync previous (.applied stamp) := by
  exact ⟨rfl, newer⟩

inductive WaitPath : Nat → Nat → Prop where
  | next {held requested : Nat} : held < requested → WaitPath held requested
  | extend {start held requested : Nat} :
      WaitPath start held → held < requested → WaitPath start requested

theorem waitPathIncreases {start requested : Nat} (path : WaitPath start requested) :
    start < requested := by
  induction path with
  | next edge => exact edge
  | extend _ edge prior => exact Nat.lt_trans prior edge

theorem strictlyOrderedLocksCannotCycle (rank : Nat) : ¬ WaitPath rank rank := by
  intro cycle
  exact Nat.lt_irrefl rank (waitPathIncreases cycle)

#print axioms manualKeyDiffersFromBackground
#print axioms linkedManualFetches
#print axioms syncedArchiveSkipsInBackground
#print axioms confirmedReactivationResumes
#print axioms missingLinkSkips
#print axioms noCompletedApplyKeepsStamp
#print axioms archivedManualSuccessCanAdvance
#print axioms strictlyOrderedLocksCannotCycle

end LegacyCourseRefresh
