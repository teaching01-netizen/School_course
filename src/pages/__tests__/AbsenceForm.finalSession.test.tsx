import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AbsenceForm from "../AbsenceForm";
import type { SessionsInRangeResponse } from "@/types";
import { renderWithProviders } from "./helpers";
import { continueThroughVerification, installPublicFormRoutes, searchForStudent } from "./helpers/absenceFormHarness";
import { MANUAL_EMAIL_STUDENT, PUBLIC_FORM_SESSIONS } from "./fixtures/absenceFormFixtures";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("react-router-dom", () => ({
  useNavigate: () => vi.fn(),
  useLocation: () => ({ pathname: "/absence" }),
}));

const sessions: SessionsInRangeResponse = {
  subjects: [{
    ...PUBLIC_FORM_SESSIONS.subjects[0],
    total_course_days: 10,
    used_absence_days: 0,
    maximum_absence_days: 2,
    remaining_absence_days: 2,
    absence_limit_reached: false,
    sessions: Array.from({ length: 10 }, (_, index) => {
      const date = `2027-01-${String(index + 1).padStart(2, "0")}`;
      return {
        id: `math-lesson-${index + 1}`,
        date,
        start_at: `${date}T02:00:00Z`,
        end_at: `${date}T03:30:00Z`,
        already_absent: false,
      };
    }),
    sit_in: {
      sit_in_method: "physical",
      rule_type: "level_ladder",
      sit_in_course: { id: "math-target", code: "MATH301", name: "Mathematics Level 3" },
      available_sessions: [
        { id: "target-earlier", start_at: "2027-01-11T02:00:00Z", end_at: "2027-01-11T03:30:00Z", course_id: "math-target" },
        { id: "target-final", start_at: "2027-01-18T02:00:00Z", end_at: "2027-01-18T03:30:00Z", course_id: "math-target" },
      ],
    },
  }],
};

describe("AbsenceForm final sessions outside SAT Verbal policy", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    window.localStorage.clear();
    window.sessionStorage.clear();
  });

  it.each(["student", "staff"] as const)("lets %s select and submit final-session leave and a final-session sit-in", async (mode) => {
    installPublicFormRoutes(mockApiJson, { sessions });
    const publicRoutes = mockApiJson.getMockImplementation()!;
    const submitPath = mode === "staff" ? "/api/v1/absences/staff-form-batch" : "/api/v1/absences/batch";
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      if (url.includes("/admin/absences/student-lookup")) return MANUAL_EMAIL_STUDENT;
      if (url === submitPath) return {
        ids: ["final-session-absence"],
        items: [{
          id: "final-session-absence",
          wcode: MANUAL_EMAIL_STUDENT.wcode,
          course_id: "course-math",
          course_name: "Mathematics",
          subject_id: "subject-math",
          subject_name: "Mathematics",
          student_name: MANUAL_EMAIL_STUDENT.full_name,
          status: "pending",
          date_from: "2027-01-10",
          date_to: "2027-01-10",
          reason: "Medical appointment",
          sit_in_method: "physical",
          sit_in_course_id: "math-target",
          version: 1,
          missed_sessions: [{ ...sessions.subjects[0].sessions[9], session_id: "math-lesson-10" }],
          sit_ins: [{ ...sessions.subjects[0].sit_in!.available_sessions![1], session_id: "target-final" }],
        }],
      };
      return publicRoutes(url, init);
    });
    renderWithProviders(<AbsenceForm mode={mode === "staff" ? "staff" : undefined} />);
    const user = userEvent.setup();
    await searchForStudent(user, MANUAL_EMAIL_STUDENT.wcode);
    await screen.findByText("Student ID found");
    if (mode === "student") await continueThroughVerification(user);
    else await user.click(screen.getByRole("button", { name: /continue to classes/i }));

    await user.click(await screen.findByRole("checkbox", { name: /mathematics/i }));
    const finalLesson = (await screen.findAllByRole("checkbox")).find((element) => element.id === "session-math-lesson-10");
    expect(finalLesson).toBeDefined();
    expect(finalLesson).toBeEnabled();
    await user.click(finalLesson!);

    const picker = await screen.findByRole("combobox");
    expect(within(picker).getByRole("option", { name: /11 Jan/i })).toBeEnabled();
    expect(within(picker).getByRole("option", { name: /18 Jan/i })).toBeEnabled();
    await user.selectOptions(picker, "target-final");
    await user.type(screen.getByRole("textbox", { name: /reason for absence/i }), "Medical appointment");
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    await user.click(await screen.findByRole("button", { name: /^submit absence$/i }));
    expect(await screen.findByRole("heading", { name: mode === "staff" ? "Absence recorded" : "Absence submitted" })).toBeInTheDocument();

    await waitFor(() => {
      const call = mockApiJson.mock.calls.find(([url]) => url === submitPath);
      expect(call).toBeDefined();
      expect(JSON.parse(String(call![1]?.body)).items).toEqual([expect.objectContaining({
        course_id: "course-math",
        date_from: "2027-01-10",
        date_to: "2027-01-10",
        missed_session_ids: ["math-lesson-10"],
        sit_in_course_id: "math-target",
        sit_in_session_ids: ["target-final"],
      })]);
    });
  }, 30000);
});
