import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AbsenceForm from "../AbsenceForm";
import { renderWithProviders } from "./helpers";
import { PUBLIC_FORM_CONFIG, PUBLIC_FORM_SESSIONS, MANUAL_EMAIL_STUDENT } from "./fixtures/absenceFormFixtures";
import { chooseMakeUp, closeMakeUpSheet, makeUpValue, openMakeUpSheet, sheetChoices } from "./fixtures/makeUpSheet";
import type { StudentLookupResponse, SubjectSessions } from "@/types";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("react-router-dom", () => ({
  useNavigate: () => vi.fn(),
  useLocation: () => ({ pathname: "/staff/absence" }),
}));

const STAFF_SUBMISSION = {
  id: "absence-staff-1",
  wcode: MANUAL_EMAIL_STUDENT.wcode,
  status: "pending" as const,
  course_id: "course-math",
  course_code: "MATH201",
  course_name: "Mathematics",
  subject_id: "subject-math",
  subject_code: "MATH",
  subject_name: "Mathematics",
  student_name: MANUAL_EMAIL_STUDENT.full_name,
  date_from: "2026-08-03",
  date_to: "2026-08-03",
  reason: "Medical appointment",
  sit_in_method: null,
  version: 1,
  created_at: "2026-08-01T09:00:00Z",
  updated_at: "2026-08-01T09:00:00Z",
  missed_sessions: [{
    id: "missed-1",
    session_id: "session-math-1",
    course_id: "course-math",
    course_code: "MATH201",
    course_name: "Mathematics",
    start_at: "2026-08-03T02:00:00Z",
    end_at: "2026-08-03T03:30:00Z",
  }],
};

function installStaffRoutes({ lookup = MANUAL_EMAIL_STUDENT, subjects = () => PUBLIC_FORM_SESSIONS.subjects }: {
  lookup?: StudentLookupResponse;
  subjects?: (path: string) => SubjectSessions[];
} = {}) {
  mockApiJson.mockImplementation(async (url: string) => {
    const path = String(url);
    if (path.includes("absence-form-config")) return PUBLIC_FORM_CONFIG;
    if (path.includes("/admin/absences/student-lookup")) return lookup;
    if (path.includes("/absences/sessions-in-range")) return { subjects: subjects(path) };
    if (path.includes("/absences/staff-form-batch")) return { ids: [STAFF_SUBMISSION.id], items: [STAFF_SUBMISSION] };
    throw new Error(`Unmocked API call: ${path}`);
  });
}

async function searchStaffStudent(user: ReturnType<typeof userEvent.setup>) {
  const input = await screen.findByRole("textbox", { name: /student id/i });
  await user.type(input, MANUAL_EMAIL_STUDENT.wcode);
  await user.click(screen.getByRole("button", { name: /^search$/i }));
  expect(await screen.findByText("Student ID found")).toBeInTheDocument();
}

async function selectStaffClass(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /continue to classes/i }));
  expect(await screen.findByText("Courses & classes")).toBeInTheDocument();
  await user.click(screen.getByRole("checkbox", { name: /mathematics/i }));
  const session = (await screen.findAllByRole("checkbox")).find((checkbox) => checkbox.id.startsWith("session-"));
  if (!session) throw new Error("No session checkbox found");
  await user.click(session);
}

