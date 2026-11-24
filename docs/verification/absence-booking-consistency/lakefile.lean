import Lake
open Lake DSL

package absenceBookingConsistency where

@[default_target]
lean_lib AbsenceBookingConsistency where
  roots := #[`proof.Policy, `proof.Batch, `proof.Traces]
