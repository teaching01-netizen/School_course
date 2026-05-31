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
    sms_success_template: "Warwick Institute: {{nickname}} ได้แจ้งลาเรียนคลาส {{class_name}} ในวันที่ {{absence_date}} และมีกำหนดเข้าเรียนชดเชย คลาส {{sit_in_class}} ในวันที่ {{sit_in_date_time}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ",
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
    if (path.endsWith("/absences") && init?.method === "POST") {
      return overrides?.submission ?? SUBMISSION_RESPONSE;
    }

    throw new Error(`Unmocked API call: ${url}`);
  });
}

async function lookupStudent(user: ReturnType<typeof userEvent.setup>) {
  await user.clear(screen.getByPlaceholderText("e.g. W250389"));
  await user.type(screen.getByPlaceholderText("e.g. W250389"), "W250389");
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

    // Select a course first to reveal sessions
    await user.click(screen.getByRole("button", { name: /select all/i }));
    await waitFor(() => expect(screen.getAllByText(/▼ MATH - Mathematics/i).length).toBeGreaterThan(0));

    const sessionSelectAll = screen.getAllByRole("button", { name: /^select all$/i })[1];
    await user.click(sessionSelectAll);
    await user.click(screen.getByRole("button", { name: /submit absence/i }));

    expect(await screen.findByText("Your absence request has been sent and is waiting for review.")).toBeInTheDocument();
    expect(screen.getByText("Absence form")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /done/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /notify another absence/i })).not.toBeInTheDocument();
    expect(screen.queryByText("ABS-ABC12345")).not.toBeInTheDocument();

    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences",
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({
          "Idempotency-Key": expect.any(String),
        }),
        body: expect.stringContaining('"verification_token":"otp-token-123"'),
      }),
    );
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

  it("shows the student's own subject as Absence class, not the sit-in target's subject", async () => {
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

    expect(screen.getByText(/^Absence class: math_advance$/)).toBeInTheDocument();
    expect(screen.queryByText(/^Absence class: Math inter$/)).not.toBeInTheDocument();
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
