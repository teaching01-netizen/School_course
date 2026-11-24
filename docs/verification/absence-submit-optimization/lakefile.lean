import Lake
open Lake DSL

package absenceSubmitOptimization where

@[default_target]
lean_lib AbsenceSubmitOptimization where
  roots := #[`AbsenceSubmitOptimization, `proof.Selection, `proof.Model, `proof.Safety, `proof.Counterexamples, `proof.Audit]
