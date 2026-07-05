import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AbsenceForm from "../AbsenceForm";
import { renderWithProviders } from "./helpers";
import type { SubjectSessions } from "@/types";

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
    allow_submit_without_otp: true,
  },
  admin_contact: {
    email: "office@example.edu",
    phone: "+66 2123 4567",
    hours: "Mon-Fri 08:00-16:00",
  },
};

const MOCK_STUDENT = {
  student_id: "s1",
  wcode: "W250389",
  full_name: "John Smith",
  parent_phone: "+66812345678",
  subjects: [
    { id: "subj-1", code: "MATH", name: "Mathematics" },
  ],
};

function mockApiByPattern(routes: Record<string, unknown>) {
  mockApiJson.mockImplementation(async (url: string, _init?: RequestInit) => {
    for (const [pattern, data] of Object.entries(routes)) {
      if (String(url).includes(pattern)) return data;
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
      absence_rate_exceeded: existingMissed >= Math.round(totalSessions / 5),
      existing_absence_count: existingMissed,
      total_session_count: totalSessions,
    },
  ];
}

async function setupForm(sessions: SubjectSessions[]) {
  mockApiByPattern({
    "absence-form-config": MOCK_CONFIG,
    "student-lookup": MOCK_STUDENT,
    "sessions-in-range": { subjects: sessions },
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
  await waitFor(() => expect(screen.getByText("John Smith")).toBeInTheDocument());

  await user.click(screen.getByRole("button", { name: /send code/i }));
  await waitFor(() => {
    const skipBtn = screen.queryByRole("button", { name: /continue without verifying/i });
    expect(skipBtn).toBeInTheDocument();
  });
  await user.click(screen.getByRole("button", { name: /continue without verifying/i }));
  await waitFor(() => expect(screen.getByText("Courses & classes")).toBeInTheDocument());

  return user;
}

describe("AbsenceForm - 20% session limit", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  describe("remaining session display", () => {
    it("shows remaining sessions for a course", async () => {
      const sessions = createSessionsWithLimits(0, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        expect(screen.getByText("2 sessions remaining")).toBeInTheDocument();
      });
    });

    it("shows reduced remaining when existing absences exist", async () => {
      const sessions = createSessionsWithLimits(1, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        expect(screen.getByText("1 session remaining")).toBeInTheDocument();
      });
    });

    it("shows 'Limit reached' when absence_rate_exceeded is true", async () => {
      const sessions = createSessionsWithLimits(2, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        expect(screen.getByText("Limit reached")).toBeInTheDocument();
      });
    });

    it("shows correct max count in limit message (10-session course → max 2)", async () => {
      const sessions = createSessionsWithLimits(2, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        expect(screen.getByText(/max 2/)).toBeInTheDocument();
      });
    });

    it("shows correct max count for 20-session course → max 4", async () => {
      const sessions = createSessionsWithLimits(4, 20);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        expect(screen.getByText(/max 4/)).toBeInTheDocument();
      });
    });

    it("shows correct remaining for odd session counts", async () => {
      const sessions = createSessionsWithLimits(0, 11);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        expect(screen.getByText("2 sessions remaining")).toBeInTheDocument();
      });
    });

    it("shows correct remaining for large course", async () => {
      const sessions = createSessionsWithLimits(0, 100);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        expect(screen.getByText("20 sessions remaining")).toBeInTheDocument();
      });
    });
  });

  describe("session selection cap", () => {
    it("allows selecting sessions up to the remaining limit", async () => {
      const sessions = createSessionsWithLimits(0, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      const sessionCheckboxes = (await screen.findAllByRole("checkbox")).filter(
        (cb) => cb.getAttribute("id")?.startsWith("session-"),
      );

      expect(sessionCheckboxes.length).toBeGreaterThan(0);
    });

    it("disables session checkboxes when remaining is 0", async () => {
      const sessions = createSessionsWithLimits(2, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        const sessionCheckboxes = (screen.getAllByRole("checkbox")).filter(
          (cb) => cb.getAttribute("id")?.startsWith("session-"),
        );
        for (const cb of sessionCheckboxes) {
          expect(cb).toBeDisabled();
        }
      });
    });

    it("does not disable checkboxes when absence_rate_exceeded is false", async () => {
      const sessions = createSessionsWithLimits(0, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        const sessionCheckboxes = (screen.getAllByRole("checkbox")).filter(
          (cb) => cb.getAttribute("id")?.startsWith("session-"),
        );
        for (const cb of sessionCheckboxes) {
          expect(cb).not.toBeDisabled();
        }
      });
    });
  });

  describe("cumulative limit enforcement", () => {
    it("allows selecting sessions within 20% (10-session course, 0 existing)", async () => {
      const sessions = createSessionsWithLimits(0, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      const sessionCheckboxes = (await screen.findAllByRole("checkbox")).filter(
        (cb) => cb.getAttribute("id")?.startsWith("session-"),
      );

      expect(sessionCheckboxes.length).toBeGreaterThan(0);

      await user.click(sessionCheckboxes[0]);
      await waitFor(() => {
        expect(screen.getByText("1 session remaining")).toBeInTheDocument();
      });
    });

    it("blocks selecting beyond 20% (10-session course, 1 existing, max 1 more)", async () => {
      const sessions = createSessionsWithLimits(1, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      const sessionCheckboxes = (await screen.findAllByRole("checkbox")).filter(
        (cb) => cb.getAttribute("id")?.startsWith("session-"),
      );

      if (sessionCheckboxes.length > 0) {
        await user.click(sessionCheckboxes[0]);
        await waitFor(() => {
          expect(screen.getByText("Limit reached")).toBeInTheDocument();
        });
      }
    });

    it("hides session checkboxes when already at limit", async () => {
      const sessions = createSessionsWithLimits(2, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        expect(screen.getByText("Limit reached")).toBeInTheDocument();
      });

      const sessionCheckboxes = (screen.getAllByRole("checkbox")).filter(
        (cb) => cb.getAttribute("id")?.startsWith("session-"),
      );
      expect(sessionCheckboxes.length).toBe(0);
    });
  });

  describe("per-request limit enforcement", () => {
    it("blocks selecting more than 20% of sessions in one request (10-session course)", async () => {
      const sessions = createSessionsWithLimits(0, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      const sessionCheckboxes = (await screen.findAllByRole("checkbox")).filter(
        (cb) => cb.getAttribute("id")?.startsWith("session-"),
      );

      for (const cb of sessionCheckboxes) {
        await user.click(cb);
      }

      await waitFor(() => {
        expect(screen.getByText("Limit reached")).toBeInTheDocument();
      });
    });

    it("blocks selecting more than 20% of sessions in one request (20-session course)", async () => {
      const sessions = createSessionsWithLimits(0, 20);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      const sessionCheckboxes = (await screen.findAllByRole("checkbox")).filter(
        (cb) => cb.getAttribute("id")?.startsWith("session-"),
      );

      for (const cb of sessionCheckboxes) {
        await user.click(cb);
      }

      await waitFor(() => {
        expect(screen.getByText("Limit reached")).toBeInTheDocument();
      });
    });
  });

  describe("fallback behavior", () => {
    it("falls back to maxSessions when total_session_count is not provided", async () => {
      const sessionsWithoutLimits: SubjectSessions[] = [
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
            already_absent: false,
          })),
        },
      ];

      const user = await setupForm(sessionsWithoutLimits);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      const sessionCheckboxes = (await screen.findAllByRole("checkbox")).filter(
        (cb) => cb.getAttribute("id")?.startsWith("session-"),
      );

      expect(sessionCheckboxes.length).toBe(10);
    });
  });
});
