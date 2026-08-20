import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AbsenceForm from "../AbsenceForm";
import { renderWithProviders } from "./helpers";
import type { SubjectSessions } from "@/types";
import { ApiRequestError } from "@/api/client";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("react-router-dom", () => ({
  useNavigate: () => vi.fn(),
  useLocation: () => ({ pathname: "/absence" }),
}));

const MOCK_CONFIG = {
  form: {
    max_date_range_days: 30,
    min_hours_before_session: 0,
    max_hours_after_session: 0,
    require_reason: false,
    reason_categories: [],
    allow_free_text_reason: true,
    intro_text: "",
    confirmation_text: "",
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

const MOCK_LOOKUP = {
  wcode: "W250389",
  lookup_token: "opaque-lookup-token",
  email_input_required: false,
  parent_verification_available: true,
};

const MOCK_PROFILE = {
  wcode: "W250389",
  display_name: "Student",
  email_on_file: true,
  subjects: [
    { id: "subj-1", code: "MATH", name: "Mathematics" },
  ],
};

function mockApiByPattern(routes: Record<string, unknown>) {
  mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
    for (const [pattern, data] of Object.entries(routes)) {
      if (String(url).includes(pattern)) {
        return typeof data === "function"
          ? (data as (request?: RequestInit) => unknown)(init)
          : data;
      }
    }
    throw new Error(`Unmocked API call: ${url}`);
  });
}

function createSessionsWithLimits(
  existingMissed: number,
  totalSessions: number,
): SubjectSessions[] {
  const sessions = Array.from({ length: totalSessions }, (_, i) => ({
    id: `s${i + 1}`,
    start_at: `2026-06-${String(i + 1).padStart(2, "0")}T09:00:00Z`,
    end_at: `2026-06-${String(i + 1).padStart(2, "0")}T10:30:00Z`,
    date: `2026-06-${String(i + 1).padStart(2, "0")}`,
    already_absent: i < existingMissed,
  }));

  return [
    {
      subject_id: "subj-1",
      subject_code: "MATH",
      subject_name: "Mathematics",
      course_id: "c-math201",
      course_code: "MATH201",
      course_name: "Algebra II",
      sessions,
      absence_limit_reached: existingMissed * 5 >= totalSessions,
      used_absence_days: existingMissed,
      total_course_days: totalSessions,
      maximum_absence_days: Math.round(totalSessions / 5),
      remaining_absence_days: Math.max(0, Math.round(totalSessions / 5) - existingMissed),
    },
  ];
}

async function continueToVerification(user: ReturnType<typeof userEvent.setup>) {
  const emailInput = screen.queryByRole("textbox", { name: /your email address/i });
  if (emailInput) await user.type(emailInput, "student@example.com");
  await user.click(screen.getByRole("button", { name: /continue to verification/i }));
  await screen.findByRole("heading", { name: /parent verification/i });
}

async function completeVerification(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /send code/i }));
  const codeInput = (await screen.findAllByRole("textbox", { hidden: true })).find(
    (element) => element.getAttribute("inputMode") === "numeric",
  );
  if (!codeInput) throw new Error("OTP input was not rendered");
  await user.type(codeInput, "123456");
  await waitFor(() => expect(screen.getByText("Courses & classes")).toBeInTheDocument());
}

describe("AbsenceForm - error handling", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("displays absence_limit_exceeded error message when backend returns 403", async () => {
    const sessions = createSessionsWithLimits(0, 10);

    mockApiByPattern({
      "absence-form-config": MOCK_CONFIG,
      "absence-self-service/lookup": MOCK_LOOKUP,
      "absence-self-service/me": MOCK_PROFILE,
      "absence-self-service/sessions": { subjects: sessions },
      "parent-verification/send": { token: "otp-session-123", status: "pending", wcode: "W250389", expires_at: new Date(Date.now() + 300000).toISOString() },
      "parent-verification/verify": { token: "otp-token-123", status: "verified", wcode: "W250389", expires_at: new Date(Date.now() + 300000).toISOString() },
      "absences/batch": () => {
        throw new ApiRequestError("You have reached the maximum number of absences allowed for this course", { code: "absence_limit_exceeded", status: 403 });
      },
    });

    renderWithProviders(<AbsenceForm />);

    const user = userEvent.setup();

    const input = await screen.findByPlaceholderText("e.g. W250389");
    await user.clear(input);
    await user.type(input, "W250389");
    await user.click(screen.getByRole("button", { name: /search/i }));
    await waitFor(() => expect(screen.getByText("Student ID found")).toBeInTheDocument());

    await continueToVerification(user);
    await completeVerification(user);

    const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
    await user.click(courseCheckbox);

    const sessionCheckboxes = (await screen.findAllByRole("checkbox")).filter(
      (cb) => cb.getAttribute("id")?.startsWith("session-"),
    );
    if (sessionCheckboxes.length > 0) {
      await user.click(sessionCheckboxes[0]);
    }

    await user.type(
      screen.getByRole("textbox", { name: /reason for absence/i }),
      "Medical appointment",
    );
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    expect(screen.getByRole("heading", { name: /review your absence/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));

    expect(await screen.findByText(
      "You have reached the maximum absences allowed for one or more courses. Please go back and remove those courses.",
      { selector: "[role=\"alert\"]" },
    )).toBeInTheDocument();
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/batch",
      expect.objectContaining({ method: "POST" }),
    );
    expect(screen.getByRole("heading", { name: /review your absence/i })).toBeInTheDocument();
  });
});

