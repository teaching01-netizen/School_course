import { describe, expect, it } from "vitest";
import { buildImpactIssue } from "../test/builders";
import { groupIssues } from "./grouping";

describe("groupIssues", () => {
  it("critical issues appear first", () => {
    const warning = buildImpactIssue({ id: "w1", severity: "warning", status: "open" });
    const critical = buildImpactIssue({ id: "c1", severity: "critical", status: "open" });
    const result = groupIssues([warning, critical]);
    expect(result[0].id).toBe("c1");
    expect(result[1].id).toBe("w1");
  });

  it("needs_review issues are grouped separately from critical", () => {
    const review = buildImpactIssue({ id: "r1", severity: "warning", status: "needs_review" });
    const critical = buildImpactIssue({ id: "c1", severity: "critical", status: "open" });
    const result = groupIssues([review, critical]);
    expect(result[0].id).toBe("c1");
    expect(result[1].id).toBe("r1");
  });

  it("warning issues do not duplicate manual-review items", () => {
    const reviewWarning = buildImpactIssue({ id: "r1", severity: "warning", status: "needs_review" });
    const openWarning = buildImpactIssue({ id: "w1", severity: "warning", status: "open" });
    const result = groupIssues([reviewWarning, openWarning]);
    expect(result.map((i) => i.id)).toEqual(["r1", "w1"]);
  });

  it("stable server ordering is retained inside each group", () => {
    const c1 = buildImpactIssue({ id: "c1", severity: "critical", status: "open" });
    const c2 = buildImpactIssue({ id: "c2", severity: "critical", status: "open" });
    const c3 = buildImpactIssue({ id: "c3", severity: "critical", status: "open" });
    const result = groupIssues([c2, c3, c1]);
    expect(result.map((i) => i.id)).toEqual(["c2", "c3", "c1"]);
  });

  it("resolved issues are excluded from unresolved groups", () => {
    const resolved = buildImpactIssue({ id: "resolved-1", severity: "critical", status: "resolved" });
    const open = buildImpactIssue({ id: "open-1", severity: "critical", status: "open" });
    const result = groupIssues([resolved, open]);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe("open-1");
  });

  it("dismissed issues are excluded from unresolved groups", () => {
    const dismissed = buildImpactIssue({ id: "d1", severity: "warning", status: "dismissed" });
    const open = buildImpactIssue({ id: "o1", severity: "warning", status: "open" });
    const result = groupIssues([dismissed, open]);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe("o1");
  });

  it("superseded issues are excluded from unresolved groups", () => {
    const superseded = buildImpactIssue({ id: "s1", severity: "critical", status: "superseded" });
    const open = buildImpactIssue({ id: "o1", severity: "critical", status: "open" });
    const result = groupIssues([superseded, open]);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe("o1");
  });

  it("empty input produces empty output", () => {
    expect(groupIssues([])).toEqual([]);
  });

  it("all severities and statuses combine correctly", () => {
    const items = [
      buildImpactIssue({ id: "w-open", severity: "warning", status: "open" }),
      buildImpactIssue({ id: "c-open", severity: "critical", status: "open" }),
      buildImpactIssue({ id: "w-review", severity: "warning", status: "needs_review" }),
      buildImpactIssue({ id: "c-review", severity: "critical", status: "needs_review" }),
      buildImpactIssue({ id: "w-resolved", severity: "warning", status: "resolved" }),
      buildImpactIssue({ id: "c-dismissed", severity: "critical", status: "dismissed" }),
    ];
    const grouped = groupIssues(items);
    // c-open, c-review (critical first), then w-review (needs_review non-critical), then w-open (warning non-review)
    // w-resolved and c-dismissed are excluded
    expect(grouped.map((i) => i.id)).toEqual(["c-open", "c-review", "w-review", "w-open"]);
  });

  it("missing or unexpected severity falls into warning group (non-critical, non-review)", () => {
    const unknown = buildImpactIssue({ id: "u1", severity: "warning" as const, status: "open" });
    const result = groupIssues([unknown]);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe("u1");
  });
});
