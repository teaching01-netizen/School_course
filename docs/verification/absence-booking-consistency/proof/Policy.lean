import Std

namespace Booking

structure Course where
  rank : Nat
  code : Nat
  bookable : Bool
  deriving DecidableEq, Repr

def Precedes (a b : Course) : Prop :=
  a.rank < b.rank ∨ (a.rank = b.rank ∧ a.code ≤ b.code)

instance (a b : Course) : Decidable (Precedes a b) :=
  inferInstanceAs (Decidable (a.rank < b.rank ∨ (a.rank = b.rank ∧ a.code ≤ b.code)))

theorem precedes_total (a b : Course) : Precedes a b ∨ Precedes b a := by
  unfold Precedes
  omega

theorem precedes_trans {a b c : Course} (ab : Precedes a b) (bc : Precedes b c) :
    Precedes a c := by
  unfold Precedes at *
  omega

def choose : List Course → Option Course
  | [] => none
  | a :: rest =>
    match choose rest with
    | none => some a
    | some b => some (if Precedes a b then a else b)

theorem choose_none_iff (courses : List Course) : choose courses = none ↔ courses = [] := by
  cases courses with
  | nil => simp [choose]
  | cons a rest =>
    cases h : choose rest <;> simp [choose, h]

theorem choose_minimum (courses : List Course) (c : Course) (h : choose courses = some c) :
    c ∈ courses ∧ ∀ d ∈ courses, Precedes c d := by
  induction courses generalizing c with
  | nil => simp [choose] at h
  | cons a rest ih =>
    cases he : choose rest with
    | none =>
      have empty := (choose_none_iff rest).mp he
      subst rest
      simp [choose] at h
      subst c
      simp [Precedes]
    | some b =>
      obtain ⟨hb, hmin⟩ := ih b he
      by_cases hab : Precedes a b
      · simp [choose, he, hab] at h
        subst c
        refine ⟨by simp, ?_⟩
        intro d hd
        rcases List.mem_cons.mp hd with rfl | hd
        · simp [Precedes]
        · exact precedes_trans hab (hmin d hd)
      · simp [choose, he, hab] at h
        subst c
        refine ⟨List.mem_cons_of_mem a hb, ?_⟩
        intro d hd
        rcases List.mem_cons.mp hd with rfl | hd
        · exact (precedes_total _ _).resolve_right hab
        · exact hmin d hd

theorem choose_permutation (xs ys : List Course) (permutation : xs.Perm ys)
    (codesUnique : ∀ a ∈ xs, ∀ b ∈ xs, a.code = b.code → a = b) :
    choose xs = choose ys := by
  cases hx : choose xs with
  | none =>
    have empty := (choose_none_iff xs).mp hx
    subst xs
    have : ys = [] := permutation.symm.eq_nil
    simp [this, choose]
  | some a =>
    obtain ⟨ha, hmin⟩ := choose_minimum xs a hx
    cases hy : choose ys with
    | none =>
      have empty := (choose_none_iff ys).mp hy
      have impossible := permutation.mem_iff.mp ha
      simp [empty] at impossible
    | some b =>
      obtain ⟨hb, hmin'⟩ := choose_minimum ys b hy
      have hb' := permutation.mem_iff.mpr hb
      have ab := hmin b hb'
      have ba := hmin' a (permutation.mem_iff.mp ha)
      have sameCode : a.code = b.code := by
        unfold Precedes at *
        omega
      rw [codesUnique a ha b hb' sameCode]

inductive Audience where
  | student | studentFacingStaff | admin
  deriving DecidableEq, Repr

def visible (view : Audience) (courses : List Course) : List Course :=
  match view with
  | .admin => courses
  | _ => courses.filter Course.bookable

theorem visibility_exact (view : Audience) (courses : List Course) (c : Course) :
    c ∈ visible view courses ↔ c ∈ courses ∧ (view = .admin ∨ c.bookable = true) := by
  cases view <;> simp [visible]

inductive RulePath where
  | override (id : Nat)
  | rootPriorities (ids : List Nat)
  | fallback (id : Option Nat)
  deriving DecidableEq, Repr

def chooseRule (override : Option Nat) (hasRoot : Bool) (priorities : List Nat)
    (fallback : Option Nat) : RulePath :=
  match override with
  | some rule => .override rule
  | none =>
    if hasRoot && !priorities.isEmpty then .rootPriorities priorities else .fallback fallback

theorem explicit_override_wins (rule : Nat) (root : Bool) (priorities : List Nat) (fallback : Option Nat) :
    chooseRule (some rule) root priorities fallback = .override rule := rfl

theorem inherited_rule_keeps_priorities (root : Bool) (priorities : List Nat) (fallback : Option Nat) :
    chooseRule none root priorities fallback = .rootPriorities priorities ↔
      root = true ∧ priorities ≠ [] := by
  cases root <;> cases priorities <;> simp [chooseRule]

theorem no_applicable_priorities_uses_fallback (root : Bool) (priorities : List Nat) (fallback : Option Nat) :
    chooseRule none root priorities fallback = .fallback fallback ↔ root = false ∨ priorities = [] := by
  cases root <;> cases priorities <;> simp [chooseRule]

structure Calendar where
  midnight : Int → Int
  dayOf : Int → Int
  ordered : ∀ a b, a ≤ b → midnight a ≤ midnight b
  contains : ∀ t, midnight (dayOf t) ≤ t ∧ t < midnight (dayOf t + 1)

def inQueryWindow (cal : Calendar) (first through instant : Int) : Prop :=
  cal.midnight first ≤ instant ∧ instant < cal.midnight (through + 1)

theorem local_dates_iff_half_open (cal : Calendar) (first through instant : Int) :
    inQueryWindow cal first through instant ↔ first ≤ cal.dayOf instant ∧ cal.dayOf instant ≤ through := by
  obtain ⟨lo, hi⟩ := cal.contains instant
  constructor
  · rintro ⟨hlo, hhi⟩
    constructor
    · by_cases h : first ≤ cal.dayOf instant
      · exact h
      · have bound := cal.ordered (cal.dayOf instant + 1) first (by omega)
        omega
    · by_cases h : cal.dayOf instant ≤ through
      · exact h
      · have bound := cal.ordered (through + 1) (cal.dayOf instant) (by omega)
        omega
  · rintro ⟨hlo, hhi⟩
    have left := cal.ordered _ _ hlo
    have right := cal.ordered (cal.dayOf instant + 1) (through + 1) (by omega)
    exact ⟨by omega, by omega⟩

def bangkokHours : Calendar where
  midnight d := d * 24 - 7
  dayOf t := (t + 7) / 24
  ordered := by intro a b h; omega
  contains := by intro t; omega

theorem bangkok_early_and_next_midnight :
    inQueryWindow bangkokHours 0 0 (-1) ∧ ¬ inQueryWindow bangkokHours 0 0 17 := by
  simp [inQueryWindow, bangkokHours]

#print axioms choose_minimum
#print axioms choose_permutation
#print axioms visibility_exact
#print axioms explicit_override_wins
#print axioms inherited_rule_keeps_priorities
#print axioms no_applicable_priorities_uses_fallback
#print axioms local_dates_iff_half_open
#print axioms bangkok_early_and_next_midnight

end Booking
