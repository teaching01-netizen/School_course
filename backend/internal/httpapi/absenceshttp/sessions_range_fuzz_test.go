package absenceshttp

// Step-20 fuzz gates: request parsing, dates, UUID lists, merge
// structures, priorities, mappings, and submission payloads.
//
// Each Fuzz target feeds arbitrary bytes into a pure parse/validate
// entry and requires CONTROLLED outcomes only: a typed result or a
// rejection — never a panic, hang, or unbounded allocation. Collection
// sizes are capped before materializing (see maxFuzzCollect). Seed-corpus
// cases pin the interesting edges (empty, boundary, hostile).
// Run: go test -fuzz=FuzzParse -fuzztime=30s ./internal/httpapi/absenceshttp/

import (
  "strings"
  "testing"

  "warwick-institute/internal/httpapi/httpadapter"
)

// maxFuzzCollect caps materialized collection sizes inside fuzz targets.
// Inputs promising more elements are rejected before allocating.
const maxFuzzCollect = 1024

// fuzzAdapter is a no-auth adapter for pure UUID parse helpers.
func fuzzAdapter() httpadapter.Adapter {
  return httpadapter.Adapter{}
}

// FuzzParseInstituteLocalDate: date strings x timezone names must parse
// or return a controlled error — never panic.
func FuzzParseInstituteLocalDate(f *testing.F) {
  for _, seed := range []string{"2026-09-07", "", "not-a-date", "2026-13-45", "2026-02-30", "0000-00-00", "9999-12-31", "2026-09-07T09:00:00Z", "  2026-09-07  ", "2026-9-7", "\u00002026-09-07", "2026\u0000-09-07"} {
    f.Add(seed, "Asia/Bangkok")
    f.Add(seed, "America/New_York")
    f.Add(seed, "")
  }
  f.Fuzz(func(t *testing.T, dateStr, tz string) {
    if len(dateStr) > 256 || len(tz) > 256 {
      t.Skip("oversize input bounded before parse")
    }
    _, _ = parseInstituteLocalDate(dateStr, tz)
  })
}
// FuzzParseSubjectIDFilter: UUID-list strings must parse to a bounded set
// or return a controlled error — never panic, never unbounded growth.
func FuzzParseSubjectIDFilter(f *testing.F) {
  seeds := []string{"", "  ", "not-a-uuid", "11111111-1111-1111-1111-111111111111", "11111111-1111-1111-1111-111111111111, 11111111-1111-1111-1111-111111111111", "a,b,c,,,,", "00000000-0000-0000-0000-000000000000"}
  for _, s := range seeds {
    f.Add(s)
  }
  f.Fuzz(func(t *testing.T, raw string) {
    if len(raw) > 64*1024 {
      t.Skip("oversize list bounded before parse")
    }
    if strings.Count(raw, ",") > maxFuzzCollect {
      t.Skip("element count bounded before materializing")
    }
    _, _ = parseSubjectIDFilter(fuzzAdapter(), raw)
  })
}

// FuzzSessionDateKey: timestamp strings must render a day or "" — never panic.
func FuzzSessionDateKey(f *testing.F) {
  seeds := []string{"", "2026-09-07T02:00:00Z", "2026-09-07T02:00:00.123456789Z", "not-a-time", "2026-09-07", "0000-00-00", "2026-06-02T03:00:00+07:00"}
  for _, s := range seeds {
    f.Add(s, "Asia/Bangkok")
    f.Add(s, "America/New_York")
  }
  f.Fuzz(func(t *testing.T, ts, tz string) {
    if len(ts) > 512 || len(tz) > 256 {
      t.Skip("oversize input bounded")
    }
    _ = sessionDateKey(ts, tz)
  })
}

// FuzzParsePredicate: rule-predicate JSON must decode or error — never panic.
// Covers merge structures, priorities, and mapping-adjacent shapes via the
// shared predicate decoder.
func FuzzParsePredicate(f *testing.F) {
  seeds := []string{`{}`, `{"level_1_action":"zoom"}`, `{"chains":[{"from_rank":1,"to_rank":2}]}`, `[1,2,3]`, `"str"`, `null`, ``}
  for _, s := range seeds {
    f.Add([]byte(s))
  }
  f.Fuzz(func(t *testing.T, raw []byte) {
    if len(raw) > 64*1024 {
      t.Skip("oversize payload bounded")
    }
    p, err := parsePredicate(raw)
    if err != nil {
      return
    }
    if len(p.Chains) > 100000 {
      t.Fatalf("unbounded chains decoded: %d", len(p.Chains))
    }
    _, _ = EvaluateRule(EvaluateRuleInput{RuleType: RuleTypeLevelLadder, Predicate: p, StudentLevel: 2})
  })
}

// FuzzEvaluateRuleTypes: rule-type strings must resolve or controlled-error.
func FuzzEvaluateRuleTypes(f *testing.F) {
  for _, s := range []string{RuleTypeLevelLadder, RuleTypeCrossSection, RuleTypeAnyDayExceptLast, RuleTypeRankChain, RuleTypeTeacherCase, "", "unknown", "LEVEL_LADDER", "level ladder"} {
    f.Add(s)
  }
  f.Fuzz(func(t *testing.T, ruleType string) {
    if len(ruleType) > 256 {
      t.Skip("oversize type bounded")
    }
    _, _ = EvaluateRule(EvaluateRuleInput{RuleType: ruleType, StudentLevel: 2})
  })
}

// FuzzNormalizeWCode: wcode normalization must never panic and must stay
// bounded (weakened-identity mutations are killed by TestMutation_Identity).
func FuzzNormalizeWCode(f *testing.F) {
  for _, s := range []string{"", "w123", "W123", "  w123  ", "\u0000", "a/b?c&d=e"} {
    f.Add(s)
  }
  f.Fuzz(func(t *testing.T, raw string) {
    if len(raw) > 1024 {
      t.Skip("oversize wcode bounded")
    }
    out := normalizeWCode(raw)
    if len(out) > 1024 {
      t.Fatalf("wcode normalization amplified input: %d", len(out))
    }
  })
}

// FuzzSatMappedRule: SAT mapping rule JSON must decode or controlled-error.
func FuzzSatMappedRule(f *testing.F) {
  seeds := []string{`{}`, `{"x":1}`, ``, `null`, `[]`}
  for _, s := range seeds {
    f.Add([]byte(s))
  }
  f.Fuzz(func(t *testing.T, raw []byte) {
    if len(raw) > 64*1024 {
      t.Skip("oversize payload bounded")
    }
    _, _ = decodeSatVerbalMappedRule(raw)
  })
}

// FuzzSubmissionSitInMethod: submission payload method strings accept or
// controlled-reject — never panic.
func FuzzSubmissionSitInMethod(f *testing.F) {
  for _, s := range []string{"", "physical", "zoom", "PHYSICAL", " teacher_case ", "none", "bogus"} {
    f.Add(s)
  }
  f.Fuzz(func(t *testing.T, raw string) {
    if len(raw) > 256 {
      t.Skip("oversize method bounded")
    }
    m := raw
    _, _ = normalizeSubmissionSitInMethod(&m)
  })
}
