import type { Page } from "@playwright/test";

// Asia/Bangkok is the institute zone the app renders in, so fixture dates are
// computed there and stay valid however far in the future the suite runs.
const INSTITUTE_OFFSET_MS = 7 * 60 * 60_000;

/** Institute (Bangkok) calendar date N days from today, as YYYY-MM-DD. */
export function instituteDate(dayOffset: number): string {
  return new Date(Date.now() + INSTITUTE_OFFSET_MS + dayOffset * 86_400_000).toISOString().slice(0, 10);
}

/** ISO instant for a wall-clock time on an institute date. */
export function instituteTime(dateKey: string, hour: number, minute = 0): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return new Date(`${dateKey}T${pad(hour)}:${pad(minute)}:00+07:00`).toISOString();
}

export type SubmittedPayload = {
  wcode: string;
  email?: string;
  reason: string;
  items: Array<{
    subject_id: string;
    course_id: string;
    date_from: string;
    date_to: string;
    reason?: string;
    sit_in_method?: string;
    sit_in_course_id?: string;
    missed_session_ids: string[];
    sit_in_session_ids: string[];
  }>;
};

export const studentLookup = {
  student_id: "student-1",
  wcode: "W250389",
  full_name: "John Smith",
  nickname: "Johnny",
  school: "Bangkok Prep",
  parent_phone: "+66812345678",
  subjects: [{ id: "subject-math", code: "MATH", name: "Mathematics" }],
};

/** The one reportable class (tomorrow, institute time) the fixture exposes. */
export const fixtureMissedDate = instituteDate(1);

export const boundarySitInSession = {
  id: "sit-boundary",
  course_id: "course-sitin",
  start_at: instituteTime(fixtureMissedDate, 0, 30),
  end_at: instituteTime(fixtureMissedDate, 1, 30),
  class_name: "Boundary Make-up",
};

export const boundaryMissedSession = {
  id: "missed-boundary",
  start_at: instituteTime(fixtureMissedDate, 0, 0),
  end_at: instituteTime(fixtureMissedDate, 1, 0),
  date: fixtureMissedDate,
  already_absent: false,
};

const config = {
  form: {
    max_date_range_days: 30,
    require_reason: false,
    reason_categories: [],
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
    sms_parent_template: "",
    sms_success_template: "",
    sms_special_approved_template: "",
  },
  admin_contact: {
    email: "office@example.edu",
    phone: "+66 2123 4567",
    hours: "Mon-Fri 08:00-16:00",
  },
};

const publicLookup = {
  wcode: studentLookup.wcode,
  lookup_token: "lookup-token",
  email_input_required: true,
  parent_verification_available: true,
};

const studentProfile = {
  wcode: studentLookup.wcode,
  display_name: "John",
  email_on_file: true,
  subjects: studentLookup.subjects,
};

const sessionsInRange = {
  subjects: [
    {
      subject_id: "subject-math",
      subject_code: "MATH",
      subject_name: "Mathematics",
      course_id: "course-math",
      course_code: "MATH101",
      course_name: "Mathematics 101",
      absence_rate_exceeded: false,
      sessions: [boundaryMissedSession],
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: {
          id: "course-sitin",
          code: "MATH201",
          name: "Mathematics Make-up",
          subject_code: "MATH",
          subject_name: "Mathematics",
        },
        available_sessions: [boundarySitInSession],
      },
    },
  ],
};

const verification = {
  token: "verification-token",
  status: "pending",
  wcode: studentLookup.wcode,
  parent_phone: studentLookup.parent_phone,
  delivery_status: "accepted",
  otp_last_sent_at: new Date(Date.now() - 60_000).toISOString(),
  otp_code_expires_at: new Date(Date.now() + 10 * 60_000).toISOString(),
  expires_at: new Date(Date.now() + 60 * 60_000).toISOString(),
};

export async function installAbsenceRoutes(page: Page, submitted: SubmittedPayload[]) {
  await page.route("**/api/v1/me", (route) =>
    route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ code: "unauthorized", message: "Not authenticated" }),
    }),
  );
  await page.route("**/api/v1/absence-form-config", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify(config) }),
  );
  await page.route("**/api/v1/absence-self-service/lookup", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify(publicLookup) }),
  );
  await page.route("**/api/v1/absence-self-service/me", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify(studentProfile) }),
  );
  await page.route("**/api/v1/absence-self-service/sessions**", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify(sessionsInRange) }),
  );
  await page.route("**/api/v1/absences/parent-verification/send", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify(verification) }),
  );
  await page.route("**/api/v1/absences/parent-verification/status", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify(verification) }),
  );
  await page.route("**/api/v1/absences/parent-verification/verify", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        ...verification,
        status: "verified",
        verified_at: new Date().toISOString(),
      }),
    }),
  );
  await page.route("**/api/v1/absences/batch", async (route) => {
    submitted.push(route.request().postDataJSON() as SubmittedPayload);
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          {
            id: "absence-accessibility",
            wcode: studentLookup.wcode,
            status: "pending",
            version: 1,
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
            student_name: studentLookup.full_name,
            subject_id: "subject-math",
            subject_code: "MATH",
            subject_name: "Mathematics",
            course_id: "course-math",
            course_code: "MATH101",
            course_name: "Mathematics 101",
            date_from: fixtureMissedDate,
            date_to: fixtureMissedDate,
            reason: "Accessibility regression test",
            sit_in_method: "physical",
            sit_in_course_id: "course-sitin",
            sit_in_course_code: "MATH201",
            sit_in_course_name: "Mathematics Make-up",
            sit_in_subject_name: "Mathematics",
            missed_sessions: [
              {
                id: "missed-record",
                session_id: boundaryMissedSession.id,
                course_id: "course-math",
                course_code: "MATH101",
                course_name: "Mathematics 101",
                subject_name: "Mathematics",
                start_at: boundaryMissedSession.start_at,
                end_at: boundaryMissedSession.end_at,
              },
            ],
            sit_ins: [
              {
                id: "sit-in-record",
                session_id: boundarySitInSession.id,
                course_id: "course-sitin",
                course_code: "MATH201",
                course_name: "Mathematics Make-up",
                subject_name: "Mathematics",
                start_at: boundarySitInSession.start_at,
                end_at: boundarySitInSession.end_at,
              },
            ],
          },
        ],
      }),
    });
  });
}
