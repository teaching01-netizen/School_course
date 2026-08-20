import { expect, type Page } from "@playwright/test";
import {
  boundarySitInSession,
  studentLookup,
} from "../fixtures/absence";

export async function selectAbsenceCheckbox(page: Page, id: string) {
  const checkbox = page.locator(`#${id}`);
  if (await checkbox.isChecked()) return;

  const ancestorLabel = checkbox.locator("xpath=ancestor::label[1]");
  if (await ancestorLabel.count()) {
    await ancestorLabel.click();
  } else {
    await page.locator(`label[for="${id}"]`).click();
  }
  await expect(checkbox).toBeChecked();
}

export async function chooseMakeUpClass(page: Page) {
  const select = page.getByRole("combobox");
  if (await select.isVisible()) {
    await select.selectOption(boundarySitInSession.id);
  } else {
    await page.getByRole("button", { name: /choose a make-up class/i }).click();
    const dialog = page.getByRole("dialog", { name: /choose a make-up class/i });
    await dialog.getByRole("radio").first().check();
    await dialog.getByRole("button", { name: "Confirm make-up class" }).click();
  }
  await expect(page.locator("#sit-in-missed-boundary")).toHaveValue(boundarySitInSession.id);
}

export async function completeToClasses(page: Page, email = "student@example.com") {
  await page.goto("/absence");
  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: /search/i }).click();
  await page.getByLabel(/your email address/i).fill(email);
  await page.getByRole("button", { name: "Continue to verification" }).click();
  await page.getByRole("button", { name: /send code/i }).click();
  await page.locator('input[aria-label="Verification code"]').fill("123456", { force: true });
  await expect(page.getByRole("heading", { name: "Courses & classes" })).toBeVisible();
}

export async function completeToReview(page: Page, reason = "Medical appointment", email = "student@example.com") {
  await completeToClasses(page, email);
  await selectAbsenceCheckbox(page, "subject-subject-math");
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  await chooseMakeUpClass(page);
  await page.getByLabel(/reason for absence/i).fill(reason);
  await page.getByRole("button", { name: "Review absence" }).click();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
}
