import { describe, expect, it } from "vitest";
import type { SubjectSessions } from "../../types";
import {
  firstPriorityLevel,
  nextPriorityLevel,
  previousPriorityLevel,
  hasPriorityLevel,
  prioritiesForLevel,
  hasServerPriorityReveal,
  sitInForMissedSession,
  groupWithSitInForMissedSession,
  availableSessionsForMissedSessions,
  unavailableSessionsForMissedSession,
  rootAvailableSessionsForMissedSessions,
  resolveSitInSubjectName,
  getSitInCourseDisplayName,
  getPriorityTargetDisplayName,
  getCurrentSitInDisplayName,
  getReviewSitInLabel,
  getSitInSessionLabel,
  getSitInSessionGroupLabel,
} from "../sitInResolution";

const baseGroup = {
  subject_id: "subj-1",
  subject_code: "MATH",
  subject_name: "Mathematics",
  course_id: "course-1",
  course_code: "MATH101",
  course_name: "Mathematics 101",
  absence_rate_exceeded: false,
  sessions: [],
} satisfies SubjectSessions;

describe("firstPriorityLevel", () => {
  it("returns lowest level from priorities", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", priorities: [{ level: 3, label: "P3" }, { level: 1, label: "P1" }] } };
    expect(firstPriorityLevel(group)).toBe(1);
  });

  it("returns 1 when no priorities", () => {
    expect(firstPriorityLevel(baseGroup)).toBe(1);
  });

  it("returns 1 when sit_in is null", () => {
    expect(firstPriorityLevel({ ...baseGroup, sit_in: undefined })).toBe(1);
  });
});

describe("nextPriorityLevel", () => {
  it("returns next higher level", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", priorities: [{ level: 1, label: "P1" }, { level: 3, label: "P3" }, { level: 5, label: "P5" }] } };
    expect(nextPriorityLevel(group, 1)).toBe(3);
  });

  it("returns null when at top level", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", priorities: [{ level: 1, label: "P1" }, { level: 3, label: "P3" }] } };
    expect(nextPriorityLevel(group, 3)).toBeNull();
  });

  it("skips non-consecutive levels to find next", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", priorities: [{ level: 1, label: "P1" }, { level: 5, label: "P5" }] } };
    expect(nextPriorityLevel(group, 1)).toBe(5);
  });
});

describe("previousPriorityLevel", () => {
  it("returns previous lower level", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", priorities: [{ level: 1, label: "P1" }, { level: 3, label: "P3" }, { level: 5, label: "P5" }] } };
    expect(previousPriorityLevel(group, 3)).toBe(1);
  });

  it("returns null when at bottom level", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", priorities: [{ level: 1, label: "P1" }] } };
    expect(previousPriorityLevel(group, 1)).toBeNull();
  });

  it("skips non-consecutive levels to find previous", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", priorities: [{ level: 1, label: "P1" }, { level: 5, label: "P5" }] } };
    expect(previousPriorityLevel(group, 5)).toBe(1);
  });
});

describe("hasPriorityLevel", () => {
  it("returns true when level exists", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", priorities: [{ level: 2, label: "P2" }] } };
    expect(hasPriorityLevel(group, 2)).toBe(true);
  });

  it("returns false when level missing", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", priorities: [{ level: 1, label: "P1" }] } };
    expect(hasPriorityLevel(group, 2)).toBe(false);
  });

  it("returns false when sit_in is null", () => {
    expect(hasPriorityLevel(baseGroup, 1)).toBe(false);
  });
});

describe("prioritiesForLevel", () => {
  it("filters priorities by level", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", priorities: [{ level: 1, label: "P1a" }, { level: 2, label: "P2" }, { level: 1, label: "P1b" }] } };
    expect(prioritiesForLevel(group, 1)).toHaveLength(2);
  });

  it("returns empty array when no match", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", priorities: [{ level: 2, label: "P2" }] } };
    expect(prioritiesForLevel(group, 1)).toEqual([]);
  });
});

describe("hasServerPriorityReveal", () => {
  it("returns true when current_priority_level is set", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", current_priority_level: 1 } };
    expect(hasServerPriorityReveal(group)).toBe(true);
  });

  it("returns true when has_next_priority is set", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", has_next_priority: true } };
    expect(hasServerPriorityReveal(group)).toBe(true);
  });

  it("returns false when neither is set", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical" } };
    expect(hasServerPriorityReveal(group)).toBe(false);
  });

  it("returns false when sit_in is null", () => {
    expect(hasServerPriorityReveal(baseGroup)).toBe(false);
  });
});

