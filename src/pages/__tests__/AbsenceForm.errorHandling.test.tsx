import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  renderPublicAbsenceForm,
  completeToReview,
  completeToClasses,
  selectClass,
  startReport,
  confirmIdentity,
  fillEmailIfNeeded,
  sendParentCode,
  enterCode,
} from "./helpers/absenceFormHarness";
import { ApiRequestError } from "@/api/client";
import type { SessionsInRangeResponse } from "@/types";
import { relativeDateKey, relativeISO } from "./fixtures/absenceFormFixtures";

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

const PHYSICAL_SESSIONS: SessionsInRangeResponse = {
  subjects: [
    {
      subject_id: "subject-math",
      subject_code: "MATH",
      subject_name: "Mathematics",
      course_id: "course-math",
      course_code: "MATH201",
      course_name: "Mathematics",
      sessions: [
        {
          id: "session-math-1",
          start_at: relativeISO(17, 0, 0),
          end_at: relativeISO(19, 0, 0),
          date: relativeDateKey(0),
          already_absent: false,
        },
      ],
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: {
          id: "course-sitin",
          code: "MATH202",
          name: "Mathematics Make-up",
          subject_code: "MATH",
          subject_name: "Mathematics",
        },
        available_sessions: [
          {
            id: "sit-a",
            start_at: "2026-08-04T02:00:00Z",
            end_at: "2026-08-04T03:30:00Z",
            course_id: "course-sitin",
            class_name: "Mathematics Make-up",
            subject_name: "Mathematics",
            course_name: "Mathematics Make-up",
          },
        ],
      },
    },
  ],
};

const SUBMISSION_RESPONSE = {
  items: [
    {
      id: "abc12345",
      wcode: "W250389",
      status: "pending",
      course_id: "course-math",
      course_code: "MATH201",
      course_name: "Mathematics",
      subject_id: "subject-math",
      subject_code: "MATH",
      subject_name: "Mathematics",
      student_name: "Alex Student",
      date_from: "2026-08-03",
      date_to: "2026-08-03",
      reason: "Appointment",
      sit_in_method: "physical",
      sit_in_course_id: "course-sitin",
      missed_sessions: [
        {
          id: "missed-record",
          session_id: "session-math-1",
          course_id: "course-math",
          course_code: "MATH201",
          course_name: "Mathematics",
          subject_name: "Mathematics",
          start_at: "2026-08-03T02:00:00Z",
          end_at: "2026-08-03T03:30:00Z",
        },
      ],
      sit_ins: [
        {
          id: "sit-record",
          session_id: "sit-a",
          course_id: "course-sitin",
          course_code: "MATH202",
          course_name: "Mathematics Make-up",
          subject_name: "Mathematics",
          start_at: "2026-08-04T02:00:00Z",
          end_at: "2026-08-04T03:30:00Z",
        },
      ],
    },
  ],
};

