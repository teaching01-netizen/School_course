import { expect, test } from "@playwright/test";
import { installAbsenceRoutes, type SubmittedPayload } from "./fixtures/absence";
import { completeToReview } from "./helpers/absenceFlow";

test("successful physical make-up submission shows one receipt", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await completeToReview(page, "Medical appointment");

  await expect(page.getByText("Medical appointment")).toBeVisible();
  await page.getByRole("button", { name: "Submit absence" }).click();

  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);
});

test("student without an email can complete the flow with a valid manual email", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await completeToReview(page, "Manual email regression", "student@example.edu");

  await page.getByRole("button", { name: "Submit absence" }).click();
  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);
  expect(submitted[0]?.email).toBe("student@example.edu");
});

test("invalid student ID never calls the lookup API or advances", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  let lookupRequests = 0;
  await installAbsenceRoutes(page, submitted);
  await page.route("**/api/v1/absence-self-service/lookup", (route) => {
    lookupRequests += 1;
    return route.fallback();
  });

  await page.goto("/absence");
  await page.getByPlaceholder("e.g. W250389").fill("not-a-wcode");
  await page.getByRole("button", { name: /search/i }).click();
  await expect(page.getByRole("alert").filter({ hasText: /student id/i })).toBeVisible();

  await expect(page.getByRole("heading", { name: "Find your profile" })).toBeVisible();
  expect(lookupRequests).toBe(0);
});

test("editing from review preserves the selected class and reason", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await completeToReview(page, "Original review reason");

  await page.getByRole("button", { name: "Edit reason" }).click();
  await expect(page.locator("#absence-reason")).toHaveValue("Original review reason");
  await expect(page.locator("#subject-subject-math")).toBeChecked();
  await expect(page.locator("#session-missed-boundary")).toBeChecked();

  await page.getByLabel(/reason for absence/i).fill("Updated review reason");
  await page.getByRole("button", { name: "Review absence" }).click();
  await expect(page.getByText("Updated review reason")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
});
