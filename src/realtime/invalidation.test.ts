import { describe, expect, it } from "vitest";
import { invalidationKeysForEvent } from "./invalidation";

describe("invalidationKeysForEvent", () => {
  it("invalidates every session-derived operational projection", () => {
    expect(invalidationKeysForEvent({ type: "session.updated", channel: "sessions:all", id: "session-1" })).toEqual([
      ["sessions"],
      ["attendance"],
      ["operations-calendar"],
      ["teacher-dashboards"],
    ]);
  });

  it("invalidates absence collections, matching detail, stats, and dashboards", () => {
    expect(invalidationKeysForEvent({ type: "absence.updated", channel: "absent:all", id: "absence-1" })).toEqual([
      ["absences"],
      ["absences", "detail", "absence-1"],
      ["absence-stats"],
      ["operations-calendar"],
      ["teacher-dashboards"],
    ]);
  });

  it("invalidates course and roster data together", () => {
    expect(invalidationKeysForEvent({ type: "course.updated", channel: "courses:all", id: "course-1" })).toEqual([
      ["courses"],
      ["course-rosters", "course-1"],
    ]);
  });

  it("ignores malformed or unrelated events", () => {
    expect(invalidationKeysForEvent({ type: "message", channel: "unknown" })).toEqual([]);
  });
});
