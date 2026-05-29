import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AbsenceForm from "../AbsenceForm";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>(
    "@/api/client",
  );
  return { ...actual, apiJson: mockApiJson };
});

const mockNavigate = vi.hoisted(() => vi.fn());
vi.mock("react-router-dom", () => ({
  useNavigate: () => mockNavigate,
}));

function renderWithProviders(ui: React.ReactElement) {
  return render(<ToastProvider>{ui}</ToastProvider>);
}

const MOCK_STUDENT = {
  student_id: "s1",
  wcode: "W250389",
  full_name: "John Smith",
  subjects: [
    { id: "subj-1", code: "MATH", name: "Mathematics" },
    { id: "subj-2", code: "PHYS", name: "Physics" },
  ],
};

const MOCK_STUDENT_WITH_ACTIVE = {
  student_id: "s1",
  wcode: "W250389",
  full_name: "John Smith",
  subjects: [
    {
      id: "subj-1",
      code: "MATH",
      name: "Mathematics",
      active_course_id: "c-math201",
      active_course_code: "MATH201",
      active_cycle_label: "Cycle 2",
    },
    {
      id: "subj-2",
      code: "PHYS",
      name: "Physics",
      active_course_id: null,
      active_course_code: null,
      active_cycle_label: null,
    },
  ],
};

const MOCK_CONFIG = {
  form: {
    max_date_range_days: 30,
    require_reason: false,
    reason_categories: [
      { value: "medical", label: "Medical" },
      { value: "family", label: "Family" },
    ],
    allow_free_text_reason: true,
    intro_text: "",
    confirmation_text: "",
  },
  sit_in: { auto_resolve_enabled: true, zoom_description: "Zoom session - no physical class attendance required.", max_sessions_per_absence: 10 },
};

const MOCK_ZOOM_RESULT = {
  sit_in_method: "zoom" as const,
  missed_count: 2,
};

const MOCK_PHYSICAL_RESULT = {
  sit_in_method: "physical" as const,
  sit_in_course: { id: "c-sit", code: "MATH-301", name: "Calculus III" },
  missed_count: 2,
  missed_sessions: [
    { id: "ms1", start_at: "2025-06-02T09:00:00Z", end_at: "2025-06-02T10:30:00Z" },
    { id: "ms2", start_at: "2025-06-04T09:00:00Z", end_at: "2025-06-04T10:30:00Z" },
  ],
  available_sessions: [
    { id: "as1", start_at: "2025-06-02T11:00:00Z", end_at: "2025-06-02T12:30:00Z" },
    { id: "as2", start_at: "2025-06-04T11:00:00Z", end_at: "2025-06-04T12:30:00Z" },
  ],
  pre_selected: [
    { id: "as1", start_at: "2025-06-02T11:00:00Z", end_at: "2025-06-02T12:30:00Z" },
  ],
};

const MOCK_SUBMIT_RESULT = {
  id: "abs-1",
  wcode: "W250389",
  subject_id: "subj-1",
  course_id: "c-sit",
  date_from: "2025-06-02",
  date_to: "2025-06-06",
  sit_in_method: "physical",
};

function getWCodeInput() {
  return screen.getByPlaceholderText("e.g., W250389");
}

async function completeStep1(user: ReturnType<typeof userEvent.setup>) {
  await user.type(getWCodeInput(), "W250389");
  await user.click(screen.getByRole("button", { name: /look up/i }));
  await waitFor(() => {
    expect(screen.getByText("John Smith")).toBeInTheDocument();
  });
  await user.click(screen.getByRole("button", { name: /next/i }));
}

async function completeStep2(user: ReturnType<typeof userEvent.setup>, dateFrom = "2025-06-02", dateTo = "2025-06-06") {
  await user.selectOptions(screen.getByRole("combobox", { name: /subject/i }), "subj-1");
  const fromInput = screen.getByLabelText("From");
  const toInput = screen.getByLabelText("To");
  await user.clear(fromInput);
  await user.type(fromInput, dateFrom);
  await user.clear(toInput);
  await user.type(toInput, dateTo);
  const nextBtn = screen.getByRole("button", { name: /check availability|next/i });
  await user.click(nextBtn);
}

async function completeStep3(user: ReturnType<typeof userEvent.setup>) {
  await waitFor(() => {
    expect(screen.getByRole("button", { name: /next|submit absence/i })).toBeInTheDocument();
  });
  const nextBtn = screen.getByRole("button", { name: /next|submit absence/i });
  if (nextBtn.textContent?.includes("Next")) {
    await user.click(nextBtn);
  }
}

