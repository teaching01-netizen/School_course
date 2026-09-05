import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  renderPublicAbsenceForm,
  startReport,
  confirmIdentity,
  sendParentCode,
  enterCode,
  completeToClasses,
  completeToReview,
  selectClass,
  fillEmailIfNeeded,
} from "./helpers/absenceFormHarness";
import {
  CRM_EMAIL_STUDENT,
  MANUAL_EMAIL_STUDENT,
} from "./fixtures/absenceFormFixtures";
import { ABSENCE_DRAFT_STORAGE_KEY } from "@/features/absences/storage/absenceDraftStorage";
import { STUDENT_RESUME_STORAGE_KEY } from "@/features/absences/constants";

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
      sit_in_method: "none",
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
    },
  ],
};

describe("AbsenceForm — student identity, privacy & recovery", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockNavigate.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("masks the parent phone and never shows the full name before verification", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    await startReport(user);

    // The full name is never shown pre-verification; only a masked hint is.
    expect(screen.queryByText("Alex Student")).not.toBeInTheDocument();
    expect(screen.getByText("A***")).toBeInTheDocument();

    await confirmIdentity(user);
    // The phone is masked; the copy is student language.
    expect(screen.getAllByText(/••78/).length).toBeGreaterThan(0);
  });

  it("keeps the schedule hidden until parent confirmation succeeds", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    await startReport(user);
    expect(screen.queryByRole("heading", { name: /which class will you miss/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/session-math/i)).not.toBeInTheDocument();
  });

  it("skips the email screen when an email is already on file", async () => {
    renderPublicAbsenceForm(mockApiJson, { lookup: CRM_EMAIL_STUDENT });
    const user = userEvent.setup();

    await startReport(user, CRM_EMAIL_STUDENT.wcode);
    await confirmIdentity(user);
    await sendParentCode(user);
    await enterCode(user);

    await screen.findByRole("heading", { name: /which class will you miss/i });
    expect(screen.queryByRole("heading", { name: /where should we send updates/i })).not.toBeInTheDocument();
  });

  it("requests an email only when the school is missing one", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    await startReport(user);
    await confirmIdentity(user);
    await sendParentCode(user);
    await enterCode(user);

    // The email screen appears because the school has no address on file.
    await screen.findByRole("heading", { name: /where should we send updates/i });
    await fillEmailIfNeeded(user);
    await screen.findByRole("heading", { name: /which class will you miss/i });
  });

  it("blocks continuing with an invalid email", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    await startReport(user);
    await confirmIdentity(user);
    await sendParentCode(user);
    await enterCode(user);
    await screen.findByRole("heading", { name: /where should we send updates/i });

    const emailInput = screen.getByRole("textbox", { name: /^email$/i });
    await user.type(emailInput, "not-an-email");
    expect(screen.getByRole("button", { name: /^continue$/i })).toBeDisabled();
  });

  it("Not me keeps the ID focused and selected for one-keystroke correction", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    await startReport(user);
    await user.click(screen.getByRole("button", { name: /not me/i }));

    await screen.findByRole("heading", { name: /report an absence/i });
    const input = screen.getByRole("textbox", { name: /student id/i }) as HTMLInputElement;
    // The ID survives and is selected, so typing immediately overwrites it.
    expect(input).toHaveValue("W250389");
    expect(input).toHaveFocus();
    expect(input.selectionStart).toBe(0);
    expect(input.selectionEnd).toBe(7);
  });

  it("restores an in-progress draft with a calm review notice", async () => {
    const now = Date.now();
    window.sessionStorage.setItem(STUDENT_RESUME_STORAGE_KEY, JSON.stringify({
      wcode: MANUAL_EMAIL_STUDENT.wcode,
      collectedEmail: "student@example.edu",
    }));
    window.sessionStorage.setItem(ABSENCE_DRAFT_STORAGE_KEY, JSON.stringify({
      schemaVersion: 1,
      updatedAt: now,
      wcode: MANUAL_EMAIL_STUDENT.wcode,
      collectedEmail: "student@example.edu",
      step: 1,
      selectedSubjectIds: ["subject-math"],
      selectedSessionIds: ["session-math-1"],
      sitInSelections: {},
      sitInPriorityLevels: {},
      reason: "Appointment",
    }));

    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    // The app asks whether to resume the saved report before re-identifying.
    await screen.findByRole("heading", { name: /continue your absence report/i });
    expect(screen.getByText(/appointment/i)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /is this you/i });
    await confirmIdentity(user);
    await sendParentCode(user);
    await enterCode(user);

    await screen.findByRole("heading", { name: /which class will you miss/i });
    await waitFor(() => {
      expect(screen.getByText(/we restored your report/i)).toBeInTheDocument();
      // The restored selection lands on the additive summary.
      expect(screen.getByRole("button", { name: /remove mathematics/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /add another class/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /^continue$/i })).toBeDisabled();
    });

    await user.click(screen.getByRole("button", { name: /i've reviewed/i }));
    await waitFor(() => expect(screen.getByRole("button", { name: /^continue$/i })).toBeEnabled());

    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /your make-up/i });
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /why will you be away/i });

    // The restored reason is preserved and already selected.
    expect(screen.getByRole("radio", { name: "Appointment" })).toHaveAttribute("aria-checked", "true");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /review your absence/i });
    expect(screen.getByText(/appointment/i)).toBeInTheDocument();
  });

  it("clears the draft and resume after a successful submission", async () => {
    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    await completeToReview(user, "Appointment");
    await user.click(screen.getByRole("button", { name: /submit absence/i }));
    await screen.findByRole("heading", { name: /absence submitted/i });

    expect(window.sessionStorage.getItem(ABSENCE_DRAFT_STORAGE_KEY)).toBeNull();
    expect(window.sessionStorage.getItem(STUDENT_RESUME_STORAGE_KEY)).toBeNull();
  });

  it("collects an email once and never asks again on the same journey", async () => {
    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    await completeToClasses(user);
    await selectClass(user, "Mathematics");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /your make-up/i });

    await user.click(screen.getByRole("button", { name: /^back$/i }));
    await screen.findByRole("heading", { name: /which class will you miss/i });
    await user.click(screen.getByRole("button", { name: /^back$/i }));

    // Back from classes goes to verify, not back into email collection.
    await screen.findByRole("heading", { name: /confirmed|enter the code|confirm with a parent/i });
    expect(screen.queryByRole("heading", { name: /where should we send updates/i })).not.toBeInTheDocument();
  });

  it("restores an in-progress report when the same Student ID is re-entered mid-flow", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    // Build an in-progress report: verified, email collected, one class picked.
    await completeToClasses(user, { email: "student@example.edu" });
    await selectClass(user, "Mathematics");
    // Auto-save is debounced; wait until the selection is actually persisted.
    await waitFor(() => {
      const raw = window.sessionStorage.getItem(ABSENCE_DRAFT_STORAGE_KEY);
      expect(raw).toContain("session-math-1");
    });

    // Wander back to Identify (classes -> verify -> confirm -> identify).
    await user.click(screen.getByRole("button", { name: /^back$/i }));
    await screen.findByRole("heading", { name: /confirmed|enter the code|confirm with a parent/i });
    await user.click(screen.getByRole("button", { name: /^back$/i }));
    await screen.findByRole("heading", { name: /is this you/i });
    await user.click(screen.getByRole("button", { name: /^back$/i }));
    await screen.findByRole("heading", { name: /report an absence/i });

    // Re-enter the same Student ID — the saved report must come back.
    await startReport(user);
    await confirmIdentity(user);
    await sendParentCode(user);
    await enterCode(user);
    await screen.findByRole("heading", { name: /which class will you miss/i });

    await waitFor(() => {
      expect(screen.getByText(/we restored your report/i)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /remove mathematics/i })).toBeInTheDocument();
    });
    // The restore gate holds Continue until the student reviews the classes.
    expect(screen.getByRole("button", { name: /^continue$/i })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: /i've reviewed/i }));
    await waitFor(() => expect(screen.getByRole("button", { name: /^continue$/i })).toBeEnabled());
  });

  it("Start over truly discards the saved report for the same Student ID", async () => {
    const now = Date.now();
    window.sessionStorage.setItem(STUDENT_RESUME_STORAGE_KEY, JSON.stringify({
      wcode: MANUAL_EMAIL_STUDENT.wcode,
      collectedEmail: "student@example.edu",
    }));
    window.sessionStorage.setItem(ABSENCE_DRAFT_STORAGE_KEY, JSON.stringify({
      schemaVersion: 1,
      updatedAt: now,
      wcode: MANUAL_EMAIL_STUDENT.wcode,
      collectedEmail: "student@example.edu",
      step: 1,
      selectedSubjectIds: ["subject-math"],
      selectedSessionIds: ["session-math-1"],
      sitInSelections: {},
      sitInPriorityLevels: {},
      reason: "Appointment",
    }));

    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    await screen.findByRole("heading", { name: /continue your absence report/i });
    await user.click(screen.getByRole("button", { name: /start over/i }));
    await screen.findByRole("heading", { name: /report an absence/i });
    expect(window.sessionStorage.getItem(ABSENCE_DRAFT_STORAGE_KEY)).toBeNull();

    // Re-entering the same Student ID must NOT resurrect the discarded report.
    await startReport(user);
    await confirmIdentity(user);
    await sendParentCode(user);
    await enterCode(user);
    await screen.findByRole("heading", { name: /where should we send updates/i });
    await fillEmailIfNeeded(user, "fresh@example.edu");
    await screen.findByRole("heading", { name: /which class will you miss/i });

    expect(screen.queryByText(/we restored your report/i)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /remove mathematics/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^continue$/i })).toBeDisabled();
  });
});