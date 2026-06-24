import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "../../../hooks/useToast";
import StaffCreateAbsenceModal from "../StaffCreateAbsenceModal";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderModal(props?: { onClose?: () => void; onCreated?: () => void }) {
  return render(
    <ToastProvider>
      <StaffCreateAbsenceModal
        onClose={props?.onClose ?? vi.fn()}
        onCreated={props?.onCreated ?? vi.fn()}
      />
    </ToastProvider>,
  );
}

const MOCK_STUDENT = {
  student_id: "s1",
  wcode: "W001",
  full_name: "Test Student",
  subjects: [{ id: "sub1", code: "MATH", name: "Mathematics", active_course_id: "c1" }],
};

const MOCK_SESSIONS = {
  subjects: [
    {
      subject_id: "sub1",
      subject_code: "MATH",
      subject_name: "Mathematics",
      course_id: "c1",
      course_code: "MATH-1",
      course_name: "Math 101",
      sessions: [
        { id: "sess1", start_at: "2026-06-24T10:00:00Z", end_at: "2026-06-24T11:00:00Z", date: "2026-06-24", already_absent: false },
      ],
    },
  ],
};

const MOCK_SIT_IN_OPTIONS = {
  sit_in_method: "physical",
  sit_in_course: { id: "sitcourse1", code: "SCI-1", name: "Science 101", subject_name: "Science" },
  available_sessions: [
    { id: "sit1", start_at: "2026-06-24T14:00:00Z", end_at: "2026-06-24T15:00:00Z" },
  ],
};

const MOCK_FORM_CONFIG = {
  form: {
    max_date_range_days: 30,
    require_reason: false,
    reason_categories: [
      { value: "medical", label: "Medical" },
      { value: "personal", label: "Personal" },
    ],
    allow_free_text_reason: true,
    intro_text: "",
    confirmation_text: "",
    min_hours_before_session: 0,
    max_hours_after_session: 0,
  },
  sit_in: { auto_resolve_enabled: true, zoom_description: "", max_sessions_per_absence: 5 },
};

describe("StaffCreateAbsenceModal", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("renders step 1 with student lookup input", () => {
    renderModal();
    expect(screen.getByLabelText(/w-code/i)).toBeInTheDocument();
  });

  it("shows error toast on student lookup failure", async () => {
    const user = userEvent.setup();
    mockApiJson.mockRejectedValueOnce(new Error("Student not found"));
    renderModal();

    await user.type(screen.getByLabelText(/w-code/i), "W999");
    await user.click(screen.getByRole("button", { name: /look up/i }));

    await waitFor(() => {
      expect(screen.getByText(/student not found/i)).toBeInTheDocument();
    });
  });

  it("fetches student data on lookup and shows subjects", async () => {
    const user = userEvent.setup();
    mockApiJson.mockResolvedValueOnce(MOCK_STUDENT);
    renderModal();

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));

    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
      expect(screen.getByText("MATH — Mathematics")).toBeInTheDocument();
    });
  });

  it("shows validation toast when advancing without required fields", async () => {
    const user = userEvent.setup();
    mockApiJson.mockResolvedValueOnce(MOCK_STUDENT);
    renderModal();

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByText(/select a student, subject, and date range/i)).toBeInTheDocument();
    });
  });

  it("advances through all steps to confirm", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce(MOCK_SIT_IN_OPTIONS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG);
    renderModal();

    // Step 1: Student lookup
    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });

    await user.selectOptions(screen.getByLabelText(/subject/i), "sub1");
    await user.type(screen.getByLabelText(/from/i), "2026-06-24");
    await user.type(screen.getByLabelText(/to/i), "2026-06-25");

    // Step 1 → 2
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByText(/select the sessions/i)).toBeInTheDocument();
    });

    // Step 2: Wait for sessions to load, then select one
    const sessionButton = await screen.findByRole("button", { name: /17:00/i });
    await user.click(sessionButton);
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 3: Wait for sit-in options to load
    await waitFor(() => {
      expect(screen.getByText("Science")).toBeInTheDocument();
    });

    // Step 3: Select sit-in session (wait for it to load)
    const sitInButton = await screen.findByRole("button", { name: /21:00/i });
    await user.click(sitInButton);
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 4: Confirm step
    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });

    // Verify form config categories loaded
    expect(screen.getByRole("option", { name: "Medical" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Personal" })).toBeInTheDocument();
  });

  it("shows toast when no sessions selected on step 2", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_SESSIONS);
    renderModal();

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });

    await user.selectOptions(screen.getByLabelText(/subject/i), "sub1");
    await user.type(screen.getByLabelText(/from/i), "2026-06-24");
    await user.type(screen.getByLabelText(/to/i), "2026-06-25");
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByText(/select the sessions/i)).toBeInTheDocument();
    });

    // Don't select any session, click Next
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByText(/select at least one missed session/i)).toBeInTheDocument();
    });
  });

  it("submits to staff-create endpoint on confirm", async () => {
    const user = userEvent.setup();
    const onCreated = vi.fn();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce(MOCK_SIT_IN_OPTIONS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG)
      .mockResolvedValueOnce({ id: "new-absence", status: "pending" });
    renderModal({ onCreated });

    // Step 1
    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });

    await user.selectOptions(screen.getByLabelText(/subject/i), "sub1");
    await user.type(screen.getByLabelText(/from/i), "2026-06-24");
    await user.type(screen.getByLabelText(/to/i), "2026-06-25");
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByText(/select the sessions/i)).toBeInTheDocument();
    });

    // Step 2
    const sessionButton = await screen.findByRole("button", { name: /17:00/i });
    await user.click(sessionButton);
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 3
    await waitFor(() => {
      expect(screen.getByText("Science")).toBeInTheDocument();
    });
    const sitInButton = await screen.findByRole("button", { name: /21:00/i });
    await user.click(sitInButton);
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 4
    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /create absence/i }));
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/staff-create",
        expect.objectContaining({ method: "POST" }),
      );
      expect(onCreated).toHaveBeenCalled();
    });
  });
});
