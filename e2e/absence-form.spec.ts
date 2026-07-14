import { expect, test } from "@playwright/test";
import {
  boundarySitInSession,
  installAbsenceRoutes,
  studentLookup,
  type SubmittedPayload,
} from "./fixtures/absence";

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

  await expect(page.getByRole("heading", { name: "Courses & classes" })).toBeVisible();
  await page.locator("#subject-subject-math").check({ force: true });
  await page.locator("#session-missed-boundary").check();
  await page.getByRole("combobox").selectOption(boundarySitInSession.id);
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
