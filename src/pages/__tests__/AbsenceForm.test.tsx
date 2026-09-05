import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  renderPublicAbsenceForm,
  completeToReview,
  completeToClasses,
  enterCode,
  startReport,
  confirmIdentity,
  sendParentCode,
  selectClass,
} from "./helpers/absenceFormHarness";
import { PUBLIC_FORM_CONFIG } from "./fixtures/absenceFormFixtures";

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

function submittedPayloads() {
  return mockApiJson.mock.calls
    .filter((call) => String(call[0]).endsWith("/absences/batch"))
    .map((call) => JSON.parse(String((call[1] as RequestInit | undefined)?.body)));
}

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

describe("AbsenceForm — conversational flow", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockNavigate.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("opens on a quiet identify screen with one question", async () => {
    renderPublicAbsenceForm(mockApiJson);

    expect(await screen.findByRole("heading", { name: /report an absence/i })).toBeInTheDocument();
    expect(screen.getByText(/enter your student id to begin/i)).toBeInTheDocument();
    expect(screen.getByText(/your information stays private/i)).toBeInTheDocument();

    const input = screen.getByRole("textbox", { name: /student id/i });
    const continueButton = screen.getByRole("button", { name: /^continue$/i });
    expect(continueButton).toBeDisabled();

    await userEvent.setup().type(input, "W250389");
    expect(continueButton).toBeEnabled();
    // No stepper, no numbered steps.
    expect(screen.queryByText(/step \d of 4/i)).not.toBeInTheDocument();
  });

  it("confirms identity before exposing anything private", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    await startReport(user);

    expect(screen.getByRole("heading", { name: /is this you/i })).toBeInTheDocument();
    expect(screen.getByText("W250389")).toBeInTheDocument();
    // The profile (subjects, schedule) is not revealed before parent confirmation.
    expect(screen.queryByText(/mathematics/i)).not.toBeInTheDocument();
  });

  it("walks the full happy path and submits one absence", async () => {
    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    await completeToReview(user);

    await user.click(screen.getByRole("button", { name: /submit absence/i }));
    await screen.findByRole("heading", { name: /absence submitted/i });

    expect(screen.getByText(/we'll review your request and send updates to your email/i)).toBeInTheDocument();
    expect(screen.getByText(/ABC12345/)).toBeInTheDocument();
    await waitFor(() => expect(submittedPayloads()).toHaveLength(1));
    const payload = submittedPayloads()[0];
    expect(payload.reason).toBe("Appointment");
    expect(payload.email).toBe("student@example.edu");
    expect(payload.items[0].missed_session_ids).toContain("session-math-1");
  });

  it("submits exactly once when the button is double-tapped", async () => {
    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    await completeToReview(user);

    const submit = screen.getByRole("button", { name: /submit absence/i });
    await user.dblClick(submit);
    await screen.findByRole("heading", { name: /absence submitted/i });
    await waitFor(() => expect(submittedPayloads()).toHaveLength(1));
  });

  it("keeps selections when editing from review", async () => {
    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    await completeToReview(user, "School activity");

    await user.click(screen.getByRole("button", { name: /edit reason/i }));
    await screen.findByRole("heading", { name: /why will you be away/i });
    expect(screen.getByRole("radio", { name: "School activity" })).toHaveAttribute("aria-checked", "true");

    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /review your absence/i });
    expect(screen.getByText(/school activity/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /edit classes/i }));
    await screen.findByRole("heading", { name: /which class will you miss/i });
    // Editing lands on the additive summary with the class still picked.
    expect(screen.getByRole("button", { name: /remove mathematics/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /add another class/i })).toBeInTheDocument();
  });

  it("removing a class offers a brief undo that restores the selection", async () => {
    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    await completeToClasses(user);
    await selectClass(user, "Mathematics");
    expect(screen.getByRole("button", { name: /remove mathematics/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /remove mathematics/i }));
    // The removal is reversible: an inline Undo appears instead of a dialog.
    await waitFor(() => expect(screen.getByRole("button", { name: /^undo$/i })).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: /remove mathematics/i })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^undo$/i }));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /remove mathematics/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /^continue$/i })).toBeEnabled();
    });
  });

  it("treats a no-make-up-required class as a calm outcome, not a failure", async () => {
    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    await completeToClasses(user);
    await selectClass(user, "Mathematics");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /your make-up/i });

    expect(screen.getByText(/no make-up needed/i)).toBeInTheDocument();
    expect(screen.getByText(/you don't need to attend another class for this absence/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /why will you be away/i });
  });

  it("review shows the class and its make-up outcome", async () => {
    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    await completeToReview(user, "Appointment");

    const review = screen.getByRole("heading", { name: /review your absence/i }).closest("div");
    expect(review?.textContent).toMatch(/mathematics/i);
    expect(review?.textContent).toMatch(/make-up: to arrange/i);
    expect(review?.textContent).toMatch(/appointment/i);
  });

  it("Done returns to a fresh identify screen", async () => {
    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    await completeToReview(user);
    await user.click(screen.getByRole("button", { name: /submit absence/i }));
    await screen.findByRole("heading", { name: /absence submitted/i });

    await user.click(screen.getByRole("button", { name: /^done$/i }));
    await screen.findByRole("heading", { name: /report an absence/i });
    expect(screen.getByRole("textbox", { name: /student id/i })).toHaveValue("");
  });

  it("Back from classes returns toward identity without losing the report", async () => {
    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    await completeToClasses(user);
    await selectClass(user, "Mathematics");
    // The picker collapsed into the additive summary holding the selection.
    expect(screen.getByRole("button", { name: /remove mathematics/i })).toBeInTheDocument();
    expect(screen.getByText(/1 class selected/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^back$/i }));
    await screen.findByRole("heading", { name: /enter the code|confirm with a parent|confirmed/i });
    await user.click(screen.getByRole("button", { name: /^back$/i }));
    await screen.findByRole("heading", { name: /is this you/i });
  });

  it("shows a calm confirmed moment before advancing after the OTP", async () => {
    renderPublicAbsenceForm(mockApiJson, { submission: SUBMISSION_RESPONSE });
    const user = userEvent.setup();

    await startReport(user);
    await confirmIdentity(user);
    await sendParentCode(user);
    await enterCode(user);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /confirmed/i })).toBeInTheDocument();
    });
    await screen.findByRole("heading", { name: /where should we send updates/i });
  });

  it("always loads form settings from the server", async () => {
    renderPublicAbsenceForm(mockApiJson, {
      config: { ...PUBLIC_FORM_CONFIG, form: { ...PUBLIC_FORM_CONFIG.form, require_reason: false } },
    });
    await screen.findByRole("heading", { name: /report an absence/i });
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absence-form-config",
      expect.objectContaining({ method: "GET" }),
    );
  });
});