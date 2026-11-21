namespace CourseLevelWithoutCycle

structure Course where
  cycle : Option String
  level : Option Nat

structure SaveChecks where
  groupEditable : Bool
  rowEditable : Bool
  duplicateInCycle : Bool

def addable (course : Course) (standalone : Bool) : Bool :=
  standalone && course.level.isNone

abbrev validLevel (value : Nat) : Prop := 1 ≤ value ∧ value ≤ 32767

def maySave (course : Course) (checks : SaveChecks) (value : Nat) : Bool :=
  checks.groupEditable && checks.rowEditable && decide (validLevel value) &&
    !(course.cycle.isSome && checks.duplicateInCycle)

def save (course : Course) (checks : SaveChecks) (value : Nat) : Option Course :=
  if maySave course checks value then some { course with level := some value } else none

def ready (courses : List Course) (configured hasGap : Bool) : Bool :=
  !courses.isEmpty && configured && courses.all (fun course => course.level.isSome) && !hasGap

theorem addableWithoutCycle : addable ⟨none, none⟩ true = true := by
  rfl

theorem validLevelSavesWithoutCycle (previous : Option Nat) (duplicate : Bool)
    (value : Nat) (valid : validLevel value) :
    save ⟨none, previous⟩ ⟨true, true, duplicate⟩ value = some ⟨none, some value⟩ := by
  simp [save, maySave, valid]

theorem successfulSavePreservesCycle (course updated : Course) (checks : SaveChecks) (value : Nat)
    (saved : save course checks value = some updated) : updated.cycle = course.cycle := by
  unfold save at saved
  split at saved
  · cases saved
    rfl
  · contradiction

theorem blockedGroupCannotSave (course : Course) (rowEditable duplicate : Bool) (value : Nat) :
    save course ⟨false, rowEditable, duplicate⟩ value = none := by
  rfl

theorem invalidLevelCannotSave (course : Course) (checks : SaveChecks) (value : Nat)
    (invalid : ¬ validLevel value) : save course checks value = none := by
  simp [save, maySave, invalid]

theorem duplicateInAssignedCycleCannotSave (cycle : String) (previous : Option Nat)
    (checksGroup checksRow : Bool) (value : Nat) :
    save ⟨some cycle, previous⟩ ⟨checksGroup, checksRow, true⟩ value = none := by
  simp [save, maySave]

theorem configuredGroupReadyRegardlessOfCycles (first : Course) (rest : List Course)
    (allLevelled : (first :: rest).all (fun course => course.level.isSome) = true) :
    ready (first :: rest) true false = true := by
  unfold ready
  rw [allLevelled]
  rfl

#print axioms addableWithoutCycle
#print axioms validLevelSavesWithoutCycle
#print axioms successfulSavePreservesCycle
#print axioms blockedGroupCannotSave
#print axioms invalidLevelCannotSave
#print axioms duplicateInAssignedCycleCannotSave
#print axioms configuredGroupReadyRegardlessOfCycles

end CourseLevelWithoutCycle
