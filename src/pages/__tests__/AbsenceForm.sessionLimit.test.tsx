import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  renderPublicAbsenceForm,
  completeToClasses,
  selectClass,
  expandCalendar,
  openDayOffset,
} from "./helpers/absenceFormHarness";
import { relativeDateKey, relativeISO } from "./fixtures/absenceFormFixtures";
import type { SessionsInRangeResponse } from "@/types";
import type { SubjectSessions } from "@/features/absences/types";

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

function mathSession(id: string, dayOffset: number, alreadyAbsent = false) {
  return {
    id,
    start_at: relativeISO(17, 0, dayOffset),
    end_at: relativeISO(19, 0, dayOffset),
    date: relativeDateKey(dayOffset),
    already_absent: alreadyAbsent,
  };
}

function mathGroup(overrides: Partial<SubjectSessions> = {}): SubjectSessions {
  return {
    subject_id: "subject-math",
    subject_code: "MATH",
    subject_name: "Mathematics",
    course_id: "course-math",
    course_code: "MATH201",
    course_name: "Mathematics",
    teacher_name: "Ms. Jane",
    ...overrides,
    sessions: overrides.sessions ?? [mathSession("session-math-1", 0)],
  };
}

const LIMITED_SESSIONS: SessionsInRangeResponse = {
  subjects: [
    mathGroup({
      remaining_absence_days: 1,
      sessions: [mathSession("session-math-1", 0), mathSession("session-math-2", 2)],
    }),
  ],
};

const LIMIT_REACHED_SESSIONS: SessionsInRangeResponse = {
  subjects: [
    mathGroup({
      absence_limit_reached: true,
      sessions: [mathSession("session-math-1", 0)],
    }),
  ],
};

const ABSENT_SESSIONS: SessionsInRangeResponse = {
  subjects: [
    mathGroup({
      sessions: [mathSession("session-math-1", 0, true)],
    }),
  ],
};

describe("AbsenceForm — absence limits", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockNavigate.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("lets the student pick one class when one absence day remains", async () => {
    renderPublicAbsenceForm(mockApiJson, { sessions: LIMITED_SESSIONS });
    const user = userEvent.setup();

    await completeToClasses(user);
    // The calendar opens focused on today — the next reportable day.
    await expandCalendar(user);
    await selectClass(user, "Mathematics");
    // The choice stays visible next to the calendar, in a persistent summary.
    expect(screen.getByText(/1 class day selected/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^continue$/i })).toBeEnabled();
    expect(screen.getByRole("button", { name: /add another class/i })).toBeInTheDocument();
  });

  it("explains the limit at the moment of interaction instead of showing a quota dashboard", async () => {
    renderPublicAbsenceForm(mockApiJson, { sessions: LIMITED_SESSIONS });
    const user = userEvent.setup();

    await completeToClasses(user);
    await selectClass(user, "Mathematics");
    expect(screen.getByText(/1 class day selected/i)).toBeInTheDocument();

    // A second reportable class sits two days later on the calendar.
    await openDayOffset(user, 2);
    const second = screen.getByRole("checkbox", { name: /mathematics/i });
    await user.click(second);
    expect(second).not.toBeChecked();
    const notices = [...screen.queryAllByRole("status"), ...screen.queryAllByRole("alert")];
    const notice = notices.find((node) => node.textContent?.includes("can't report another absence"));
    expect(notice).toBeInTheDocument();
    expect(notice?.textContent).toMatch(/contact student services/i);
    // The UI never exposes "0/2" style quota language.
    expect(screen.queryByText(/0\s*\/\s*2/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/quota/i)).not.toBeInTheDocument();
  });

  it("marks a course at its limit and reveals help only when tapped", async () => {
    renderPublicAbsenceForm(mockApiJson, { sessions: LIMIT_REACHED_SESSIONS });
    const user = userEvent.setup();

    await completeToClasses(user);

    const limitRow = screen.getByRole("button", { name: /mathematics/i });
    expect(limitRow).toHaveTextContent(/absence limit reached/i);

    await user.click(limitRow);
    expect(screen.getByText(/you can't report another absence for mathematics/i)).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: /mathematics/i })).not.toBeInTheDocument();
  });

  it("shows already-reported classes as a quiet end state with no reportable option", async () => {
    renderPublicAbsenceForm(mockApiJson, { sessions: ABSENT_SESSIONS });
    const user = userEvent.setup();

    await completeToClasses(user);

    expect(screen.getByText(/already been reported/i)).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: /mathematics/i })).not.toBeInTheDocument();
    // Nothing reportable: the only action is leaving the flow.
    expect(screen.queryByRole("button", { name: /^continue$/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^done$/i })).toBeEnabled();
  });
});
