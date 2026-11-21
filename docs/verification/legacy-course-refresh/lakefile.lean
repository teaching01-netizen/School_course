import Lake
open Lake DSL

package legacyCourseRefresh where

@[default_target]
lean_lib LegacyCourseRefresh where
  roots := #[`proof.Refresh]
