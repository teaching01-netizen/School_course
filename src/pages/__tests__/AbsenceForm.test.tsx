import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AbsenceForm from "../AbsenceForm";
import { renderWithProviders, createMockSessionsInRange } from "./helpers";
import { ApiRequestError } from "@/api/client";

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
const LEGACY_VERIFICATION_STORAGE_KEY = `${SESSION_STORAGE_KEY}:parent-verification`;
const STUDENT_RESUME_STORAGE_KEY = "warwick-absence-form-student-v1";
const VERIFICATION_STORAGE_KEY = "warwick-absence-parent-verification-v1";

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
    sms_parent_template: "template",
    sms_success_template: "success template",
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
  nickname?: string | null;
  parent_phone: string | null;
  subjects: Array<{
    id: string;
    code: string;
    name: string;
    merge_group_id?: string;
    merge_group_name?: string;
  }>;
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

function publicLookupFor(student: typeof MOCK_STUDENT) {
  // Mirrors the server: the hint prefers the nickname and falls back to the
  // full name, masked to the first character; the phone hint shows the last
  // four digits only.
  const hintSource = student.nickname?.trim() || student.full_name.trim();
  const phoneDigits = student.parent_phone?.replace(/\D/g, "") ?? "";
  return {
    wcode: student.wcode,
    lookup_token: "lookup-token",
    email_input_required: true,
    parent_verification_available: Boolean(student.parent_phone),
    ...(hintSource ? { nickname_hint: `${Array.from(hintSource)[0]}***` } : {}),
    ...(phoneDigits.length >= 4 ? { parent_phone_hint: `••••${phoneDigits.slice(-4)}` } : {}),
  };
}

function verifiedProfileFor(student: typeof MOCK_STUDENT) {
  return {
    wcode: student.wcode,
    display_name: student.full_name,
    email_on_file: false,
    nickname_set: Boolean(student.nickname?.trim()),
    subjects: student.subjects,
  };
}

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
  otp_code_expires_at: new Date(Date.now() + 10 * 60_000).toISOString(),
  expires_at: new Date(Date.now() + 60 * 60_000).toISOString(),
};

const OTP_VERIFY_RESPONSE = {
  ...OTP_SEND_RESPONSE,
  status: "verified" as const,
  verified_at: "2026-05-30T08:02:00Z",
};

function installHappyPathMocks(overrides?: {
  student?: typeof MOCK_STUDENT;
  sessions?: unknown | (() => unknown);
  send?: unknown;
  verify?: unknown;
  verificationStatus?: unknown;
  submission?: unknown;
  config?: unknown;
}) {
  mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
    const path = String(url);
    const student = overrides?.student ?? MOCK_STUDENT;
    if (path.includes("absence-form-config")) return overrides?.config ?? MOCK_CONFIG;
    if (path.endsWith("/absence-self-service/lookup")) return publicLookupFor(student);
    if (path.endsWith("/absence-self-service/me")) return verifiedProfileFor(student);
    if (path.includes("/absence-self-service/sessions")) {
      const sessions = overrides?.sessions;
      return typeof sessions === "function" ? sessions() : sessions ?? MOCK_SESSIONS;
    }
    if (path.endsWith("/parent-verification/status") && init?.method === "POST") {
      if (overrides?.verificationStatus instanceof Error) throw overrides.verificationStatus;
      return overrides?.verificationStatus ?? OTP_SEND_RESPONSE;
    }
    if (path.endsWith("/parent-verification/send")) return overrides?.send ?? OTP_SEND_RESPONSE;
    if (path.endsWith("/parent-verification/verify")) {
      if (typeof overrides?.verify === "function") return overrides.verify(init);
      return overrides?.verify ?? OTP_VERIFY_RESPONSE;
    }
    if (path.endsWith("/absences/batch") && init?.method === "POST") {
      if (typeof overrides?.submission === "function") {
        return overrides.submission(init);
      }
      return overrides?.submission ?? { items: [SUBMISSION_RESPONSE] };
    }
    if (path.endsWith("/absences") && init?.method === "POST") {
      return overrides?.submission ?? SUBMISSION_RESPONSE;
    }
    throw new Error(`Unmocked API call: ${url}`);
  });
}

async function lookupStudent(user: ReturnType<typeof userEvent.setup>, wcode = "W250389") {
  const input = await screen.findByPlaceholderText("e.g. W250389");
  await user.clear(input);
  await user.type(input, wcode);
  await user.click(screen.getByRole("button", { name: /search/i }));
  await waitFor(() => expect(screen.getByText("Student ID found")).toBeInTheDocument());
}

async function verifyParent(user: ReturnType<typeof userEvent.setup>) {
  const emailInput = screen.queryByRole("textbox", { name: /your email address/i });
  if (emailInput && !(emailInput as HTMLInputElement).value) {
    await user.type(emailInput, "student@example.com");
  }
  const continueButton = screen.queryByRole("button", { name: /continue to verification/i });
  if (continueButton) await user.click(continueButton);
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
  await waitFor(() => {
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/parent-verification/verify",
      expect.objectContaining({ method: "POST" }),
    );
  });
}

async function goToVerification(user: ReturnType<typeof userEvent.setup>) {
  const emailInput = screen.queryByRole("textbox", { name: /your email address/i });
  if (emailInput && !(emailInput as HTMLInputElement).value) {
    await user.type(emailInput, "student@example.com");
  }
  await user.click(screen.getByRole("button", { name: /continue to verification/i }));
  await screen.findByRole("heading", { name: /parent verification/i });
}

async function goToCourses(_user: ReturnType<typeof userEvent.setup>) {
  await waitFor(() => expect(screen.getByText("Courses & classes")).toBeInTheDocument());
}

async function toggleAllCourseSwitches(user: ReturnType<typeof userEvent.setup>) {
  const courseCheckboxes = (await screen.findAllByRole("checkbox")).filter(
    (cb) => cb.getAttribute("id")?.startsWith("subject-"),
  );
  for (const cb of courseCheckboxes) {
    await user.click(cb);
  }
  await waitFor(() => {
    expect(courseCheckboxes[0]).toBeChecked();
  });
}

async function findSessionCheckbox(): Promise<HTMLElement> {
  const all = await screen.findAllByRole("checkbox");
  const session = all.find(cb => cb.getAttribute("id")?.startsWith("session-"));
  if (!session) throw new Error("No session checkbox found");
  return session;
}

function renderAbsenceForm(overrides?: Parameters<typeof installHappyPathMocks>[0]) {
  installHappyPathMocks(overrides);
  renderWithProviders(<AbsenceForm />);
}