describe("sitInForMissedSession", () => {
  it("returns override when missed session has specific sit-in", () => {
    const override = { sit_in_method: "zoom" as const };
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", sit_in_by_missed_session: { "missed-1": override } } };
    expect(sitInForMissedSession(group, "missed-1")).toBe(override);
  });

  it("returns group sit_in when no override exists", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical" } };
    expect(sitInForMissedSession(group, "missed-1")).toBe(group.sit_in);
  });

  it("returns group sit_in when sit_in_by_missed_session is empty", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", sit_in_by_missed_session: {} } };
    expect(sitInForMissedSession(group, "missed-1")).toBe(group.sit_in);
  });
});

describe("groupWithSitInForMissedSession", () => {
  it("returns new group with override sit_in when override exists", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical", sit_in_by_missed_session: { "missed-1": { sit_in_method: "zoom" } } } };
    const result = groupWithSitInForMissedSession(group, "missed-1");
    expect(result).not.toBe(group);
    expect(result.sit_in?.sit_in_method).toBe("zoom");
  });

  it("returns original group when no override", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical" } };
    expect(groupWithSitInForMissedSession(group, "missed-1")).toBe(group);
  });
});

describe("availableSessionsForMissedSessions", () => {
  it("returns all available when no session has missed_session_id", () => {
    const priority = { level: 1, label: "P1", available_sessions: [{ id: "s1" }, { id: "s2" }] };
    expect(availableSessionsForMissedSessions(priority, ["missed-1"])).toHaveLength(2);
  });

  it("filters by missed_session_id when present", () => {
    const priority = {
      level: 1, label: "P1",
      available_sessions: [
        { id: "s1", missed_session_id: "missed-1" },
        { id: "s2", missed_session_id: "missed-2" },
      ],
    };
    expect(availableSessionsForMissedSessions(priority, ["missed-1"])).toHaveLength(1);
  });

  it("returns empty when no matches", () => {
    const priority = {
      level: 1, label: "P1",
      available_sessions: [
        { id: "s1", missed_session_id: "missed-1" },
      ],
    };
    expect(availableSessionsForMissedSessions(priority, ["missed-3"])).toHaveLength(0);
  });
});

describe("unavailableSessionsForMissedSession", () => {
  it("returns all unavailable when none has missed_session_id", () => {
    const priority = { level: 1, label: "P1", unavailable_sessions: [{ id: "us1", session: { id: "s1" }, reason: "Conflict", reason_code: "conflict" }] };
    expect(unavailableSessionsForMissedSession(priority, "missed-1")).toHaveLength(1);
  });

  it("filters by missed_session_id when present", () => {
    const priority = {
      level: 1, label: "P1",
      unavailable_sessions: [
        { id: "us1", reason: "a", reason_code: "a", missed_session_id: "missed-1" },
        { id: "us2", reason: "b", reason_code: "b", missed_session_id: "missed-2" },
      ],
    };
    expect(unavailableSessionsForMissedSession(priority, "missed-1")).toHaveLength(1);
  });

  it("returns empty array when no match found", () => {
    const priority = {
      level: 1, label: "P1",
      unavailable_sessions: [
        { id: "us1", reason: "a", reason_code: "a", missed_session_id: "missed-1" },
      ],
    };
    expect(unavailableSessionsForMissedSession(priority, "missed-3")).toHaveLength(0);
  });
});

describe("rootAvailableSessionsForMissedSessions", () => {
  it("returns all available when none has missed_session_id", () => {
    const sitIn = { sit_in_method: "physical" as const, available_sessions: [{ id: "s1" }, { id: "s2" }] };
    expect(rootAvailableSessionsForMissedSessions(sitIn, ["missed-1"])).toHaveLength(2);
  });

  it("filters by missed_session_id when present", () => {
    const sitIn = {
      sit_in_method: "physical" as const,
      available_sessions: [
        { id: "s1", missed_session_id: "missed-1" },
        { id: "s2", missed_session_id: "missed-2" },
      ],
    };
    expect(rootAvailableSessionsForMissedSessions(sitIn, ["missed-1"])).toHaveLength(1);
  });

  it("returns empty array when no matches", () => {
    const sitIn = {
      sit_in_method: "physical" as const,
      available_sessions: [
        { id: "s1", missed_session_id: "missed-1" },
      ],
    };
    expect(rootAvailableSessionsForMissedSessions(sitIn, ["missed-3"])).toHaveLength(0);
  });

  it("returns empty array when sit_in is null", () => {
    expect(rootAvailableSessionsForMissedSessions(undefined, ["missed-1"])).toEqual([]);
  });
});

