import { describe, expect, it } from "vitest";
import { groupByDay, splitMergedSessionValue } from "../sessionGrouping";

describe("absence session grouping", () => {
  it("merges same-day sessions into a sorted day range", () => {
    const groups = groupByDay([
      { id: "later", start_at: "2026-06-02T11:00:00+07:00", end_at: "2026-06-02T12:00:00+07:00", date: "2026-06-02" },
      { id: "earlier", start_at: "2026-06-02T09:00:00+07:00", end_at: "2026-06-02T10:00:00+07:00", date: "2026-06-02" },
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0]).toMatchObject({
      id: "earlier|later",
      date: "2026-06-02",
      start_at: "2026-06-02T09:00:00+07:00",
      end_at: "2026-06-02T12:00:00+07:00",
    });
  });

  it("uses the institute day when a session has no explicit date", () => {
    const groups = groupByDay([
      { id: "late-bangkok", start_at: "2026-06-01T18:30:00Z", end_at: "2026-06-01T19:30:00Z" },
    ]);

    expect(groups[0].date).toBe("2026-06-02");
  });

  it("drops empty merged session fragments", () => {
    expect(splitMergedSessionValue("a||b|")).toEqual(["a", "b"]);
  });
});