describe("AbsenceForm", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockResolvedValueOnce(MOCK_CONFIG);
  });

  it("renders w-code input initially", () => {
    renderWithProviders(<AbsenceForm />);
    expect(getWCodeInput()).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /look up/i })).toBeInTheDocument();
  });

  it("w-code lookup reveals subject picker and dates", async () => {
    mockApiJson.mockResolvedValueOnce(MOCK_STUDENT);
    const user = userEvent.setup();
    renderWithProviders(<AbsenceForm />);

    await completeStep1(user);

    const options = screen.getAllByRole("option");
    expect(options.map((o) => o.textContent).join(" ")).toMatch(/MATH.*Physics/);
  });

  it("shows active cycle label in subject badges when available", async () => {
    mockApiJson.mockResolvedValueOnce(MOCK_STUDENT_WITH_ACTIVE);
    const user = userEvent.setup();
    renderWithProviders(<AbsenceForm />);

    await user.type(getWCodeInput(), "W250389");
    await user.click(screen.getByRole("button", { name: /look up/i }));

    await waitFor(() => {
      expect(screen.getByText("John Smith")).toBeInTheDocument();
    });

    expect(screen.getByText("MATH (Cycle 2)")).toBeInTheDocument();
  });

  it("dates step validates 30-day max cap", async () => {
    mockApiJson.mockResolvedValueOnce(MOCK_STUDENT);
    const user = userEvent.setup();
    renderWithProviders(<AbsenceForm />);

    await completeStep1(user);
    await completeStep2(user, "2025-06-01", "2025-07-15");

    await waitFor(() => {
      expect(screen.getByText(/30 days/i)).toBeInTheDocument();
    });
  });

  it("applies administrator-configured date and reason rules", async () => {
    mockApiJson.mockReset();
    mockApiJson
      .mockResolvedValueOnce({
        ...MOCK_CONFIG,
        form: { ...MOCK_CONFIG.form, max_date_range_days: 7, require_reason: true, intro_text: "Report early." },
      })
      .mockResolvedValueOnce(MOCK_STUDENT);
    const user = userEvent.setup();
    renderWithProviders(<AbsenceForm />);

    expect(await screen.findByText("Report early.")).toBeInTheDocument();
    await completeStep1(user);
    await completeStep2(user, "2025-06-01", "2025-06-15");

    expect(screen.getByText(/7 days/i)).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: /reason category/i })).toBeInTheDocument();
  });

  it("allows submission for staff assignment when auto-resolution is disabled", async () => {
    mockApiJson.mockReset();
    mockApiJson
      .mockResolvedValueOnce({ ...MOCK_CONFIG, sit_in: { ...MOCK_CONFIG.sit_in, auto_resolve_enabled: false } })
      .mockResolvedValueOnce(MOCK_STUDENT);
    const user = userEvent.setup();
    renderWithProviders(<AbsenceForm />);

    await completeStep1(user);
    await completeStep2(user);

    await waitFor(() => {
      expect(screen.getByText(/assigned by staff/i)).toBeInTheDocument();
    });
    await completeStep3(user);

    expect(screen.getByRole("button", { name: /submit absence/i })).toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([path]) => String(path).includes("sit-in-options"))).toBe(false);
  });

  it("zoom result shows blue banner without session list", async () => {
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ZOOM_RESULT);
    const user = userEvent.setup();
    renderWithProviders(<AbsenceForm />);

    await completeStep1(user);
    await completeStep2(user);

    await waitFor(() => {
      expect(screen.getByText(/zoom session/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/no physical class/i)).toBeInTheDocument();
  });

  it("physical result shows day-by-day timeline with pre-selected sessions", async () => {
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_PHYSICAL_RESULT);
    const user = userEvent.setup();
    renderWithProviders(<AbsenceForm />);

    await completeStep1(user);
    await completeStep2(user);

    await waitFor(() => {
      expect(screen.getByText("MATH-301")).toBeInTheDocument();
    });

    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes.length).toBeGreaterThanOrEqual(1);
    expect(checkboxes[0]).toBeChecked();
  });

  it("includes course_id in submission when active course is set", async () => {
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT_WITH_ACTIVE)
      .mockResolvedValueOnce(MOCK_PHYSICAL_RESULT)
      .mockResolvedValueOnce(MOCK_SUBMIT_RESULT);
    const user = userEvent.setup();
    renderWithProviders(<AbsenceForm />);

    await completeStep1(user);
    await completeStep2(user);

    await waitFor(() => {
      expect(screen.getByText("MATH-301")).toBeInTheDocument();
    });

    await completeStep3(user);

    await waitFor(() => {
      expect(screen.getByText(/MATH201 \(Cycle 2\)/)).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences",
        expect.objectContaining({
          method: "POST",
          body: expect.stringContaining("W250389"),
        }),
      );
    });
    const submitCall = mockApiJson.mock.calls.find(([path]) => path === "/api/v1/absences");
    expect(submitCall).toBeDefined();
    const callBody = JSON.parse(submitCall![1].body);
    expect(callBody.wcode).toBe("W250389");
    expect(callBody.subject_id).toBe("subj-1");
    expect(callBody.course_id).toBe("c-math201");
    expect(callBody.date_from).toBe("2025-06-02");
    expect(callBody.date_to).toBe("2025-06-06");
    expect(callBody.sit_in_method).toBe("physical");
    expect(callBody.sit_in_course_id).toBe("c-sit");
    expect(callBody.sit_in_session_ids).toEqual(["as1"]);
  });

  it("omits course_id when subject has no active course", async () => {
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_PHYSICAL_RESULT)
      .mockResolvedValueOnce(MOCK_SUBMIT_RESULT);
    const user = userEvent.setup();
    renderWithProviders(<AbsenceForm />);

    await completeStep1(user);
    await completeStep2(user);

    await waitFor(() => {
      expect(screen.getByText("MATH-301")).toBeInTheDocument();
    });

    await completeStep3(user);
    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    await waitFor(() => {
      const submitCall = mockApiJson.mock.calls.find(([path]) => path === "/api/v1/absences");
      expect(submitCall).toBeDefined();
      const body = JSON.parse(submitCall![1].body);
      expect(body.course_id).toBeUndefined();
    });
  });

  it("confirmation shows submitted details", async () => {
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_PHYSICAL_RESULT)
      .mockResolvedValueOnce(MOCK_SUBMIT_RESULT);
    const user = userEvent.setup();
    renderWithProviders(<AbsenceForm />);

    await completeStep1(user);
    await completeStep2(user);

    await waitFor(() => {
      expect(screen.getByText("MATH-301")).toBeInTheDocument();
    });

    await completeStep3(user);

    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    await waitFor(() => {
      expect(screen.getByText("Absence Submitted")).toBeInTheDocument();
    });
    expect(screen.getByText(/W250389/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /submit another/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /done/i })).toBeInTheDocument();
  });
});
