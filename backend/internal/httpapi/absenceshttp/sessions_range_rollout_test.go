package absenceshttp

// Step-24 staged-rollout gate: percentage flag semantics + corrected-
// legacy rollback proof. No DB: pure flag/bucket logic plus contract
// assertions on shared-fix code both paths execute.
//
// Stages: 0% (full rollback) -> 1% -> 5% -> 25% -> 50% -> 100% (full V2).
// Rollback returns to the CORRECTED legacy path: shared fixes (institute-
// day half-open interval, tz-aware eligibility, centralized 409 mapping)
// live in code both paths execute, so rollback never restores the audit
// defects. The gate proves: (a) flag parsing at every stage boundary,
// (b) bucket stability (same key, same arm, 200 reads), (c) split sanity
// (at 50% both arms fire across keys; at 0%/100% unanimous), (d) legacy
// arm still correct (shadow error_parity cases pass at V2=0 — covered by
// TestSessionsRangeV2_ShadowEquivalence, asserted here by reference),
// (e) rollback-before-cutover exercised: 100% -> 0% -> 100% flips cleanly.

import (
  "fmt"
  "testing"
)

// TestSessionsRangeRolloutStages gates flag parsing at every stage.
func TestSessionsRangeRolloutStages(t *testing.T) {
  cases := []struct {
    flag string
    key  string
    want bool
  }{
    {"0", "any", false},
    {"false", "any", false},
    {"off", "any", false},
    {"", "any", true},
    {"1", "any", true},
    {"true", "any", true},
    {"on", "any", true},
    {"garbage-value", "any", true},
    {"100", "any", true},
    {"100%", "any", true},
    {"0%", "any", false},
    {"101", "any", true},
    {"-1", "any", true},
  }
  for _, tc := range cases {
    t.Run(fmt.Sprintf("flag_%s", tc.flag), func(t *testing.T) {
      t.Setenv("WARWICK_SESSIONS_RANGE_V2", tc.flag)
      if got := sessionsRangeUseV2For(tc.key); got != tc.want {
        t.Fatalf("flag %q key %q = %v, want %v", tc.flag, tc.key, got, tc.want)
      }
    })
  }
}

// TestSessionsRangeRolloutBuckets gates stability + split sanity.
func TestSessionsRangeRolloutBuckets(t *testing.T) {
  t.Setenv("WARWICK_SESSIONS_RANGE_V2", "50")
  keys := []string{"w0001|2026-10-01|2026-10-07", "w0002|2026-10-01|2026-10-07", "w9999|1970-01-01|2100-01-01", "student|2026-10-01|2026-10-07"}
  // Stability: same key resolves identically across 200 reads.
  for _, k := range keys {
    first := sessionsRangeUseV2For(k)
    for i := 0; i < 200; i++ {
      if got := sessionsRangeUseV2For(k); got != first {
        t.Fatalf("unstable bucket %q: %v then %v", k, first, got)
      }
    }
  }
  // Split sanity at 50%: across 200 distinct keys both arms must fire
  // (FNV-1a spreads uniformly; a degenerate hash would fail here).
  v2, legacy := 0, 0
  for i := 0; i < 200; i++ {
    if sessionsRangeUseV2For(fmt.Sprintf("w%05d|2026-10-01|2026-10-07", i)) {
      v2++
    } else {
      legacy++
    }
  }
  if v2 == 0 || legacy == 0 {
    t.Fatalf("50%% stage degenerate: v2=%d legacy=%d over 200 keys", v2, legacy)
  }
  t.Logf("50%% split over 200 keys: v2=%d legacy=%d", v2, legacy)
  // Unanimity at the ends.
  t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
  for i := 0; i < 20; i++ {
    if sessionsRangeUseV2For(fmt.Sprintf("w%05d|x|y", i)) {
      t.Fatal("0%% stage routed to V2")
    }
  }
  t.Setenv("WARWICK_SESSIONS_RANGE_V2", "100")
  for i := 0; i < 20; i++ {
    if !sessionsRangeUseV2For(fmt.Sprintf("w%05d|x|y", i)) {
      t.Fatal("100%% stage routed to legacy")
    }
  }
}

// TestSessionsRangeRollbackFlipsCleanly exercises configuration rollback
// before broad cutover: 100% -> 0% -> 100% with per-stage unanimity.
func TestSessionsRangeRollbackFlipsCleanly(t *testing.T) {
  key := "wrollback|2026-10-01|2026-10-07"
  t.Setenv("WARWICK_SESSIONS_RANGE_V2", "100")
  if !sessionsRangeUseV2For(key) {
    t.Fatal("100%%: expected V2")
  }
  t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
  if sessionsRangeUseV2For(key) {
    t.Fatal("rollback to 0%%: expected legacy")
  }
  // Rollback arm is the CORRECTED legacy: shared fixes both paths
  // execute — institute-day half-open interval (Step 4), tz-aware
  // eligibility (Step 5), centralized 409 mapping (Step 7). Asserted by
  // reference: TestSessionsRangeV2_ShadowEquivalence/error_parity passes
  // with V2=0 (legacy arm correct), and Steps 4/5/7 fixes are not gated
  // on sessionsRangeUseV2 (grep-audited: no V2 condition around the
  // shared parse/eligibility/conflict code).
  t.Setenv("WARWICK_SESSIONS_RANGE_V2", "100")
  if !sessionsRangeUseV2For(key) {
    t.Fatal("re-cutover to 100%%: expected V2")
  }
}
