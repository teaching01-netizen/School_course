import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  renderPublicAbsenceForm,
  completeToClasses,
  openDayOffset,
  selectClass,
} from "./helpers/absenceFormHarness";
import { relativeDateKey, relativeISO } from "./fixtures/absenceFormFixtures";
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

function availableSession(
  id: string,
  startDayOffset: number,
  startHour: number,
  startMinute: number,
  endHour: number,
  endMinute: number,
  courseId: string,
) {
  return {
    id,
    start_at: relativeISO(startHour, startMinute, startDayOffset),
    end_at: relativeISO(endHour, endMinute, startDayOffset),
    course_id: courseId,
    class_name: "Make-up",
    subject_name: "Make-up",
    course_name: "Make-up",
  };
}

// Two missed classes (Math today, Chemistry tomorrow). Math's recommended
// make-up (day +2, 16:00-18:00) overlaps Chemistry's recommended make-up
// (day +2, 16:30-18:30); both subjects also offer clear alternatives later.
const OVERLAP_SESSIONS: SessionsInRangeResponse = {
  subjects: [
    {
      subject_id: "subject-math",
      subject_code: "MATH",
      subject_name: "Mathematics",
      course_id: "course-math",
      course_code: "MATH201",
      course_name: "Mathematics",
      teacher_name: "Ms. Jane",
      sessions: [
        {
          id: "session-math-1",
          start_at: relativeISO(9, 0, 0),
          end_at: relativeISO(11, 0, 0),
          date: relativeDateKey(0),
          already_absent: false,
        },
      ],
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: {
          id: "course-math-sitin",
          code: "MATH202",
          name: "Mathematics Make-up",
          subject_code: "MATH",
          subject_name: "Mathematics",
        },
        available_sessions: [
          availableSession("sit-math-a", 2, 16, 0, 18, 0, "course-math-sitin"),
          availableSession("sit-math-c", 3, 16, 0, 18, 0, "course-math-sitin"),
        ],
      },
    },
    {
      subject_id: "subject-chem",
      subject_code: "CHEM",
      subject_name: "Chemistry",
      course_id: "course-chem",
      course_code: "CHEM101",
      course_name: "Chemistry",
      teacher_name: "Dr. Nui",
      sessions: [
        {
          id: "session-chem-1",
          start_at: relativeISO(13, 0, 1),
          end_at: relativeISO(15, 0, 1),
          date: relativeDateKey(1),
          already_absent: false,
        },
      ],
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: {
          id: "course-chem-sitin",
          code: "CHEM102",
          name: "Chemistry Make-up",
          subject_code: "CHEM",
          subject_name: "Chemistry",
        },
        available_sessions: [
          availableSession("sit-chem-b", 2, 16, 30, 18, 30, "course-chem-sitin"),
          availableSession("sit-chem-d", 3, 19, 0, 21, 0, "course-chem-sitin"),
        ],
      },
    },
  ],
};

async function chooseOptionFromSheet(
  user: ReturnType<typeof userEvent.setup>,
  rowIndex: number,
  optionName: RegExp,
) {
  const openButton = screen.getAllByRole("button", { name: /choose a time|change time/i })[rowIndex];
  await user.click(openButton);
  const dialog = await screen.findByRole("dialog", { name: /choose a make-up time/i });
  await user.click(within(dialog).getByRole("radio", { name: optionName }));
  await user.click(within(dialog).getByRole("button", { name: /use this time/i }));
}

/** Pick the overlapping recommended make-up for a row (the first option). */
function recommendedName(): RegExp {
  return /recommended/i;
}

describe("AbsenceForm — make-up overlap validation", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockNavigate.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("flags two chosen make-ups that overlap and keeps Continue blocked until one changes", async () => {
    renderPublicAbsenceForm(mockApiJson, { sessions: OVERLAP_SESSIONS });
    const user = userEvent.setup();

    await completeToClasses(user);
    await selectClass(user, "Mathematics");
    await openDayOffset(user, 1);
    await selectClass(user, "Chemistry");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /your make-up/i });

    // Choose each recommended (overlapping) make-up: Math day+2 16:00-18:00,
    // Chemistry day+2 16:30-18:30.
    await chooseOptionFromSheet(user, 0, recommendedName());
    await chooseOptionFromSheet(user, 1, recommendedName());

    // Each row names the OTHER class's make-up it clashes with; the plan cannot continue.
    await waitFor(() => {
      expect(screen.getByText(/overlaps with your chemistry make-up/i)).toBeInTheDocument();
      expect(screen.getByText(/overlaps with your mathematics make-up/i)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /^continue$/i })).toBeDisabled();
    });

    // Moving Chemistry to its clear alternative (day+3 19:00-21:00) resolves both flags.
    await chooseOptionFromSheet(user, 1, /19:00/);

    await waitFor(() => {
      expect(screen.queryByText(/overlaps with your .* make-up/i)).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: /^continue$/i })).toBeEnabled();
    });
  });
});
