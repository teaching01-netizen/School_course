import { describe, expect, it } from "vitest";
import {
  groupByDay,
  splitMergedSessionValue,
  isDayGroupSelected,
  countSelectedAbsenceDays,
  countSelectedAbsenceDaysForGroup,
  countSelectedAbsenceDaysForScope,
  getSelectedSessionsForGroup,
  mergedSessionValue,
  uniqueValues,
  dayKey,
  instituteDateKey,
} from "../sessionGrouping";

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

  it("returns single value when no separator in input", () => {
    expect(splitMergedSessionValue("single")).toEqual(["single"]);
  });

  it("returns empty array for undefined input", () => {
    expect(splitMergedSessionValue(undefined)).toEqual([]);
  });
});

describe("isDayGroupSelected", () => {
  const group = {
    id: "g1",
    date: "2026-06-01",
    start_at: "2026-06-01T09:00:00+07:00",
    end_at: "2026-06-01T12:00:00+07:00",
    items: [
      { id: "s1", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00" },
      { id: "s2", start_at: "2026-06-01T10:00:00+07:00", end_at: "2026-06-01T12:00:00+07:00" },
    ],
  };

  it("returns true when all sessions selected", () => {
    expect(isDayGroupSelected(group, new Set(["s1", "s2"]))).toBe(true);
  });

  it("returns false when only some selected", () => {
    expect(isDayGroupSelected(group, new Set(["s1"]))).toBe(false);
  });

  it("returns true when no items and nothing selected (vacuous truth)", () => {
    expect(isDayGroupSelected({ ...group, items: [] }, new Set())).toBe(true);
  });
});

describe("countSelectedAbsenceDays", () => {
  it("counts fully-selected day groups across subjects", () => {
    const groups = [
      {
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Math",
        course_id: "c1",
        course_code: "MATH101",
        course_name: "Math 101",
        absence_limit_reached: false,
        sessions: [
          { id: "ms1", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00", date: "2026-06-01", already_absent: false },
          { id: "ms2", start_at: "2026-06-01T10:00:00+07:00", end_at: "2026-06-01T11:00:00+07:00", date: "2026-06-01", already_absent: false },
          { id: "ms3", start_at: "2026-06-02T09:00:00+07:00", end_at: "2026-06-02T10:00:00+07:00", date: "2026-06-02", already_absent: false },
        ],
      },
    ];
    expect(countSelectedAbsenceDays(groups, new Set(["ms1", "ms2", "ms3"]))).toBe(2);
    expect(countSelectedAbsenceDaysForGroup(groups[0], new Set(["ms1", "ms2", "ms3"]))).toBe(2);
  });

  it("counts 0 when no sessions selected", () => {
    const groups = [
      {
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Math",
        course_id: "c1",
        course_code: "MATH101",
        course_name: "Math 101",
        absence_limit_reached: false,
        sessions: [
          { id: "ms1", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00", date: "2026-06-01", already_absent: false },
        ],
      },
    ];
    expect(countSelectedAbsenceDays(groups, new Set())).toBe(0);
  });

  it("does not count an already-absent session even if its stale id remains selected", () => {
    const groups = [
      {
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Math",
        course_id: "c1",
        course_code: "MATH101",
        course_name: "Math 101",
        absence_limit_reached: false,
        sessions: [
          { id: "reported", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00", date: "2026-06-01", already_absent: true },
        ],
      },
    ];

    expect(countSelectedAbsenceDays(groups, new Set(["reported"]))).toBe(0);
  });

  it("counts a shared merge-group day once across both source courses", () => {
    const groups = [
      {
        subject_id: "subj-reading",
        subject_code: "READ",
        subject_name: "Reading",
        course_id: "course-reading",
        course_code: "READ-1",
        course_name: "Reading",
        merge_group_id: "merge-1",
        merge_group_name: "SAT Verbal Rank 2 C3",
        absence_limit_reached: false,
        sessions: [
          { id: "reading-day-1", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00", date: "2026-06-01", already_absent: false },
          { id: "reading-day-2", start_at: "2026-06-02T09:00:00+07:00", end_at: "2026-06-02T10:00:00+07:00", date: "2026-06-02", already_absent: false },
        ],
      },
      {
        subject_id: "subj-writing",
        subject_code: "WRITE",
        subject_name: "Writing",
        course_id: "course-writing",
        course_code: "WRITE-1",
        course_name: "Writing",
        merge_group_id: "merge-1",
        merge_group_name: "SAT Verbal Rank 2 C3",
        absence_limit_reached: false,
        sessions: [
          { id: "writing-day-1", start_at: "2026-06-01T10:00:00+07:00", end_at: "2026-06-01T11:00:00+07:00", date: "2026-06-01", already_absent: false },
          { id: "writing-day-3", start_at: "2026-06-03T10:00:00+07:00", end_at: "2026-06-03T11:00:00+07:00", date: "2026-06-03", already_absent: false },
        ],
      },
    ];

    const selected = new Set(["reading-day-1", "writing-day-1", "reading-day-2", "writing-day-3"]);
    expect(countSelectedAbsenceDaysForScope(groups, selected, "merge:merge-1")).toBe(3);
    expect(countSelectedAbsenceDays(groups, selected)).toBe(3);
  });
});

describe("getSelectedSessionsForGroup", () => {
  const group = {
    subject_id: "subj-1",
    subject_code: "MATH",
    subject_name: "Math",
    course_id: "c1",
    course_code: "MATH101",
    course_name: "Math 101",
    absence_limit_reached: false,
    sessions: [
      { id: "s3", start_at: "2026-06-02T09:00:00+07:00", end_at: "2026-06-02T10:00:00+07:00", date: "2026-06-02", already_absent: false },
      { id: "s1", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00", date: "2026-06-01", already_absent: false },
      { id: "s2", start_at: "2026-06-01T10:00:00+07:00", end_at: "2026-06-01T11:00:00+07:00", date: "2026-06-01", already_absent: false },
    ],
  };

  it("filters and sorts selected sessions by start_at", () => {
    const result = getSelectedSessionsForGroup(group, new Set(["s1", "s3"]));
    expect(result).toHaveLength(2);
    expect(result[0].id).toBe("s1");
    expect(result[1].id).toBe("s3");
  });

  it("returns empty array when no matches", () => {
    const result = getSelectedSessionsForGroup(group, new Set(["nonexistent"]));
    expect(result).toEqual([]);
  });
});

describe("mergedSessionValue", () => {
  it("joins ids with pipe", () => {
    expect(mergedSessionValue([{ id: "a" }, { id: "b" }])).toBe("a|b");
  });

  it("returns single id unchanged", () => {
    expect(mergedSessionValue([{ id: "a" }])).toBe("a");
  });

  it("returns empty string for empty array", () => {
    expect(mergedSessionValue([])).toBe("");
  });
});

describe("uniqueValues", () => {
  it("deduplicates strings", () => {
    expect(uniqueValues(["a", "b", "a", "c"])).toEqual(["a", "b", "c"]);
  });

  it("returns empty for empty input", () => {
    expect(uniqueValues([])).toEqual([]);
  });

  it("preserves order of first occurrence", () => {
    expect(uniqueValues(["c", "a", "b", "a"])).toEqual(["c", "a", "b"]);
  });
});

describe("instituteDateKey", () => {
  it("handles valid ISO date string", () => {
    const key = instituteDateKey("2026-06-02T18:30:00Z");
    expect(key).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });

  it("uses the Bangkok calendar date for UTC midnight-boundary sessions", () => {
    expect(instituteDateKey("2026-01-15T17:00:00Z")).toBe("2026-01-16");
  });

  it("falls back to first 10 chars for invalid dates", () => {
    expect(instituteDateKey("not-a-date")).toBe("not-a-date");
  });
});

describe("dayKey", () => {
  it("prefers explicit date over computed", () => {
    expect(dayKey({ id: "s1", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00", date: "2026-06-02" })).toBe("2026-06-02");
  });

  it("computes from start_at when no date provided", () => {
    const key = dayKey({ id: "s1", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00" });
    expect(key).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });
});
