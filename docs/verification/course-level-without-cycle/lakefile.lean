import Lake
open Lake DSL

package courseLevelWithoutCycle where

@[default_target]
lean_lib CourseLevelWithoutCycle where
  roots := #[`proof.CourseLevel]