describe("resolveSitInSubjectName", () => {
  it("returns subject_name from sit_in_course", () => {
    const course = { id: "c1", code: "MATH301", name: "Calc III", subject_name: "Calculus" };
    expect(resolveSitInSubjectName(course, [])).toBe("Calculus");
  });

  it("looks up in allSubjects when sit_in_course has no subject_name", () => {
    const course = { id: "c1", code: "MATH301", name: "Calc III" };
    const allSubjects = [{ ...baseGroup, course_id: "c1", subject_name: "Calculus" }];
    expect(resolveSitInSubjectName(course, allSubjects)).toBe("Calculus");
  });

  it("returns undefined when nothing found", () => {
    const course = { id: "c1", code: "MATH301", name: "Calc III" };
    expect(resolveSitInSubjectName(course, [])).toBeUndefined();
  });

  it("returns undefined for null/undefined input", () => {
    expect(resolveSitInSubjectName(undefined, [])).toBeUndefined();
  });
});

describe("getSitInCourseDisplayName", () => {
  const allSubjects: SubjectSessions[] = [{ ...baseGroup, subject_name: "Advanced Math", course_id: "c2" }];

  it("resolves from resolveSitInSubjectName (subject_name > name > subject_code > fallback > code)", () => {
    expect(getSitInCourseDisplayName({ id: "c1", code: "MATH301", name: "Calc III", subject_name: "Calculus" }, "Fallback", [])).toBe("Calculus");
  });

  it("falls back to sit_in_course.name", () => {
    expect(getSitInCourseDisplayName({ id: "c1", code: "MATH301", name: "Calc III" }, "Fallback", [])).toBe("Calc III");
  });

  it("falls back to subject_code", () => {
    expect(getSitInCourseDisplayName({ id: "c1", code: "MATH301", subject_code: "MATH" }, "Fallback", [])).toBe("MATH");
  });

  it("falls back to fallbackSubjectName", () => {
    expect(getSitInCourseDisplayName({ id: "c1", code: "MATH301" }, "Fallback Class", [])).toBe("Fallback Class");
  });

  it("falls back to code", () => {
    expect(getSitInCourseDisplayName({ id: "c1", code: "MATH301" }, "", [])).toBe("MATH301");
  });

  it("returns empty string when everything empty", () => {
    expect(getSitInCourseDisplayName({ id: "c1", code: "" }, "", [])).toBe("");
  });
});

describe("getPriorityTargetDisplayName", () => {
  it("returns course name when priority has sit_in_course", () => {
    const priority = { level: 1, label: "P1", sit_in_course: { id: "c1", code: "MATH301", name: "Calc III", subject_name: "Calculus" } };
    expect(getPriorityTargetDisplayName(priority, "Fallback", [])).toBe("Calculus");
  });

  it("falls back to session fields when no sit_in_course", () => {
    const priority = { level: 1, label: "P1", available_sessions: [{ id: "s1", class_name: "Morning Class" }] };
    expect(getPriorityTargetDisplayName(priority, "Fallback", [])).toBe("Morning Class");
  });

  it("falls back to fallbackSubjectName when nothing available", () => {
    const priority = { level: 1, label: "P1" };
    expect(getPriorityTargetDisplayName(priority, "Generic Class", [])).toBe("Generic Class");
  });
});

describe("getCurrentSitInDisplayName", () => {
  it("returns 'Zoom' for zoom method", () => {
    expect(getCurrentSitInDisplayName({ sit_in_method: "zoom" }, [], "", [])).toBe("Zoom");
  });

  it("returns 'To arrange' for non-physical non-zoom", () => {
    expect(getCurrentSitInDisplayName({ sit_in_method: "teacher_case" }, [], "", [])).toBe("To arrange");
  });

  it("joins priority display names for physical with priorities", () => {
    const sitIn = { sit_in_method: "physical" as const, priorities: [{ level: 1, label: "P1", sit_in_course: { id: "c1", code: "MATH301", name: "Calc III", subject_name: "Calculus" } }, { level: 2, label: "P2", sit_in_course: { id: "c2", code: "PHYS101", name: "Physics", subject_name: "Physics" } }] };
    expect(getCurrentSitInDisplayName(sitIn, sitIn.priorities ?? [], "", [])).toBe("Calculus, Physics");
  });

  it("returns 'Not available' when priorities all empty", () => {
    const sitIn = { sit_in_method: "physical" as const, priorities: [{ level: 1, label: "P1", available_sessions: [] }] };
    expect(getCurrentSitInDisplayName(sitIn, sitIn.priorities ?? [], "", [])).toBe("Not available");
  });

  it("delegates to getSitInCourseDisplayName when no priorities", () => {
    const sitIn = { sit_in_method: "physical" as const, sit_in_course: { id: "c1", code: "MATH301", name: "Calc III" } };
    expect(getCurrentSitInDisplayName(sitIn, [], "Fallback", [])).toBe("Calc III");
  });
});

