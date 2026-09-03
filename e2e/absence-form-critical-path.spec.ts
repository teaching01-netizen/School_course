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
  const continueButton = page.getByRole("button", { name: "Continue" });
  await expect(continueButton).toBeDisabled();
  await continueButton.click({ force: true });

  await expect(page.getByRole("heading", { name: "Report an absence" })).toBeVisible();
  expect(lookupRequests).toBe(0);
});

test("editing from review preserves the selected class and reason", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await completeToReview(page, "Original review reason");

  await page.getByRole("button", { name: "Edit reason" }).click();
  await expect(page.getByRole("heading", { name: "Why will you be away?" })).toBeVisible();
  await expect(page.locator("#absence-reason")).toHaveValue("Original review reason");
  await expect(page.getByRole("radio", { name: "Other" })).toHaveAttribute("aria-checked", "true");

  await page.locator("#absence-reason").fill("Updated review reason");
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByText("Updated review reason")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
});