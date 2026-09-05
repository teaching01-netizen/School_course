import { describe, expect, it } from "vitest";
import {
  findChosenSitInOverlaps,
  type ChosenSitInRow,
} from "../sitInResolution";
import type { SubjectSessions } from "../../types";

function iso(dayOffset: number, hour: number, minute = 0): string {
  const d = new Date(Date.UTC(2026, 8, 1 + dayOffset, hour, minute, 0));
  return d.toISOString();
}

function group(
  subjectId: string,
  courseId: string,
  name: string,
  session: SubjectSessions["sessions"][number],
  sitIn?: SubjectSessions["sit_in"],
): SubjectSessions {
  return {
    subject_id: subjectId,
    subject_code: subjectId.toUpperCase().slice(0, 4),
    subject_name: name,
    course_id: courseId,
    course_code: courseId.toUpperCase(),
    course_name: name,
    teacher_name: "Ms. T",
    sessions: [session],
    ...(sitIn ? { sit_in: sitIn } : {}),
  };
}

const dayKey = (offset: number) => `2026-09-${String(1 + offset).padStart(2, "0")}`;

function sitInCourse(subjectId: string): NonNullable<SubjectSessions["sit_in"]>["sit_in_course"] {
  return { id: `${subjectId}-makeup`, code: "MKUP", name: "Make-up", subject_code: subjectId, subject_name: "Make-up" };
}

const attendedGroup = group(
  "subject-physics",
  "course-physics",
  "Physics",
  {
    id: "s-physics-1",
    start_at: iso(1, 17, 30),
    end_at: iso(1, 19, 30),
    date: dayKey(1),
    already_absent: false,
  },
);

const missedMath = group(
  "subject-math",
  "course-math",
  "Mathematics",
  {
    id: "s-math-1",
    start_at: iso(0, 9, 0),
    end_at: iso(0, 11, 0),
    date: dayKey(0),
    already_absent: false,
  },
  {
    sit_in_method: "physical",
    sit_in_course: sitInCourse("subject-math"),
    available_sessions: [
      { id: "sit-math-early", start_at: iso(1, 17, 0), end_at: iso(1, 19, 0), course_id: "course-math-makeup", class_name: "Math make-up", subject_name: "Mathematics", course_name: "Math make-up" },
      { id: "sit-math-late", start_at: iso(1, 20, 0), end_at: iso(1, 22, 0), course_id: "course-math-makeup", class_name: "Math make-up", subject_name: "Mathematics", course_name: "Math make-up" },
    ],
  },
);

const missedChemistry = group(
  "subject-chem",
  "course-chem",
  "Chemistry",
  {
    id: "s-chem-1",
    start_at: iso(2, 9, 0),
    end_at: iso(2, 11, 0),
    date: dayKey(2),
    already_absent: false,
  },
  {
    sit_in_method: "physical",
    sit_in_course: sitInCourse("subject-chem"),
    available_sessions: [
      { id: "sit-chem-mid", start_at: iso(1, 18, 0), end_at: iso(1, 20, 0), course_id: "course-chem-makeup", class_name: "Chem make-up", subject_name: "Chemistry", course_name: "Chem make-up" },
    ],
  },
);

function row(key: string, groupValue: SubjectSessions, value: string): ChosenSitInRow {
  return { sessionKey: key, missedSessionId: key, group: groupValue, value };
}

describe("findChosenSitInOverlaps", () => {
  it("flags a chosen make-up that overlaps an attended class", () => {
    const overlaps = findChosenSitInOverlaps(
      [row("s-math-1", missedMath, "sit-math-early")],
      [attendedGroup, missedMath],
      new Set(["s-math-1"]),
    );
    expect(overlaps).toHaveLength(1);
    expect(overlaps[0].sessionKey).toBe("s-math-1");
    expect(overlaps[0].message).toMatch(/Overlaps with/i);
    expect(overlaps[0].message).toMatch(/Physics/i);
    expect(overlaps[0].message).toMatch(/Choose another time/i);
  });

  it("leaves a non-overlapping make-up alone", () => {
    const overlaps = findChosenSitInOverlaps(
      [row("s-math-1", missedMath, "sit-math-late")],
      [attendedGroup, missedMath],
      new Set(["s-math-1"]),
    );
    expect(overlaps).toHaveLength(0);
  });

  it("flags two chosen make-ups that overlap each other, one per row", () => {
    const overlaps = findChosenSitInOverlaps(
      [
        row("s-math-1", missedMath, "sit-math-early"),
        row("s-chem-1", missedChemistry, "sit-chem-mid"),
      ],
      // No attended classes here: this case is purely make-up vs make-up.
      [missedMath, missedChemistry],
      new Set(["s-math-1", "s-chem-1"]),
    );
    const keys = overlaps.map((overlap) => overlap.sessionKey).sort();
    expect(keys).toEqual(["s-chem-1", "s-math-1"]);
    for (const overlap of overlaps) {
      expect(overlap.message).toMatch(/Overlaps with your .* make-up/i);
    }
  });

  it("names the OTHER make-up in each overlapping row's message", () => {
    const overlaps = findChosenSitInOverlaps(
      [
        row("s-math-1", missedMath, "sit-math-early"),
        row("s-chem-1", missedChemistry, "sit-chem-mid"),
      ],
      [missedMath, missedChemistry],
      new Set(["s-math-1", "s-chem-1"]),
    );
    const mathMessage = overlaps.find((overlap) => overlap.sessionKey === "s-math-1")?.message ?? "";
    const chemMessage = overlaps.find((overlap) => overlap.sessionKey === "s-chem-1")?.message ?? "";
    // Math row must reference Chemistry's make-up, not itself.
    expect(mathMessage).toMatch(/your Chemistry make-up/i);
    expect(mathMessage).not.toMatch(/your Mathematics make-up/i);
    expect(chemMessage).toMatch(/your Mathematics make-up/i);
    expect(chemMessage).not.toMatch(/your Chemistry make-up/i);
  });
});
