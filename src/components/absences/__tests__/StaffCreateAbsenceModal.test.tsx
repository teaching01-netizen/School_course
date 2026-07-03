import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "../../../hooks/useToast";
import StaffCreateAbsenceModal from "../StaffCreateAbsenceModal";

vi.mock("../../../features/absences/domain/submissionPayload", async () => {
  const actual = await vi.importActual<
    typeof import("../../../features/absences/domain/submissionPayload")
  >("../../../features/absences/domain/submissionPayload");
  return { ...actual, selectedSitInCourseIDForGroup: () => null };
});

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual =
    await vi.importActual<typeof import("@/api/client")>("@/api/client");
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
  subjects: [
    { id: "sub1", code: "MATH", name: "Mathematics", active_course_id: "c1" },
  ],
};

const MOCK_ALL_SUBJECTS = [
  { id: "sub1", code: "MATH", name: "Mathematics" },
  { id: "sub-special", code: "ART", name: "Art" },
];

const MOCK_SESSIONS = {
  subjects: [
    {
      subject_id: "sub1",
      subject_code: "MATH",
      subject_name: "Mathematics",
      course_id: "c1",
      course_code: "MATH-1",
      course_name: "Math 101",
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: {
          id: "sitcourse1",
          code: "SCI-1",
          name: "Science 101",
          subject_name: "Science",
        },
        available_sessions: [
          {
            id: "sit1",
            start_at: "2026-06-24T14:00:00Z",
            end_at: "2026-06-24T15:00:00Z",
            course_id: "sitcourse1",
          },
        ],
      },
      sessions: [
        {
          id: "sess1",
          start_at: "2026-06-24T10:00:00Z",
          end_at: "2026-06-24T11:00:00Z",
          date: "2026-06-24",
          already_absent: false,
        },
      ],
    },
  ],
};

