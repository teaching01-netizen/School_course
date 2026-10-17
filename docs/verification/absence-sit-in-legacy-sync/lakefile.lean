import Lake
open Lake DSL

package absenceSitInLegacySync where

@[default_target]
lean_lib AbsenceSitInLegacySync where
  roots := #[`AbsenceSitInLegacySync, `proof.Model, `proof.PolicyAudit]
