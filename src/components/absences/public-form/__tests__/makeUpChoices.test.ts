import { describe, expect, it } from "vitest";
import type { SubjectSessions } from "@/types";
import { publicMakeUpChoices } from "../makeUpChoices";

const missed = { id: "missed", date: "2026-10-20", start_at: "2026-10-20T09:00:00+07:00", end_at: "2026-10-20T10:00:00+07:00", already_absent: false };
const first = { id: "first", start_at: "2026-10-24T09:00:00+07:00", end_at: "2026-10-24T10:00:00+07:00" };
const second = { id: "second", start_at: "2026-10-25T09:00:00+07:00", end_at: "2026-10-25T10:00:00+07:00" };
const group: SubjectSessions = {
  subject_id: "english", subject_code: "ENG", subject_name: "English", course_id: "course", course_code: "ENG1", course_name: "English 1",
  sessions: [missed], sit_in: { sit_in_method: "physical", current_priority_level: 1, has_next_priority: true,
    priorities: [{ level: 1, label: "First", sit_in_course: { id: "target1", code: "T1", name: "English 2" }, available_sessions: [first] }],
  },
};

function choices(overrides: Partial<Parameters<typeof publicMakeUpChoices>[0]> = {}) {
  return publicMakeUpChoices({ group, sessionIds: [missed.id], history: undefined, selectedLevel: undefined, sessions: [group],
    selectedSubjectIds: ["english"], selectedSessionIds: new Set([missed.id]), selections: {}, label: "English", ...overrides });
}

describe("public make-up choices", () => {
  it("only appends disclosed batches and preserves the priority of each offered choice", () => {
    const next: SubjectSessions = { ...group, sit_in: { sit_in_method: "physical", current_priority_level: 3, has_next_priority: false,
      priorities: [{ level: 3, label: "Third", sit_in_course: { id: "target3", code: "T3", name: "English 3" }, available_sessions: [second] }],
    } };
    expect(choices().options.map((option) => option.value)).toEqual(["first"]);
    const result = choices({ history: { 1: group, 3: next }, selectedLevel: 1, selections: { missed: "first" } });
    expect(result.options.map((option) => [option.value, option.priorityLevel, option.disabled])).toEqual([["first", 1, false], ["second", 3, false]]);
    expect(result.latestLevel).toBe(3);
    expect(result.hasMore).toBe(false);
    expect(result.groupsByLevel.get(1)?.sit_in?.priorities?.[0].sit_in_course?.id).toBe("target1");
  });

  it("identifies the absence that owns a choice without reserving hidden course selections", () => {
    const other = { ...missed, id: "other", date: "2026-10-21", start_at: "2026-10-21T09:00:00+07:00", end_at: "2026-10-21T10:00:00+07:00" };
    const withOther = { ...group, sessions: [missed, other] };
    const result = choices({ group: withOther, sessions: [withOther], selectedSessionIds: new Set(["missed", "other"]), selections: { other: "first" } });
    expect(result.options[0]).toMatchObject({ disabled: true, description: "Chosen for another absence in this form", conflictDetails: expect.stringContaining("21 Oct") });
    const ignored = choices({ selections: { hidden: "first" } });
    expect(ignored.options[0].disabled).toBe(false);
  });

  it("keeps a self-owned choice valid but explains a timetable overlap", () => {
    expect(choices({ selections: { missed: "first" } }).options[0].disabled).toBe(false);
    const chemistry: SubjectSessions = { ...group, subject_id: "chemistry", course_id: "chemistry-course", subject_name: "Chemistry",
      sessions: [{ ...first, id: "chemistry-class", date: "2026-10-24", already_absent: false }], sit_in: undefined };
    const result = choices({ sessions: [group, chemistry], selections: { missed: "first" } });
    expect(result.options[0]).toMatchObject({ disabled: true, description: "Overlaps another scheduled class", conflictDetails: expect.stringContaining("Chemistry") });
  });

  it("invalidates a historical choice using fresh server conflicts", () => {
    const fresh: SubjectSessions = { ...group, sit_in: { ...group.sit_in!, priorities: [{ ...group.sit_in!.priorities![0], available_sessions: [{ ...first, conflict: { absence_id: "private-id", absence_subject_name: "English" } }] }] } };
    const result = choices({ group: fresh, sessions: [fresh], history: { 1: group }, selections: { missed: "first" } });
    expect(result.options[0]).toMatchObject({ disabled: true, description: "Already assigned to another absence" });
    expect(JSON.stringify(result.options)).not.toContain("private-id");
  });

  it("keeps one selected merged class when a later response changes the order of its sessions", () => {
    const target = { id: "target", code: "T", name: "English", merge_group_id: "merged-target" };
    const partA = { ...first, id: "part-a" };
    const partB = { ...first, id: "part-b", start_at: first.end_at, end_at: "2026-10-24T11:00:00+07:00" };
    const initial: SubjectSessions = { ...group, sit_in: { ...group.sit_in!, priorities: [{ level: 1, label: "First", sit_in_course: target, available_sessions: [partA, partB] }] } };
    const next: SubjectSessions = { ...group, sit_in: { sit_in_method: "physical", current_priority_level: 3, has_next_priority: false,
      priorities: [{ level: 3, label: "Third", sit_in_course: target, available_sessions: [partB, partA] }],
    } };
    const result = choices({ group: initial, sessions: [initial], history: { 1: initial, 3: next }, selections: { missed: "part-a|part-b" } });
    expect(result.options).toHaveLength(1);
    expect(result.options[0]).toMatchObject({ value: "part-a|part-b", disabled: false });
  });
});