const MOCK_SPECIAL_SESSIONS = {
  subjects: [
    {
      subject_id: "sub-special",
      subject_code: "ART",
      subject_name: "Art",
      course_id: "c-special",
      course_code: "ART-1",
      course_name: "Art 101",
      sit_in: {
        sit_in_method: "physical",
        available_sessions: [
          {
            id: "sit-special",
            start_at: "2026-06-25T14:00:00Z",
            end_at: "2026-06-25T15:00:00Z",
            course_id: "c-special-alt",
            course_code: "ART-2",
            course_name: "Art Workshop",
            subject_code: "ART",
            subject_name: "Art",
          },
        ],
      },
      sessions: [
        {
          id: "special-missed",
          start_at: "2026-06-24T10:00:00Z",
          end_at: "2026-06-24T11:00:00Z",
          date: "2026-06-24",
          already_absent: false,
        },
      ],
    },
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
  sit_in: {
    auto_resolve_enabled: true,
    zoom_description: "",
    max_sessions_per_absence: 5,
  },
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
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS);
    renderModal();

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));

    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
      expect(screen.getByText("Mathematics")).toBeInTheDocument();
      expect(screen.getByText("MATH")).toBeInTheDocument();
    });
  });

  it("shows validation toast when advancing with no subject selected", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS);
    renderModal();

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });

    // Don't select any subject, click Next
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(
        screen.getByText(/select at least one subject/i),
      ).toBeInTheDocument();
    });
  });

  it("advances through all steps to confirm", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG);
    renderModal();

    // Step 1: Student lookup
    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });

    // Select subject via checkbox
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 2: Sessions loaded, select the day group checkbox
    await waitFor(() => {
      expect(screen.getByText(/1 class day/)).toBeInTheDocument();
    });
    const sessionCheckbox = await screen.findByRole("checkbox");
    await user.click(sessionCheckbox);

    // Wait for sit-in select to appear
    const sitInSelect = await screen.findByRole(
      "combobox",
      {},
      { timeout: 3000 },
    );
    await user.selectOptions(sitInSelect, "sit1");

    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 3: Confirm step
    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });

    // Verify form config categories loaded
    expect(screen.getByRole("option", { name: "Medical" })).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: "Personal" }),
    ).toBeInTheDocument();
  });

  it("shows toast when no sessions selected on step 2", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS);
    renderModal();

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Don't select any session, click Next
    await waitFor(() => {
      expect(screen.getByText(/1 class day/)).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(
        screen.getByText(/select at least one missed class/i),
      ).toBeInTheDocument();
    });
  });

  it("loads sessions with wide date bounds and bypass_timing on step 2", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS);
    renderModal();

    // Step 1: lookup student
    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });

    // Select subject and advance to step 2
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 2: verify sessions loaded with wide date bounds and bypass_timing
    await waitFor(() => {
      expect(screen.getByText(/1 class day/)).toBeInTheDocument();
    });

    const sessionsUrl = mockApiJson.mock.calls.find((call: unknown[]) =>
      String(call[0]).includes("sessions-in-range"),
    )?.[0] as string;
    expect(sessionsUrl).toContain("date_from=1970-01-01");
    expect(sessionsUrl).toContain("date_to=2100-01-01");
    expect(sessionsUrl).toContain("bypass_timing=true");
  });

  it("loads all sessions for a special-case subject selected from the dropdown", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SPECIAL_SESSIONS);
    renderModal();

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });

    await user.selectOptions(
      screen.getByLabelText(/special case subject/i),
      "sub-special",
    );
    expect(screen.getByText("Art")).toBeInTheDocument();
    expect(screen.getByText("ART · special case")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByText(/1 class day/)).toBeInTheDocument();
    });

    const specialSessionsUrl = mockApiJson.mock.calls.find((call: unknown[]) =>
      String(call[0]).includes("include_all_subjects=true"),
    )?.[0] as string;
    expect(specialSessionsUrl).toContain("subject_ids=sub-special");
    expect(specialSessionsUrl).toContain("bypass_timing=true");

    await user.click(await screen.findByRole("checkbox"));
    await waitFor(() => {
      const sitInSelect = screen.getByRole("combobox") as HTMLSelectElement;
      expect(
        Array.from(sitInSelect.options).map((option) => option.value),
      ).toContain("sit-special");
    });
  });

  it("lets staff choose a special sit-in subject and session for an enrolled absence", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce(MOCK_SPECIAL_SESSIONS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG)
      .mockResolvedValueOnce({ id: "new-absence", status: "pending" })
      .mockResolvedValueOnce({ preview: { phones: [], message: "" } });
    renderModal();

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(screen.getByText(/1 class day/)).toBeInTheDocument();
    });
    await user.click(await screen.findByRole("checkbox"));
    await user.click(screen.getByRole("button", { name: /special sit-in/i }));
    await user.selectOptions(
      screen.getByLabelText(/special sit-in subject/i),
      "sub-special",
    );

    await waitFor(() => {
      const specialSessionsUrl = mockApiJson.mock.calls
        .map((call: unknown[]) => String(call[0]))
        .find((url) => url.includes("include_all_subjects=true"));
      expect(specialSessionsUrl).toContain("subject_ids=sub-special");
      expect(specialSessionsUrl).toContain("bypass_timing=true");
    });

    const specialSessionSelect = await screen.findByLabelText(
      /special sit-in session/i,
    );
    await user.selectOptions(specialSessionSelect, "sit-special");
    await user.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/Art/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /create absence/i }));

    await waitFor(() => {
      const staffCreateCall = mockApiJson.mock.calls.find(
        (call: unknown[]) => call[0] === "/api/v1/absences/staff-create",
      );
      expect(staffCreateCall).toBeTruthy();
      const body = JSON.parse(
        (staffCreateCall?.[1] as RequestInit).body as string,
      );
      expect(body.sit_in_method).toBe("physical");
      expect(body.sit_in_course_id).toBe("c-special-alt");
      expect(body.sit_in_session_ids).toEqual(["sit-special"]);
    });
  });

  it("submits to staff-create endpoint on confirm", async () => {
    const user = userEvent.setup();
    const onCreated = vi.fn();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG)
      .mockResolvedValueOnce({ id: "new-absence", status: "pending" });
    renderModal({ onCreated });

    // Step 1: lookup + select subject
    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 2: select session + sit-in
    await waitFor(() => {
      expect(screen.getByText(/1 class day/)).toBeInTheDocument();
    });
    const sessionCheckbox = await screen.findByRole("checkbox");
    await user.click(sessionCheckbox);
    const sitInSelect = await screen.findByRole(
      "combobox",
      {},
      { timeout: 3000 },
    );
    await user.selectOptions(sitInSelect, "sit1");
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 3: submit
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

describe("accessibility and validation", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("w-code input has required", () => {
    renderModal();
    expect(screen.getByLabelText(/w-code/i)).toBeRequired();
  });

  it("w-code input has autoComplete off", () => {
    renderModal();
    expect(screen.getByLabelText(/w-code/i)).toHaveAttribute(
      "autoComplete",
      "off",
    );
  });

  it("renders w-code error message in DOM", () => {
    renderModal();
    expect(
      screen.getByText("Enter a student W-Code to continue"),
    ).toBeInTheDocument();
  });

  it("w-code error message has role alert", () => {
    renderModal();
    expect(
      screen.getByText("Enter a student W-Code to continue"),
    ).toHaveAttribute("role", "alert");
  });

  async function navigateToStep3(user: ReturnType<typeof userEvent.setup>) {
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG);
    renderModal();
    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByText(/1 class day/)).toBeInTheDocument();
    });
    await user.click(await screen.findByRole("checkbox"));
    const sitInSelect = await screen.findByRole(
      "combobox",
      {},
      { timeout: 3000 },
    );
    await user.selectOptions(sitInSelect, "sit1");
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });
  }

  it("reason category select has required", async () => {
    const user = userEvent.setup();
    await navigateToStep3(user);
    expect(screen.getByLabelText(/reason category/i)).toBeRequired();
  });

  it("reason category error message has role alert", async () => {
    const user = userEvent.setup();
    await navigateToStep3(user);
    expect(screen.getByText("Select a reason category")).toHaveAttribute(
      "role",
      "alert",
    );
  });

  it("step indicator shows aria-current on active step", () => {
    const { container } = renderModal();
    expect(container.querySelector('[aria-current="step"]')).toHaveTextContent(
      "1",
    );
  });

  it("focuses step heading on step change", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS);
    renderModal();
    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(document.activeElement).toHaveTextContent(
        "Step 2: Select missed classes and make-up sessions",
      );
    });
  });

  it("sets aria-invalid on blur for empty w-code", async () => {
    const user = userEvent.setup();
    renderModal();
    const input = screen.getByLabelText(/w-code/i);
    vi.spyOn(input, "checkValidity" as any).mockReturnValue(false);
    await user.click(input);
    await user.tab();
    expect(input).toHaveAttribute("aria-invalid", "true");
  });

  it("removes aria-invalid on input after previously invalid", async () => {
    const user = userEvent.setup();
    renderModal();
    const input = screen.getByLabelText(/w-code/i);
    const checkValidity = vi.spyOn(input, "checkValidity" as any);
    checkValidity.mockReturnValue(false);
    await user.click(input);
    await user.tab();
    expect(input).toHaveAttribute("aria-invalid", "true");
    checkValidity.mockReturnValue(true);
    await user.type(input, "W");
    await waitFor(() => {
      expect(input).not.toHaveAttribute("aria-invalid");
    });
  });

  it("falls back to group.course_id when selectedSitInCourseIDForGroup returns null", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG)
      .mockResolvedValueOnce({ id: "new-absence", status: "pending" });
    renderModal();

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByText(/1 class day/)).toBeInTheDocument();
    });
    await user.click(await screen.findByRole("checkbox"));
    const sitInSelect = await screen.findByRole(
      "combobox",
      {},
      { timeout: 3000 },
    );
    await user.selectOptions(sitInSelect, "sit1");
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /create absence/i }));

    await waitFor(() => {
      const calls = mockApiJson.mock.calls.filter(
        (c: unknown[]) => c[0] === "/api/v1/absences/staff-create",
      );
      expect(calls.length).toBeGreaterThan(0);
      const body = JSON.parse((calls[0][1] as RequestInit).body as string);
      expect(body.sit_in_course_id).toBe("c1");
    });
  });
});

