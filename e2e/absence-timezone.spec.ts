import { expect, test, type Browser, type Page } from "@playwright/test";
import { acceptResumePrompt, advanceThroughVerification } from "./helpers/absenceFlow";
import {
  boundaryMissedSession,
  boundarySitInSession,
  fixtureMissedDate,
  type SubmittedPayload,
} from "./fixtures/absence";

type FlowResult = {
  reviewText: string;
  successSummary: string;
  submittedPayload: SubmittedPayload;
};

const absenceFormConfig = {
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

const studentLookup = {
  student_id: "student-1",
  wcode: "W250389",
  full_name: "John Smith",
  nickname: "Johnny",
  school: "Bangkok Prep",
  parent_phone: "+66812345678",
  subjects: [{ id: "subject-math", code: "MATH", name: "Mathematics" }],
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
  email_on_file: false,
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

const verificationSend = {
  token: "verification-token",
  status: "pending",
  wcode: studentLookup.wcode,
  parent_phone: studentLookup.parent_phone,
  delivery_status: "accepted",
  otp_last_sent_at: new Date(Date.now() - 60_000).toISOString(),
  otp_code_expires_at: new Date(Date.now() + 10 * 60_000).toISOString(),
  expires_at: new Date(Date.now() + 60 * 60_000).toISOString(),
};

const verificationVerified = {
  ...verificationSend,
  status: "verified",
  verified_at: new Date().toISOString(),
};

async function installAbsenceRoutes(page: Page, submitted: SubmittedPayload[]) {
  await page.route("**/api/v1/me", async (route) => {
    await route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ code: "unauthorized", message: "Not authenticated" }),
    });
  });

  await page.route("**/api/v1/absence-form-config", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(absenceFormConfig) });
  });
  await page.route("**/api/v1/absence-self-service/lookup", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(publicLookup) });
  });

  await page.route("**/api/v1/absence-self-service/me", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(studentProfile) });
  });

  await page.route("**/api/v1/absence-self-service/sessions**", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(sessionsInRange) });
  });

  await page.route("**/api/v1/absences/parent-verification/send", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(verificationSend) });
  });

  await page.route("**/api/v1/absences/parent-verification/status", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(verificationSend) });
  });

  await page.route("**/api/v1/absences/parent-verification/verify", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(verificationVerified) });
  });

  await page.route("**/api/v1/absences/batch", async (route) => {
    const payload = route.request().postDataJSON() as SubmittedPayload;
    submitted.push(payload);
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          {
            id: "absence-boundary",
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
            reason: "Timezone regression test",
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

async function verifyParent(page: Page) {
  await page.getByRole("button", { name: /^(send code|resend code)$/i }).click();
  await page.locator('input[aria-label="Confirmation code"]').fill("123456", { force: true });
  await expect(page.getByRole("heading", { name: "Where should we send updates?" })).toBeVisible();
  await page.getByLabel(/^email$/i).fill("student@example.com");
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Which class will you miss?" })).toBeVisible();
}

async function runAbsenceTimezoneFlow(browser: Browser, timezoneId: string): Promise<FlowResult> {
  const context = await browser.newContext({ timezoneId });
  const page = await context.newPage();
  const submitted: SubmittedPayload[] = [];

  await installAbsenceRoutes(page, submitted);
  await page.goto("/absence");

  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Is this you?" })).toBeVisible();
  await page.getByRole("button", { name: "Yes, continue" }).click();
  await verifyParent(page);

  // The calendar opens focused on tomorrow (the fixture's only reportable
  // class); its agenda row is right there. Only the institute-fixed time
  // labels are asserted literally — dates depend on when the suite runs.
  const classRow = page
    .locator("label")
    .filter({ hasText: "Mathematics" })
    .filter({ has: page.locator('input[type="checkbox"]') })
    .first();
  await expect(classRow).toContainText("00:00–01:00");
  await classRow.click();
  await expect(page.getByRole("button", { name: /add another class/i })).toBeVisible();
  await page.getByRole("button", { name: "Continue" }).click();

  await expect(page.getByRole("heading", { name: "Your make-up" })).toBeVisible();
  await expect(page.locator("main")).toContainText("00:00–01:00");
  await expect(page.locator("main")).toContainText("00:30–01:30");
  await page.getByRole("button", { name: /^(Use this class|Continue with this make-up)$/i }).click();

  await expect(page.getByRole("heading", { name: "Why will you be away?" })).toBeVisible();
  await page.getByRole("radio", { name: "Other" }).click();
  await page.getByLabel(/tell us a little more/i).fill("Timezone regression test");
  await page.getByRole("button", { name: "Continue" }).click();

  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
  const reviewText = await page.locator("main").innerText();
  expect(reviewText).toContain("00:00–01:00");
  expect(reviewText).toContain("Make-up:");
  expect(reviewText).toContain("00:30-01:30");

  await page.getByRole("button", { name: "Submit absence" }).click();
  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);

  const successSummary = await page.locator("main").innerText();
  await context.close();

  return {
    reviewText,
    successSummary,
    submittedPayload: submitted[0],
  };
}

test("absence flow renders institute dates and times identically in every browser timezone", async ({ browser }) => {
  const bangkok = await runAbsenceTimezoneFlow(browser, "Asia/Bangkok");
  const losAngeles = await runAbsenceTimezoneFlow(browser, "America/Los_Angeles");

  // The app renders on the institute calendar, so a Bangkok browser and a Los
  // Angeles browser must produce byte-identical copy, review and summary.
  expect(bangkok).toEqual(losAngeles);
  expect(bangkok.submittedPayload.items).toHaveLength(1);
  expect(bangkok.submittedPayload.items[0]).toMatchObject({
    date_from: fixtureMissedDate,
    date_to: fixtureMissedDate,
    missed_session_ids: [boundaryMissedSession.id],
    sit_in_session_ids: [boundarySitInSession.id],
  });
  expect(bangkok.submittedPayload.email).toBe("student@example.com");
  expect(bangkok.submittedPayload.reason).toBe("Other: Timezone regression test");
});

test("mobile public absence flow resumes student details safely and completes without horizontal overflow", async ({ browser }) => {
  const context = await browser.newContext({
    timezoneId: "Asia/Bangkok",
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 2,
  });
  const page = await context.newPage();
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await page.goto("/absence");

  await expect(page.getByPlaceholder("e.g. W250389")).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(page.getByPlaceholder("e.g. W250389")).toBeFocused();
  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("button", { name: "Yes, continue" }).click();
  await page.getByRole("button", { name: /^(send code|resend code)$/i }).click();
  await page.locator('input[aria-label="Confirmation code"]').fill("123456", { force: true });
  await expect(page.getByRole("heading", { name: "Where should we send updates?" })).toBeVisible();
  await page.getByLabel(/^email$/i).fill("student@example.com");

  await page.reload();
  // The resume prompt explains what is saved, then re-identifies the student.
  await acceptResumePrompt(page);
  await expect(page.getByRole("heading", { name: "Is this you?" })).toBeVisible();
  await expect(page.getByText("W250389")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  await page.setViewportSize({ width: 320, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

  await page.getByRole("button", { name: "Yes, continue" }).click();
  await advanceThroughVerification(page);
  // The restored email means the email screen is skipped.
  await expect(page.getByRole("heading", { name: "Which class will you miss?" })).toBeVisible();

  const classRow = page
    .locator("label")
    .filter({ hasText: "Mathematics" })
    .filter({ has: page.locator('input[type="checkbox"]') })
    .first();
  await classRow.click();
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("button", { name: /^(Use this class|Continue with this make-up)$/i }).click();
  await page.getByRole("radio", { name: "Other" }).click();
  await page.getByLabel(/tell us a little more/i).fill("Mobile resume test");
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
  await page.getByRole("button", { name: "Submit absence" }).click();
  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);
  expect(submitted[0]?.email).toBe("student@example.com");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  await context.close();
});
