import { expect, test } from "@playwright/test";
import {
  installAbsenceRoutes,
  studentLookup,
  type SubmittedPayload,
} from "./fixtures/absence";
import { acceptResumePrompt, completeToReview } from "./helpers/absenceFlow";
test.setTimeout(120_000);

test("keeps verification capabilities out of browser URLs", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  const verificationRequestURLs: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/api/v1/absences/parent-verification/")) {
      verificationRequestURLs.push(request.url());
    }
  });
  await installAbsenceRoutes(page, submitted);

  await page.goto("/absence");
  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("button", { name: "Yes, continue" }).click();
  await page.getByRole("button", { name: /^(send code|resend code)$/i }).click();

  await expect(page).toHaveURL(/\/absence$/);
  expect(new URL(page.url()).search).toBe("");
  for (const requestURL of verificationRequestURLs) {
    const parsed = new URL(requestURL);
    expect(parsed.searchParams.has("token")).toBe(false);
    expect(parsed.pathname).toMatch(/\/parent-verification\/(?:send|status|verify)$/);
  }
});

test("quota rejection keeps the reviewed absence available for correction", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await page.route("**/api/v1/absences/batch", async (route) => {
    submitted.push(route.request().postDataJSON() as SubmittedPayload);
    await route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({
        code: "absence_limit_exceeded",
        message: "Absence limit exceeded",
      }),
    });
  });

  await completeToReview(page, "Quota recovery");

  await page.getByRole("button", { name: "Submit absence" }).click();

  // The submission rejection is announced as an alert on the intact Review screen.
  await expect(page.getByRole("alert").filter({ hasText: /reached the absence limit/i })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
  await expect(page.getByText("Quota recovery")).toBeVisible();
  expect(submitted).toHaveLength(1);
});

test("duplicate submit taps create one logical absence request", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await page.route("**/api/v1/absences/batch", async (route) => {
    await page.waitForTimeout(150);
    await route.fallback();
  });

  await completeToReview(page, "Double tap regression");

  await page.getByRole("button", { name: "Submit absence" }).dblclick();

  await expect(page.getByRole("heading", { name: "Absence report submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);
});

test("a restored draft with a missing make-up never reaches review", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await page.addInitScript((draft) => {
    window.sessionStorage.setItem("warwick.absence.draft.v1", JSON.stringify(draft));
  }, {
    schemaVersion: 1,
    updatedAt: Date.now(),
    wcode: studentLookup.wcode,
    collectedEmail: "student@example.com",
    step: 2,
    selectedSubjectIds: ["subject-math"],
    selectedSessionIds: ["missed-boundary"],
    sitInSelections: {},
    sitInPriorityLevels: {},
    reason: "Saved reason",
  });
  await installAbsenceRoutes(page, submitted);

  await page.goto("/absence");
  // A saved draft is never surfaced before identity + parent verification.
  await expect(page.getByRole("heading", { name: "Is this you?" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Continue your absence report?" })).toHaveCount(0);
  await page.getByRole("button", { name: "Yes, continue" }).click();
  await page.getByRole("button", { name: /^(send code|resend code)$/i }).click();
  await page.locator('input[aria-label="Confirmation code"]').fill("123456", { force: true });
  // The saved report is offered for resume only after verification.
  await acceptResumePrompt(page);
  await expect(page.getByRole("heading", { name: "Review your absence" })).toHaveCount(0);

  // The restored selection still demands a make-up choice before review.
  await page.getByRole("button", { name: /i've reviewed/i }).click();
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Your make-up" })).toBeVisible();
  await expect(page.getByRole("button", { name: /choose a time|change time/i })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toHaveCount(0);
});