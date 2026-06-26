import { describe, expect, it } from "vitest";
import type { SubjectSessions } from "../../types";
import { buildSubmissionPayloads } from "../submissionPayload";

const baseGroup = {
  subject_id: "subj-1",
  subject_code: "MATH",
  subject_name: "Mathematics",
  course_id: "course-1",
  course_code: "MATH101",
  course_name: "Mathematics 101",
  absence_rate_exceeded: false,
  sessions: [
    { id: "missed-1", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00", date: "2026-06-01", already_absent: false },
  ],
} satisfies SubjectSessions;

describe("absence submission payload builder", () => {
  it("builds one payload with trimmed reason and selected sit-in session ids", () => {
    const result = buildSubmissionPayloads({
      lookupWcode: "W250389",
      sessions: [{ ...baseGroup, sit_in: { sit_in_method: "zoom" } }],
      selectedSubjectIds: ["subj-1"],
      selectedSessionIds: new Set(["missed-1"]),
      sitInSelections: { "missed-1": "sit-1|sit-2" },
      reason: "  Medical appointment  ",
      maxDateRangeDays: 30,
    });

    expect(result).toEqual({
      ok: true,
      payloads: [
        {
          subject_id: "subj-1",
          course_id: "course-1",
          date_from: "2026-06-01",
          date_to: "2026-06-01",
          reason: "Medical appointment",
          sit_in_method: "zoom",
          sit_in_course_id: "course-1",
          missed_session_ids: ["missed-1"],
          sit_in_session_ids: ["sit-1", "sit-2"],
        },
      ],
    });
  });

  it("rejects one course payload when selected physical sit-ins span multiple target courses", () => {
    const result = buildSubmissionPayloads({
      lookupWcode: "W250389",
      sessions: [
        {
          ...baseGroup,
          sessions: [
            ...baseGroup.sessions,
            { id: "missed-2", start_at: "2026-06-02T09:00:00+07:00", end_at: "2026-06-02T10:00:00+07:00", date: "2026-06-02", already_absent: false },
          ],
          sit_in: {
            sit_in_method: "physical",
            priorities: [
              {
                level: 1,
                label: "Priority 1",
                sit_in_course: { id: "target-a", code: "A", name: "Target A" },
                available_sessions: [{ id: "sit-a", start_at: "2026-06-03T09:00:00+07:00", end_at: "2026-06-03T10:00:00+07:00" }],
              },
              {
                level: 2,
                label: "Priority 2",
                sit_in_course: { id: "target-b", code: "B", name: "Target B" },
                available_sessions: [{ id: "sit-b", start_at: "2026-06-04T09:00:00+07:00", end_at: "2026-06-04T10:00:00+07:00" }],
              },
            ],
          },
        },
      ],
      selectedSubjectIds: ["subj-1"],
      selectedSessionIds: new Set(["missed-1", "missed-2"]),
      sitInSelections: { "missed-1": "sit-a", "missed-2": "sit-b" },
      reason: "Travel",
      maxDateRangeDays: 30,
    });

    expect(result).toEqual({
      ok: false,
      error: "Mathematics has sit-in selections from more than one priority class. Split them into separate submissions.",
    });
  });

  it("rejects a selected course when the selected dates exceed the configured range", () => {
    const result = buildSubmissionPayloads({
      lookupWcode: "W250389",
      sessions: [
        {
          ...baseGroup,
          sessions: [
            ...baseGroup.sessions,
            { id: "missed-2", start_at: "2026-06-10T09:00:00+07:00", end_at: "2026-06-10T10:00:00+07:00", date: "2026-06-10", already_absent: false },
          ],
          sit_in: { sit_in_method: "zoom" },
        },
      ],
      selectedSubjectIds: ["subj-1"],
      selectedSessionIds: new Set(["missed-1", "missed-2"]),
      sitInSelections: {},
      reason: "Travel",
      maxDateRangeDays: 3,
    });

    expect(result).toEqual({
      ok: false,
      error: "Mathematics spans more than 3 days. Split it into separate submissions.",
    });
  });
});
