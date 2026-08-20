import { expect, test } from "@playwright/test";
import {
  boundarySitInSession,
  installAbsenceRoutes,
  studentLookup,
  type SubmittedPayload,
} from "./fixtures/absence";
import { selectAbsenceCheckbox } from "./helpers/absenceFlow";
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
  await page.getByRole("button", { name: /search/i }).click();
  await page.getByLabel(/your email address/i).fill("student@example.com");
  await page.getByRole("button", { name: "Continue to verification" }).click();
  await page.getByRole("button", { name: /send code/i }).click();

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

  await page.goto("/absence");
  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: /search/i }).click();
  await page.getByLabel(/your email address/i).fill("student@example.com");
  await page.getByRole("button", { name: "Continue to verification" }).click();
  await page.getByRole("button", { name: /send code/i }).click();
  await page.locator('input[aria-label="Verification code"]').fill("123456", { force: true });

  await selectAbsenceCheckbox(page, "subject-subject-math");
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  const makeUpSelect = page.getByRole("combobox");
  if (await makeUpSelect.isVisible()) {
    await makeUpSelect.selectOption(boundarySitInSession.id);
  } else {
    await page.getByRole("button", { name: /choose a make-up class/i }).click();
    const makeUpDialog = page.getByRole("dialog", { name: /choose a make-up class/i });
    await makeUpDialog.getByRole("radio").first().check();
    await makeUpDialog.getByRole("button", { name: "Confirm make-up class" }).click();
  }
  await expect(page.locator("#sit-in-missed-boundary")).toHaveValue(boundarySitInSession.id);
  await page.getByLabel(/reason for absence/i).fill("Medical appointment");
  await page.getByRole("button", { name: "Review absence" }).click();
  await page.getByRole("button", { name: "Submit absence" }).click();

  await expect(
    page.getByRole("alert").filter({ hasText: /maximum absences allowed/i }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
  await expect(page.getByText("Medical appointment")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Absence submitted" })).toHaveCount(0);
  expect(submitted).toHaveLength(1);
});
test("duplicate submit taps create one logical absence request", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await page.route("**/api/v1/absences/batch", async (route) => {
    await page.waitForTimeout(150);
    await route.fallback();
  });

  await page.goto("/absence");
  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: /search/i }).click();
  await page.getByLabel(/your email address/i).fill("student@example.com");
  await page.getByRole("button", { name: "Continue to verification" }).click();
  await page.getByRole("button", { name: /send code/i }).click();
  await page.locator('input[aria-label="Verification code"]').fill("123456", { force: true });

  await selectAbsenceCheckbox(page, "subject-subject-math");
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  const makeUpSelect = page.getByRole("combobox");
  if (await makeUpSelect.isVisible()) {
    await makeUpSelect.selectOption(boundarySitInSession.id);
  } else {
    await page.getByRole("button", { name: /choose a make-up class/i }).click();
    const makeUpDialog = page.getByRole("dialog", { name: /choose a make-up class/i });
    await makeUpDialog.getByRole("radio").first().check();
    await makeUpDialog.getByRole("button", { name: "Confirm make-up class" }).click();
  }
  await page.getByLabel(/reason for absence/i).fill("Double tap regression");
  await page.getByRole("button", { name: "Review absence" }).click();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();

  await page.getByRole("button", { name: "Submit absence" }).dblclick();

  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);
});
test("missing make-up selection focuses the visible control", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);

  await page.goto("/absence");
  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: /search/i }).click();
  await page.getByLabel(/your email address/i).fill("student@example.com");
  await page.getByRole("button", { name: "Continue to verification" }).click();
  await page.getByRole("button", { name: /send code/i }).click();
  await page.locator('input[aria-label="Verification code"]').fill("123456", { force: true });

  await selectAbsenceCheckbox(page, "subject-subject-math");
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  await page.getByRole("button", { name: "Review absence" }).click();

  await expect(page.getByRole("alert").filter({ hasText: /pick a make-up class/i })).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const active = document.activeElement;
        if (!(active instanceof HTMLElement)) return false;
        const styles = getComputedStyle(active);
        return (
          active.matches('[data-make-up-trigger], select[aria-label*="make-up" i], select') &&
          styles.display !== "none" &&
          styles.visibility !== "hidden" &&
          active.getClientRects().length > 0
        );
      }),
    )
    .toBe(true);
});