describe("AbsenceForm", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockNavigate.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("renders the lookup form initially", async () => {
    installHappyPathMocks();
    renderWithProviders(<AbsenceForm />);
    expect(await screen.findByPlaceholderText("e.g. W250389")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: /search/i })).toBeInTheDocument();
    expect(await screen.findByText("Find your profile")).toBeInTheDocument();
  });

  it("uses a contained app shell with numbered steps and no progress bar", async () => {
    installHappyPathMocks();
    renderWithProviders(<AbsenceForm />);

    expect(await screen.findByRole("banner")).toBeInTheDocument();
    expect(screen.getByRole("main")).toHaveAttribute("id", "absence-form-content");
    expect(screen.getByRole("contentinfo")).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Student - current" })).toHaveTextContent("1");
  });

  it("uses four ordered steps and keeps verification and session loading on their dedicated screens", async () => {
    const user = userEvent.setup();
    renderAbsenceForm();

    const progress = await screen.findByRole("navigation", { name: /progress/i });
    expect(within(progress).getAllByRole("button", { hidden: true }).map((button) => button.getAttribute("aria-label"))).toEqual([
      "Student - current",
      "Verify",
      "Classes",
      "Review",
    ]);
    expect(screen.getByText("Step 1 of 4: Student")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Continue to verification" })).toBeInTheDocument();

    await lookupStudent(user);
    expect(screen.queryByRole("heading", { name: /parent verification/i })).not.toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([url]) => String(url).includes("/absence-self-service/sessions"))).toBe(false);

    await user.type(screen.getByRole("textbox", { name: /your email address/i }), "student@example.com");
    await user.click(screen.getByRole("button", { name: /continue to verification/i }));
    expect(await screen.findByText(/parent verification/i)).toBeInTheDocument();
    expect(screen.getByText("Step 2 of 4: Verify")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Continue to classes" })).toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([url]) => String(url).includes("/absence-self-service/sessions"))).toBe(false);

    await verifyParent(user);
    expect(await screen.findByText("Courses & classes")).toBeInTheDocument();
    expect(screen.getByText("Step 3 of 4: Classes")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Review absence" })).toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([url]) => String(url).includes("/absence-self-service/sessions"))).toBe(true);
  });

  it("discards the legacy absence draft instead of restoring critical selections", async () => {
    window.sessionStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify({
      step: 2,
      lookup: MOCK_STUDENT,
      lookupInput: MOCK_STUDENT.wcode,
      selectedSubjectIds: ["subj-1"],
      selectedSessionIds: ["session-1"],
      sitInSelections: { "session-1": "sit-in-1" },
      sitInPriorityLevels: { "session-1": 2 },
      reason: "Stale reason",
    }));
    window.sessionStorage.setItem(LEGACY_VERIFICATION_STORAGE_KEY, JSON.stringify({ token: "legacy-token" }));

    renderAbsenceForm();

    expect(await screen.findByText("Find your profile")).toBeInTheDocument();
    expect(screen.queryByText("Review your absence")).not.toBeInTheDocument();
    expect(screen.queryByDisplayValue("Stale reason")).not.toBeInTheDocument();
    await waitFor(() => expect(window.sessionStorage.getItem(SESSION_STORAGE_KEY)).toBeNull());
    expect(window.sessionStorage.getItem(LEGACY_VERIFICATION_STORAGE_KEY)).toBeNull();
  });

  it("removes a malformed student resume record", async () => {
    window.sessionStorage.setItem(STUDENT_RESUME_STORAGE_KEY, "{malformed");

    renderAbsenceForm();

    expect(await screen.findByText("Find your profile")).toBeInTheDocument();
    await waitFor(() => expect(window.sessionStorage.getItem(STUDENT_RESUME_STORAGE_KEY)).toBeNull());
  });

  it("starts a new OTP session without reusing a verified token", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      send: { ...OTP_SEND_RESPONSE, token: "otp-token-new" },
      verify: { ...OTP_VERIFY_RESPONSE, token: "otp-token-new" },
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await user.click(screen.getByRole("button", { name: /^back$/i }));

    expect(await screen.findByText("✓ Verified")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /send new code/i }));

    await waitFor(() => {
      const sendCalls = mockApiJson.mock.calls.filter(([url]) => String(url).endsWith("/parent-verification/send"));
      expect(sendCalls).toHaveLength(2);
      expect(JSON.parse(String(sendCalls[1][1]?.body))).toEqual({ lookup_token: "lookup-token" });
    });
    expect(window.sessionStorage.getItem(VERIFICATION_STORAGE_KEY)).toContain("otp-token-new");

    const codeInput = (await screen.findAllByRole("textbox", { hidden: true })).find(
      el => el.getAttribute("inputMode") === "numeric" || el.getAttribute("aria-label") === "Enter the code",
    )!;
    await user.type(codeInput, "654321");
    await waitFor(() => {
      const verifyCalls = mockApiJson.mock.calls.filter(([url]) => String(url).endsWith("/parent-verification/verify"));
      expect(JSON.parse(String(verifyCalls.at(-1)?.[1]?.body))).toEqual({
        token: "otp-token-new",
        code: "654321",
      });
    });
    expect(await screen.findByText("Courses & classes")).toBeInTheDocument();
  });

  it("resumes only the student identifier and refetches the authoritative lookup", async () => {
    window.sessionStorage.setItem(STUDENT_RESUME_STORAGE_KEY, JSON.stringify({
      wcode: MOCK_STUDENT.wcode,
      collectedEmail: "student@example.com",
    }));

    renderAbsenceForm();

    expect(await screen.findByText("Student ID found")).toBeInTheDocument();
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absence-self-service/lookup",
      expect.objectContaining({ method: "POST", body: JSON.stringify({ wcode: MOCK_STUDENT.wcode }) }),
    );
    expect(screen.getByDisplayValue("student@example.com")).toBeInTheDocument();
    expect(screen.getByText("Find your profile")).toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([url]) => String(url).includes("/absence-self-service/sessions"))).toBe(false);
  });

  it("revalidates a stored verified token without restoring the classes step", async () => {
    window.sessionStorage.setItem(STUDENT_RESUME_STORAGE_KEY, JSON.stringify({ wcode: MOCK_STUDENT.wcode }));
    window.sessionStorage.setItem(VERIFICATION_STORAGE_KEY, JSON.stringify({
      token: "stored-verified-token",
      expiresAt: Date.now() + 60_000,
    }));

    renderAbsenceForm({
      verificationStatus: { ...OTP_VERIFY_RESPONSE, token: "stored-verified-token" },
    });

    await screen.findByText("Student ID found");
    await goToVerification(userEvent.setup());
    expect(await screen.findByText("✓ Verified")).toBeInTheDocument();
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/parent-verification/status",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ token: "stored-verified-token" }),
      }),
    );
    expect(screen.getByText("Parent verification")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /send new code/i })).toBeInTheDocument();
  });

  it.each([
    ["consumed", { ...OTP_VERIFY_RESPONSE, token: "stored-token", status: "consumed" as const }],
    ["student mismatch", { ...OTP_VERIFY_RESPONSE, token: "stored-token", wcode: "W999999" }],
  ])("clears a stored token when its server status is %s", async (_label, verificationStatus) => {
    window.sessionStorage.setItem(STUDENT_RESUME_STORAGE_KEY, JSON.stringify({ wcode: MOCK_STUDENT.wcode }));
    window.sessionStorage.setItem(VERIFICATION_STORAGE_KEY, JSON.stringify({
      token: "stored-token",
      expiresAt: Date.now() + 60_000,
    }));

    renderAbsenceForm({ verificationStatus });

    expect(await screen.findByText("Student ID found")).toBeInTheDocument();
    await goToVerification(userEvent.setup());
    await waitFor(() => expect(window.sessionStorage.getItem(VERIFICATION_STORAGE_KEY)).toBeNull());
    expect(screen.getByRole("button", { name: /^send code$/i })).toBeInTheDocument();
    expect(screen.queryByText("✓ Verified")).not.toBeInTheDocument();
  });

  it.each([
    ["expired", new ApiRequestError("Verification token expired", { code: "otp_expired", status: 410 })],
    ["invalid", new ApiRequestError("Invalid verification token", { code: "bad_token", status: 400 })],
  ])("clears a stored %s token after server revalidation", async (_label, verificationError) => {
    window.sessionStorage.setItem(STUDENT_RESUME_STORAGE_KEY, JSON.stringify({ wcode: MOCK_STUDENT.wcode }));
    window.sessionStorage.setItem(VERIFICATION_STORAGE_KEY, JSON.stringify({
      token: "stored-token",
      expiresAt: Date.now() + 60_000,
    }));

    renderAbsenceForm({ verificationStatus: verificationError });

    expect(await screen.findByText("Student ID found")).toBeInTheDocument();
    await goToVerification(userEvent.setup());
    await waitFor(() => expect(window.sessionStorage.getItem(VERIFICATION_STORAGE_KEY)).toBeNull());
    expect(screen.getByRole("button", { name: /^send code$/i })).toBeInTheDocument();
  });

  it("does not trust a stored token when server revalidation is temporarily unavailable", async () => {
    window.sessionStorage.setItem(STUDENT_RESUME_STORAGE_KEY, JSON.stringify({ wcode: MOCK_STUDENT.wcode }));
    window.sessionStorage.setItem(VERIFICATION_STORAGE_KEY, JSON.stringify({
      token: "stored-token",
      expiresAt: Date.now() + 60_000,
    }));

    renderAbsenceForm({ verificationStatus: new TypeError("Network unavailable") });

    await screen.findByText("Student ID found");
    await goToVerification(userEvent.setup());
    expect(await screen.findByText(/could not validate saved verification/i)).toBeInTheDocument();
    expect(screen.queryByText("✓ Verified")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /retry verification check/i })).toBeInTheDocument();
    expect(window.sessionStorage.getItem(VERIFICATION_STORAGE_KEY)).toContain("stored-token");
  });

  it("normalizes a lowercase w-code before searching", async () => {
    const user = userEvent.setup();
    renderAbsenceForm();
    await lookupStudent(user, "w250389");
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absence-self-service/lookup",
      expect.objectContaining({ method: "POST", body: JSON.stringify({ wcode: "W250389" }) }),
    );
  });

  it("walks through lookup, verification, courses, sessions, and direct submission", async () => {
    const user = userEvent.setup();
    window.sessionStorage.setItem(
      SESSION_STORAGE_KEY,
      JSON.stringify({ dateFrom: "2000-01-01", dateTo: "2000-01-02" }),
    );
    renderAbsenceForm();

    await lookupStudent(user);

    await verifyParent(user);

    await goToCourses(user);

    const sessionsCall = mockApiJson.mock.calls.find(([url]) => String(url).includes("/absence-self-service/sessions"));
    expect(sessionsCall).toBeDefined();

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");

    await toggleAllCourseSwitches(user);

    // Select first session checkbox
    const sessionCheckbox = await findSessionCheckbox();
    await user.click(sessionCheckbox);

    // Click Review & Submit in sticky footer
    await user.click(screen.getByRole("button", { name: /review absence/i }));

    // Step 2 - Review page
    expect(screen.getByText("Review your absence")).toBeInTheDocument();
    expect(screen.getByText(/John Smith/)).toBeInTheDocument();

    // Submit from sticky footer
    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

    expect(await screen.findByText("Your absence request has been sent and is waiting for review.")).toBeInTheDocument();
    expect(screen.queryByText("Pending review")).not.toBeInTheDocument();

    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/batch",
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({
          "Idempotency-Key": expect.any(String),
        }),
        body: expect.not.stringContaining('"verification_token"'),
      }),
    );
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/batch",
      expect.objectContaining({
        method: "POST",
        body: expect.stringContaining('"items":['),
      }),
    );
  }, 30000);

  it("keeps Review state and the idempotency key after an interrupted submission retry", async () => {
    const user = userEvent.setup();
    const interruptedSubmission = vi.fn((_init: RequestInit) => {
      throw new TypeError("Failed to fetch");
    });
    renderAbsenceForm({ submission: interruptedSubmission });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await user.type(
      screen.getByPlaceholderText("Tell us why you'll be away from class..."),
      "Medical appointment",
    );
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());
    await user.click(screen.getByRole("button", { name: /review absence/i }));

    const submitButton = screen.getByRole("button", { name: /^submit absence$/i });
    await user.click(submitButton);

    const interruptionMessage =
      "Your connection was interrupted, so we couldn't confirm whether your absence was received. Stay on this page, check your connection, then tap Submit again. This retry will not create a duplicate.";
    expect(
      await screen.findByText(interruptionMessage, { selector: '[role="alert"]' }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /review your absence/i })).toBeInTheDocument();
    expect(screen.getByText("Medical appointment")).toBeInTheDocument();
    await waitFor(() => expect(submitButton).toBeEnabled());

    await user.click(submitButton);
    await waitFor(() => expect(interruptedSubmission).toHaveBeenCalledTimes(4));

    const requestInits = interruptedSubmission.mock.calls.map(
      ([init]) => init as RequestInit,
    );
    const idempotencyKeys = requestInits.map(
      (init) => (init.headers as Record<string, string>)["Idempotency-Key"],
    );
    expect(new Set(idempotencyKeys).size).toBe(1);
    expect(new Set(requestInits.map((init) => init.body)).size).toBe(1);
    expect(screen.getByRole("heading", { name: /review your absence/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^submit absence$/i })).toBeEnabled();
  }, 30000);

  it("prioritizes Classes validation, focuses the first invalid control, and exposes one validation alert", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      config: {
        ...MOCK_CONFIG,
        form: { ...MOCK_CONFIG.form, require_reason: true },
      },
    });
    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.click(screen.getByRole("button", { name: /review absence/i }));
    const courseAlert = await screen.findByText("Select at least one course.");
    expect(courseAlert).toHaveAttribute("role", "alert");
    await waitFor(() => expect(document.activeElement?.id).toMatch(/^subject-/));

    const subjectCheckbox = screen.getAllByRole("checkbox").find((checkbox) => checkbox.id.startsWith("subject-"))!;
    await user.click(subjectCheckbox);
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    const sessionAlert = await screen.findByText("Select at least one class you will miss.");
    expect(sessionAlert).toHaveAttribute("role", "alert");
    await waitFor(() => expect(document.activeElement?.id).toMatch(/^session-/));

    await user.click(await findSessionCheckbox());
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    const reasonSummary = await screen.findByText("Please tell us why you'll be away.", { selector: '[role="alert"]' });
    expect(reasonSummary).toBeInTheDocument();
    const reasonInline = screen.getByText("Please tell us why you'll be away.", { selector: "#reason-error" });
    expect(reasonInline).not.toHaveAttribute("role");
    expect(screen.getByRole("textbox", { name: /reason for absence/i })).toHaveAttribute("aria-describedby", "reason-error");
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("textbox", { name: /reason for absence/i })));
  });

  it("allows review without a reason when the form configuration makes it optional", async () => {
    const user = userEvent.setup();
    renderAbsenceForm();
    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    expect(screen.getByRole("textbox", { name: /reason for absence/i })).not.toBeRequired();
    await user.click(screen.getByRole("button", { name: /review absence/i }));

    expect(screen.getByRole("heading", { name: /review your absence/i })).toBeInTheDocument();
  });
  it("preserves selected classes and reason when editing from review", async () => {
    const user = userEvent.setup();
    renderAbsenceForm();
    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());
    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Initial reason");
    await user.click(screen.getByRole("button", { name: /review absence/i }));

    await user.click(screen.getByRole("button", { name: "Edit reason" }));

    const reason = screen.getByRole("textbox", { name: /reason for absence/i });
    expect(reason).toHaveValue("Initial reason");
    expect(screen.getByRole("checkbox", { name: /mathematics/i })).toBeChecked();
    expect(screen.getByRole("button", { name: /review absence/i })).toBeEnabled();

    await user.clear(reason);
    await user.type(reason, "Updated reason");
    await user.click(screen.getByRole("button", { name: /review absence/i }));

    expect(screen.getByRole("heading", { name: /review your absence/i })).toBeInTheDocument();
    expect(screen.getByText("Updated reason")).toBeInTheDocument();
  });
  it("expands one selected subject at a time for mobile disclosure and keeps completion summaries", async () => {
    const user = userEvent.setup();
    renderAbsenceForm();
    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await toggleAllCourseSwitches(user);

    const mathControl = screen.getByRole("button", { name: /mathematics.*choose classes/i });
    const physicsControl = screen.getByRole("button", { name: /physics.*open/i });
    expect(mathControl).toHaveAttribute("aria-expanded", "false");
    expect(physicsControl).toHaveAttribute("aria-expanded", "true");

    await user.click(mathControl);
    expect(mathControl).toHaveAttribute("aria-expanded", "true");
    expect(physicsControl).toHaveAttribute("aria-expanded", "false");
    await user.click(await findSessionCheckbox());
    expect(mathControl).toHaveAccessibleName(/1 class day selected/i);
  });

  it("returns to Verify when the student session expires while loading Classes", async () => {
    let sessionsReadCount = 0;
    const sessions = () => {
      sessionsReadCount += 1;
      if (sessionsReadCount === 1) return MOCK_SESSIONS;
      throw new ApiRequestError("Student verification is required", {
        code: "unauthorized",
        status: 401,
      });
    };
    const user = userEvent.setup();
    renderAbsenceForm({ sessions });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await user.click(screen.getByRole("button", { name: /^back$/i }));
    await screen.findByRole("heading", { name: /parent verification/i });
    await user.click(screen.getByRole("button", { name: /continue to classes/i }));

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /parent verification/i })).toBeInTheDocument();
      expect(screen.getByText(/verified session expired/i)).toBeInTheDocument();
    });
    expect(sessionsReadCount).toBe(2);
  });

  it("returns to Verify when the student session expires during submission", async () => {
    const user = userEvent.setup();
    const expiredSubmission = vi.fn(() => {
      throw new ApiRequestError("Student verification is required", {
        code: "unauthorized",
        status: 401,
      });
    });
    renderAbsenceForm({ submission: expiredSubmission });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await user.type(screen.getByRole("textbox", { name: /reason for absence/i }), "Keep review work");
    await user.click(screen.getAllByRole("checkbox").find((checkbox) => checkbox.id.startsWith("subject-"))!);
    await user.click(await findSessionCheckbox());
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    expect(screen.getByRole("heading", { name: /review your absence/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /parent verification/i })).toBeInTheDocument();
      expect(screen.getByText(/verified session expired/i)).toBeInTheDocument();
    });
    expect(expiredSubmission).toHaveBeenCalledOnce();
  });

  it("loads sessions without explicit date range when max_hours_after_session is set", async () => {
    const user = userEvent.setup();

    renderAbsenceForm({
      config: {
        ...MOCK_CONFIG,
        form: { ...MOCK_CONFIG.form, max_hours_after_session: 48 },
      },
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    const sessionsCall = mockApiJson.mock.calls.find(([url]) => String(url).includes("/absence-self-service/sessions"));
    expect(sessionsCall).toBeDefined();
    const sessionsUrl = new URL(String(sessionsCall?.[0]), "https://example.test");
    expect(sessionsUrl.searchParams.has("date_from")).toBe(false);
    expect(sessionsUrl.searchParams.has("date_to")).toBe(false);
  }, 30000);

  it("keeps active edits in memory and refetches sessions when returning from Review", async () => {
    const user = userEvent.setup();
    renderAbsenceForm();

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Keep this active edit");
    const subjectCheckbox = (await screen.findAllByRole("checkbox")).find(
      checkbox => checkbox.getAttribute("id")?.startsWith("subject-"),
    )!;
    await user.click(subjectCheckbox);
    const sessionCheckbox = await findSessionCheckbox();
    await user.click(sessionCheckbox);
    await user.click(screen.getByRole("button", { name: /review absence/i }));

    const callsBeforeReturn = mockApiJson.mock.calls.filter(([url]) => String(url).includes("/absence-self-service/sessions")).length;
    await user.click(screen.getByRole("button", { name: /classes - completed/i }));

    expect(await screen.findByDisplayValue("Keep this active edit")).toBeInTheDocument();
    expect(subjectCheckbox).toBeChecked();
    expect(sessionCheckbox).toBeChecked();
    await waitFor(() => {
      const callsAfterReturn = mockApiJson.mock.calls.filter(([url]) => String(url).includes("/absence-self-service/sessions")).length;
      expect(callsAfterReturn).toBeGreaterThan(callsBeforeReturn);
    });
  }, 30000);

  it("submits selected sessions across more than one day in a single batch", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Mathematics",
          course_id: "c-math201",
          course_code: "MATH201",
          course_name: "Mathematics",
          sessions: [
            { id: "s1", start_at: "2026-06-01T09:00:00Z", end_at: "2026-06-01T10:30:00Z", date: "2026-06-01", already_absent: false },
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
            { id: "s2", start_at: "2026-06-02T11:00:00Z", end_at: "2026-06-02T12:30:00Z", date: "2026-06-02", already_absent: false },
          ],
        },
      ]),
      submission: { items: [SUBMISSION_RESPONSE, SECOND_SUBMISSION_RESPONSE] },
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");

    await toggleAllCourseSwitches(user);

    // Select session checkboxes
    const sessionCheckboxes = (await screen.findAllByRole("checkbox")).filter(
      (cb) => cb.getAttribute("id")?.startsWith("session-"),
    );
    await user.click(sessionCheckboxes[0]);
    await user.click(sessionCheckboxes[1]);

    // Review & Submit
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    expect(screen.getByText("Review your absence")).toBeInTheDocument();

    // Submit
    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

    expect(await screen.findByText("Your 2 absence requests have been sent and are waiting for review.")).toBeInTheDocument();

    const batchCall = mockApiJson.mock.calls.find(([url]) => url === "/api/v1/absences/batch");
    expect(batchCall).toBeDefined();
    const [, batchInit] = batchCall!;
    const parsedBody = JSON.parse(String(batchInit?.body)) as {
      items: Array<{ course_id: string; date_from: string; date_to: string }>;
    };
    expect(parsedBody.items).toHaveLength(2);
    expect(parsedBody.items[0]).toMatchObject({ course_id: "c-math201", date_from: "2026-06-01", date_to: "2026-06-01" });
    expect(parsedBody.items[1]).toMatchObject({ course_id: "c-phys301", date_from: "2026-06-02", date_to: "2026-06-02" });
  });

  it("accepts a public sit-in session whose component course differs from the selected sit-in course", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-1", code: "MATH", name: "Mathematics" }] },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Mathematics",
          course_id: "c-enrolled",
          course_code: "MATH-ENROLLED",
          course_name: "Mathematics",
          sessions: [
            { id: "missed-session", start_at: "2026-06-02T09:00:00Z", end_at: "2026-06-02T10:30:00Z", date: "2026-06-02", already_absent: false },
          ],
          sit_in: {
            sit_in_method: "physical",
            sit_in_course: { id: "c-selected-target", code: "TARGET", name: "Merged target" },
            available_sessions: [
              { id: "component-session", course_id: "c-target-component", start_at: "2026-06-04T09:00:00Z", end_at: "2026-06-04T10:30:00Z", course_name: "Target component" },
            ],
          },
        },
      ]),
      submission: { items: [{ ...SUBMISSION_RESPONSE, course_id: "c-enrolled", sit_in_course_id: "c-selected-target" }] },
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());
    await user.selectOptions(await screen.findByRole("combobox"), "component-session");
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

    expect(await screen.findByText("Your absence request has been sent and is waiting for review.")).toBeInTheDocument();
    expect(screen.queryByText(/don't match the selected course/i)).not.toBeInTheDocument();
    const batchCall = mockApiJson.mock.calls.find(([url]) => url === "/api/v1/absences/batch");
    const parsedBody = JSON.parse(String(batchCall?.[1]?.body)) as { items: Array<Record<string, unknown>> };
    expect(parsedBody.items[0]).toMatchObject({
      sit_in_course_id: "c-selected-target",
      sit_in_session_ids: ["component-session"],
    });
  });

  it("shows one submitted class card for the source records of a merged course", async () => {
    const user = userEvent.setup();
    const mergeGroupName = "SAT Verbal Rank 3 Section 1 C3";
    renderAbsenceForm({
      student: {
        ...MOCK_STUDENT,
        subjects: [
          { id: "subj-writing", code: "WRITING", name: "SAT Verbal Writing", merge_group_id: "merge-r3s1", merge_group_name: mergeGroupName },
          { id: "subj-reading", code: "READING", name: "SAT Verbal Reading", merge_group_id: "merge-r3s1", merge_group_name: mergeGroupName },
        ],
      },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-writing",
          subject_code: "WRITING",
          subject_name: "SAT Verbal Writing",
          course_id: "c-writing",
          course_code: "WRITING-C3",
          course_name: "SAT Verbal Writing",
          merge_group_id: "merge-r3s1",
          merge_group_name: mergeGroupName,
          sessions: [{ id: "missed-writing", start_at: "2026-06-02T09:00:00Z", end_at: "2026-06-02T10:30:00Z", date: "2026-06-02", already_absent: false }],
          sit_in: { sit_in_method: "zoom" },
        },
        {
          subject_id: "subj-reading",
          subject_code: "READING",
          subject_name: "SAT Verbal Reading",
          course_id: "c-reading",
          course_code: "READING-C3",
          course_name: "SAT Verbal Reading",
          merge_group_id: "merge-r3s1",
          merge_group_name: mergeGroupName,
          sessions: [{ id: "missed-reading", start_at: "2026-06-02T11:00:00Z", end_at: "2026-06-02T12:30:00Z", date: "2026-06-02", already_absent: false }],
          sit_in: { sit_in_method: "zoom" },
        },
      ]),
      submission: {
        items: [
          { ...SUBMISSION_RESPONSE, id: "submitted-writing", course_id: "c-writing", subject_id: "subj-writing", subject_name: "SAT Verbal Writing", course_name: "SAT Verbal Writing", sit_in_method: "physical", sit_in_merge_group_name: "SAT Verbal Rank 3 Section 2 C3" },
          { ...SECOND_SUBMISSION_RESPONSE, id: "submitted-reading", course_id: "c-reading", subject_id: "subj-reading", subject_name: "SAT Verbal Reading", course_name: "SAT Verbal Reading", sit_in_method: "physical", sit_in_merge_group_name: "SAT Verbal Rank 3 Section 2 C3" },
        ],
      },
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");
    await toggleAllCourseSwitches(user);
    const sessionCheckboxes = (await screen.findAllByRole("checkbox")).filter((cb) => cb.getAttribute("id")?.startsWith("session-"));
    await user.click(sessionCheckboxes[0]);
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

    expect(await screen.findByText("Absence submitted")).toBeInTheDocument();
    const submittedClasses = screen.getByText("Submitted classes").parentElement;
    expect(submittedClasses).not.toBeNull();
    expect(within(submittedClasses!).getAllByRole("article")).toHaveLength(1);
    expect(within(submittedClasses!).getByText(mergeGroupName, { exact: true })).toBeInTheDocument();
    expect(within(submittedClasses!).getByText("SAT Verbal Rank 3 Section 2 C3", { exact: true })).toBeInTheDocument();
    expect(screen.queryByText("2 absences submitted")).not.toBeInTheDocument();
    expect(screen.queryByText("SAT Verbal Writing : Rank 3 (Section 1) C3")).not.toBeInTheDocument();
    expect(screen.queryByText("SAT Verbal Reading : Rank 3 (Section 1) C3")).not.toBeInTheDocument();
  });

  it("merges same-day absence sessions into one selectable row and submits all missed session IDs", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-1", code: "MATH", name: "Mathematics" }] },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Mathematics",
          course_id: "c-math201",
          course_code: "MATH201",
          course_name: "Mathematics",
          sessions: [
            { id: "s1", start_at: "2026-06-02T09:00:00+07:00", end_at: "2026-06-02T10:30:00+07:00", date: "2026-06-02", already_absent: false },
            { id: "s2", start_at: "2026-06-02T10:45:00+07:00", end_at: "2026-06-02T12:00:00+07:00", date: "2026-06-02", already_absent: false },
          ],
          sit_in: { sit_in_method: "zoom" },
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");
    await toggleAllCourseSwitches(user);

    const sessionCheckboxes = (await screen.findAllByRole("checkbox")).filter(
      (cb) => cb.getAttribute("id")?.startsWith("session-"),
    );
    expect(sessionCheckboxes).toHaveLength(1);
    expect(screen.getByText(/2 Jun 2026 09:00-12:00/)).toBeInTheDocument();

    await user.click(sessionCheckboxes[0]);
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    expect(screen.getByText("Review your absence")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

    const batchCall = await waitFor(() => {
      const call = mockApiJson.mock.calls.find(([url]) => url === "/api/v1/absences/batch");
      expect(call).toBeDefined();
      return call!;
    });
    const parsedBody = JSON.parse(String(batchCall[1]?.body)) as {
      items: Array<{ missed_session_ids: string[]; date_from: string; date_to: string }>;
    };
    expect(parsedBody.items[0]).toMatchObject({
      date_from: "2026-06-02",
      date_to: "2026-06-02",
      missed_session_ids: ["s1", "s2"],
    });
  }, 30000);

  it("shows same-day physical sit-in sessions as separate subject/time options", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-1", code: "MATH", name: "Mathematics" }] },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Mathematics",
          course_id: "c-math201",
          course_code: "MATH201",
          course_name: "Mathematics",
          sessions: [
            { id: "s1", start_at: "2026-06-02T09:00:00+07:00", end_at: "2026-06-02T10:30:00+07:00", date: "2026-06-02", already_absent: false },
          ],
          sit_in: {
            sit_in_method: "physical",
            sit_in_course: { id: "c-math301", code: "MATH301", name: "Calculus III" },
            available_sessions: [
              { id: "as1", start_at: "2026-06-04T13:00:00+07:00", end_at: "2026-06-04T14:30:00+07:00", subject_name: "Mathematics", course_name: "Calculus III" },
              { id: "as2", start_at: "2026-06-04T14:45:00+07:00", end_at: "2026-06-04T16:30:00+07:00", subject_name: "Mathematics", course_name: "Calculus III" },
            ],
          },
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    const makeUpSelect = await screen.findByRole("combobox");
    const makeUpOptions = screen.getAllByRole("option").filter((option) => option.getAttribute("value"));
    expect(makeUpOptions).toHaveLength(2);
    expect(makeUpOptions[0]).toHaveTextContent(/Mathematics.*Calculus III.*4 Jun 2026 13:00-14:30/);
    expect(makeUpOptions[1]).toHaveTextContent(/Mathematics.*Calculus III.*4 Jun 2026 14:45-16:30/);

    await user.selectOptions(makeUpSelect, makeUpOptions[0].getAttribute("value")!);
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

    const batchCall = await waitFor(() => {
      const call = mockApiJson.mock.calls.find(([url]) => url === "/api/v1/absences/batch");
      expect(call).toBeDefined();
      return call!;
    });
    const parsedBody = JSON.parse(String(batchCall[1]?.body)) as {
      items: Array<{ sit_in_course_id: string; sit_in_session_ids: string[] }>;
    };
    expect(parsedBody.items[0]).toMatchObject({
      sit_in_course_id: "c-math301",
      sit_in_session_ids: ["as1"],
    });
  }, 30000);

  it("shows a used sit-in session as unavailable in the public form", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-1", code: "MATH", name: "Mathematics" }] },
      sessions: createMockSessionsInRange([{
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        course_id: "c-math201",
        course_code: "MATH201",
        course_name: "Mathematics",
        sessions: [{ id: "missed-1", start_at: "2026-06-02T09:00:00+07:00", end_at: "2026-06-02T10:30:00+07:00", date: "2026-06-02", already_absent: false }],
        sit_in: {
          sit_in_method: "physical",
          sit_in_course: { id: "c-math301", code: "MATH301", name: "Calculus III" },
          unavailable_sessions: [{
            session: { id: "used-day-session", start_at: "2026-06-02T13:00:00+07:00", end_at: "2026-06-02T14:30:00+07:00" },
            reason: "This sit-in session is already assigned to this student's absence.",
            reason_code: "sit_in_session_already_used",
          }],
        },
      }]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    expect(await screen.findByText("This sit-in session is already used.")).toBeInTheDocument();
    expect(screen.getByText("Choose another sit-in session.")).toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  }, 30000);

  it("returns to classes and refreshes after a stale public sit-in session conflict", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-1", code: "MATH", name: "Mathematics" }] },
      sessions: createMockSessionsInRange([{
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        course_id: "c-math201",
        course_code: "MATH201",
        course_name: "Mathematics",
        sessions: [{ id: "missed-1", start_at: "2026-06-02T09:00:00+07:00", end_at: "2026-06-02T10:30:00+07:00", date: "2026-06-02", already_absent: false }],
        sit_in: {
          sit_in_method: "physical",
          sit_in_course: { id: "c-math301", code: "MATH301", name: "Calculus III" },
          available_sessions: [{ id: "available-day-session", start_at: "2026-06-04T13:00:00+07:00", end_at: "2026-06-04T14:30:00+07:00" }],
        },
      }]),
      submission: () => {
        throw new ApiRequestError("This sit-in session is already assigned", { code: "sit_in_session_already_used", status: 409 });
      },
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());
    await user.selectOptions(screen.getByRole("combobox"), "available-day-session");
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

    expect(await screen.findByText(/That sit-in session was just used for this student/)).toBeInTheDocument();
    expect(screen.getByText("Courses & classes")).toBeInTheDocument();
    await waitFor(() => {
      expect(mockApiJson.mock.calls.filter(([url]) => String(url).includes("/absence-self-service/sessions")).length).toBeGreaterThanOrEqual(2);
    });
  }, 30000);

  it("refreshes sit-in availability before submitting a stale selection", async () => {
    const user = userEvent.setup();
    let sessionRequestCount = 0;
    const freshSessions = createMockSessionsInRange([{
      subject_id: "subj-1",
      subject_code: "MATH",
      subject_name: "Mathematics",
      course_id: "c-math201",
      course_code: "MATH201",
      course_name: "Mathematics",
      sessions: [{ id: "missed-1", start_at: "2026-06-02T09:00:00+07:00", end_at: "2026-06-02T10:30:00+07:00", date: "2026-06-02", already_absent: false }],
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: { id: "c-math301", code: "MATH301", name: "Calculus III" },
        available_sessions: [{ id: "available-day-session", start_at: "2026-06-04T13:00:00+07:00", end_at: "2026-06-04T14:30:00+07:00" }],
      },
    }]);
    const blockedSessions = createMockSessionsInRange([{
      subject_id: "subj-1",
      subject_code: "MATH",
      subject_name: "Mathematics",
      course_id: "c-math201",
      course_code: "MATH201",
      course_name: "Mathematics",
      sessions: [{ id: "missed-1", start_at: "2026-06-02T09:00:00+07:00", end_at: "2026-06-02T10:30:00+07:00", date: "2026-06-02", already_absent: false }],
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: { id: "c-math301", code: "MATH301", name: "Calculus III" },
        unavailable_sessions: [{
          session: { id: "available-day-session", start_at: "2026-06-04T13:00:00+07:00", end_at: "2026-06-04T14:30:00+07:00" },
          reason: "This sit-in session is already assigned to this student's absence.",
          reason_code: "sit_in_session_already_used",
        }],
      },
    }]);
    renderAbsenceForm({
      sessions: () => {
        sessionRequestCount += 1;
        return sessionRequestCount === 1 ? freshSessions : blockedSessions;
      },
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());
    await user.selectOptions(screen.getByRole("combobox"), "available-day-session");
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

    expect(await screen.findByText(/selected sit-in session is no longer available/i)).toBeInTheDocument();
    expect(screen.getByText("Courses & classes")).toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([url, init]) => String(url).endsWith("/absences/batch") && init?.method === "POST")).toBe(false);
  }, 30000);

  it("submits the selected priority sit-in course for SAT Verbal priority options", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-satv",
          subject_code: "SATV",
          subject_name: "SAT Verbal",
          course_id: "c-r3s3",
          course_code: "R3S3",
          course_name: "SAT Verbal Rank 3 Section 3",
          sessions: [
            { id: "missed-r3s3-lesson-2", start_at: "2026-06-02T09:00:00Z", end_at: "2026-06-02T10:30:00Z", date: "2026-06-02", already_absent: false },
          ],
          sit_in: {
            sit_in_method: "physical",
            priorities: [
              {
                level: 1,
                label: "1st Priority: Another Rank 3 section (same lesson #)",
                sit_in_course: { id: "c-r3s1", code: "R3S1", name: "SAT Verbal Rank 3 Section 1" },
                available_sessions: [{ id: "sit-r3s1-lesson-2", start_at: "2026-06-04T09:00:00Z", end_at: "2026-06-04T10:30:00Z", course_name: "SAT Verbal Rank 3 Section 1" }],
              },
              {
                level: 3,
                label: "3rd Priority: Rank 4 Reading or Writing",
                sit_in_course: { id: "c-r4r", code: "R4R", name: "SAT Verbal Reading Rank 4" },
                available_sessions: [{ id: "sit-r4r", start_at: "2026-06-05T09:00:00Z", end_at: "2026-06-05T10:30:00Z", course_name: "SAT Verbal Reading Rank 4" }],
              },
            ],
          },
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());
    await user.selectOptions(await screen.findByRole("combobox"), "sit-r3s1-lesson-2");

    await user.click(screen.getByRole("button", { name: /review absence/i }));
    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

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
        sessions: [{ id: "missed-r3s3", start_at: "2026-06-02T09:00:00Z", end_at: "2026-06-02T10:30:00Z", date: "2026-06-02", already_absent: false }],
        sit_in: {
          sit_in_method: "physical",
          current_priority_level: 1,
          has_next_priority: true,
          priorities: [{
            level: 1,
            label: "1st Priority: Another Rank 3 section (same lesson #)",
            sit_in_course: { id: "c-r3s1", code: "R3S1", name: "SAT Verbal Rank 3 Section 1" },
            available_sessions: [{ id: "sit-r3s1", start_at: "2026-06-04T09:00:00Z", end_at: "2026-06-04T10:30:00Z" }],
          }],
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
          priorities: [{
            level: 3,
            label: "3rd Priority: Rank 4 Reading or Writing",
            sit_in_course: { id: "c-r4r", code: "R4R", name: "SAT Verbal Reading Rank 4" },
            available_sessions: [{ id: "sit-r4r", start_at: "2026-06-05T09:00:00Z", end_at: "2026-06-05T10:30:00Z" }],
          }],
        },
      },
    ]);
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      const path = String(url);
      if (path.includes("absence-form-config")) return MOCK_CONFIG;
      if (path.endsWith("/absence-self-service/lookup")) return publicLookupFor({ ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] });
      if (path.endsWith("/absence-self-service/me")) return verifiedProfileFor({ ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] });
      if (path.includes("/absence-self-service/sessions") && path.includes("sat_verbal_after_priority=1")) return nextSessions;
      if (path.includes("/absence-self-service/sessions")) return initialSessions;
      if (path.endsWith("/parent-verification/status") && init?.method === "POST") return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/send")) return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/verify")) return OTP_VERIFY_RESPONSE;
      if (path.endsWith("/absences/batch") && init?.method === "POST") return { items: [SUBMISSION_RESPONSE] };
      throw new Error(`Unmocked API call: ${url}`);
    });
    renderWithProviders(<AbsenceForm />);

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    expect(await screen.findByRole("option", { name: /SAT Verbal Rank 3 Section 1/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /SAT Verbal Reading Rank 4/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /see other times/i }));
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        expect.stringContaining("sat_verbal_after_priority=1"),
        expect.anything(),
      );
    });
    expect(await screen.findByRole("option", { name: /SAT Verbal Reading Rank 4/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /see previous times/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /see other times/i })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /see previous times/i }));
    expect(await screen.findByRole("option", { name: /SAT Verbal Rank 3 Section 1/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /SAT Verbal Reading Rank 4/ })).not.toBeInTheDocument();
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
        sessions: [{ id: "missed-writing-1", start_at: "2026-06-16T17:00:00Z", end_at: "2026-06-16T20:20:00Z", date: "2026-06-16", already_absent: false }],
        sit_in: {
          sit_in_method: "physical",
          current_priority_level: 1,
          has_next_priority: true,
          priorities: [{
            level: 1,
            label: "1st Priority: Same Writing Beginner lesson in another section",
            available_sessions: [],
            unavailable_sessions: [{
              session: { id: "checked-writing-2", missed_session_id: "missed-writing-1", start_at: "2026-06-08T17:00:00Z", end_at: "2026-06-08T20:20:00Z", course_name: "SAT Verbal Writing Beginner Section 2 C2/26" },
              missed_session_id: "missed-writing-1",
              occurrence_number: 3,
              reason_code: "before_request_date",
              reason: "This same-number sit-in slot is before today/request date.",
            }],
          }],
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
          priorities: [{
            level: 2,
            label: "2nd Priority: SAT Verbal Writing Rank 5",
            sit_in_course: { id: "c-writing-rank5", code: "WR5", name: "SAT Verbal Writing Rank 5 C2/26" },
            available_sessions: [{ id: "sit-writing-rank5", start_at: "2026-06-17T17:00:00Z", end_at: "2026-06-17T20:20:00Z", course_name: "SAT Verbal Writing Rank 5 C2/26" }],
          }],
        },
      },
    ]);
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      const path = String(url);
      if (path.endsWith("/absence-self-service/lookup")) return publicLookupFor({ ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] });
      if (path.endsWith("/absence-self-service/me")) return verifiedProfileFor({ ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] });
      if (path.includes("/absence-self-service/sessions") && path.includes("sat_verbal_after_priority=1")) return nextSessions;
      if (path.includes("/absence-self-service/sessions")) return initialSessions;
      if (path.endsWith("/parent-verification/status") && init?.method === "POST") return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/send")) return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/verify")) return OTP_VERIFY_RESPONSE;
      throw new Error(`Unmocked API call: ${url}`);
    });
    renderWithProviders(<AbsenceForm />);

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    expect(screen.getByText("No available make-up class for this priority.")).toBeInTheDocument();
    expect(screen.getByText("Checked same-number slot:")).toBeInTheDocument();
    expect(screen.getByText(/before today\/request date/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /see other times/i }));
    expect(await screen.findByRole("option", { name: /SAT Verbal Writing Rank 5/ })).toBeInTheDocument();
  }, 30000);

  it("shows every SAT Verbal target returned at the current priority level", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-satv",
          subject_code: "SATV",
          subject_name: "SAT Verbal",
          course_id: "c-r3s3",
          course_code: "R3S3",
          course_name: "SAT Verbal Rank 3 Section 3",
          sessions: [{ id: "missed-r3s3", start_at: "2026-06-02T09:00:00Z", end_at: "2026-06-02T10:30:00Z", date: "2026-06-02", already_absent: false }],
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
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    expect(await screen.findByRole("option", { name: /SAT Verbal Rank 3 Section 1/ })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /SAT Verbal Rank 3 Section 2/ })).toBeInTheDocument();
  }, 30000);

  it("filters SAT Verbal same-occurrence options to each selected missed session", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-satv",
          subject_code: "SATV",
          subject_name: "SAT Verbal Writing Beginner Section 1 C2/26",
          course_id: "c-writing-1",
          course_code: "W1",
          course_name: "SAT Verbal Writing Beginner Section 1 C2/26",
          sessions: [
            { id: "missed-writing-09", start_at: "2026-06-09T17:00:00Z", end_at: "2026-06-09T20:20:00Z", date: "2026-06-09", already_absent: false },
            { id: "missed-writing-23", start_at: "2026-06-23T17:00:00Z", end_at: "2026-06-23T20:20:00Z", date: "2026-06-23", already_absent: false },
          ],
          sit_in: {
            sit_in_method: "physical",
            current_priority_level: 1,
            has_next_priority: true,
            priorities: [
              {
                level: 1,
                label: "1st Priority",
                sit_in_course: { id: "c-writing-2", code: "W2", name: "SAT Verbal Writing Beginner Section 2 C2/26" },
                available_sessions: [
                  { id: "sit-writing-2-09", missed_session_id: "missed-writing-09", start_at: "2026-06-14T17:00:00Z", end_at: "2026-06-14T20:20:00Z", course_name: "SAT Verbal Writing Beginner Section 2 C2/26" },
                  { id: "sit-writing-2-23", missed_session_id: "missed-writing-23", start_at: "2026-06-28T17:00:00Z", end_at: "2026-06-28T20:20:00Z", course_name: "SAT Verbal Writing Beginner Section 2 C2/26" },
                ],
              },
              {
                level: 1,
                label: "1st Priority",
                sit_in_course: { id: "c-writing-3", code: "W3", name: "SAT Verbal Writing Beginner Section 3 C2/26" },
                available_sessions: [
                  { id: "sit-writing-3-09", missed_session_id: "missed-writing-09", start_at: "2026-06-13T17:00:00Z", end_at: "2026-06-13T20:20:00Z", course_name: "SAT Verbal Writing Beginner Section 3 C2/26" },
                  { id: "sit-writing-3-23", missed_session_id: "missed-writing-23", start_at: "2026-06-27T17:00:00Z", end_at: "2026-06-27T20:20:00Z", course_name: "SAT Verbal Writing Beginner Section 3 C2/26" },
                ],
              },
            ],
          },
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    const sessionCheckboxes = await screen.findAllByRole("checkbox");
    for (const checkbox of sessionCheckboxes) {
      if (checkbox.getAttribute("id")?.startsWith("session-")) {
        await user.click(checkbox);
      }
    }

    const selects = await screen.findAllByRole("combobox");
    expect(selects).toHaveLength(2);
    expect(within(selects[0]).getByRole("option", { name: /Sun, 14 Jun 2026/ })).toBeInTheDocument();
    expect(within(selects[0]).queryByRole("option", { name: /Sun, 28 Jun 2026/ })).not.toBeInTheDocument();
    expect(within(selects[1]).getByRole("option", { name: /Sun, 28 Jun 2026/ })).toBeInTheDocument();
    expect(within(selects[1]).queryByRole("option", { name: /Sun, 14 Jun 2026/ })).not.toBeInTheDocument();
  }, 30000);

  it("ignores stale restored priority levels when the selected June 16 class has available sit-ins", async () => {
    const user = userEvent.setup();
    const missedSessionId = "1d9d68c1-9487-48aa-8696-b07326c0a0da";
    window.sessionStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify({
      sitInPriorityLevels: { [missedSessionId]: 2 },
    }));
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "24af31dc-5b2b-4d2f-ab0f-4ee75b3cecaf", code: "17", name: "SAT Verbal Writing Beginner Section 1 C2/26" }] },
      sessions: createMockSessionsInRange([
        {
          subject_id: "24af31dc-5b2b-4d2f-ab0f-4ee75b3cecaf",
          subject_code: "17",
          subject_name: "SAT Verbal Writing Beginner Section 1 C2/26",
          course_id: "a7645da2-6d71-44a0-98d8-759cd1d49e56",
          course_code: "0000000013",
          course_name: "",
          sessions: [{ id: missedSessionId, start_at: "2026-06-16T10:00:00Z", end_at: "2026-06-16T13:20:00Z", date: "2026-06-16", already_absent: false }],
          sit_in: {
            rule_name: "SAT Verbal Policy",
            rule_type: "sat_verbal_policy",
            sit_in_method: "physical",
            current_priority_level: 1,
            has_next_priority: true,
            priorities: [{
              level: 1,
              label: "1st Priority: Same Writing Beginner lesson in another section",
              sit_in_course: { id: "2d460d39-92dd-4cc2-8460-9f4a08fc4b5e", code: "0000000014", name: "", subject_code: "18", subject_name: "SAT Verbal Writing Beginner Section 2 C2/26" },
              available_sessions: [{ id: "b1381c0e-72df-4fc3-99b0-b38e40c81f35", start_at: "2026-06-14T10:00:00Z", end_at: "2026-06-14T13:20:00Z", missed_session_id: missedSessionId }],
            }],
          },
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    expect(screen.queryByText("No more options available")).not.toBeInTheDocument();
    expect(await screen.findByRole("option", { name: /SAT Verbal Writing Beginner Section 2 C2\/26/ })).toBeInTheDocument();
  }, 30000);

  it("keeps SAT Verbal priority reveals isolated per selected missed session", async () => {
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
          { id: "missed-writing-16", start_at: "2026-06-16T17:00:00Z", end_at: "2026-06-16T20:20:00Z", date: "2026-06-16", already_absent: false },
          { id: "missed-writing-23", start_at: "2026-06-23T17:00:00Z", end_at: "2026-06-23T20:20:00Z", date: "2026-06-23", already_absent: false },
        ],
        sit_in: {
          sit_in_method: "physical",
          current_priority_level: 1,
          has_next_priority: true,
          priorities: [{
            level: 1,
            label: "1st Priority",
            sit_in_course: { id: "c-writing-2", code: "W2", name: "SAT Verbal Writing Beginner Section 2 C2/26" },
            available_sessions: [
              { id: "sit-writing-2-16", missed_session_id: "missed-writing-16", start_at: "2026-06-21T17:00:00Z", end_at: "2026-06-21T20:20:00Z", course_name: "SAT Verbal Writing Beginner Section 2 C2/26" },
              { id: "sit-writing-2-23", missed_session_id: "missed-writing-23", start_at: "2026-06-28T17:00:00Z", end_at: "2026-06-28T20:20:00Z", course_name: "SAT Verbal Writing Beginner Section 2 C2/26" },
            ],
          }],
        },
      },
    ]);
    const nextSessions = createMockSessionsInRange([
      {
        ...initialSessions.subjects[0],
        sit_in: { sit_in_method: "physical", current_priority_level: 2, has_next_priority: false, priorities: [{ level: 2, label: "2nd Priority: SAT Verbal Writing Rank 5", sit_in_course: { id: "c-writing-rank5", code: "WR5", name: "SAT Verbal Writing Rank 5 C2/26" }, available_sessions: [{ id: "sit-writing-rank5", start_at: "2026-06-17T17:00:00Z", end_at: "2026-06-17T20:20:00Z", course_name: "SAT Verbal Writing Rank 5 C2/26" }] }] },
      },
    ]);
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      const path = String(url);
      if (path.endsWith("/absence-self-service/lookup")) return publicLookupFor({ ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] });
      if (path.endsWith("/absence-self-service/me")) return verifiedProfileFor({ ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] });
      if (path.includes("/absence-self-service/sessions") && path.includes("sat_verbal_after_priority=1")) return nextSessions;
      if (path.includes("/absence-self-service/sessions")) return initialSessions;
      if (path.endsWith("/parent-verification/status") && init?.method === "POST") return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/send")) return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/verify")) return OTP_VERIFY_RESPONSE;
      throw new Error(`Unmocked API call: ${url}`);
    });
    renderWithProviders(<AbsenceForm />);

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    const sessionCheckboxes = await screen.findAllByRole("checkbox");
    for (const checkbox of sessionCheckboxes) {
      if (checkbox.getAttribute("id")?.startsWith("session-")) {
        await user.click(checkbox);
      }
    }
    expect(await screen.findAllByRole("combobox")).toHaveLength(2);

    await user.click(screen.getAllByRole("button", { name: /see other times/i })[0]);
    expect(await screen.findByRole("option", { name: /SAT Verbal Writing Rank 5/ })).toBeInTheDocument();
  }, 30000);

  it("renders SAT Verbal same-number choices from per-missed-session sit-in results", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-satv",
          subject_code: "SATV",
          subject_name: "SAT Verbal Writing Beginner Section 1 C2/26",
          course_id: "c-writing-1",
          course_code: "W1",
          course_name: "SAT Verbal Writing Beginner Section 1 C2/26",
          sessions: [
            { id: "missed-writing-16", start_at: "2026-06-16T17:00:00Z", end_at: "2026-06-16T20:20:00Z", date: "2026-06-16", already_absent: false },
            { id: "missed-writing-23", start_at: "2026-06-23T17:00:00Z", end_at: "2026-06-23T20:20:00Z", date: "2026-06-23", already_absent: false },
          ],
          sit_in: {
            sit_in_method: "physical",
            current_priority_level: 1,
            has_next_priority: true,
            priorities: [{ level: 1, label: "1st Priority", available_sessions: [] }],
            sit_in_by_missed_session: {
              "missed-writing-16": {
                sit_in_method: "physical",
                current_priority_level: 1,
                has_next_priority: true,
                missed_occurrence_number: 3,
                priorities: [{ level: 1, label: "1st Priority", sit_in_course: { id: "c-writing-2", code: "W2", name: "SAT Verbal Writing Beginner Section 2 C2/26" }, available_sessions: [{ id: "sit-writing-2-16", missed_session_id: "missed-writing-16", start_at: "2026-06-21T17:00:00Z", end_at: "2026-06-21T20:20:00Z", course_name: "SAT Verbal Writing Beginner Section 2 C2/26" }] }],
              },
              "missed-writing-23": {
                sit_in_method: "physical",
                current_priority_level: 1,
                has_next_priority: true,
                missed_occurrence_number: 4,
                priorities: [{ level: 1, label: "1st Priority", sit_in_course: { id: "c-writing-2", code: "W2", name: "SAT Verbal Writing Beginner Section 2 C2/26" }, available_sessions: [{ id: "sit-writing-2-23", missed_session_id: "missed-writing-23", start_at: "2026-06-28T17:00:00Z", end_at: "2026-06-28T20:20:00Z", course_name: "SAT Verbal Writing Beginner Section 2 C2/26" }] }],
              },
            },
          },
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    const sessionCheckboxes = await screen.findAllByRole("checkbox");
    for (const checkbox of sessionCheckboxes) {
      if (checkbox.getAttribute("id")?.startsWith("session-")) {
        await user.click(checkbox);
      }
    }

    const selects = await screen.findAllByRole("combobox");
    expect(selects).toHaveLength(2);
    expect(within(selects[0]).getByRole("option", { name: /Mon, 22 Jun 2026/ })).toBeInTheDocument();
    expect(within(selects[0]).queryByRole("option", { name: /Mon, 29 Jun 2026/ })).not.toBeInTheDocument();
    expect(within(selects[1]).getByRole("option", { name: /Mon, 29 Jun 2026/ })).toBeInTheDocument();
    expect(within(selects[1]).queryByRole("option", { name: /Mon, 22 Jun 2026/ })).not.toBeInTheDocument();
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
        sessions: [{ id: "missed-writing-1", start_at: "2026-06-09T10:00:00Z", end_at: "2026-06-09T13:20:00Z", date: "2026-06-09", already_absent: false }],
        sit_in: {
          sit_in_method: "physical", sit_in_course: { id: "c-writing-1", code: "W1", name: "SAT Verbal Writing Beginner Section 1 C2/26" },
          current_priority_level: 1, has_next_priority: true,
          priorities: [{ level: 1, label: "1st Priority", sit_in_course: { id: "c-writing-2", code: "W2", name: "SAT Verbal Writing Beginner Section 2 C2/26" }, available_sessions: [{ id: "sit-writing-2", start_at: "2026-06-14T10:00:00Z", end_at: "2026-06-14T13:20:00Z", course_name: "SAT Verbal Writing Beginner Section 2 C2/26" }] }],
        },
      },
    ]);
    const nextSessions = createMockSessionsInRange([
      {
        ...initialSessions.subjects[0],
        sit_in: { sit_in_method: "physical", current_priority_level: 2, has_next_priority: false, priorities: [{ level: 2, label: "2nd Priority", sit_in_course: { id: "c-writing-3", code: "W3", name: "SAT Verbal Writing Beginner Section 3 C2/26" }, available_sessions: [{ id: "sit-writing-3", start_at: "2026-06-15T10:00:00Z", end_at: "2026-06-15T13:20:00Z", course_name: "SAT Verbal Writing Beginner Section 3 C2/26" }] }] },
      },
    ]);
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      const path = String(url);
      if (path.endsWith("/absence-self-service/lookup")) return publicLookupFor({ ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] });
      if (path.endsWith("/absence-self-service/me")) return verifiedProfileFor({ ...MOCK_STUDENT, subjects: [{ id: "subj-satv", code: "SATV", name: "SAT Verbal" }] });
      if (path.includes("/absence-self-service/sessions") && path.includes("sat_verbal_after_priority=1")) return nextSessions;
      if (path.includes("/absence-self-service/sessions")) return initialSessions;
      if (path.endsWith("/parent-verification/status") && init?.method === "POST") return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/send")) return OTP_SEND_RESPONSE;
      if (path.endsWith("/parent-verification/verify")) return OTP_VERIFY_RESPONSE;
      throw new Error(`Unmocked API call: ${url}`);
    });
    renderWithProviders(<AbsenceForm />);

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    expect(screen.getByRole("option", { name: /SAT Verbal Writing Beginner Section 2 C2\/26/ })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /see other times/i }));
    expect(await screen.findByRole("option", { name: /SAT Verbal Writing Beginner Section 3 C2\/26/ })).toBeInTheDocument();
  }, 30000);

  it("offers parent phone enrollment instead of a dead end when no phone is on file", async () => {
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, parent_phone: null },
    });
    const user = userEvent.setup();
    await lookupStudent(user);
    await goToVerification(user);

    const phoneInput = await screen.findByLabelText(/parent's phone number/i);
    expect(screen.queryByText(/contact admin/i)).not.toBeInTheDocument();

    const sendButton = screen.getByRole("button", { name: /^send code$/i });
    expect(sendButton).toBeDisabled();

    await user.type(phoneInput, "0899998888");
    await waitFor(() => expect(sendButton).toBeEnabled());
  });

  it("explains the enrollment flow when no parent phone is on file", async () => {
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, parent_phone: null },
    });
    const user = userEvent.setup();
    await lookupStudent(user);
    await goToVerification(user);

    expect(await screen.findByText(/we don't have your parent's phone number yet/i)).toBeInTheDocument();
    expect(screen.getByText(/one-time code to check it works/i)).toBeInTheDocument();
  });

  it("shows the masked nickname hint after lookup", async () => {
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, nickname: "Bird" },
    });
    const user = userEvent.setup();
    await lookupStudent(user);

    expect(await screen.findByText("Nickname: B***")).toBeInTheDocument();
  });

  it("shows the masked parent phone on the verification card", async () => {
    renderAbsenceForm();
    const user = userEvent.setup();
    await lookupStudent(user);
    await goToVerification(user);

    expect(await screen.findByText("Parent phone: ••••5678")).toBeInTheDocument();
  });

  it("submits an optional nickname when the profile has none, masked in review", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, nickname: null },
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    await user.click(screen.getByRole("button", { name: /review absence/i }));

    const nicknameInput = await screen.findByLabelText(/nickname/i);
    await user.type(nicknameInput, "Bird");
    expect(await screen.findByText(/saved as B\*\*\*/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

    expect(await screen.findByText("Your absence request has been sent and is waiting for review.")).toBeInTheDocument();
    const batchCall = mockApiJson.mock.calls.find(([url]) => url === "/api/v1/absences/batch");
    expect(batchCall).toBeDefined();
    expect(JSON.parse(String(batchCall?.[1]?.body))).toMatchObject({ nickname: "Bird" });
  }, 30000);

  it("still submits the absence when a nickname lands on file mid-flight", async () => {
    const user = userEvent.setup();
    let batchAttempts = 0;
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, nickname: null },
      submission: () => {
        batchAttempts += 1;
        if (batchAttempts === 1) {
          throw new ApiRequestError("A nickname is already saved for this student", { code: "bad_nickname", status: 400 });
        }
        return { items: [SUBMISSION_RESPONSE] };
      },
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Medical appointment");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    await user.click(screen.getByRole("button", { name: /review absence/i }));
    await user.type(await screen.findByLabelText(/nickname/i), "Bird");
    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

    expect(await screen.findByText("Your absence request has been sent and is waiting for review.")).toBeInTheDocument();
    expect(batchAttempts).toBe(2);
    const batchCalls = mockApiJson.mock.calls.filter(([url]) => url === "/api/v1/absences/batch");
    expect(JSON.parse(String(batchCalls[0]?.[1]?.body)).nickname).toBe("Bird");
    expect(JSON.parse(String(batchCalls[1]?.[1]?.body)).nickname).toBeUndefined();
  }, 30000);

  it("shows a no-sessions status message when no sessions exist in range", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({ sessions: { subjects: [] } });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Family matter");

    await toggleAllCourseSwitches(user);

    expect(await screen.findByText("No classes found for the selected courses.")).toBeInTheDocument();
  });

  it("shows always-visible reason textarea on Step 2 after courses are selected", async () => {
    const user = userEvent.setup();
    renderAbsenceForm();

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    expect(screen.getByPlaceholderText("Tell us why you'll be away from class...")).toBeInTheDocument();
  });

  it("shows the resolved sit-in target as Absence class", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-1", code: "02", name: "math_advance" }] },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1", subject_code: "02", subject_name: "math_advance",
          course_id: "c-adv", course_code: "0000000344", course_name: "math_advance",
          sessions: [{ id: "s1", start_at: "2026-06-02T09:00:00Z", end_at: "2026-06-02T11:00:00Z", date: "2026-06-02", already_absent: false }],
          sit_in: { sit_in_method: "physical", sit_in_course: { id: "c-int", code: "0000000348", name: "Math inter" }, available_sessions: [{ id: "as1", start_at: "2026-06-04T03:00:00Z", end_at: "2026-06-04T05:00:00Z" }] },
        },
        { subject_id: "subj-2", subject_code: "04", subject_name: "Math inter", course_id: "c-int", course_code: "0000000348", course_name: "Math inter", sessions: [] },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    const makeUpSelect = await screen.findByRole("combobox");
    expect(makeUpSelect).toHaveTextContent("Math inter");
    expect(makeUpSelect).not.toHaveTextContent("0000000348");
  });

  it("shows the subject name (not raw code) in make-up dropdown when sit_in_course.name is empty and course not in enrolled subjects", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1", subject_code: "MATH", subject_name: "Math advance",
          course_id: "c-adv", course_code: "ADV-01", course_name: "Math advance",
          sessions: [{ id: "s1", start_at: "2026-06-02T09:00:00Z", end_at: "2026-06-02T10:30:00Z", date: "2026-06-02", already_absent: false }],
          sit_in: { sit_in_method: "physical", sit_in_course: { id: "c-int", code: "0000000348", name: "" }, available_sessions: [{ id: "as1", start_at: "2026-06-04T13:00:00Z", end_at: "2026-06-04T15:00:00Z" }] },
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    const makeUpSelect = await screen.findByRole("combobox");
    expect(makeUpSelect).toHaveTextContent("Math advance");
    expect(makeUpSelect).not.toHaveTextContent("0000000348");
  });

  it("shows sit-in target course name (not student's enrolled course) in make-up dropdown when sit_in_course has name populated", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-1", code: "MATH", name: "Mathematics" }] },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics",
          course_id: "c-adv", course_code: "ADV-01", course_name: "Mathematics",
          sessions: [{ id: "s1", start_at: "2026-06-02T09:00:00Z", end_at: "2026-06-02T10:30:00Z", date: "2026-06-02", already_absent: false }],
          sit_in: { sit_in_method: "physical", sit_in_course: { id: "c-scholar", code: "SCH-01", name: "scholar" }, available_sessions: [{ id: "as1", start_at: "2026-06-04T08:00:00Z", end_at: "2026-06-04T10:00:00Z" }] },
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    const makeUpSelect = await screen.findByRole("combobox");
    expect(makeUpSelect).toHaveTextContent("scholar");
    expect(makeUpSelect).not.toHaveTextContent("0000000348");
  });

  it("uses the resolved Scholar sit-in course for mixed inter and advanced enrollments", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      student: { ...MOCK_STUDENT, subjects: [{ id: "subj-math", code: "MATH", name: "Math" }] },
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-math", subject_code: "MATH", subject_name: "Math inter",
          course_id: "c-inter", course_code: "0000000348", course_name: "Math inter",
          sessions: [{ id: "s-inter", start_at: "2026-06-04T10:00:00+07:00", end_at: "2026-06-04T12:00:00+07:00", date: "2026-06-04", already_absent: false }],
          sit_in: { sit_in_method: "physical", sit_in_course: { id: "c-scholar", code: "0000000371", name: "", subject_name: "Scholar" }, available_sessions: [{ id: "as-scholar", start_at: "2026-06-06T10:00:00+07:00", end_at: "2026-06-06T12:00:00+07:00", subject_name: "Math advance", course_name: "Math advance" }] },
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Need a make-up class");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    const makeUpSelect = await screen.findByRole("combobox");
    expect(makeUpSelect).toHaveTextContent("Scholar");
    expect(makeUpSelect).not.toHaveTextContent("0000000371");
  });

  it("shows the sit-in class name from the available session instead of the absence class name", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1", subject_code: "ADV", subject_name: "Math advance",
          course_id: "c-adv", course_code: "ADV-01", course_name: "Math advance",
          sessions: [{ id: "s1", start_at: "2026-06-02T09:00:00Z", end_at: "2026-06-02T10:30:00Z", date: "2026-06-02", already_absent: false }],
          sit_in: { sit_in_method: "physical", sit_in_course: { id: "c-int", code: "INT-01", name: "Math inter" }, available_sessions: [{ id: "as1", start_at: "2026-06-18T10:00:00Z", end_at: "2026-06-18T12:00:00Z", subject_name: "Math inter" }] },
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Need a make-up class");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());

    const makeUpSelect = await screen.findByRole("combobox");
    expect(makeUpSelect).toHaveTextContent("Math inter");
    expect(makeUpSelect).not.toHaveTextContent("Math advance");
  });

  it("shows the selected root sit-in session on the review step", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1", subject_code: "ADV", subject_name: "Math advance",
          course_id: "c-adv", course_code: "ADV-01", course_name: "Math advance",
          sessions: [{ id: "s1", start_at: "2026-06-02T09:00:00Z", end_at: "2026-06-02T10:30:00Z", date: "2026-06-02", already_absent: false }],
          sit_in: { sit_in_method: "physical", sit_in_course: { id: "c-int", code: "INT-01", name: "Math inter" }, available_sessions: [{ id: "as1", start_at: "2026-06-18T10:00:00Z", end_at: "2026-06-18T12:00:00Z", subject_name: "Math inter" }] },
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(screen.getByPlaceholderText("Tell us why you'll be away from class..."), "Need a make-up class");
    await toggleAllCourseSwitches(user);
    await user.click(await findSessionCheckbox());
    await user.selectOptions(await screen.findByRole("combobox"), "as1");
    await user.click(screen.getByRole("button", { name: /review absence/i }));

    await waitFor(() => expect(screen.getByRole("heading", { name: /review your absence/i })).toBeInTheDocument());
    expect(screen.getByText(/Make-up:/).parentElement).toHaveTextContent("Math inter");
    expect(screen.getByText(/Make-up:/).parentElement).toHaveTextContent("18 Jun 2026");
    expect(screen.queryByText("Make-up class selected")).not.toBeInTheDocument();
  });

  it("shows absence count breakdown when used_absence_days and total_course_days are present", async () => {
    const user = userEvent.setup();
    renderAbsenceForm({
      sessions: createMockSessionsInRange([
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Mathematics",
          course_id: "c-math201",
          course_code: "MATH",
          course_name: "Mathematics",
          absence_limit_reached: true,
          used_absence_days: 1,
          total_course_days: 11,
          maximum_absence_days: 2,
          remaining_absence_days: 1,
          sessions: [
            { id: "s1", start_at: "2026-06-01T09:00:00Z", end_at: "2026-06-01T10:30:00Z", date: "2026-06-01", already_absent: false },
          ],
        },
        {
          subject_id: "subj-2",
          subject_code: "PHYS",
          subject_name: "Physics",
          course_id: "c-phys301",
          course_code: "PHYS",
          course_name: "Physics",
          absence_limit_reached: true,
          used_absence_days: 3,
          total_course_days: 21,
          maximum_absence_days: 4,
          remaining_absence_days: 1,
          sessions: [
            { id: "s2", start_at: "2026-06-02T11:00:00Z", end_at: "2026-06-02T12:30:00Z", date: "2026-06-02", already_absent: false },
          ],
        },
      ]),
    });

    await lookupStudent(user);
    await verifyParent(user);
    await goToCourses(user);

    await user.type(await screen.findByPlaceholderText("Tell us why you'll be away from class..."), "Sick");
    await toggleAllCourseSwitches(user);

    // Wait for sessions to be fully rendered
    await waitFor(() => {
      expect(screen.getByText(/1 absence day used, max 2/)).toBeInTheDocument();
    }, { timeout: 10000 });
    expect(screen.getByText(/3 absence days used, max 4/)).toBeInTheDocument();
  });
});