describe("getReviewSitInLabel", () => {
  const allSubjects: SubjectSessions[] = [];
  const missedSession = { id: "ms-1" };

  it("returns 'To arrange' when no sit_in", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: undefined };
    expect(getReviewSitInLabel(missedSession, group, {}, {}, {}, allSubjects)).toBe("To arrange");
  });

  it("returns 'Zoom' for zoom method", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "zoom" } };
    expect(getReviewSitInLabel(missedSession, group, {}, {}, {}, allSubjects)).toBe("Zoom");
  });

  it("returns 'To arrange' for teacher_case", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "teacher_case" } };
    expect(getReviewSitInLabel(missedSession, group, {}, {}, {}, allSubjects)).toBe("To arrange");
  });

  it("returns 'Not yet selected' for physical with no selection", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical" } };
    expect(getReviewSitInLabel(missedSession, group, {}, {}, {}, allSubjects)).toBe("Not yet selected");
  });

  it("returns 'Make-up class selected' when selection not found in root or priorities", () => {
    const group: SubjectSessions = { ...baseGroup, sit_in: { sit_in_method: "physical" } };
    expect(getReviewSitInLabel(missedSession, group, { "ms-1": "nonexistent" }, {}, {}, allSubjects)).toBe("Make-up class selected");
  });

  it("resolves root match when session is in root available_sessions", () => {
    const group: SubjectSessions = {
      ...baseGroup,
      subject_name: "Mathematics",
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: { id: "c1", code: "MATH301", name: "Calc III" },
        available_sessions: [{ id: "root-s1", start_at: "2026-06-03T09:00:00+07:00", end_at: "2026-06-03T10:00:00+07:00" }],
      },
    };
    const result = getReviewSitInLabel(missedSession, group, { "ms-1": "root-s1" }, {}, {}, allSubjects);
    expect(result).toContain("Calc III");
  });
});

describe("getSitInSessionLabel", () => {
  it("formats label with class name and datetime", () => {
    const session = { id: "s1", start_at: "2026-06-03T09:00:00+07:00", end_at: "2026-06-03T10:00:00+07:00", class_name: "Morning Calc" };
    const result = getSitInSessionLabel(session, { id: "c1", code: "MATH301", name: "Calc III" }, "Fallback", []);
    expect(result).toMatch(/Calc III —/);
  });

  it("falls back through subject_name, course_name, subject_code, course_code", () => {
    const session = { id: "s1", start_at: "2026-06-03T09:00:00+07:00", end_at: "2026-06-03T10:00:00+07:00", subject_name: "Algebra" };
    const result = getSitInSessionLabel(session, undefined, "Fallback", []);
    expect(result).toMatch(/Algebra —/);
  });

  it("falls back to fallbackSubjectName when nothing else", () => {
    const session = { id: "s1", start_at: "2026-06-03T09:00:00+07:00", end_at: "2026-06-03T10:00:00+07:00" };
    const result = getSitInSessionLabel(session, undefined, "Generic", []);
    expect(result).toMatch(/Generic —/);
  });
});

describe("getSitInSessionGroupLabel", () => {
  it("delegates to getSitInSessionLabel for single session", () => {
    const sessions = [{ id: "s1", start_at: "2026-06-03T09:00:00+07:00", end_at: "2026-06-03T10:00:00+07:00", class_name: "Morning Calc" }];
    const result = getSitInSessionGroupLabel(sessions, { id: "c1", code: "MATH301", name: "Calc III" }, "Fallback", []);
    expect(result).toMatch(/Calc III —/);
  });

  it("returns merged range for multiple sessions", () => {
    const sessions = [
      { id: "s1", start_at: "2026-06-03T09:00:00+07:00", end_at: "2026-06-03T10:00:00+07:00", date: "2026-06-03" },
      { id: "s2", start_at: "2026-06-03T11:00:00+07:00", end_at: "2026-06-03T12:00:00+07:00", date: "2026-06-03" },
    ];
    const result = getSitInSessionGroupLabel(sessions, { id: "c1", code: "MATH301", name: "Calc III" }, "Fallback", []);
    expect(result).toMatch(/Calc III —/);
  });
});
