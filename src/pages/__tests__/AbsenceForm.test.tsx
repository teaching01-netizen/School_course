import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AbsenceForm from "../AbsenceForm";
import { renderWithProviders, createMockSessionsInRange } from "./helpers";

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

const SESSION_STORAGE_KEY = "warwick-absence-form-state-v3";

function prePopulateSessionStorage(dateFrom: string, dateTo: string) {
  window.sessionStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify({
    dateFrom,
    dateTo,
  }));
}

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
    confirmation_text: "Thank you for reporting.",
  },
  sit_in: {
    auto_resolve_enabled: true,
    zoom_description: "Zoom session.",
    max_sessions_per_absence: 10,
  },
  notifications: {
    sms_parent_enabled: true,
    sms_parent_template: "Warwick Institute: {{student_name}} ได้แจ้งความประสงค์ขอลาเรียน กรุณาแจ้งรหัส {{code}} ให้แก่นักเรียน เพื่อยืนยันว่าผู้ปกครองได้รับทราบแล้ว",
    sms_success_template: "Warwick Institute: {{nickname}} ได้แจ้งลาเรียน {{absence_summary}} และมีกำหนดเข้าเรียนชดเชย {{sit_in_summary}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ",
    allow_submit_without_otp: false,
  },
  admin_contact: {
    email: "office@example.edu",
    phone: "+66 2123 4567",
    hours: "Mon-Fri 08:00-16:00",
  },
};

const MOCK_STUDENT: {
  student_id: string;
  wcode: string;
  full_name: string;
  parent_phone: string | null;
  subjects: Array<{ id: string; code: string; name: string }>;
} = {
  student_id: "s1",
  wcode: "W250389",
  full_name: "John Smith",
  parent_phone: "+66812345678",
  subjects: [
    { id: "subj-1", code: "MATH", name: "Mathematics" },
    { id: "subj-2", code: "PHYS", name: "Physics" },
  ],
};

const MOCK_SESSIONS = createMockSessionsInRange();

const SUBMISSION_RESPONSE = {
  id: "abc12345",
  wcode: "W250389",
  status: "pending" as const,
  course_id: "c-math201",
  course_code: "MATH201",
  course_name: "Algebra II",
  subject_id: "subj-1",
  subject_code: "MATH",
  subject_name: "Mathematics",
  student_name: "John Smith",
  date_from: "2026-06-01",
  date_to: "2026-06-07",
  reason_category: "medical",
  reason: "Appointment",
  sit_in_method: "zoom",
  sit_in_course_id: "c-math201",
  sit_in_course_code: "MATH201",
  sit_in_course_name: "Algebra II",
  version: 1,
  created_at: "2026-05-27T09:00:00Z",
  updated_at: "2026-05-27T09:00:00Z",
};

const SECOND_SUBMISSION_RESPONSE = {
  id: "def67890",
  wcode: "W250389",
  status: "pending" as const,
  course_id: "c-phys301",
  course_code: "PHYS301",
  course_name: "Physics 301",
  subject_id: "subj-2",
  subject_code: "PHYS",
  subject_name: "Physics",
  student_name: "John Smith",
  date_from: "2026-06-02",
  date_to: "2026-06-02",
  reason_category: "medical",
  reason: "Appointment",
  sit_in_method: "physical",
  sit_in_course_id: "c-phys301",
  sit_in_course_code: "PHYS301",
  sit_in_course_name: "Physics 301",
  version: 1,
  created_at: "2026-05-27T09:01:00Z",
  updated_at: "2026-05-27T09:01:00Z",
};

const OTP_SEND_RESPONSE = {
  token: "otp-token-123",
  status: "pending" as const,
  wcode: MOCK_STUDENT.wcode,
  parent_phone: MOCK_STUDENT.parent_phone,
  otp_last_sent_at: "2026-05-30T08:00:00Z",
  otp_code_expires_at: "2026-06-30T08:10:00Z",
  expires_at: "2026-06-30T08:00:00Z",
};

const OTP_VERIFY_RESPONSE = {
  ...OTP_SEND_RESPONSE,
  status: "verified" as const,
  verified_at: "2026-05-30T08:02:00Z",
};

