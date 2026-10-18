import { expect, test } from "@playwright/test";
import { installAbsenceRoutes, publicLookup, studentLookup } from "./fixtures/absence";
import { expectAccessiblePage, expectNoHorizontalOverflow } from "./helpers/absenceAssertions";

test.use({ timezoneId: "Asia/Bangkok" });
test.beforeEach(async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
});

test("recognizes the student without email housekeeping or a second lookup action", async ({ page }, testInfo) => {
  await installAbsenceRoutes(page, []);
  await page.route("**/api/v1/absence-self-service/lookup", (route) => route.fulfill({ json: {
    ...publicLookup, nickname: "Bird", email_input_required: false,
  } }));
  await page.goto("/absence");
  const footer = page.getByRole("contentinfo");
  await expect(footer.getByRole("button")).toHaveCount(1);
  await expect(footer.getByRole("button", { name: "Find student" })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("student-search.png") });
  await page.getByLabel("Student ID (W-Code)").fill(studentLookup.wcode);
  await page.getByLabel("Student ID (W-Code)").press("Enter");

  const nickname = page.getByRole("heading", { name: "Bird", exact: true });
  await expect(nickname).toBeFocused();
  await expect(nickname).toHaveAccessibleDescription(`Student ID: ${studentLookup.wcode}`);
  await expect(page.getByText(studentLookup.wcode, { exact: true })).toHaveCount(1);
  await expect(page.getByRole("textbox")).toHaveCount(0);
  await expect(page.getByText(/email|Student ID found|your parent/i)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Find student", exact: true })).toHaveCount(0);
  await expect(footer.getByRole("button", { name: "Continue", exact: true })).toBeEnabled();
  // Short landscape layouts collapse the visual hint, but Continue still describes the next step.
  await expect(footer.getByRole("button", { name: "Continue", exact: true })).toHaveAccessibleDescription(/phone ending in 5678/);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.screenshot({ path: testInfo.outputPath("student-recognition.png") });

  await footer.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Parent verification" })).toBeVisible();
  await expect(page.getByText("Bird", { exact: true })).toBeVisible();
});

test("lets a parent change students and confirms the newly found identity", async ({ page }) => {
  await installAbsenceRoutes(page, []);
  await page.route("**/api/v1/absence-self-service/lookup", (route) => {
    const wcode = (route.request().postDataJSON() as { wcode: string }).wcode;
    return route.fulfill({ json: {
      ...publicLookup,
      wcode,
      nickname: wcode === studentLookup.wcode ? "Bird" : "Beam",
      parent_phone_hint: wcode === studentLookup.wcode ? "••••5678" : "••••9999",
      email_input_required: false,
    } });
  });
  await page.goto("/absence");
  await page.getByLabel("Student ID (W-Code)").fill(studentLookup.wcode);
  await page.getByRole("button", { name: "Find student" }).click();
  await expect(page.getByRole("heading", { name: "Bird", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Change student" }).click();

  const input = page.getByLabel("Student ID (W-Code)");
  await expect(input).toBeFocused();
  await expect(input).toHaveValue(studentLookup.wcode);
  await expect(page.getByRole("button", { name: "Continue", exact: true })).toHaveCount(0);
  await input.fill("w250400");
  await input.press("Enter");

  await expect(page.getByRole("heading", { name: "Beam", exact: true })).toBeFocused();
  await expect(page.getByText("W250400", { exact: true })).toBeVisible();
  await expect(page.getByText("Bird", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Continue", exact: true })).toHaveAccessibleDescription(/phone ending in 9999/);
  await expect(page.getByRole("button", { name: "Continue", exact: true })).toBeEnabled();
});

test("keeps the ID available for correction after a student is not found", async ({ page }) => {
  await installAbsenceRoutes(page, []);
  await page.route("**/api/v1/absence-self-service/lookup", (route) => route.fulfill({
    status: 404, json: { code: "student_not_found", message: "Student ID was not found. Check the W-Code and try again." },
  }));
  await page.goto("/absence");
  const input = page.getByLabel("Student ID (W-Code)");
  await input.fill("W999999");
  await page.getByRole("button", { name: "Find student" }).click();

  await expect(input).toHaveValue("W999999");
  await expect(input).toHaveAttribute("aria-invalid", "true");
  await expect(input).toHaveAccessibleDescription("Student ID was not found. Check the W-Code and try again.");
  await expect(page.getByRole("group", { name: "Selected student" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Find student" })).toBeEnabled();
  await expectAccessiblePage(page);
});

test("keeps a long nickname and the change action usable at larger text sizes", async ({ page }, testInfo) => {
  await installAbsenceRoutes(page, []);
  const nickname = "น้องออมทรัพย์ — Alexandra";
  await page.route("**/api/v1/absence-self-service/lookup", (route) => route.fulfill({ json: {
    ...publicLookup, nickname, email_input_required: false,
  } }));
  await page.goto("/absence");
  await page.getByLabel("Student ID (W-Code)").fill(studentLookup.wcode);
  await page.getByRole("button", { name: "Find student" }).click();
  await expect(page.getByRole("heading", { name: nickname, exact: true })).toBeFocused();
  await page.evaluate(() => { document.documentElement.style.fontSize = "200%"; });
  await expectNoHorizontalOverflow(page);
  await page.getByRole("button", { name: "Change student" }).click();
  await expect(page.getByLabel("Student ID (W-Code)")).toBeFocused();
  await expect(page.getByRole("button", { name: "Find student" })).toBeVisible();
  await expectNoHorizontalOverflow(page);
  await page.screenshot({ path: testInfo.outputPath("student-search-large-text.png") });
});
