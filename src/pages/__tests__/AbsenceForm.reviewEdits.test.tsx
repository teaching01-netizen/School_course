import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  renderPublicAbsenceForm,
  completeToReview,
  openDayOffset,
  selectClass,
} from "./helpers/absenceFormHarness";
import {
  CRM_EMAIL_STUDENT,
  PUBLIC_FORM_SESSIONS,
  relativeDateKey,
  relativeISO,
} from "./fixtures/absenceFormFixtures";
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

// Default agenda (math, no make-up) plus a second, tomorrow class that needs
// a physical make-up — so a classes edit can create exactly one revisit.
const SESSIONS_WITH_MAKEUP_NEEDED: SessionsInRangeResponse = {
  subjects: [
    ...PUBLIC_FORM_SESSIONS.subjects.filter((group) => group.subject_id === "subject-math"),
    {
      subject_id: "subject-physics",
      subject_code: "PHYS",
      subject_name: "Physics",
      course_id: "course-physics",
      course_code: "PHYS201",
      course_name: "Physics",
      teacher_name: "Mr. Long",
      sessions: [
        {
          id: "session-physics-1",
          start_at: relativeISO(17, 0, 1),
          end_at: relativeISO(19, 0, 1),
          date: relativeDateKey(1),
          already_absent: false,
        },
      ],
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: {
          id: "course-physics-sitin",
          code: "PHYS202",
          name: "Physics Make-up",
          subject_code: "PHYS",
          subject_name: "Physics",
        },
        available_sessions: [
          {
            id: "sit-physics-1",
            start_at: relativeISO(16, 0, 2),
            end_at: relativeISO(17, 30, 2),
            course_id: "course-physics-sitin",
            class_name: "Physics Make-up",
            subject_name: "Physics",
            course_name: "Physics Make-up",
          },
        ],
      },
    },
  ],
};

describe("AbsenceForm — review edits return to review", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockNavigate.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("Edit classes returns straight to Review when no new make-up is needed", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    await completeToReview(user, "Appointment");

    await user.click(screen.getByRole("button", { name: /edit classes/i }));
    await screen.findByRole("heading", { name: /which class will you miss/i });
    // Add tomorrow's class; it needs no make-up in the default agenda.
    await openDayOffset(user, 1);
    await selectClass(user, "Physics");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));

    // Directly back on Review — no make-up or Details stages in between.
    await screen.findByRole("heading", { name: /review your absence/i });
    expect(await screen.findByText(/your classes are updated/i)).toBeInTheDocument();
    expect(screen.getByText(/step 5 of 5 · review/i)).toBeInTheDocument();
    const review = screen.getByRole("heading", { name: /review your absence/i }).closest("div");
    expect(review?.textContent).toMatch(/mathematics/i);
    expect(review?.textContent).toMatch(/physics/i);
    expect(screen.queryByRole("heading", { name: /why will you be away/i })).not.toBeInTheDocument();
  });

  it("a changed class revisits only its make-up row, then returns to Review", async () => {
    renderPublicAbsenceForm(mockApiJson, { sessions: SESSIONS_WITH_MAKEUP_NEEDED });
    const user = userEvent.setup();

    await completeToReview(user, "Appointment");

    await user.click(screen.getByRole("button", { name: /edit classes/i }));
    await screen.findByRole("heading", { name: /which class will you miss/i });
    // The added class needs a physical make-up — the revisit lands on the plan
    // (only that row needs a choice; math keeps its untouched outcome).
    await openDayOffset(user, 1);
    await selectClass(user, "Physics");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));

    await screen.findByRole("heading", { name: /your make-up/i });
    expect(screen.getAllByText(/no make-up needed/i).length).toBeGreaterThan(0);
    await user.click(screen.getByRole("button", { name: /choose a time/i }));
    const dialog = await screen.findByRole("dialog", { name: /choose a make-up time/i });
    const options = within(dialog).getAllByRole("radio");
    expect(options.length).toBeGreaterThan(0);
    await user.click(options[0]);
    await user.click(within(dialog).getByRole("button", { name: /use this time/i }));

    // Complete the plan back to Review — Details is not re-walked.
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await screen.findByRole("heading", { name: /review your absence/i });
    expect(screen.queryByRole("heading", { name: /why will you be away/i })).not.toBeInTheDocument();
  });

  it("Edit reason keeps the reason and returns to Review without extra stages", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    await completeToReview(user, "School activity");

    await user.click(screen.getByRole("button", { name: /edit reason/i }));
    await screen.findByRole("heading", { name: /why will you be away/i });
    await user.click(screen.getByRole("radio", { name: "Travel" }));
    await user.click(screen.getByRole("button", { name: /^continue$/i }));

    await screen.findByRole("heading", { name: /review your absence/i });
    expect(screen.getByText(/travel/i)).toBeInTheDocument();
  });

  it("Review shows the update destination with Edit email returning to Review", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    await completeToReview(user, "Appointment");

    // The collected destination is listed with its own control.
    expect(screen.getByRole("heading", { name: /update destination/i })).toBeInTheDocument();
    expect(screen.getByText(/updates go to student@example\.edu/i)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /edit email/i }));

    // Details opens focused on the email field.
    const emailInput = await screen.findByRole("textbox", { name: /^email$/i });
    await waitFor(() => expect(emailInput).toHaveFocus());

    await user.clear(emailInput);
    await user.type(emailInput, "new@example.edu");
    await user.click(screen.getByRole("button", { name: /^continue$/i }));

    // Directly back on Review with the corrected destination.
    await screen.findByRole("heading", { name: /review your absence/i });
    expect(screen.getByText(/updates go to new@example\.edu/i)).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /why will you be away/i })).not.toBeInTheDocument();
  });

  it("shows the on-file destination without an Edit email when the school has one", async () => {
    renderPublicAbsenceForm(mockApiJson, { lookup: CRM_EMAIL_STUDENT });
    const user = userEvent.setup();

    await completeToReview(user, "Appointment", { wcode: CRM_EMAIL_STUDENT.wcode });

    expect(screen.getByText(/emails go to the address the school has on file/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /edit email/i })).not.toBeInTheDocument();
  });

  it("Back from a review edit cancels back to Review without losing the report", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();

    await completeToReview(user, "Appointment");

    await user.click(screen.getByRole("button", { name: /edit classes/i }));
    await screen.findByRole("heading", { name: /which class will you miss/i });
    await user.click(screen.getByRole("button", { name: /^back$/i }));

    await screen.findByRole("heading", { name: /review your absence/i });
    expect(screen.getByText(/mathematics/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /submit absence/i })).toBeEnabled();
  });
});
