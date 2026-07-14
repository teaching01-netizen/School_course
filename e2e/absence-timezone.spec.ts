import { expect, test, type Browser, type Page } from "@playwright/test";

type SubmittedPayload = {
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

type FlowResult = {
  absenceLabel: string;
  sitInReviewLabel: string;
  successSummary: string;
  submittedPayload: SubmittedPayload;
};

const boundaryMissedSession = {
  id: "missed-boundary",
  start_at: "2026-01-15T17:00:00Z",
  end_at: "2026-01-15T18:00:00Z",
  date: "2026-01-16",
  already_absent: false,
};

const boundarySitInSession = {
  id: "sit-boundary",
  course_id: "course-sitin",
  start_at: "2026-01-15T17:30:00Z",
  end_at: "2026-01-15T18:30:00Z",
  class_name: "Boundary Make-up",
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
    sms_parent_enabled: false,
    sms_parent_template: "",
    sms_success_template: "",
    sms_special_approved_template: "",
    allow_submit_without_otp: true,
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
  otp_last_sent_at: "2026-08-15T10:00:00Z",
  otp_code_expires_at: "2026-08-15T10:10:00Z",
  expires_at: "2026-08-15T11:00:00Z",
};

const verificationVerified = {
  ...verificationSend,
  status: "verified",
  verified_at: "2026-08-15T10:02:00Z",
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

  await page.route("**/api/v1/absences/student-lookup**", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(studentLookup) });
  });

  await page.route("**/api/v1/absences/sessions-in-range**", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(sessionsInRange) });
  });

  await page.route("**/api/v1/absences/parent-verification/send", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(verificationSend) });
  });

  await page.route("**/api/v1/absences/parent-verification/verification-token", async (route) => {
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
            created_at: "2026-01-15T12:00:00Z",
            updated_at: "2026-01-15T12:00:00Z",
            student_name: studentLookup.full_name,
            subject_id: "subject-math",
            subject_code: "MATH",
            subject_name: "Mathematics",
            course_id: "course-math",
            course_code: "MATH101",
            course_name: "Mathematics 101",
            date_from: "2026-01-16",
            date_to: "2026-01-16",
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

async function runAbsenceTimezoneFlow(browser: Browser, timezoneId: string): Promise<FlowResult> {
  const context = await browser.newContext({ timezoneId });
  const page = await context.newPage();
  const submitted: SubmittedPayload[] = [];

  await installAbsenceRoutes(page, submitted);
  await page.goto("/absence");

  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: /search/i }).click();
  await expect(page.getByText(studentLookup.nickname)).toBeVisible();

  await page.getByLabel(/your email address/i).fill("student@example.com");
  await page.getByRole("button", { name: "Continue to verification" }).click();
  await page.getByRole("button", { name: /send code/i }).click();
  await page.locator('input[aria-label="Verification code"]').fill("123456", { force: true });

  await expect(page.getByRole("heading", { name: "Courses & classes" })).toBeVisible();
  await page.locator("#subject-subject-math").check({ force: true });
  await expect(page.getByText("Fri, 16 Jan 2026 00:00-01:00")).toBeVisible();
  await page.locator("#session-missed-boundary").check();
  const sitInSelect = page.getByRole("combobox");
  await sitInSelect.selectOption(boundarySitInSession.id);
  await expect(sitInSelect).toContainText("16 Jan 2026 00:30-01:30");
  await page.getByLabel(/reason for absence/i).fill("Timezone regression test");
  await page.getByRole("button", { name: "Review absence" }).click();

  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
  const reviewText = await page.getByText(/Fri, 16 Jan 2026 00:00.01:00/).innerText();
  expect(reviewText).toContain("Make-up:");
  expect(reviewText).toContain("16 Jan 2026 00:30-01:30");

  await page.getByRole("button", { name: "Submit absence" }).click();
  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);

  const successSummary = await page.locator("article").first().innerText();
  await context.close();

  return {
    absenceLabel: "Fri, 16 Jan 2026 00:00-01:00",
    sitInReviewLabel: reviewText,
    successSummary,
    submittedPayload: submitted[0],
  };
}

test("absence flow keeps session dates and times in Bangkok across browser timezones", async ({ browser }) => {
  const bangkok = await runAbsenceTimezoneFlow(browser, "Asia/Bangkok");
  const losAngeles = await runAbsenceTimezoneFlow(browser, "America/Los_Angeles");

  expect(bangkok).toEqual(losAngeles);
  expect(bangkok.submittedPayload.items[0]).toMatchObject({
    date_from: "2026-01-16",
    date_to: "2026-01-16",
    missed_session_ids: ["missed-boundary"],
    sit_in_session_ids: ["sit-boundary"],
  });
  expect(bangkok.successSummary).toContain("16 Jan 2026");
  expect(bangkok.successSummary).toContain("Fri, 16 Jan 2026 00:30-01:30");
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

  await expect(page.getByText("Step 1 of 4: Student")).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(page.getByPlaceholder("e.g. W250389")).toBeFocused();
  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: /search/i }).click();
  await page.getByLabel(/your email address/i).fill("student@example.com");

  await page.reload();
  await expect(page.getByText("Step 1 of 4: Student")).toBeVisible();
  await expect(page.getByPlaceholder("e.g. W250389")).toHaveValue(studentLookup.wcode);
  await expect(page.getByLabel(/your email address/i)).toHaveValue("student@example.com");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  await page.setViewportSize({ width: 320, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

  await page.getByRole("button", { name: "Continue to verification" }).click();
  await expect(page.getByText("Step 2 of 4: Verify")).toBeVisible();
  await page.getByRole("button", { name: /send code/i }).click();
  await page.locator('input[aria-label="Verification code"]').fill("123456", { force: true });
  await expect(page.getByText("Step 3 of 4: Classes")).toBeVisible();
  await page.locator("#subject-subject-math").check({ force: true });
  await expect(page.getByRole("button", { name: /mathematics.*open/i })).toHaveAttribute("aria-expanded", "true");
  await page.locator("#session-missed-boundary").check();
  await page.getByRole("combobox").selectOption(boundarySitInSession.id);
  await page.getByLabel(/reason for absence/i).fill("Mobile resume test");
  await page.getByRole("button", { name: "Review absence" }).click();
  await expect(page.getByText("Step 4 of 4: Review")).toBeVisible();
  await page.getByRole("button", { name: "Submit absence" }).click();
  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  await context.close();
});
