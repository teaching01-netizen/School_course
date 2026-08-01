import { describe, expect, it } from "vitest";
import { impactQueryKeys } from "../queryKeys";
import type { QueueURLState } from "../types";

describe("impactQueryKeys", () => {
  it("all returns stable base key", () => {
    expect(impactQueryKeys.all).toEqual(["schedule-impact"]);
  });

  it("queue includes full filter state", () => {
    const filters: QueueURLState = {
      view: "queue",
      query: "Alice",
      severity: "critical",
      status: "open",
      offset: 25,
      limit: 50,
    };
    const key = impactQueryKeys.queue(filters);
    expect(key).toEqual(["schedule-impact", "queue", filters]);
  });

  it("queue different filters produce different keys", () => {
    const a = impactQueryKeys.queue({
      view: "queue", query: "", severity: "", status: "all", offset: 0, limit: 25,
    });
    const b = impactQueryKeys.queue({
      view: "queue", query: "Alice", severity: "", status: "all", offset: 0, limit: 25,
    });
    expect(a).not.toEqual(b);
  });

  it("queueSimple creates key from primitives", () => {
    const key = impactQueryKeys.queueSimple("open", "test", "critical");
    expect(key).toEqual(["schedule-impact", "queue", { status: "open", q: "test", severity: "critical" }]);
  });

  it("issue key includes issue ID", () => {
    expect(impactQueryKeys.issue("abc-123")).toEqual(["schedule-impact", "issue", "abc-123"]);
  });

  it("candidates key includes issue ID", () => {
    expect(impactQueryKeys.candidates("abc-123")).toEqual(["schedule-impact", "candidates", "abc-123"]);
  });

  it("processing is a stable array", () => {
    expect(impactQueryKeys.processing).toEqual(["schedule-impact", "processing"]);
  });

  it("history includes filter object", () => {
    const filters = { q: "test", date: "2026-07-31" };
    expect(impactQueryKeys.history(filters)).toEqual(["schedule-impact", "history", filters]);
  });

  it("navSummary is a stable array", () => {
    expect(impactQueryKeys.navSummary).toEqual(["schedule-impact", "nav-summary"]);
  });

  it("allQueues is a stable array prefix", () => {
    expect(impactQueryKeys.allQueues).toEqual(["schedule-impact", "queue"]);
  });

  it("issue keys are unique per issue", () => {
    const key1 = impactQueryKeys.issue("issue-1");
    const key2 = impactQueryKeys.issue("issue-2");
    expect(key1).not.toEqual(key2);
  });
});
