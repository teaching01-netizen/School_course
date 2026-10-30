import { expect, type Page } from "@playwright/test";
import {
  boundarySitInSession,
  studentLookup,
} from "../fixtures/absence";

export async function selectAbsenceCheckbox(page: Page, id: string) {
  const checkbox = page.locator(`[id="${id}"]`);
  if (await checkbox.isChecked()) return;

  const ancestorLabel = checkbox.locator("xpath=ancestor::label[1]");
  if (await ancestorLabel.count()) {
    await ancestorLabel.click();
  } else {
    await page.locator(`label[for="${id}"]`).click();
  }
  await expect(checkbox).toBeChecked();
}

/** The public picker is a trigger plus sheet at every width. */
export async function chooseMakeUpClass(page: Page) {
  await page.getByRole("button", { name: /choose a make-up class/i }).click();
  const dialog = page.getByRole("dialog", { name: /choose a make-up class/i });
  await dialog.locator('input[type="radio"]:not([value=""]):not(:disabled)').first().check();
  await dialog.getByRole("button", { name: "Confirm make-up class" }).click();
  await expectMakeUpValue(page, boundarySitInSession.id);
}

/** Reads the committed choice: the sheet opens with it checked, then cancels unchanged. */
export async function expectMakeUpValue(page: Page, value: string, id = "sit-in-missed-boundary") {
  const trigger = page.locator(`[id="${id}"]`);
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: /choose a make-up class/i });
  const checked = dialog.locator('fieldset input[type="radio"]:checked');
  if (value) await expect(checked).toHaveValue(value);
  else await expect(checked).toHaveCount(0);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(trigger).toBeFocused();
}

export async function completeToClasses(page: Page, email = "student@example.com") {
  await page.goto("/absence");
  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: "Find student" }).click();
  await page.getByLabel(/email address/i).fill(email);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.getByRole("button", { name: /send code/i }).click();
  await page.locator('input[aria-label="Verification code"]').fill("123456", { force: true });
  await expect(page.getByRole("heading", { name: "Courses & classes" })).toBeVisible();
}

export async function completeToReview(page: Page, reason = "Medical appointment", email = "student@example.com") {
  await completeToClasses(page, email);
  await selectAbsenceCheckbox(page, "subject-subject:subject-math");
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  await chooseMakeUpClass(page);
  await page.getByLabel(/reason for absence/i).fill(reason);
  await page.getByRole("button", { name: "Review absence" }).click();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
}