const MOCK_STUDENT_TWO_SUBJECTS = {
  student_id: "s1",
  wcode: "W001",
  full_name: "Test Student",
  subjects: [
    { id: "sub1", code: "MATH", name: "Mathematics", active_course_id: "c1" },
    { id: "sub2", code: "ENG", name: "English", active_course_id: "c2" },
  ],
};

const MOCK_SESSIONS_TWO_SUBJECTS = {
  subjects: [
    {
      subject_id: "sub1",
      subject_code: "MATH",
      subject_name: "Mathematics",
      course_id: "c1",
      course_code: "MATH-1",
      course_name: "Math 101",
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: {
          id: "sitcourse1",
          code: "SCI-1",
          name: "Science 101",
          subject_name: "Science",
        },
        available_sessions: [
          {
            id: "sit1",
            start_at: "2026-06-24T14:00:00Z",
            end_at: "2026-06-24T15:00:00Z",
            course_id: "sitcourse1",
          },
        ],
      },
      sessions: [
        {
          id: "sess1",
          start_at: "2026-06-24T10:00:00Z",
          end_at: "2026-06-24T11:00:00Z",
          date: "2026-06-24",
          already_absent: false,
        },
      ],
    },
    {
      subject_id: "sub2",
      subject_code: "ENG",
      subject_name: "English",
      course_id: "c2",
      course_code: "ENG-1",
      course_name: "English 101",
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: {
          id: "sitcourse2",
          code: "HIS-1",
          name: "History 101",
          subject_name: "History",
        },
        available_sessions: [
          {
            id: "sit2",
            start_at: "2026-06-24T14:00:00Z",
            end_at: "2026-06-24T15:00:00Z",
            course_id: "sitcourse2",
          },
        ],
      },
      sessions: [
        {
          id: "sess2",
          start_at: "2026-06-24T12:00:00Z",
          end_at: "2026-06-24T13:00:00Z",
          date: "2026-06-24",
          already_absent: false,
        },
      ],
    },
  ],
};

