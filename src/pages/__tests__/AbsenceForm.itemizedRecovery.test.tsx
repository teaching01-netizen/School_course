import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  renderPublicAbsenceForm,
  completeToClasses,
  selectClass,
  openDayOffset,
  fillEmailIfNeeded,
} from "./helpers/absenceFormHarness";
import { relativeDateKey, relativeISO } from "./fixtures/absenceFormFixtures";
import { ApiRequestError } from "@/api/client";
import type { SessionsInRangeResponse } from "@/types";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const mockNavigate = vi.hoisted(() => vi.fn());
vi.mock("react-router-dom", () => ({
  useNavigate: () => mockNavigate,
  useLocation: () => ({ pathname: "/absence" }),
}));

function makeUpSession(id: string, dayOffset: number, startHour: number, endHour: number, courseId: string) {
  return {
    id,
    start_at: relativeISO(startHour, 0, dayOffset),
    end_at: relativeISO(endHour, 0, dayOffset),
    course_id: courseId,
    class_name: "Make-up",
    subject_name: "Make-up",
    course_name: "Make-up",
  };
}

function subjectGroup(
  subjectId: string,
  courseId: string,
  name: string,
  missedStartHour: number,
  missedDayOffset: number,
  sitInCourseId: string,
  sitInStartHour: number,
  sitInSessionIds: string[],
  unavailableOldIds: string[],
): SessionsInRangeResponse["subjects"][number] {
  return {
    subject_id: subjectId,
    subject_code: subjectId.toUpperCase().slice(0, 4),
    subject_name: name,
    course_id: courseId,
    course_code: courseId.toUpperCase(),
    course_name: name,
    teacher_name: "Ms. T",
    sessions: [
      {
        id: `session-${subjectId}-1`,
        start_at: relativeISO(missedStartHour, 0, missedDayOffset),
        end_at: relativeISO(missedStartHour + 2, 0, missedDayOffset),
        date: relativeDateKey(missedDayOffset),
        already_absent: false,
      },
    ],
    sit_in: {
      sit_in_method: "physical",
      sit_in_course: {
        id: sitInCourseId,
        code: `${subjectId.toUpperCase()}202`,
        name: `${name} Make-up`,
        subject_code: subjectId.toUpperCase().slice(0, 4),
        subject_name: name,
      },
      available_sessions: sitInSessionIds.map((id, index) =>
        makeUpSession(id, 2 + index, sitInStartHour, sitInStartHour + 2, sitInCourseId),
      ),
      unavailable_sessions: unavailableOldIds.map((oldId) => ({
        session: { id: oldId, start_at: relativeISO(sitInStartHour, 0, 2), end_at: relativeISO(sitInStartHour + 2, 0, 2), course_id: sitInCourseId },
        reason_code: "sit_in_session_already_used" as const,
        missed_session_id: `session-${subjectId}-1`,
        reason: "already used",
      })),
    },
  };
}

// Two missed classes on two days. Each has one clear make-up (day +2, at
// different hours so the two choices never overlap each other), which will be
// taken by someone else before submission and must be re-chosen.
function freshSessions(): SessionsInRangeResponse {
  return {
    subjects: [
      subjectGroup("subject-math", "course-math", "Mathematics", 17, 0, "course-math-sitin", 9, ["sit-math-a"], []),
      subjectGroup("subject-chem", "course-chem", "Chemistry", 13, 1, "course-chem-sitin", 14, ["sit-chem-a"], []),
    ],
  };
}

function staleSessions(): SessionsInRangeResponse {
  return {
    subjects: [
      subjectGroup("subject-math", "course-math", "Mathematics", 17, 0, "course-math-sitin", 9, ["sit-math-b"], ["sit-math-a"]),
      subjectGroup("subject-chem", "course-chem", "Chemistry", 13, 1, "course-chem-sitin", 14, ["sit-chem-b"], ["sit-chem-a"]),
    ],
  };
}

async function chooseRecommendedForRow(user: ReturnType<typeof userEvent.setup>, rowIndex: number) {
  const openButton = screen.getAllByRole("button", { name: /choose a time|change time/i })[rowIndex];
  await user.click(openButton);
  const dialog = await screen.findByRole("dialog", { name: /choose a make-up time/i });
  const options = within(dialog).getAllByRole("radio");
  expect(options.length).toBeGreaterThan(0);
  await user.click(options[0]);
  await user.click(within(dialog).getByRole("button", { name: /use this time/i }));
}

describe("AbsenceForm — itemized recovery", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockNavigate.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("names every class whose make-up became unavailable, keeping each row to a new choice", async () => {
    let stale = false;
    renderPublicAbsenceForm(mockApiJson, {
      sessions: () => (stale ? staleSessions() : freshSessions()),
      submission: () => {
        stale = true;
        throw new ApiRequestError("Sit-in session already used", {
          code: "sit_in_session_already_used",
          status: 409,
        });
      },
    });
    const user = userEvent.setup();

    await completeToClasses(user);
    await selectClass(user, "Mathematics");
    await openDayOffset(user, 1);
    await selectClass(user, "Chemistry");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /your make-up/i });

    await chooseRecommendedForRow(user, 0);
    await chooseRecommendedForRow(user, 1);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /why will you be away/i });
    await user.click(screen.getByRole("radio", { name: "Appointment" }));
    await fillEmailIfNeeded(user, "student@example.edu");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /review your absence/i });
    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    // Both make-ups were taken: the notice lists BOTH affected classes, and
    // each row is left needing its own new choice — nothing else changed.
    await screen.findByRole("heading", { name: /your make-up/i });
    const recoveryNotice = screen.queryAllByRole("status").find((node) =>
      node.textContent?.includes("no longer available"),
    );
    expect(recoveryNotice?.textContent).toMatch(/mathematics/i);
    expect(recoveryNotice?.textContent).toMatch(/chemistry/i);
    expect(recoveryNotice?.textContent).toMatch(/choose another time for each/i);
    expect(screen.getAllByText(/the time you chose is no longer available/i).length).toBe(2);
    expect(screen.getAllByRole("button", { name: /choose a time/i }).length).toBe(2);
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /^continue$/i })).toBeDisabled();
    });

    // Choosing the new suggestion for each row clears both flags and unlocks the plan.
    await chooseRecommendedForRow(user, 0);
    await chooseRecommendedForRow(user, 1);
    await waitFor(() => {
      expect(screen.queryByText(/no longer available/i)).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: /^continue$/i })).toBeEnabled();
    });
  });
});
