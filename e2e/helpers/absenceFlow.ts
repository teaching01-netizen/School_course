import { expect, type Page } from "@playwright/test";
import { studentLookup } from "../fixtures/absence";

export async function selectClass(page: Page, label: string) {
  // The fixture schedules its single class on the default focused calendar
  // day, so the agenda row is visible without navigating dates.
  const row = page
    .locator("label")
    .filter({ hasText: label })
    .filter({ has: page.locator('input[type="checkbox"]') })
    .first();
  await row.click();
  // The choice stays visible in the persistent selection summary.
  await expect(page.getByRole("button", { name: /add another class/i })).toBeVisible();
}

/** Accepts the "Continue your absence report?" prompt when a saved draft exists. */
export async function acceptResumePrompt(page: Page) {
  const resumeHeading = page.getByRole("heading", { name: "Continue your absence report?" });
  if (await resumeHeading.waitFor({ state: "visible", timeout: 3000 }).then(() => true).catch(() => false)) {
    await page.getByRole("button", { name: "Continue", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Is this you?" })).toBeVisible();
  }
}

/** Passes parent confirmation whether it is a fresh OTP or a restored session. */
export async function advanceThroughVerification(page: Page) {
  const confirmed = page.getByRole("heading", { name: "Confirmed" });
  const sendCode = page.getByRole("button", { name: /^(send code|resend code)$/i });
  await Promise.race([
    confirmed.waitFor({ state: "visible", timeout: 5000 }),
    sendCode.waitFor({ state: "visible", timeout: 5000 }),
  ]).catch(() => {});
  if (await confirmed.isVisible().catch(() => false)) {
    // A still-valid verified session was restored automatically.
    await page.getByRole("button", { name: "Continue" }).click();
    return;
  }
  await sendCode.click();
  await page.locator('input[aria-label="Confirmation code"]').fill("123456", { force: true });
}

export async function completeToClasses(page: Page, email = "student@example.com") {
  await page.goto("/absence");
  await acceptResumePrompt(page);
  const input = page.getByPlaceholder("e.g. W250389");
  const confirmHeading = page.getByRole("heading", { name: "Is this you?" });
  // The identify screen mounts after the form chunk + config load; a resumed
  // draft may land directly on the confirmation screen. Wait for whichever
  // arrives instead of probing the DOM once (which races the mount).
  await Promise.race([
    input.waitFor({ state: "visible", timeout: 10_000 }),
    confirmHeading.waitFor({ state: "visible", timeout: 10_000 }),
  ]).catch(() => {});
  if (await input.isVisible().catch(() => false)) {
    await input.fill(studentLookup.wcode);
    await page.getByRole("button", { name: "Continue" }).click();
  }
  await expect(confirmHeading).toBeVisible();
  await page.getByRole("button", { name: "Yes, continue" }).click();
  // The verify screen either asks for a fresh code or, when a still-valid
  // verified session was restored from storage, already shows "Confirmed".
  // advanceThroughVerification handles both paths.
  await advanceThroughVerification(page);

  // The email screen only appears when the school has no address on file.
  const emailHeading = page.getByRole("heading", { name: "Where should we send updates?" });
  if (await emailHeading.waitFor({ state: "visible", timeout: 3000 }).then(() => true).catch(() => false)) {
    await page.getByLabel(/^email$/i).fill(email);
    await page.getByRole("button", { name: "Continue" }).click();
  }
  await expect(page.getByRole("heading", { name: "Which class will you miss?" })).toBeVisible();
}

export async function completeMakeUp(page: Page) {
  await expect(page.getByRole("heading", { name: "Your make-up" })).toBeVisible();
  await page.getByRole("button", { name: "Use this class" }).click();
  await expect(page.getByRole("heading", { name: "Why will you be away?" })).toBeVisible();
}

export async function completeToReview(
  page: Page,
  reason = "Medical appointment",
  email = "student@example.com",
) {
  await completeToClasses(page, email);
  await selectClass(page, "Mathematics");
  await page.getByRole("button", { name: "Continue" }).click();
  await completeMakeUp(page);
  if (reason) {
    await page.getByRole("radio", { name: "Other" }).click();
    await page.getByLabel(/tell us a little more/i).fill(reason);
  }
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
}