describe("AbsenceForm staff mode", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    window.localStorage.clear();
    window.sessionStorage.clear();
  });

  it("uses three steps, staff APIs, no email or verification UI, and records silently", async () => {
    const user = userEvent.setup();
    installStaffRoutes();
    renderWithProviders(<AbsenceForm mode="staff" />);

    const progress = await screen.findByRole("navigation", { name: /progress/i });
    expect(withinLabels(progress)).toEqual(["Student - current", "Classes", "Review"]);

    await searchStaffStudent(user);
    expect(screen.queryByRole("textbox", { name: /email/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /parent verification/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/sms|email|notification/i)).not.toBeInTheDocument();

    await selectStaffClass(user);
    await user.type(screen.getByRole("textbox", { name: /reason for absence/i }), "Medical appointment");
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    expect(await screen.findByRole("heading", { name: /review your absence/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));
    expect(await screen.findByRole("heading", { name: "Absence recorded" })).toBeInTheDocument();
    expect(screen.getByText("Your absence request has been recorded and is waiting for review.")).toBeInTheDocument();
    expect(screen.queryByText(/has been sent|sent sms|sent email/i)).not.toBeInTheDocument();

    const calls: Array<[string, RequestInit | undefined]> = mockApiJson.mock.calls.map(
      ([url, init]): [string, RequestInit | undefined] => [String(url), init as RequestInit | undefined],
    );
    expect(calls.some(([url]) => url.includes("/admin/absences/student-lookup"))).toBe(true);
    expect(calls.some(([url]) => url.includes("/absences/sessions-in-range"))).toBe(true);
    expect(calls.find(([url]) => url.includes("/admin/absences/student-lookup"))?.[0]).toContain("student_view=true");
    expect(calls.find(([url]) => url.includes("/absences/sessions-in-range"))?.[0]).toContain("student_view=true");
    expect(calls.some(([url]) => url.endsWith("/absences/staff-form-batch"))).toBe(true);
    expect(calls.some(([url]) => url.endsWith("/absences/batch"))).toBe(false);
    expect(calls.some(([url]) => url.includes("parent-verification") || /sms|email/i.test(url))).toBe(false);

    const submission = calls.find(([url]) => url.endsWith("/absences/staff-form-batch"));
    expect(submission?.[1]?.body).toEqual(expect.stringContaining('"status":"pending"'));
    expect(submission?.[1]?.body).not.toEqual(expect.stringContaining("email"));
  }, 30000);

  it("keeps staff on Classes and shows the shared required-reason validation", async () => {
    const user = userEvent.setup();
    installStaffRoutes();
    renderWithProviders(<AbsenceForm mode="staff" />);

    await searchStaffStudent(user);
    await selectStaffClass(user);
    await user.click(screen.getByRole("button", { name: /review absence/i }));

    expect(await screen.findByText("Please tell us why you'll be away.", { selector: '[role="alert"]' })).toBeInTheDocument();
    expect(screen.getByText("Courses & classes")).toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([url]) => String(url).includes("staff-form-batch"))).toBe(false);
  });

  it("appends available alternatives and preserves the selection in Staff preview", async () => {
    const user = userEvent.setup();
    const math = PUBLIC_FORM_SESSIONS.subjects[0];
    const firstCourse = { id: "target-1", code: "T1", name: "First course" };
    const nextCourse = { id: "target-2", code: "T2", name: "Second course" };
    const first = { id: "first", course_id: firstCourse.id, start_at: "2026-08-06T02:00:00Z", end_at: "2026-08-06T03:30:00Z" };
    const next = { id: "next", course_id: nextCourse.id, start_at: "2026-08-07T02:00:00Z", end_at: "2026-08-07T03:30:00Z" };
    const final = { ...next, id: "final", start_at: "2026-08-08T02:00:00Z", end_at: "2026-08-08T03:30:00Z" };
    const initialSitIn: SubjectSessions["sit_in"] = {
      sit_in_method: "physical", current_priority_level: 1, has_next_priority: true,
      priorities: [{ level: 1, label: "First", sit_in_course: firstCourse, available_sessions: [first] }],
    };
    const revealedSitIn: SubjectSessions["sit_in"] = {
      sit_in_method: "physical", current_priority_level: 2, has_next_priority: false,
      priorities: [
        { level: 2, label: "Second", sit_in_course: firstCourse, unavailable_sessions: [{ session: first, reason: "Already offered", reason_code: "already_offered" }] },
        { level: 2, label: "Second", sit_in_course: nextCourse, available_sessions: [next], unavailable_sessions: [{ session: final, reason: "Final class", reason_code: "target_final_class" }] },
      ],
    };
    installStaffRoutes({ subjects: (path) => [{ ...math, sit_in: path.includes("sat_verbal_after_priority=1") ? revealedSitIn : initialSitIn }] });
    renderWithProviders(<AbsenceForm mode="staff" />);

    await searchStaffStudent(user);
    await selectStaffClass(user);
    await chooseMakeUp(user, first.id);
    const dialog = await openMakeUpSheet(user);
    await user.click(within(dialog).getByRole("button", { name: "See other times" }));
    await within(dialog).findByText("No more times to show.");
    const offered = await sheetChoices(user, dialog);
    expect(offered).toEqual([
      expect.objectContaining({ value: first.id, disabled: false, checked: true }),
      expect.objectContaining({ value: next.id, disabled: false }),
    ]);
    await closeMakeUpSheet(user, dialog);
    expect(await makeUpValue(user)).toBe(first.id);

    await chooseMakeUp(user, next.id);
    await user.type(screen.getByRole("textbox", { name: /reason for absence/i }), "Medical appointment");
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    await screen.findByRole("heading", { name: /review your absence/i });
    expect(screen.getByText(/Second course/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^submit absence$/i }));
    await screen.findByRole("heading", { name: "Absence recorded" });
    const submission = mockApiJson.mock.calls.find(([url]) => String(url).endsWith("/staff-form-batch"));
    expect(JSON.parse(String(submission?.[1]?.body)).items).toEqual([expect.objectContaining({ sit_in_course_id: nextCourse.id, sit_in_session_ids: [next.id] })]);
  }, 30000);

  it("counts a date shared by two merged courses once in the staff summary", async () => {
    const user = userEvent.setup();
    const merge = { merge_group_id: "merged-course", merge_group_name: "Mathematics and Physics" };
    const math = PUBLIC_FORM_SESSIONS.subjects[0];
    const physics = PUBLIC_FORM_SESSIONS.subjects[1];
    installStaffRoutes({
      lookup: { ...MANUAL_EMAIL_STUDENT, subjects: [
        { id: math.subject_id, code: math.subject_code, name: math.subject_name, ...merge },
        { id: physics.subject_id, code: physics.subject_code, name: physics.subject_name, ...merge },
      ] },
      subjects: () => [
        { ...math, ...merge },
        { ...physics, ...merge, sessions: [{ ...math.sessions[0], id: "physics-on-same-day" }] },
      ],
    });
    renderWithProviders(<AbsenceForm mode="staff" />);

    await searchStaffStudent(user);
    await selectStaffClass(user);
    const summary = screen.getByRole("button", { name: /mathematics and physics.*selected/i });
    expect(summary).toHaveTextContent("1 class day selected");
  });

  it("stores staff drafts under a separate session key", async () => {
    const user = userEvent.setup();
    installStaffRoutes();
    renderWithProviders(<AbsenceForm mode="staff" />);

    await searchStaffStudent(user);
    await waitFor(() => {
      expect(window.sessionStorage.getItem("warwick.absence.staff-draft.v1")).toContain(MANUAL_EMAIL_STUDENT.wcode);
    });
    expect(window.sessionStorage.getItem("warwick.absence.draft.v1")).toBeNull();
  });
});

function withinLabels(element: HTMLElement) {
  return Array.from(element.querySelectorAll("button")).map((button) => button.getAttribute("aria-label"));
}