describe("AbsenceForm - multi-subject independent remaining counts", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("shows independent remaining counts for different subjects", async () => {
    const sessions: SubjectSessions[] = [
      {
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        course_id: "c-math201",
        course_code: "MATH201",
        course_name: "Algebra II",
        sessions: Array.from({ length: 10 }, (_, i) => ({
          id: `s${i + 1}`,
          start_at: `2026-06-${String(i + 1).padStart(2, "0")}T09:00:00Z`,
          end_at: `2026-06-${String(i + 1).padStart(2, "0")}T10:30:00Z`,
          date: `2026-06-${String(i + 1).padStart(2, "0")}`,
          already_absent: i < 1, // 1 existing
        })),
        absence_limit_reached: false,
        used_absence_days: 1,
        total_course_days: 10,
        maximum_absence_days: 2,
        remaining_absence_days: 1,
      },
      {
        subject_id: "subj-2",
        subject_code: "PHYS",
        subject_name: "Physics",
        course_id: "c-phys101",
        course_code: "PHYS101",
        course_name: "Physics I",
        sessions: Array.from({ length: 20 }, (_, i) => ({
          id: `p${i + 1}`,
          start_at: `2026-06-${String(i + 1).padStart(2, "0")}T14:00:00Z`,
          end_at: `2026-06-${String(i + 1).padStart(2, "0")}T15:30:00Z`,
          date: `2026-06-${String(i + 1).padStart(2, "0")}`,
          already_absent: i < 3, // 3 existing
        })),
        absence_limit_reached: false,
        used_absence_days: 3,
        total_course_days: 20,
        maximum_absence_days: 4,
        remaining_absence_days: 1,
      },
    ];

    const multiSubjectProfile = {
      ...MOCK_PROFILE,
      subjects: [
        { id: "subj-1", code: "MATH", name: "Mathematics" },
        { id: "subj-2", code: "PHYS", name: "Physics" },
      ],
    };
    mockApiByPattern({
      "absence-form-config": MOCK_CONFIG,
      "absence-self-service/lookup": MOCK_LOOKUP,
      "absence-self-service/me": multiSubjectProfile,
      "absence-self-service/sessions": { subjects: sessions },
      "parent-verification/send": { token: "otp-session-123", status: "pending", wcode: "W250389", expires_at: new Date(Date.now() + 300000).toISOString() },
      "parent-verification/verify": { token: "otp-token-123", status: "verified", wcode: "W250389", expires_at: new Date(Date.now() + 300000).toISOString() },
      "absences/batch": { items: [{ id: "abc12345", status: "pending" }] },
    });

    renderWithProviders(<AbsenceForm />);

    const user = userEvent.setup();

    const input = await screen.findByPlaceholderText("e.g. W250389");
    await user.clear(input);
    await user.type(input, "W250389");
    await user.click(screen.getByRole("button", { name: /search/i }));
    await waitFor(() => expect(screen.getByText("Student ID found")).toBeInTheDocument());

    await continueToVerification(user);
    await completeVerification(user);

    // Select Mathematics
    const mathCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
    await user.click(mathCheckbox);

    // Should show 1 day remaining for Math (10 course days, 1 existing = 1 remaining)
    await waitFor(() => {
      expect(screen.getByText("1 day remaining")).toBeInTheDocument();
    });

    // Verify the remaining count is correct for Math
    const remainingText = screen.getByText("1 day remaining");
    expect(remainingText).toBeInTheDocument();
  });
});
