import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "../../../hooks/useToast";
import StaffCreateAbsenceModal from "../StaffCreateAbsenceModal";
import { ApiRequestError } from "../../../api/client";

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

afterEach(() => {
  cleanup();
});

async function advanceToSubjectsStep(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /next/i }));
  await waitFor(() => {
    expect(screen.getByLabelText(/w-code/i)).toBeInTheDocument();
  });
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

  it("renders step 1 with student lookup input", async () => {
    const user = userEvent.setup();
    renderModal();
    await advanceToSubjectsStep(user);
    expect(screen.getByLabelText(/w-code/i)).toBeInTheDocument();
  });

  it("shows error toast on student lookup failure", async () => {
    const user = userEvent.setup();
    mockApiJson.mockRejectedValueOnce(new Error("Student not found"));
    renderModal();
    await advanceToSubjectsStep(user);

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
    await advanceToSubjectsStep(user);

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));

    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
      expect(screen.getByText("Mathematics")).toBeInTheDocument();
    });
  });

  it("shows merged enrolled subjects as one selectable course", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce({
        ...MOCK_STUDENT,
        subjects: [
          {
            id: "sub-writing",
            code: "WRITING",
            name: "SAT Verbal Writing : Rank 3 (Section 1) C3",
            teacher_name: "AJ. RYU",
            merge_group_id: "merge-r3s1",
            merge_group_name: "SAT Verbal Rank 3 Section 1 C3",
          },
          {
            id: "sub-reading",
            code: "READING",
            name: "SAT Verbal Reading : Rank 3 (Section 1) C3",
            teacher_name: "AJ. NICE",
            merge_group_id: "merge-r3s1",
            merge_group_name: "SAT Verbal Rank 3 Section 1 C3",
          },
        ],
      })
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS);
    renderModal();
    await advanceToSubjectsStep(user);

    await user.type(screen.getByLabelText(/w-code/i), "W000012");
    await user.click(screen.getByRole("button", { name: /look up/i }));

    await waitFor(() => {
      expect(
        screen.getByRole("checkbox", {
          name: /SAT Verbal Rank 3 Section 1 C3/,
        }),
      ).toBeInTheDocument();
    });
    expect(
      screen.queryByText(/SAT Verbal Writing : Rank 3 \(Section 1\) C3/),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(/SAT Verbal Reading : Rank 3 \(Section 1\) C3/),
    ).not.toBeInTheDocument();

    await user.click(
      screen.getByRole("checkbox", {
        name: /SAT Verbal Rank 3 Section 1 C3/,
      }),
    );
    expect(screen.getByText("1 subject selected")).toBeInTheDocument();
  });

  it("shows student nickname as main name and school as secondary", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce({ ...MOCK_STUDENT, nickname: "Testy", school: "Bangkok Prep" })
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS);
    renderModal();
    await advanceToSubjectsStep(user);

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));

    await waitFor(() => {
      expect(screen.getByText("Testy")).toBeInTheDocument();
    });
    expect(screen.getByText("Test Student")).toBeInTheDocument();
    expect(screen.getByText("Bangkok Prep")).toBeInTheDocument();
    expect(screen.getByText("W001")).toBeInTheDocument();
  });

  it("falls back to full name as main when nickname is absent", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS);
    renderModal();
    await advanceToSubjectsStep(user);

    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));

    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });
    expect(screen.queryByText(/bangkok/i)).not.toBeInTheDocument();
  });

  it("shows validation toast when advancing with no subject selected", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS);
    renderModal();
    await advanceToSubjectsStep(user);

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

  it("disables a staff sit-in option that overlaps another enrolled class", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce({
        ...MOCK_STUDENT,
        subjects: [
          ...MOCK_STUDENT.subjects,
          { id: "sub2", code: "SCI", name: "Science" },
        ],
      })
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce({
        subjects: [
          ...MOCK_SESSIONS.subjects,
          {
            subject_id: "sub2",
            subject_code: "SCI",
            subject_name: "Science",
            course_id: "c2",
            course_code: "SCI-1",
            course_name: "Science 101",
            sessions: [
              {
                id: "science-session",
                start_at: "2026-06-24T14:30:00Z",
                end_at: "2026-06-24T15:30:00Z",
                date: "2026-06-24",
                already_absent: false,
              },
            ],
          },
        ],
      });
    renderModal();
    await advanceToSubjectsStep(user);

    await user.type(screen.getByLabelText(/w-code/i), "W000012");
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

    const sitInOption = await screen.findByRole("option", { name: /Science/ });
    expect(sitInOption).toBeDisabled();
    expect(sitInOption).toHaveValue("sit1");
  });

  it("shows a used sit-in session as unavailable in the staff form", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce({
        subjects: [{
          ...MOCK_SESSIONS.subjects[0],
          sit_in: {
            sit_in_method: "physical",
            sit_in_course: MOCK_SESSIONS.subjects[0].sit_in.sit_in_course,
            unavailable_sessions: [{
              session: { id: "used-day-session", start_at: "2026-06-24T14:00:00Z", end_at: "2026-06-24T15:00:00Z" },
              reason: "This sit-in session is already assigned to this student's absence.",
              reason_code: "sit_in_session_already_used",
            }],
          },
        }],
      });
    renderModal();
    await advanceToSubjectsStep(user);
    await user.type(screen.getByLabelText(/w-code/i), "W000012");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => expect(screen.getByText("Test Student")).toBeInTheDocument());
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => expect(screen.getByText(/1 class day/)).toBeInTheDocument());
    await user.click(await screen.findByRole("checkbox"));

    expect(await screen.findByText("This sit-in session is already used.")).toBeInTheDocument();
    expect(screen.getByText("Choose another sit-in session.")).toBeInTheDocument();
  });

  it("refreshes the staff sessions after a stale sit-in session conflict", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockRejectedValueOnce(new ApiRequestError("This sit-in session is already assigned", { code: "sit_in_session_already_used", status: 409 }))
      .mockResolvedValueOnce(MOCK_SESSIONS);
    renderModal();
    await advanceToSubjectsStep(user);
    await user.type(screen.getByLabelText(/w-code/i), "W000012");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => expect(screen.getByText("Test Student")).toBeInTheDocument());
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => expect(screen.getByText(/1 class day/)).toBeInTheDocument());
    await user.click(await screen.findByRole("checkbox"));
    await user.selectOptions(await screen.findByRole("combobox"), "sit1");
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: /create absence/i }));

    expect(await screen.findByText(/That sit-in session was just used for this student/)).toBeInTheDocument();
    await waitFor(() => {
      expect(mockApiJson.mock.calls.filter(([url]) => String(url).includes("sessions-in-range")).length).toBeGreaterThanOrEqual(2);
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

    // Step 0: Type selection
    await user.click(screen.getByRole("button", { name: /next/i }));

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

    await user.click(screen.getByRole("button", { name: /next/i }));
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

    await user.click(screen.getByRole("button", { name: /next/i }));
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

    await user.click(screen.getByRole("button", { name: /next/i }));
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
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce({ id: "new-absence", status: "pending" })
      .mockResolvedValueOnce({ preview: { phones: [], message: "" } });
    renderModal();

    await user.click(screen.getByRole("button", { name: /next/i }));
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
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce({ id: "new-absence", status: "pending" });
    renderModal({ onCreated });

    await user.click(screen.getByRole("button", { name: /next/i }));
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

  it("w-code input has required", async () => {
    const user = userEvent.setup();
    renderModal();
    await advanceToSubjectsStep(user);
    expect(screen.getByLabelText(/w-code/i)).toBeRequired();
  });

  it("w-code input has autoComplete off", async () => {
    const user = userEvent.setup();
    renderModal();
    await advanceToSubjectsStep(user);
    expect(screen.getByLabelText(/w-code/i)).toHaveAttribute(
      "autoComplete",
      "off",
    );
  });

  it("renders w-code error message in DOM", async () => {
    const user = userEvent.setup();
    renderModal();
    await advanceToSubjectsStep(user);
    expect(
      screen.getByText("Enter a student W-Code to continue"),
    ).toBeInTheDocument();
  });

  it("w-code error message has role alert", async () => {
    const user = userEvent.setup();
    renderModal();
    await advanceToSubjectsStep(user);
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
    await advanceToSubjectsStep(user);
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
    await advanceToSubjectsStep(user);
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
    await advanceToSubjectsStep(user);
    const input = screen.getByLabelText(/w-code/i);
    vi.spyOn(input, "checkValidity" as any).mockReturnValue(false);
    await user.click(input);
    await user.tab();
    expect(input).toHaveAttribute("aria-invalid", "true");
  });

  it("removes aria-invalid on input after previously invalid", async () => {
    const user = userEvent.setup();
    renderModal();
    await advanceToSubjectsStep(user);
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
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce({ id: "new-absence", status: "pending" });
    renderModal();

    await user.click(screen.getByRole("button", { name: /next/i }));
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

describe("Absence type selection (Step 0)", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("defaults to Normal Absence with aria-pressed", async () => {
    renderModal();
    const normalBtn = screen.getByRole("button", { name: /normal absence/i });
    const specialBtn = screen.getByRole("button", { name: /special absence/i });
    expect(normalBtn).toHaveAttribute("aria-pressed", "true");
    expect(specialBtn).toHaveAttribute("aria-pressed", "false");
  });

  it("selecting Special Absence shows info box and toggles aria-pressed", async () => {
    const user = userEvent.setup();
    renderModal();
    const specialBtn = screen.getByRole("button", { name: /special absence/i });
    await user.click(specialBtn);

    expect(specialBtn).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: /normal absence/i })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByText(/special approved/i)).toBeInTheDocument();
    expect(screen.getByText(/will not count toward the student/i)).toBeInTheDocument();
  });

  it("back button is hidden on type step", () => {
    renderModal();
    expect(screen.queryByRole("button", { name: /back/i })).not.toBeInTheDocument();
  });

  it("back from subjects returns to type step with state preserved", async () => {
    const user = userEvent.setup();
    renderModal();

    // Select special
    await user.click(screen.getByRole("button", { name: /special absence/i }));
    // Advance to subjects
    await user.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(screen.getByLabelText(/w-code/i)).toBeInTheDocument();
    });

    // Go back
    await user.click(screen.getByRole("button", { name: /back/i }));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /special absence/i })).toHaveAttribute("aria-pressed", "true");
    });
  });

  it("special type sends status: special_approved in request body", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce({ id: "new-absence", status: "special_approved" });
    renderModal();

    // Select special type
    await user.click(screen.getByRole("button", { name: /special absence/i }));
    await user.click(screen.getByRole("button", { name: /next/i }));

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
    const sitInSelect = await screen.findByRole("combobox", {}, { timeout: 3000 });
    await user.selectOptions(sitInSelect, "sit1");
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Step 3: submit
    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /create absence/i }));

    await waitFor(() => {
      const staffCreateCall = mockApiJson.mock.calls.find(
        (call: unknown[]) => call[0] === "/api/v1/absences/staff-create",
      );
      expect(staffCreateCall).toBeTruthy();
      const body = JSON.parse((staffCreateCall?.[1] as RequestInit).body as string);
      expect(body.status).toBe("special_approved");
    });
  });

  it("normal type omits status field from request body", async () => {
    const user = userEvent.setup();
    mockApiJson
      .mockResolvedValueOnce(MOCK_STUDENT)
      .mockResolvedValueOnce(MOCK_ALL_SUBJECTS)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce(MOCK_FORM_CONFIG)
      .mockResolvedValueOnce(MOCK_SESSIONS)
      .mockResolvedValueOnce({ id: "new-absence", status: "pending" });
    renderModal();

    // Default is normal, just advance
    await user.click(screen.getByRole("button", { name: /next/i }));

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
    const sessionCheckbox = await screen.findByRole("checkbox");
    await user.click(sessionCheckbox);
    const sitInSelect = await screen.findByRole("combobox", {}, { timeout: 3000 });
    await user.selectOptions(sitInSelect, "sit1");
    await user.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /create absence/i }));

    await waitFor(() => {
      const staffCreateCall = mockApiJson.mock.calls.find(
        (call: unknown[]) => call[0] === "/api/v1/absences/staff-create",
      );
      expect(staffCreateCall).toBeTruthy();
      const body = JSON.parse((staffCreateCall?.[1] as RequestInit).body as string);
      expect(body.status).toBeUndefined();
    });
  });
});

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
      .mockResolvedValueOnce(MOCK_SESSIONS_TWO_SUBJECTS)
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

    await user.click(screen.getByRole("button", { name: /next/i }));
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
      .mockResolvedValueOnce(MOCK_SESSIONS)
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

    await user.click(screen.getByRole("button", { name: /next/i }));
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