describe("AbsenceForm — recovery & errors", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockNavigate.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("shows a calm not-found message next to the Student ID field", async () => {
    renderPublicAbsenceForm(mockApiJson, {
      lookup: () => {
        throw new ApiRequestError("Student not found", { code: "not_found", status: 404 });
      },
    });
    const user = userEvent.setup();

    const input = await screen.findByRole("textbox", { name: /student id/i });
    await user.type(input, "W250389");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));

    const alerts = await screen.findAllByRole("alert");
    const alert = alerts.find((node) => node.textContent?.includes("couldn't find that student ID"));
    expect(alert).toBeDefined();
    // Still on the same screen, next to the input that can fix it.
    expect(screen.getByRole("heading", { name: /report an absence/i })).toBeInTheDocument();
    // The primary action stays a quiet Continue next to the inline error.
    expect(screen.getByRole("button", { name: /^continue$/i })).toBeEnabled();
  });

  it("offers a direct retry when classes fail to load", async () => {
    let calls = 0;
    renderPublicAbsenceForm(mockApiJson, {
      sessions: () => {
        calls += 1;
        if (calls === 1) throw new Error("Couldn't load your classes");
        return { subjects: [] };
      },
    });
    const user = userEvent.setup();

    await completeToClasses(user);
    const alerts = await screen.findAllByRole("alert");
    const alert = alerts.find((node) => node.textContent?.includes("Couldn't load your classes"));
    expect(alert).toBeDefined();

    await user.click(screen.getByRole("button", { name: /try again/i }));
    await screen.findByText(/no upcoming classes/i);
  });

  it("never claims failure when the network dies mid-submission", async () => {
    renderPublicAbsenceForm(mockApiJson, {
      submission: () => { throw new TypeError("Failed to fetch"); },
    });
    const user = userEvent.setup();

    await completeToReview(user);
    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    const alerts = await screen.findAllByRole("alert");
    const notice = alerts.find((node) => node.textContent?.includes("couldn't confirm the submission"));
    expect(notice).toHaveTextContent(/we couldn't confirm the submission/i);
    expect(notice).toHaveTextContent(/it's safe to try again/i);
    expect(notice).toHaveTextContent(/you won't create a duplicate/i);
    // The review stays intact and editable.
    expect(screen.getByRole("heading", { name: /review your absence/i })).toBeInTheDocument();
  });

  it("explains an absence-limit rejection without a scary banner", async () => {
    renderPublicAbsenceForm(mockApiJson, {
      submission: () => {
        throw new ApiRequestError("Absence limit exceeded", { code: "absence_limit_exceeded", status: 403 });
      },
    });
    const user = userEvent.setup();

    await completeToReview(user);
    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    const alerts = await screen.findAllByRole("alert");
    const notice = alerts.find((node) => node.textContent?.includes("absence limit"));
    expect(notice).toHaveTextContent(/reached the absence limit/i);
    expect(screen.getByRole("heading", { name: /review your absence/i })).toBeInTheDocument();
  });

  it("recovers a stale make-up by invalidating only that class-day and recommending the next option", async () => {
    const original = PHYSICAL_SESSIONS.subjects[0];
    const nextSubject: SessionsInRangeResponse["subjects"][number] = {
      ...original,
      sit_in: {
        ...original.sit_in!,
        available_sessions: [
          {
            id: "sit-b",
            start_at: "2026-08-05T02:00:00Z",
            end_at: "2026-08-05T03:30:00Z",
            course_id: "course-sitin",
            class_name: "Mathematics Make-up",
            subject_name: "Mathematics",
            course_name: "Mathematics Make-up",
          },
        ],
        unavailable_sessions: [
          {
            session: { id: "sit-a", start_at: "2026-08-04T02:00:00Z", end_at: "2026-08-04T03:30:00Z", course_id: "course-sitin" },
            reason_code: "sit_in_session_already_used",
            missed_session_id: "session-math-1",
            reason: "already used",
          },
        ],
      },
    };
    let stale = false;
    renderPublicAbsenceForm(mockApiJson, {
      sessions: () => (stale ? { subjects: [nextSubject] } : PHYSICAL_SESSIONS),
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
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /your make-up/i });

    // One clear suggestion; the choice is explicit, never auto-booked.
    expect(screen.getAllByText(/suggested make-up/i).length).toBeGreaterThan(0);
    await user.click(screen.getByRole("button", { name: /choose a time/i }));
    const firstDialog = await screen.findByRole("dialog", { name: /choose a make-up time/i });
    await user.click(within(firstDialog).getByRole("button", { name: /use this time/i }));
    await user.click(screen.getByRole("button", { name: /^continue$/i }));

    await screen.findByRole("heading", { name: /why will you be away/i });
    await user.click(screen.getByRole("radio", { name: "Appointment" }));
    // The school has no email on file, so Details also asks for one.
    await fillEmailIfNeeded(user, "student@example.edu");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /review your absence/i });
    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    // Back on the plan: only the affected class-day asks for a new time; the
    // taken time is gone and the next option is the new suggestion.
    await screen.findByRole("heading", { name: /your make-up/i });
    // The recovery notice names the affected class rather than speaking in generalities.
    const recoveryNotices = screen.queryAllByRole("status").filter((node) =>
      node.textContent?.includes("no longer available"),
    );
    expect(recoveryNotices.length).toBeGreaterThan(0);
    const itemizedNotice = recoveryNotices.find((node) => node.textContent?.includes("Mathematics"));
    expect(itemizedNotice?.textContent).toMatch(/mathematics/i);
    expect(itemizedNotice?.textContent).toMatch(/choose another time/i);
    await waitFor(() => expect(screen.getByRole("button", { name: /choose a time/i })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: /choose a time/i }));
    const retryDialog = await screen.findByRole("dialog", { name: /choose a make-up time/i });
    expect(within(retryDialog).getByText(/recommended/i)).toBeInTheDocument();
    await user.click(within(retryDialog).getByRole("button", { name: /use this time/i }));
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /why will you be away/i });
  });

  it("hides unavailable make-up options instead of listing them as disabled", async () => {
    renderPublicAbsenceForm(mockApiJson, {
      sessions: PHYSICAL_SESSIONS,
      submission: SUBMISSION_RESPONSE,
    });
    const user = userEvent.setup();

    await completeToClasses(user);
    await selectClass(user, "Mathematics");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /your make-up/i });

    await user.click(screen.getByRole("button", { name: /choose a time/i }));
    const dialog = await screen.findByRole("dialog", { name: /choose a make-up time/i });
    expect(dialog).toHaveTextContent(/mathematics/i);
    expect(within(dialog).getAllByText(/recommended/i).length).toBeGreaterThan(0);
    expect(screen.queryByText(/unavailable/i)).not.toBeInTheDocument();
  });

  it("keeps the student on the page when verification expires before submit", async () => {
    renderPublicAbsenceForm(mockApiJson, {
      submission: () => {
        throw new ApiRequestError("Unauthorized", { code: "unauthorized", status: 401 });
      },
    });
    const user = userEvent.setup();

    await completeToReview(user);
    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    // Back on verification with an explanation — never a raw error.
    await screen.findByRole("heading", { name: /enter the code|confirm with a parent/i });
    expect(screen.getAllByText(/expired/i).length).toBeGreaterThan(0);
  });

  it("returns the student to Review after re-verifying an expired session", async () => {
    renderPublicAbsenceForm(mockApiJson, {
      submission: () => {
        throw new ApiRequestError("Unauthorized", { code: "unauthorized", status: 401 });
      },
    });
    const user = userEvent.setup();

    await completeToReview(user);
    await user.click(screen.getByRole("button", { name: /submit absence/i }));
    await screen.findByRole("heading", { name: /enter the code|confirm with a parent/i });

    // Re-verify with the parent — the selections were preserved, so the
    // student lands straight back on the Review screen they were on.
    await sendParentCode(user);
    await enterCode(user);

    await screen.findByRole("heading", { name: /review your absence/i });
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /submit absence/i })).toBeEnabled();
    });
    expect(screen.getByText(/mathematics/i)).toBeInTheDocument();
  });

  it("shows a friendly empty state when there are no reportable classes", async () => {
    renderPublicAbsenceForm(mockApiJson, { sessions: { subjects: [] } });
    const user = userEvent.setup();

    await completeToClasses(user);

    expect(await screen.findByText(/no upcoming classes/i)).toBeInTheDocument();
    expect(screen.getByText(/contact student services/i)).toBeInTheDocument();
    // Done exits to a fresh start — nothing to report.
    await user.click(screen.getByRole("button", { name: /^done$/i }));
    await screen.findByRole("heading", { name: /report an absence/i });
  });

  it("shows the offline banner on the parent screen and unblocks when connectivity returns", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();
    await startReport(user);
    await confirmIdentity(user);

    expect(screen.queryByText(/you're offline/i)).not.toBeInTheDocument();

    // Connectivity is driven by real window online/offline events via the
    // useConnectivity hook — dispatch them the way the browser would.
    act(() => window.dispatchEvent(new Event("offline")));
    expect(screen.getByText(/you're offline\. reconnect to send or verify the parent code/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^send code$/i })).toBeDisabled();

    act(() => window.dispatchEvent(new Event("online")));
    expect(screen.queryByText(/you're offline/i)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^send code$/i })).toBeEnabled();
  });
});