function installHappyPathMocks(overrides?: {
  student?: typeof MOCK_STUDENT;
  sessions?: unknown;
  send?: unknown;
  verify?: unknown;
  resume?: unknown;
  submission?: unknown;
  config?: unknown;
}) {
  mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
    const path = String(url);

    if (path.includes("absence-form-config")) return overrides?.config ?? MOCK_CONFIG;
    if (path.includes("student-lookup")) return overrides?.student ?? MOCK_STUDENT;
    if (path.includes("sessions-in-range")) return overrides?.sessions ?? MOCK_SESSIONS;
    if (path.includes("/parent-verification/") && init?.method === "GET") {
      return overrides?.resume ?? OTP_SEND_RESPONSE;
    }
    if (path.endsWith("/parent-verification/send")) return overrides?.send ?? OTP_SEND_RESPONSE;
    if (path.endsWith("/parent-verification/verify")) return overrides?.verify ?? OTP_VERIFY_RESPONSE;
    if (path.endsWith("/absences/batch") && init?.method === "POST") {
      return overrides?.submission ?? { items: [SUBMISSION_RESPONSE] };
    }
    if (path.endsWith("/absences") && init?.method === "POST") {
      return overrides?.submission ?? SUBMISSION_RESPONSE;
    }

    throw new Error(`Unmocked API call: ${url}`);
  });
}

async function lookupStudent(user: ReturnType<typeof userEvent.setup>, wcode = "W250389") {
  await user.clear(screen.getByPlaceholderText("e.g. W250389"));
  await user.type(screen.getByPlaceholderText("e.g. W250389"), wcode);
  await user.click(screen.getByRole("button", { name: /search/i }));
  await waitFor(() => expect(screen.getByText("John Smith")).toBeInTheDocument());
}

async function verifyParent(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /send code/i }));
  await waitFor(() => {
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/parent-verification/send",
      expect.objectContaining({ method: "POST" }),
    );
  });

  const codeInput = (await screen.findAllByRole("textbox", { hidden: true })).find(
    el => el.getAttribute("inputMode") === "numeric" || el.getAttribute("aria-label") === "Enter the code",
  )!;
  await user.type(codeInput, "123456");
  await waitFor(() => expect(screen.getByText(/verification complete/i)).toBeInTheDocument());
}