describe("special sit-in multi-subject coverage", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  // Student is enrolled in Mathematics (sub1 / c1). Absence is recorded against
  // Mathematics, but via "Special sit-in" staff may point the make-up at ANY
  // other subject's sessions. These tests pin the CURRENT behaviour: a single
  // absence may only carry ONE special sit-in course.

  const MOCK_STUDENT_SPECIAL = {
    student_id: "s1",
    wcode: "W001",
    full_name: "Test Student",
    subjects: [
      { id: "sub1", code: "MATH", name: "Mathematics", active_course_id: "c1" },
    ],
  };

  const MOCK_ALL_SUBJECTS_SPECIAL = [
    { id: "sub1", code: "MATH", name: "Mathematics" },
    { id: "subB", code: "SCI", name: "Science" },
    { id: "subC", code: "HIS", name: "History" },
  ];

  // Absent subject: Mathematics with TWO missed class days so we can mark each
  // as a different special sit-in.
  const MOCK_SESSIONS_TWO_MISSED = {
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
              start_at: "2026-06-26T14:00:00Z",
              end_at: "2026-06-26T15:00:00Z",
              course_id: "sitcourse1",
            },
          ],
        },
        sessions: [
          {
            id: "sessA1",
            start_at: "2026-06-24T10:00:00Z",
            end_at: "2026-06-24T11:00:00Z",
            date: "2026-06-24",
            already_absent: false,
          },
          {
            id: "sessA2",
            start_at: "2026-06-25T10:00:00Z",
            end_at: "2026-06-25T11:00:00Z",
            date: "2026-06-25",
            already_absent: false,
          },
        ],
      },
    ],
  };

  // Special sit-in target B (Science) -> resolves to course "cB-alt"
  const MOCK_SPECIAL_SESSIONS_B = {
    subjects: [
      {
        subject_id: "subB",
        subject_code: "SCI",
        subject_name: "Science",
        course_id: "cB",
        course_code: "SCI-1",
        course_name: "Science 101",
        sit_in: {
          sit_in_method: "physical",
          available_sessions: [
            {
              id: "sitB1",
              start_at: "2026-06-26T14:00:00Z",
              end_at: "2026-06-26T15:00:00Z",
              course_id: "cB-alt",
              course_code: "SCI-2",
              course_name: "Science Workshop",
              subject_code: "SCI",
              subject_name: "Science",
            },
          ],
        },
        sessions: [
          {
            id: "specialB-missed",
            start_at: "2026-06-24T10:00:00Z",
            end_at: "2026-06-24T11:00:00Z",
            date: "2026-06-24",
            already_absent: false,
          },
        ],
      },
    ],
  };

  // Special sit-in target C (History) -> resolves to course "cC-alt"
  const MOCK_SPECIAL_SESSIONS_C = {
    subjects: [
      {
        subject_id: "subC",
        subject_code: "HIS",
        subject_name: "History",
        course_id: "cC",
        course_code: "HIS-1",
        course_name: "History 101",
        sit_in: {
          sit_in_method: "physical",
          available_sessions: [
            {
              id: "sitC1",
              start_at: "2026-06-27T14:00:00Z",
              end_at: "2026-06-27T15:00:00Z",
              course_id: "cC-alt",
              course_code: "HIS-2",
              course_name: "History Workshop",
              subject_code: "HIS",
              subject_name: "History",
            },
          ],
        },
        sessions: [
          {
            id: "specialC-missed",
            start_at: "2026-06-25T10:00:00Z",
            end_at: "2026-06-25T11:00:00Z",
            date: "2026-06-25",
            already_absent: false,
          },
        ],
      },
    ],
  };

  function routeSpecialMocks() {
    mockApiJson.mockImplementation(async (url: unknown) => {
      const u = String(url);
      if (u.includes("student-lookup")) return MOCK_STUDENT_SPECIAL;
      if (u === "/api/v1/subjects") return MOCK_ALL_SUBJECTS_SPECIAL;
      if (u.includes("absence-form-config")) return MOCK_FORM_CONFIG;
      if (u.includes("sessions-in-range")) {
        if (u.includes("subject_ids=subB")) return MOCK_SPECIAL_SESSIONS_B;
        if (u.includes("subject_ids=subC")) return MOCK_SPECIAL_SESSIONS_C;
        return MOCK_SESSIONS_TWO_MISSED;
      }
      if (u.includes("staff-create"))
        return { id: "abs-created", status: "special_approved" };
      return {};
    });
  }

  async function reachSessionsStep(user: ReturnType<typeof userEvent.setup>) {
    // Type step: pick Special Absence so the record is special_approved.
    await user.click(screen.getByRole("button", { name: /special absence/i }));
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Subject step
    await waitFor(() => {
      expect(screen.getByLabelText(/w-code/i)).toBeInTheDocument();
    });
    await user.type(screen.getByLabelText(/w-code/i), "W001");
    await user.click(screen.getByRole("button", { name: /look up/i }));
    await waitFor(() => {
      expect(screen.getByText("Test Student")).toBeInTheDocument();
    });
    await user.click(screen.getByRole("checkbox", { name: /Mathematics/ }));
    await user.click(screen.getByRole("button", { name: /next/i }));

    // Sessions step: two missed class days for Mathematics
    await waitFor(() => {
      expect(screen.getByText(/2 class days/)).toBeInTheDocument();
    });
  }

  it("creates one absence per special sit-in course when a special absence spans multiple subjects", async () => {
    const user = userEvent.setup();
    routeSpecialMocks();
    renderModal();
    await reachSessionsStep(user);

    // Select both missed class days (absent from Mathematics).
    const dayCheckboxes = await screen.findAllByRole("checkbox");
    expect(dayCheckboxes).toHaveLength(2);
    await user.click(dayCheckboxes[0]);
    await user.click(dayCheckboxes[1]);

    // Mark each missed day as a Special sit-in in a DIFFERENT subject
    // (Science and History) — i.e. one absence, multiple make-up subjects.
    const toggles1 = screen.getAllByRole("button", { name: "Special sit-in" });
    await user.click(toggles1[0]);
    const toggles2 = screen.getAllByRole("button", { name: "Special sit-in" });
    await user.click(toggles2[1]);

    const subjectSelects = screen.getAllByLabelText(/special sit-in subject/i);
    expect(subjectSelects).toHaveLength(2);
    await user.selectOptions(subjectSelects[0], "subB");
    await user.selectOptions(subjectSelects[1], "subC");

    // Wait for each subject's special sessions to load, then pick one.
    const sessionSelects = screen.getAllByLabelText(/special sit-in session/i);
    await waitFor(() => {
      expect(
        Array.from((sessionSelects[0] as HTMLSelectElement).options).some(
          (o) => o.value,
        ),
      ).toBe(true);
    });
    await waitFor(() => {
      expect(
        Array.from((sessionSelects[1] as HTMLSelectElement).options).some(
          (o) => o.value,
        ),
      ).toBe(true);
    });
    const valB = Array.from(
      (sessionSelects[0] as HTMLSelectElement).options,
    ).find((o) => o.value)!.value;
    const valC = Array.from(
      (sessionSelects[1] as HTMLSelectElement).options,
    ).find((o) => o.value)!.value;
    await user.selectOptions(sessionSelects[0], valB);
    await user.selectOptions(sessionSelects[1], valC);

    await user.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /create absence/i }));

    // The old "one special sit-in course per absence" restriction must be gone.
    expect(
      screen.queryByText(/use one special sit-in course per absence/i),
    ).not.toBeInTheDocument();

    // One staff-create per distinct special sit-in course.
    await waitFor(() => {
      const calls = mockApiJson.mock.calls.filter(
        (c: unknown[]) => c[0] === "/api/v1/absences/staff-create",
      );
      expect(calls).toHaveLength(2);
      const bodies = calls.map((c: unknown[]) =>
        JSON.parse((c[1] as RequestInit).body as string),
      );
      const byCourse = new Map(bodies.map((b) => [b.sit_in_course_id, b]));
      expect(byCourse.get("cB-alt")).toBeDefined();
      expect(byCourse.get("cC-alt")).toBeDefined();

      expect(byCourse.get("cB-alt").missed_session_ids).toContain("sessA1");
      expect(byCourse.get("cB-alt").sit_in_session_ids).toEqual(["sitB1"]);
      expect(byCourse.get("cB-alt").status).toBe("special_approved");

      expect(byCourse.get("cC-alt").missed_session_ids).toContain("sessA2");
      expect(byCourse.get("cC-alt").sit_in_session_ids).toEqual(["sitC1"]);
      expect(byCourse.get("cC-alt").status).toBe("special_approved");
    });
  });

  it("allows a special absence whose make-up targets a single other subject", async () => {
    const user = userEvent.setup();
    routeSpecialMocks();
    renderModal();
    await reachSessionsStep(user);

    // Select only the first missed class day.
    const dayCheckboxes = await screen.findAllByRole("checkbox");
    await user.click(dayCheckboxes[0]);

    const toggles = screen.getAllByRole("button", { name: "Special sit-in" });
    await user.click(toggles[0]);

    const subjectSelects = screen.getAllByLabelText(/special sit-in subject/i);
    await user.selectOptions(subjectSelects[0], "subB");

    const sessionSelects = screen.getAllByLabelText(/special sit-in session/i);
    await waitFor(() => {
      expect(
        Array.from((sessionSelects[0] as HTMLSelectElement).options).some(
          (o) => o.value,
        ),
      ).toBe(true);
    });
    const valB = Array.from(
      (sessionSelects[0] as HTMLSelectElement).options,
    ).find((o) => o.value)!.value;
    await user.selectOptions(sessionSelects[0], valB);

    await user.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(screen.getByLabelText(/reason category/i)).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /create absence/i }));

    await waitFor(() => {
      const staffCreateCall = mockApiJson.mock.calls.find(
        (c: unknown[]) => c[0] === "/api/v1/absences/staff-create",
      );
      expect(staffCreateCall).toBeTruthy();
      const body = JSON.parse((staffCreateCall?.[1] as RequestInit).body as string);
      // Special make-up points at Science (cB-alt), NOT the absent Mathematics.
      expect(body.sit_in_course_id).toBe("cB-alt");
      expect(body.status).toBe("special_approved");
    });
  });
});