describe("multi-subject SMS aggregation", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("sends batch endpoint with all absence IDs when multiple subjects created", async () => {
    const user = userEvent.setup();
    const onCreated = vi.fn();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT_TWO_SUBJECTS)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS_TWO_SUBJECTS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG)
      .mockResolvedValueOnce({ id: "absence-1", status: "pending" })
      .mockResolvedValueOnce({ id: "absence-2", status: "pending" })
      .mockResolvedValueOnce({
        preview: { phones: ["+66811111111"], message: "Aggregated preview" },
      })
      .mockResolvedValueOnce({
        sent: true,
        recipient_count: 1,
        absence_count: 2,
      });
    renderModal({ onCreated });

    // Step 1: lookup + select both subjects
    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("checkbox", { name: /English/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 2: select sessions for both subjects (each shows "1 class day")
    await waitFor(() => {
      expect(screen.getAllByText(/1 class day/).length).toBeGreaterThanOrEqual(
        2,
      );
    });
    const checkboxes = await screen.findAllByRole("checkbox");
    for (const cb of checkboxes) {
      await user.click(cb);
    }
    // Select sit-ins for both sessions (each subject has its own sit-in option)
    const sitInSelects = await screen.findAllByRole("combobox");
    for (const sel of sitInSelects) {
      const options = Array.from((sel as HTMLSelectElement).options).filter(
        (o) => o.value,
      );
      if (options.length > 0) await user.selectOptions(sel, options[0].value);
    }
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 3: submit
    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /create absence/i }));

    // Should call dry_run to get preview, then show SmsConfirmModal
    await waitFor(() => {
      const dryRunCalls = mockApiJson.mock.calls.filter(
        (c: unknown[]) => c[0] === "/api/v1/absences/batch-send-success-sms",
      );
      expect(dryRunCalls.length).toBeGreaterThanOrEqual(1);
      const body = JSON.parse(
        (dryRunCalls[0][1] as RequestInit).body as string,
      );
      expect(body.ids).toEqual(["absence-1", "absence-2"]);
      expect(body.dry_run).toBe(true);
    });

    await waitFor(() => {
      expect(screen.getByText("Send Absence Notification")).toBeInTheDocument();
      expect(screen.getByText("Aggregated preview")).toBeInTheDocument();
    });

    // Click Send SMS — calls batch endpoint again without dry_run
    await user.click(screen.getByRole("button", { name: /send sms/i }));

    await waitFor(() => {
      const sendCalls = mockApiJson.mock.calls.filter(
        (c: unknown[]) => c[0] === "/api/v1/absences/batch-send-success-sms",
      );
      // dry_run + send = 2 calls
      expect(sendCalls.length).toBe(2);
      const sendBody = JSON.parse(
        (sendCalls[1][1] as RequestInit).body as string,
      );
      expect(sendBody.ids).toEqual(["absence-1", "absence-2"]);
      expect(sendBody.dry_run).toBeUndefined();
    });
  });

  it("calls batch endpoint with single ID for single subject", async () => {
    const user = userEvent.setup();
    const onCreated = vi.fn();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG)
      .mockResolvedValueOnce({ id: "absence-single", status: "pending" })
      .mockResolvedValueOnce({
        preview: { phones: ["+66811111111"], message: "Single aggregated" },
      })
      .mockResolvedValueOnce({
        sent: true,
        recipient_count: 1,
        absence_count: 1,
      });
    renderModal({ onCreated });

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByText(/1 class day/)).toBeInTheDocument();
    });
    await user.click(await screen.findByRole("checkbox"));
    const sitInSelect = await screen.findByRole(
      "combobox",
      {},
      { timeout: 3000 },
    );
    await user.selectOptions(sitInSelect, "sit1");
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /create absence/i }));

    await waitFor(() => {
      expect(screen.getByText("Send Absence Notification")).toBeInTheDocument();
      expect(screen.getByText("Single aggregated")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /send sms/i }));

    await waitFor(() => {
      const batchCalls = mockApiJson.mock.calls.filter(
        (c: unknown[]) => c[0] === "/api/v1/absences/batch-send-success-sms",
      );
      // dry_run + send = 2 calls
      expect(batchCalls.length).toBe(2);
      const sendBody = JSON.parse(
        (batchCalls[1][1] as RequestInit).body as string,
      );
      expect(sendBody.ids).toEqual(["absence-single"]);
      expect(sendBody.dry_run).toBeUndefined();
    });
  });
});