describe("AbsenceForm", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockNavigate.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("renders the lookup form initially", () => {
    installHappyPathMocks();
    renderWithProviders(<AbsenceForm />);

    expect(screen.getByPlaceholderText("e.g. W250389")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /search/i })).toBeInTheDocument();
    expect(screen.getByText("Absence form")).toBeInTheDocument();
  });

  it("normalizes a lowercase w-code before searching", async () => {
    const user = userEvent.setup();
    renderWithDateRange();

    await lookupStudent(user, "w250389");

    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/student-lookup?wcode=W250389",
      expect.objectContaining({ method: "GET" }),
    );
  });

  function renderWithDateRange(overrides?: Parameters<typeof installHappyPathMocks>[0]) {
    prePopulateSessionStorage("2026-06-01", "2026-06-07");
    installHappyPathMocks(overrides);
    renderWithProviders(<AbsenceForm />);
  }

  it("walks through lookup, verification, courses, sessions, and direct submission", async () => {
    const user = userEvent.setup();
    renderWithDateRange();

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));

    expect(screen.getByRole("button", { name: /send code/i })).toBeInTheDocument();

    await verifyParent(user);
    expect(screen.getByText(/Parent confirmed! Now/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");

    // Select all courses first to reveal sessions
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await waitFor(() => expect(screen.getAllByText(/▼ Mathematics/i).length).toBeGreaterThan(0));

    // Select first session checkbox to enable submit
    const sessionCheckboxes = await screen.findAllByRole("checkbox");
    const sessionCheckbox = sessionCheckboxes.find(cb => cb.getAttribute("id")?.startsWith("session-"));
    if (sessionCheckbox) await user.click(sessionCheckbox);
    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    expect(await screen.findByText("Your absence request has been sent and is waiting for review.")).toBeInTheDocument();
    expect(screen.getByText("Submitted classes")).toBeInTheDocument();
    expect(screen.getByText("Absence form")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /done/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /notify another absence/i })).not.toBeInTheDocument();
    expect(screen.queryByText("ABS-ABC12345")).not.toBeInTheDocument();

    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/batch",
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({
          "Idempotency-Key": expect.any(String),
        }),
        body: expect.stringContaining('"verification_token":"otp-token-123"'),
      }),
    );
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/batch",
      expect.objectContaining({
        method: "POST",
        body: expect.stringContaining('"items":['),
      }),
    );
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/batch",
      expect.objectContaining({
        method: "POST",
        body: expect.stringContaining('"course_id":"c-math201"'),
      }),
    );
  }, 30000);

  it("submits selected sessions across more than one day in a single batch", async () => {
    const user = userEvent.setup();
    renderWithDateRange({
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Mathematics",
          course_id: "c-math201",
          course_code: "MATH201",
          course_name: "Mathematics",
          sessions: [
            {
              id: "s1",
              start_at: "2026-06-01T09:00:00Z",
              end_at: "2026-06-01T10:30:00Z",
              date: "2026-06-01",
              already_absent: false,
            },
          ],
        },
        {
          subject_id: "subj-2",
          subject_code: "PHYS",
          subject_name: "Physics",
          course_id: "c-phys301",
          course_code: "PHYS301",
          course_name: "Physics",
          sessions: [
            {
              id: "s2",
              start_at: "2026-06-02T11:00:00Z",
              end_at: "2026-06-02T12:30:00Z",
              date: "2026-06-02",
              already_absent: false,
            },
          ],
        },
      ]),
      submission: { items: [SUBMISSION_RESPONSE, SECOND_SUBMISSION_RESPONSE] },
    });

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");

    await user.click(screen.getByRole("button", { name: /select all/i }));
    const sessionCheckboxes = (await screen.findAllByRole("checkbox")).filter(
      (cb) => cb.getAttribute("id")?.startsWith("session-"),
    );
    await user.click(sessionCheckboxes[0]);
    await user.click(sessionCheckboxes[1]);

    expect(screen.getByRole("button", { name: /submit absence/i })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    expect(await screen.findByText("Your 2 absence requests have been sent and are waiting for review.")).toBeInTheDocument();
    expect(screen.getByText("Submitted classes")).toBeInTheDocument();

    const batchCall = mockApiJson.mock.calls.find(([url]) => url === "/api/v1/absences/batch");
    expect(batchCall).toBeDefined();
    const [, batchInit] = batchCall!;
    const parsedBody = JSON.parse(String(batchInit?.body)) as {
      items: Array<{ course_id: string; date_from: string; date_to: string }>;
    };
    expect(parsedBody.items).toHaveLength(2);
    expect(parsedBody.items[0]).toMatchObject({
      course_id: "c-math201",
      date_from: "2026-06-01",
      date_to: "2026-06-01",
    });
    expect(parsedBody.items[1]).toMatchObject({
      course_id: "c-phys301",
      date_from: "2026-06-02",
      date_to: "2026-06-02",
    });
  });

  it("submits the selected priority sit-in course for SAT Verbal priority options", async () => {
    const user = userEvent.setup();
    renderWithDateRange({
      student: {
        ...MOCK_STUDENT,
        subjects: [
          { id: "subj-satv", code: "SATV", name: "SAT Verbal" },
        ],
      },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-satv",
          subject_code: "SATV",
          subject_name: "SAT Verbal",
          course_id: "c-r3s3",
          course_code: "R3S3",
          course_name: "SAT Verbal Rank 3 Section 3",
          sessions: [
            {
              id: "missed-r3s3-lesson-2",
              start_at: "2026-06-02T09:00:00Z",
              end_at: "2026-06-02T10:30:00Z",
              date: "2026-06-02",
              already_absent: false,
            },
          ],
          sit_in: {
            sit_in_method: "physical",
            priorities: [
              {
                level: 1,
                label: "1st Priority: Another Rank 3 section (same lesson #)",
                sit_in_course: { id: "c-r3s1", code: "R3S1", name: "SAT Verbal Rank 3 Section 1" },
                available_sessions: [
                  {
                    id: "sit-r3s1-lesson-2",
                    start_at: "2026-06-04T09:00:00Z",
                    end_at: "2026-06-04T10:30:00Z",
                    course_name: "SAT Verbal Rank 3 Section 1",
                  },
                ],
              },
              {
                level: 3,
                label: "3rd Priority: Rank 4 Reading or Writing",
                sit_in_course: { id: "c-r4r", code: "R4R", name: "SAT Verbal Reading Rank 4" },
                available_sessions: [
                  {
                    id: "sit-r4r",
                    start_at: "2026-06-05T09:00:00Z",
                    end_at: "2026-06-05T10:30:00Z",
                    course_name: "SAT Verbal Reading Rank 4",
                  },
                ],
              },
            ],
          },
        },
      ]),
    });

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await user.click(await screen.findByRole("checkbox"));
    await user.selectOptions(await screen.findByRole("combobox"), "sit-r3s1-lesson-2");
    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    const batchCall = await waitFor(() => {
      const call = mockApiJson.mock.calls.find(([url]) => url === "/api/v1/absences/batch");
      expect(call).toBeDefined();
      return call!;
    });
    const parsedBody = JSON.parse(String(batchCall[1]?.body)) as {
      items: Array<{ course_id: string; sit_in_course_id: string; sit_in_session_ids: string[] }>;
    };
    expect(parsedBody.items[0]).toMatchObject({
      course_id: "c-r3s3",
      sit_in_course_id: "c-r3s1",
      sit_in_session_ids: ["sit-r3s1-lesson-2"],
    });
  }, 30000);

  it("advances and returns SAT Verbal priority display across skipped priority levels", async () => {
    const user = userEvent.setup();
    const initialSessions = createMockSessionsInRange([
      {
        subject_id: "subj-satv",
        subject_code: "SATV",
        subject_name: "SAT Verbal",
        course_id: "c-r3s3",
        course_code: "R3S3",
        course_name: "SAT Verbal Rank 3 Section 3",
        sessions: [
          {
            id: "missed-r3s3",
            start_at: "2026-06-02T09:00:00Z",
            end_at: "2026-06-02T10:30:00Z",
            date: "2026-06-02",
            already_absent: false,
          },
        ],
        sit_in: {
          sit_in_method: "physical",
          current_priority_level: 1,
          has_next_priority: true,
          priorities: [
            {
              level: 1,
              label: "1st Priority: Another Rank 3 section (same lesson #)",
              sit_in_course: { id: "c-r3s1", code: "R3S1", name: "SAT Verbal Rank 3 Section 1" },
              available_sessions: [{ id: "sit-r3s1", start_at: "2026-06-04T09:00:00Z", end_at: "2026-06-04T10:30:00Z" }],
            },
          ],
        },
      },
    ]);
    const nextSessions = createMockSessionsInRange([
      {
        ...initialSessions.subjects[0],
        sit_in: {
          sit_in_method: "physical",
          current_priority_level: 3,
          has_next_priority: false,
          priorities: [
            {
              level: 3,
              label: "3rd Priority: Rank 4 Reading or Writing",
              sit_in_course: { id: "c-r4r", code: "R4R", name: "SAT Verbal Reading Rank 4" },
              available_sessions: [{ id: "sit-r4r", start_at: "2026-06-05T09:00:00Z", end_at: "2026-06-05T10:30:00Z" }],
            },
          ],
        },
      },
    ]);
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      const path = String(url);
      if (path.includes("absence-form-config")) return MOCK_CONFIG;
      if (path.includes("student-lookup")) {
        return { ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] };
      }
      if (path.includes("sessions-in-range") && path.includes("sat_verbal_after_priority=1")) return nextSessions;
      if (path.includes("sessions-in-range")) return initialSessions;
      if (path.includes("/parent-verification/") && init?.method === "GET") return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/send")) return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/verify")) return OTP_VERIFY_RESPONSE;
      if (path.endsWith("/absences/batch") && init?.method === "POST") return { items: [SUBMISSION_RESPONSE] };
      throw new Error(`Unmocked API call: ${url}`);
    });
    prePopulateSessionStorage("2026-06-01", "2026-06-07");
    renderWithProviders(<AbsenceForm />);

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await user.click(await screen.findByRole("checkbox"));

    expect(await screen.findByText(/1st Priority: Another Rank 3 section/)).toBeInTheDocument();
    expect(screen.queryByText(/3rd Priority: Rank 4 Reading or Writing/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /see other times/i }));
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        expect.stringContaining("sat_verbal_after_priority=1"),
        expect.anything(),
      );
    });
    expect(await screen.findByText(/3rd Priority: Rank 4 Reading or Writing/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /see previous times/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /see other times/i })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /see previous times/i }));
    expect(await screen.findByText(/1st Priority: Another Rank 3 section/)).toBeInTheDocument();
    expect(screen.queryByText(/3rd Priority: Rank 4 Reading or Writing/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /see other times/i })).toBeInTheDocument();
  }, 30000);

  it("shows an unavailable first SAT Verbal priority before revealing the next priority", async () => {
    const user = userEvent.setup();
    const initialSessions = createMockSessionsInRange([
      {
        subject_id: "subj-satv",
        subject_code: "SATV",
        subject_name: "SAT Verbal Writing Beginner Section 1 C2/26",
        course_id: "c-writing-1",
        course_code: "W1",
        course_name: "SAT Verbal Writing Beginner Section 1 C2/26",
        sessions: [
          {
            id: "missed-writing-1",
            start_at: "2026-06-16T17:00:00Z",
            end_at: "2026-06-16T20:20:00Z",
            date: "2026-06-16",
            already_absent: false,
          },
        ],
        sit_in: {
          sit_in_method: "physical",
          current_priority_level: 1,
          has_next_priority: true,
          priorities: [
            {
              level: 1,
              label: "1st Priority: Same Writing Beginner lesson in another section",
              available_sessions: [],
            },
          ],
        },
      },
    ]);
    const nextSessions = createMockSessionsInRange([
      {
        ...initialSessions.subjects[0],
        sit_in: {
          sit_in_method: "physical",
          current_priority_level: 2,
          has_next_priority: false,
          priorities: [
            {
              level: 2,
              label: "2nd Priority: SAT Verbal Writing Rank 5",
              sit_in_course: {
                id: "c-writing-rank5",
                code: "WR5",
                name: "SAT Verbal Writing Rank 5 C2/26",
              },
              available_sessions: [
                {
                  id: "sit-writing-rank5",
                  start_at: "2026-06-17T17:00:00Z",
                  end_at: "2026-06-17T20:20:00Z",
                  course_name: "SAT Verbal Writing Rank 5 C2/26",
                },
              ],
            },
          ],
        },
      },
    ]);
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      const path = String(url);
      if (path.includes("absence-form-config")) return MOCK_CONFIG;
      if (path.includes("student-lookup")) {
        return { ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] };
      }
      if (path.includes("sessions-in-range") && path.includes("sat_verbal_after_priority=1")) return nextSessions;
      if (path.includes("sessions-in-range")) return initialSessions;
      if (path.includes("/parent-verification/") && init?.method === "GET") return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/send")) return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/verify")) return OTP_VERIFY_RESPONSE;
      throw new Error(`Unmocked API call: ${url}`);
    });
    prePopulateSessionStorage("2026-06-16", "2026-06-16");
    renderWithProviders(<AbsenceForm />);

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await user.click(await screen.findByRole("checkbox"));

    expect(await screen.findByText(/1st Priority: Same Writing Beginner lesson/)).toBeInTheDocument();
    expect(screen.getByText("Sit-in").closest("p")).toHaveTextContent("Not available");
    expect(screen.getByText("No available make-up class for this priority.")).toBeInTheDocument();
    expect(screen.queryByText(/2nd Priority: SAT Verbal Writing Rank 5/)).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /see other times/i }));

    expect(await screen.findByText(/2nd Priority: SAT Verbal Writing Rank 5/)).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /SAT Verbal Writing Rank 5 C2\/26/ })).toBeInTheDocument();
  }, 30000);

  it("shows every SAT Verbal target returned at the current priority level", async () => {
    const user = userEvent.setup();
    renderWithDateRange({
      student: {
        ...MOCK_STUDENT,
        subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }],
      },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-satv",
          subject_code: "SATV",
          subject_name: "SAT Verbal",
          course_id: "c-r3s3",
          course_code: "R3S3",
          course_name: "SAT Verbal Rank 3 Section 3",
          sessions: [
            {
              id: "missed-r3s3",
              start_at: "2026-06-02T09:00:00Z",
              end_at: "2026-06-02T10:30:00Z",
              date: "2026-06-02",
              already_absent: false,
            },
          ],
          sit_in: {
            sit_in_method: "physical",
            priorities: [
              {
                level: 1,
                label: "1st Priority: Another Rank 3 section (same lesson #)",
                sit_in_course: { id: "c-r3s1", code: "R3S1", name: "SAT Verbal Rank 3 Section 1" },
                available_sessions: [{ id: "sit-r3s1", start_at: "2026-06-04T09:00:00Z", end_at: "2026-06-04T10:30:00Z" }],
              },
              {
                level: 1,
                label: "1st Priority: Another Rank 3 section (same lesson #)",
                sit_in_course: { id: "c-r3s2", code: "R3S2", name: "SAT Verbal Rank 3 Section 2" },
                available_sessions: [{ id: "sit-r3s2", start_at: "2026-06-05T09:00:00Z", end_at: "2026-06-05T10:30:00Z" }],
              },
            ],
          },
        },
      ]),
    });

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await user.click(await screen.findByRole("checkbox"));

    expect(await screen.findByRole("option", { name: /SAT Verbal Rank 3 Section 1/ })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /SAT Verbal Rank 3 Section 2/ })).toBeInTheDocument();
  }, 30000);

  it("shows the current priority sit-in target in the header and dropdown", async () => {
    const user = userEvent.setup();
    const initialSessions = createMockSessionsInRange([
      {
        subject_id: "subj-satv",
        subject_code: "SATV",
        subject_name: "SAT Verbal Writing Beginner Section 1 C2/26",
        course_id: "c-writing-1",
        course_code: "W1",
        course_name: "SAT Verbal Writing Beginner Section 1 C2/26",
        sessions: [
          {
            id: "missed-writing-1",
            start_at: "2026-06-09T10:00:00Z",
            end_at: "2026-06-09T13:20:00Z",
            date: "2026-06-09",
            already_absent: false,
          },
        ],
        sit_in: {
          sit_in_method: "physical",
          sit_in_course: {
            id: "c-writing-1",
            code: "W1",
            name: "SAT Verbal Writing Beginner Section 1 C2/26",
          },
          current_priority_level: 1,
          has_next_priority: true,
          priorities: [
            {
              level: 1,
              label: "1st Priority",
              sit_in_course: {
                id: "c-writing-2",
                code: "W2",
                name: "SAT Verbal Writing Beginner Section 2 C2/26",
              },
              available_sessions: [
                {
                  id: "sit-writing-2",
                  start_at: "2026-06-14T10:00:00Z",
                  end_at: "2026-06-14T13:20:00Z",
                  course_name: "SAT Verbal Writing Beginner Section 2 C2/26",
                },
              ],
            },
          ],
        },
      },
    ]);
    const nextSessions = createMockSessionsInRange([
      {
        ...initialSessions.subjects[0],
        sit_in: {
          sit_in_method: "physical",
          current_priority_level: 2,
          has_next_priority: false,
          priorities: [
            {
              level: 2,
              label: "2nd Priority",
              sit_in_course: {
                id: "c-writing-3",
                code: "W3",
                name: "SAT Verbal Writing Beginner Section 3 C2/26",
              },
              available_sessions: [
                {
                  id: "sit-writing-3",
                  start_at: "2026-06-15T10:00:00Z",
                  end_at: "2026-06-15T13:20:00Z",
                  course_name: "SAT Verbal Writing Beginner Section 3 C2/26",
                },
              ],
            },
          ],
        },
      },
    ]);
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      const path = String(url);
      if (path.includes("absence-form-config")) return MOCK_CONFIG;
      if (path.includes("student-lookup")) {
        return { ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] };
      }
      if (path.includes("sessions-in-range") && path.includes("sat_verbal_after_priority=1")) return nextSessions;
      if (path.includes("sessions-in-range")) return initialSessions;
      if (path.includes("/parent-verification/") && init?.method === "GET") return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/send")) return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/verify")) return OTP_VERIFY_RESPONSE;
      throw new Error(`Unmocked API call: ${url}`);
    });
    prePopulateSessionStorage("2026-06-09", "2026-06-15");
    renderWithProviders(<AbsenceForm />);

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await user.click(await screen.findByRole("checkbox"));

    expect(await screen.findByText(/Subject:\s*SAT Verbal Writing Beginner Section 2 C2\/26/i)).toBeInTheDocument();
    expect(screen.getByText("Sit-in").closest("p")).toHaveTextContent("SAT Verbal Writing Beginner Section 2 C2/26");
    expect(screen.getByRole("option", { name: /SAT Verbal Writing Beginner Section 2 C2\/26/ })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /see other times/i }));
    expect(await screen.findByText(/Subject:\s*SAT Verbal Writing Beginner Section 3 C2\/26/i)).toBeInTheDocument();
    expect(screen.getByText("Sit-in").closest("p")).toHaveTextContent("SAT Verbal Writing Beginner Section 3 C2/26");
    expect(screen.getByRole("option", { name: /SAT Verbal Writing Beginner Section 3 C2\/26/ })).toBeInTheDocument();
  }, 30000);

  it("disables verify parent button when student has no parent phone", async () => {
    installHappyPathMocks({
      student: {
        ...MOCK_STUDENT,
        parent_phone: null,
      },
    });
    const user = userEvent.setup();
    renderWithProviders(<AbsenceForm />);

    await lookupStudent(user);

    const verifyBtn = screen.getByRole("button", { name: /verify with parent/i });
    expect(verifyBtn).toBeDisabled();
  });

  it("shows contact admin message when student has no parent phone", async () => {
    installHappyPathMocks({
      student: {
        ...MOCK_STUDENT,
        parent_phone: null,
      },
    });
    renderWithProviders(<AbsenceForm />);

    await lookupStudent(userEvent.setup());

    const phoneMatches = screen.getAllByText(/02-658-4880/);
    expect(phoneMatches.length).toBeGreaterThanOrEqual(1);
    const lineMatches = screen.getAllByText(/@warwick/);
    expect(lineMatches.length).toBeGreaterThanOrEqual(1);
  });

  it("shows a no-sessions status message when no sessions exist in range", async () => {
    const user = userEvent.setup();
    renderWithDateRange({ sessions: { subjects: [] } });

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());
    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Family matter");

    // Select a course to reveal sessions section
    await user.click(screen.getByRole("button", { name: /select all/i }));

    expect(await screen.findByText("No classes found for the courses and dates you picked.")).toBeInTheDocument();
  });

  it("shows always-visible reason textarea on Step 2 after courses + dates are set", async () => {
    const user = userEvent.setup();
    renderWithDateRange();

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    expect(screen.getByPlaceholderText("Tell us why you'll be away from class...")).toBeInTheDocument();
    expect(screen.getByText("Reason for absence")).toBeInTheDocument();
  });

  it("shows the resolved sit-in target as Absence class", async () => {
    const user = userEvent.setup();
    renderWithDateRange({
      student: {
        ...MOCK_STUDENT,
        subjects: [
          { id: "subj-1", code: "02", name: "math_advance" },
        ],
      },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1",
          subject_code: "02",
          subject_name: "math_advance",
          course_id: "c-adv",
          course_code: "0000000344",
          course_name: "math_advance",
          sessions: [
            {
              id: "s1",
              start_at: "2026-06-02T09:00:00Z",
              end_at: "2026-06-02T11:00:00Z",
              date: "2026-06-02",
              already_absent: false,
            },
          ],
          sit_in: {
            sit_in_method: "physical",
            sit_in_course: { id: "c-int", code: "0000000348", name: "Math inter" },
            available_sessions: [
              {
                id: "as1",
                start_at: "2026-06-04T03:00:00Z",
                end_at: "2026-06-04T05:00:00Z",
              },
            ],
          },
        },
        {
          subject_id: "subj-2",
          subject_code: "04",
          subject_name: "Math inter",
          course_id: "c-int",
          course_code: "0000000348",
          course_name: "Math inter",
          sessions: [],
        },
      ]),
    });

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await user.click(await screen.findByRole("checkbox"));

    const makeUpSelect = await screen.findByRole("combobox");
    expect(makeUpSelect).toHaveTextContent("Math inter");
    expect(makeUpSelect).not.toHaveTextContent("0000000348");

    expect(screen.getByText(/Subject:\s*Math inter/i)).toBeInTheDocument();
  });

  it("shows the subject name (not raw code) in make-up dropdown when sit_in_course.name is empty and course not in enrolled subjects", async () => {
    const user = userEvent.setup();
    renderWithDateRange({
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Math advance",
          course_id: "c-adv",
          course_code: "ADV-01",
          course_name: "Math advance",
          sessions: [
            {
              id: "s1",
              start_at: "2026-06-02T09:00:00Z",
              end_at: "2026-06-02T10:30:00Z",
              date: "2026-06-02",
              already_absent: false,
            },
          ],
          sit_in: {
            sit_in_method: "physical",
            sit_in_course: { id: "c-int", code: "0000000348", name: "" },
            available_sessions: [
              {
                id: "as1",
                start_at: "2026-06-04T13:00:00Z",
                end_at: "2026-06-04T15:00:00Z",
              },
            ],
          },
        },
      ]),
    });

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await user.click(await screen.findByRole("checkbox"));

    const makeUpSelect = await screen.findByRole("combobox");
    expect(makeUpSelect).toHaveTextContent("Math advance");
    expect(makeUpSelect).not.toHaveTextContent("0000000348");
  });

  it("shows sit-in target course name (not student's enrolled course) in make-up dropdown when sit_in_course has name populated", async () => {
    const user = userEvent.setup();
    renderWithDateRange({
      student: {
        ...MOCK_STUDENT,
        subjects: [
          { id: "subj-1", code: "MATH", name: "Mathematics" },
        ],
      },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Mathematics",
          course_id: "c-adv",
          course_code: "ADV-01",
          course_name: "Mathematics",
          sessions: [
            {
              id: "s1",
              start_at: "2026-06-02T09:00:00Z",
              end_at: "2026-06-02T10:30:00Z",
              date: "2026-06-02",
              already_absent: false,
            },
          ],
          sit_in: {
            sit_in_method: "physical",
            sit_in_course: { id: "c-scholar", code: "SCH-01", name: "scholar" },
            available_sessions: [
              {
                id: "as1",
                start_at: "2026-06-04T08:00:00Z",
                end_at: "2026-06-04T10:00:00Z",
              },
            ],
          },
        },
      ]),
    });

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await user.click(await screen.findByRole("checkbox"));

    const makeUpSelect = await screen.findByRole("combobox");
    expect(makeUpSelect).toHaveTextContent("scholar");
    expect(makeUpSelect).not.toHaveTextContent("advanced");
    expect(makeUpSelect).not.toHaveTextContent("ADV-01");
    expect(makeUpSelect).not.toHaveTextContent("0000000348");
  });

  it("uses the resolved Scholar sit-in course for mixed inter and advanced enrollments", async () => {
    const user = userEvent.setup();
    renderWithDateRange({
      student: {
        ...MOCK_STUDENT,
        subjects: [
          { id: "subj-math", code: "MATH", name: "Math" },
        ],
      },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-math",
          subject_code: "MATH",
          subject_name: "Math inter",
          course_id: "c-inter",
          course_code: "0000000348",
          course_name: "Math inter",
          sessions: [
            {
              id: "s-inter",
              start_at: "2026-06-04T10:00:00+07:00",
              end_at: "2026-06-04T12:00:00+07:00",
              date: "2026-06-04",
              already_absent: false,
            },
          ],
          sit_in: {
            sit_in_method: "physical",
            sit_in_course: { id: "c-scholar", code: "0000000371", name: "", subject_name: "Scholar" },
            available_sessions: [
              {
                id: "as-scholar",
                start_at: "2026-06-06T10:00:00+07:00",
                end_at: "2026-06-06T12:00:00+07:00",
                subject_name: "Math advance",
                course_name: "Math advance",
              },
            ],
          },
        },
      ]),
    });

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Need a make-up class");
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await user.click(await screen.findByRole("checkbox"));

    const makeUpSelect = await screen.findByRole("combobox");
    expect(makeUpSelect).toHaveTextContent("Scholar");
    expect(makeUpSelect).not.toHaveTextContent("Math advance");
    expect(makeUpSelect).not.toHaveTextContent("0000000371");
    expect(screen.getByText(/Subject:\s*Scholar/i)).toBeInTheDocument();
  });

  it("shows the sit-in class name from the available session instead of the absence class name", async () => {
    const user = userEvent.setup();
    renderWithDateRange({
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1",
          subject_code: "ADV",
          subject_name: "Math advance",
          course_id: "c-adv",
          course_code: "ADV-01",
          course_name: "Math advance",
          sessions: [
            {
              id: "s1",
              start_at: "2026-06-02T09:00:00Z",
              end_at: "2026-06-02T10:30:00Z",
              date: "2026-06-02",
              already_absent: false,
            },
          ],
          sit_in: {
            sit_in_method: "physical",
            sit_in_course: { id: "c-int", code: "INT-01", name: "Math inter" },
            available_sessions: [
              {
                id: "as1",
                start_at: "2026-06-18T10:00:00Z",
                end_at: "2026-06-18T12:00:00Z",
                subject_name: "Math inter",
              },
            ],
          },
        },
      ]),
    });

    await lookupStudent(user);
    await user.click(screen.getByRole("button", { name: /verify with parent/i }));
    await verifyParent(user);
    await user.click(screen.getByRole("button", { name: /^continue$/i }));
    await waitFor(() => expect(screen.getByText("Choose your courses")).toBeInTheDocument());

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Need a make-up class");
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await user.click(await screen.findByRole("checkbox"));

    const makeUpSelect = await screen.findByRole("combobox");
    expect(makeUpSelect).toHaveTextContent("Math inter");
    expect(makeUpSelect).not.toHaveTextContent("Math advance");
  });
});
