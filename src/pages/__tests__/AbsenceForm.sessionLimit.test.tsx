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
      absence_limit_reached: existingMissed >= Math.round(totalSessions / 5),
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

  await continueToVerification(user);
  await completeVerification(user);

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
        expect(screen.getByText("2 days remaining")).toBeInTheDocument();
      });
    });

    it("shows reduced remaining when existing absences exist", async () => {
      const sessions = createSessionsWithLimits(1, 10);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        expect(screen.getByText("1 day remaining")).toBeInTheDocument();
      });
    });

    it("shows 'Limit reached' when absence_limit_reached is true", async () => {
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
        expect(screen.getByText("2 days remaining")).toBeInTheDocument();
      });
    });

    it("shows correct remaining for large course", async () => {
      const sessions = createSessionsWithLimits(0, 100);
      const user = await setupForm(sessions);

      const courseCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(courseCheckbox);

      await waitFor(() => {
        expect(screen.getByText("20 days remaining")).toBeInTheDocument();
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

    it("does not disable checkboxes when absence_limit_reached is false", async () => {
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
        expect(screen.getByText("1 day remaining")).toBeInTheDocument();
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

      const firstAvailableSession = sessionCheckboxes.find((checkbox) => !checkbox.hasAttribute("disabled"));
      if (firstAvailableSession) {
        await user.click(firstAvailableSession);
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

  describe("per-course maxSessions", () => {
    function makeTwoCourseSetup(maxPerAbsence: number) {
      const sessions: SubjectSessions[] = [
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Mathematics",
          course_id: "c-math201",
          course_code: "MATH201",
          course_name: "Algebra II",
          sessions: Array.from({ length: 10 }, (_, i) => ({
            id: `m${i + 1}`,
            start_at: `2026-06-${String(i + 1).padStart(2, "0")}T09:00:00Z`,
            end_at: `2026-06-${String(i + 1).padStart(2, "0")}T10:30:00Z`,
            date: `2026-06-${String(i + 1).padStart(2, "0")}`,
            already_absent: false,
          })),
          absence_limit_reached: false,
          used_absence_days: 0,
          total_course_days: 10,
          maximum_absence_days: 2,
          remaining_absence_days: 2,
        },
        {
          subject_id: "subj-2",
          subject_code: "PHY",
          subject_name: "Physics",
          course_id: "c-phy301",
          course_code: "PHY301",
          course_name: "Mechanics",
          sessions: Array.from({ length: 10 }, (_, i) => ({
            id: `p${i + 1}`,
            start_at: `2026-06-${String(i + 1).padStart(2, "0")}T14:00:00Z`,
            end_at: `2026-06-${String(i + 1).padStart(2, "0")}T15:30:00Z`,
            date: `2026-06-${String(i + 1).padStart(2, "0")}`,
            already_absent: false,
          })),
          absence_limit_reached: false,
          used_absence_days: 0,
          total_course_days: 10,
          maximum_absence_days: 2,
          remaining_absence_days: 2,
        },
      ];
      return { sessions, maxPerAbsence };
    }

    async function setupTwoCourse(sessions: SubjectSessions[], maxPerAbsence: number) {
      const configSmallMax = {
        ...MOCK_CONFIG,
        sit_in: { ...MOCK_CONFIG.sit_in, max_sessions_per_absence: maxPerAbsence },
      };
      const twoCourseStudent = {
        ...MOCK_STUDENT,
        subjects: [
          { id: "subj-1", code: "MATH", name: "Mathematics" },
          { id: "subj-2", code: "PHY", name: "Physics" },
        ],
      };
      mockApiByPattern({
        "absence-form-config": configSmallMax,
        "student-lookup": twoCourseStudent,
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

      await continueToVerification(user);
      await completeVerification(user);

      return user;
    }

    const sessionCbs = () => screen.getAllByRole("checkbox").filter((cb) => cb.getAttribute("id")?.startsWith("session-"));

    it("selecting sessions from Course A does not block selecting sessions from Course B", async () => {
      const { sessions } = makeTwoCourseSetup(2);
      const user = await setupTwoCourse(sessions, 2);

      const mathCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(mathCheckbox);
      await waitFor(() => expect(screen.getByText("2 days remaining")).toBeInTheDocument());

      const mathSessions = sessionCbs().filter((cb) => cb.getAttribute("id")?.includes("m"));
      await user.click(mathSessions[0]);
      await user.click(mathSessions[1]);

      await waitFor(() => expect(screen.getByText("Limit reached")).toBeInTheDocument());

      const phyCheckbox = await screen.findByRole("checkbox", { name: /physics/i });
      await user.click(phyCheckbox);

      const phySessions = sessionCbs().filter((cb) => cb.getAttribute("id")?.includes("p"));
      expect(phySessions.length).toBeGreaterThan(0);
      expect(phySessions[0]).not.toBeDisabled();
      expect(phySessions[1]).not.toBeDisabled();
    });

    it("maxSessions caps selections within a single course", async () => {
      const { sessions } = makeTwoCourseSetup(2);
      const user = await setupTwoCourse(sessions, 2);

      const mathCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(mathCheckbox);

      const mathSessions = sessionCbs().filter((cb) => cb.getAttribute("id")?.includes("m"));
      await user.click(mathSessions[0]);
      await user.click(mathSessions[1]);

      await waitFor(() => expect(screen.getByText("Limit reached")).toBeInTheDocument());

      await user.click(mathSessions[2]);
      const mathAfter = sessionCbs().filter((cb) => cb.getAttribute("id")?.includes("m") && cb.getAttribute("id")?.startsWith("session-"));
      const mathChecked = mathAfter.filter((cb) => cb.getAttribute("id")?.startsWith("session-m") && (cb as HTMLInputElement).checked);
      expect(mathChecked.length).toBe(2);
    });

    it("deselecting frees room for re-selection in the same course", async () => {
      const { sessions } = makeTwoCourseSetup(2);
      const user = await setupTwoCourse(sessions, 2);

      const mathCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(mathCheckbox);

      const mathSessions = sessionCbs().filter((cb) => cb.getAttribute("id")?.includes("m"));
      await user.click(mathSessions[0]);
      await user.click(mathSessions[1]);
      await waitFor(() => expect(screen.getByText("Limit reached")).toBeInTheDocument());

      await user.click(mathSessions[0]);
      await waitFor(() => expect(screen.getByText("1 day remaining")).toBeInTheDocument());

      await user.click(mathSessions[2]);
      const mathChecked = sessionCbs()
        .filter((cb) => cb.getAttribute("id")?.startsWith("session-m"))
        .filter((cb) => (cb as HTMLInputElement).checked);
      expect(mathChecked.length).toBe(2);
    });

    it("each course is independently capped at maxSessions", async () => {
      const { sessions } = makeTwoCourseSetup(2);
      const user = await setupTwoCourse(sessions, 2);

      const mathCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(mathCheckbox);
      const mathSessions = sessionCbs().filter((cb) => cb.getAttribute("id")?.includes("m"));
      await user.click(mathSessions[0]);
      await user.click(mathSessions[1]);

      const phyCheckbox = await screen.findByRole("checkbox", { name: /physics/i });
      await user.click(phyCheckbox);
      const phySessions = sessionCbs().filter((cb) => cb.getAttribute("id")?.includes("p"));
      await user.click(phySessions[0]);
      await user.click(phySessions[1]);

      const allChecked = sessionCbs().filter((cb) => (cb as HTMLInputElement).checked);
      expect(allChecked.length).toBe(4);

      await user.click(phySessions[2]);
      const phyCheckedAfter = sessionCbs()
        .filter((cb) => cb.getAttribute("id")?.startsWith("session-p"))
        .filter((cb) => (cb as HTMLInputElement).checked);
      expect(phyCheckedAfter.length).toBe(2);
    });

    it("20% limit and maxSessions - lower limit wins", async () => {
      const sessions: SubjectSessions[] = [
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Mathematics",
          course_id: "c-math201",
          course_code: "MATH201",
          course_name: "Algebra II",
          sessions: Array.from({ length: 10 }, (_, i) => ({
            id: `m${i + 1}`,
            start_at: `2026-06-${String(i + 1).padStart(2, "0")}T09:00:00Z`,
            end_at: `2026-06-${String(i + 1).padStart(2, "0")}T10:30:00Z`,
            date: `2026-06-${String(i + 1).padStart(2, "0")}`,
            already_absent: false,
          })),
          absence_limit_reached: false,
          used_absence_days: 0,
          total_course_days: 10,
          maximum_absence_days: 2,
          remaining_absence_days: 2,
        },
      ];

      const configHighMax = {
        ...MOCK_CONFIG,
        sit_in: { ...MOCK_CONFIG.sit_in, max_sessions_per_absence: 5 },
      };
      mockApiByPattern({
        "absence-form-config": configHighMax,
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

      await continueToVerification(user);
      await completeVerification(user);

      const mathCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(mathCheckbox);

      await waitFor(() => expect(screen.getByText("2 days remaining")).toBeInTheDocument());

      const mathSessions = sessionCbs().filter((cb) => cb.getAttribute("id")?.includes("m"));
      await user.click(mathSessions[0]);
      await user.click(mathSessions[1]);

      await waitFor(() => expect(screen.getByText("Limit reached")).toBeInTheDocument());

      await user.click(mathSessions[2]);
      const mathChecked = sessionCbs()
        .filter((cb) => cb.getAttribute("id")?.startsWith("session-m"))
        .filter((cb) => (cb as HTMLInputElement).checked);
      expect(mathChecked.length).toBe(2);
    });

    it("session group with multiple sessions on same day consumes one absence day", async () => {
      const sessions: SubjectSessions[] = [
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Mathematics",
          course_id: "c-math201",
          course_code: "MATH201",
          course_name: "Algebra II",
          sessions: [
            { id: "m1", start_at: "2026-06-01T09:00:00Z", end_at: "2026-06-01T10:30:00Z", date: "2026-06-01", already_absent: false },
            { id: "m2", start_at: "2026-06-01T11:00:00Z", end_at: "2026-06-01T12:30:00Z", date: "2026-06-01", already_absent: false },
            { id: "m3", start_at: "2026-06-02T09:00:00Z", end_at: "2026-06-02T10:30:00Z", date: "2026-06-02", already_absent: false },
            { id: "m4", start_at: "2026-06-03T09:00:00Z", end_at: "2026-06-03T10:30:00Z", date: "2026-06-03", already_absent: false },
            { id: "m5", start_at: "2026-06-04T09:00:00Z", end_at: "2026-06-04T10:30:00Z", date: "2026-06-04", already_absent: false },
            { id: "m6", start_at: "2026-06-05T09:00:00Z", end_at: "2026-06-05T10:30:00Z", date: "2026-06-05", already_absent: false },
            { id: "m7", start_at: "2026-06-06T09:00:00Z", end_at: "2026-06-06T10:30:00Z", date: "2026-06-06", already_absent: false },
            { id: "m8", start_at: "2026-06-07T09:00:00Z", end_at: "2026-06-07T10:30:00Z", date: "2026-06-07", already_absent: false },
            { id: "m9", start_at: "2026-06-08T09:00:00Z", end_at: "2026-06-08T10:30:00Z", date: "2026-06-08", already_absent: false },
            { id: "m10", start_at: "2026-06-09T09:00:00Z", end_at: "2026-06-09T10:30:00Z", date: "2026-06-09", already_absent: false },
          ],
          absence_limit_reached: false,
          used_absence_days: 0,
          total_course_days: 10,
          maximum_absence_days: 2,
          remaining_absence_days: 2,
        },
      ];

      const configMax2 = {
        ...MOCK_CONFIG,
        sit_in: { ...MOCK_CONFIG.sit_in, max_sessions_per_absence: 2 },
      };
      mockApiByPattern({
        "absence-form-config": configMax2,
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

      await continueToVerification(user);
      await completeVerification(user);

      const mathCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(mathCheckbox);

      const day1Sessions = sessionCbs().filter((cb) => cb.getAttribute("id") === "session-m1|m2");
      expect(day1Sessions.length).toBe(1);

      await user.click(day1Sessions[0]);

      expect((day1Sessions[0] as HTMLInputElement).checked).toBe(true);

      await waitFor(() => {
        const selectedCounter = screen.getByText(/\d+ selected/);
        expect(selectedCounter.textContent).toContain("1 selected");
      });
      expect(screen.getByRole("button", { name: /mathematics\s*1 class day selected/i })).toBeInTheDocument();

      const day2Sessions = sessionCbs().filter((cb) => cb.getAttribute("id") === "session-m3");
      await user.click(day2Sessions[0]);

      expect((day2Sessions[0] as HTMLInputElement).checked).toBe(true);

      await waitFor(() => {
        const selectedCounter = screen.getByText(/\d+ selected/);
        expect(selectedCounter.textContent).toContain("2 selected");
      });

      const day3Sessions = sessionCbs().filter((cb) => cb.getAttribute("id") === "session-m4");
      expect((day3Sessions[0] as HTMLInputElement).disabled).toBe(true);
    });

    it("checkboxes disabled per-course when at maxSessions", async () => {
      const { sessions } = makeTwoCourseSetup(2);
      const user = await setupTwoCourse(sessions, 2);

      const mathCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(mathCheckbox);
      const mathSessions = sessionCbs().filter((cb) => cb.getAttribute("id")?.includes("m"));
      await user.click(mathSessions[0]);
      await user.click(mathSessions[1]);

      await waitFor(() => expect(screen.getByText("Limit reached")).toBeInTheDocument());

      const mathUnchecked = mathSessions.filter((cb) => !(cb as HTMLInputElement).checked);
      for (const cb of mathUnchecked) {
        expect(cb).toBeDisabled();
      }

      const phyCheckbox = await screen.findByRole("checkbox", { name: /physics/i });
      await user.click(phyCheckbox);
      const phySessions = sessionCbs().filter((cb) => cb.getAttribute("id")?.includes("p"));
      for (const cb of phySessions) {
        expect(cb).not.toBeDisabled();
      }
    });

    it("can select up to maxSessions after existing absences reduce room", async () => {
      const sessions: SubjectSessions[] = [
        {
          subject_id: "subj-1",
          subject_code: "MATH",
          subject_name: "Mathematics",
          course_id: "c-math201",
          course_code: "MATH201",
          course_name: "Algebra II",
          sessions: Array.from({ length: 10 }, (_, i) => ({
            id: `m${i + 1}`,
            start_at: `2026-06-${String(i + 1).padStart(2, "0")}T09:00:00Z`,
            end_at: `2026-06-${String(i + 1).padStart(2, "0")}T10:30:00Z`,
            date: `2026-06-${String(i + 1).padStart(2, "0")}`,
            already_absent: i < 1,
          })),
          absence_limit_reached: false,
          used_absence_days: 1,
          total_course_days: 10,
          maximum_absence_days: 2,
          remaining_absence_days: 1,
        },
      ];

      const configMax3 = {
        ...MOCK_CONFIG,
        sit_in: { ...MOCK_CONFIG.sit_in, max_sessions_per_absence: 3 },
      };
      mockApiByPattern({
        "absence-form-config": configMax3,
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

      await continueToVerification(user);
      await completeVerification(user);

      const mathCheckbox = await screen.findByRole("checkbox", { name: /mathematics/i });
      await user.click(mathCheckbox);

      await waitFor(() => expect(screen.getByText("1 day remaining")).toBeInTheDocument());

      const mathSessions = sessionCbs().filter((cb) => cb.getAttribute("id")?.includes("m"));
      const availableMathSessions = mathSessions.filter((cb) => !cb.hasAttribute("disabled"));
      await user.click(availableMathSessions[0]);
      await waitFor(() => expect(screen.getByText("Limit reached")).toBeInTheDocument());

      await user.click(availableMathSessions[1]);
      const mathChecked = sessionCbs()
        .filter((cb) => cb.getAttribute("id")?.startsWith("session-m"))
        .filter((cb) => (cb as HTMLInputElement).checked);
      expect(mathChecked.length).toBe(1);
    });
  });

  describe("fallback behavior", () => {
    it("falls back to maxSessions when total_course_days is not provided", async () => {
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
