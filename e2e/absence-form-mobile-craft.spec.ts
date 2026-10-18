import { expect, test } from "@playwright/test";
import { boundarySitInSession, installAbsenceRoutes, sessionsInRange, studentProfile, type SubmittedPayload } from "./fixtures/absence";
import { completeToClasses, completeToReview, selectAbsenceCheckbox } from "./helpers/absenceFlow";
import { expectAccessiblePage, expectAnimationsSettled, expectFocusedElementVisible, expectInsideVisualViewport, expectMinimumTapTarget, expectNoActionOverlap, expectNoHorizontalOverflow } from "./helpers/absenceAssertions";

test.use({ timezoneId: "Asia/Bangkok" });

test("captures the complete mobile form for craft review", async ({ page }, testInfo) => {
  await installAbsenceRoutes(page, []);
  await page.emulateMedia({ reducedMotion: "reduce" });
  const capture = async (name: string) => {
    await expectAnimationsSettled(page);
    const path = testInfo.outputPath(`${name}.png`);
    await page.screenshot({ path });
    await testInfo.attach(name, { path, contentType: "image/png" });
  };
  await page.goto("/absence");
  await expect(page.getByRole("heading", { name: "Who will be absent?" })).toBeVisible();
  await capture("student");
  await page.getByPlaceholder("e.g. W250389").fill("W250389");
  await page.getByRole("button", { name: "Find student", exact: true }).click();
  await page.getByLabel(/email address/i).fill("student@example.com");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await capture("verification");
  await page.getByRole("button", { name: "Send code", exact: true }).click();
  await page.getByLabel("Verification code", { exact: true }).fill("123456", { force: true });
  await page.getByRole("checkbox", { name: /mathematics/i }).check();
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  await capture("classes");
  await page.getByRole("button", { name: /choose a make-up class/i }).click();
  const dialog = page.getByRole("dialog");
  await capture("make-up-sheet");
  await dialog.getByRole("radio", { name: /mathematics make-up/i }).check();
  await dialog.getByRole("button", { name: "Confirm make-up class" }).click();
  await page.getByLabel(/reason for absence/i).fill("Medical appointment");
  await page.getByRole("button", { name: "Review absence" }).click();
  await capture("review");
  await page.getByRole("button", { name: "Submit absence" }).click();
  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await capture("receipt");
});

test("keeps the public picker browsable, cancellable, and keyboard contained", async ({ page }) => {
  await installAbsenceRoutes(page, []);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await completeToClasses(page);
  await page.getByRole("checkbox", { name: /mathematics/i }).check();
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  const trigger = page.getByRole("button", { name: /choose a make-up class/i });
  await trigger.click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading")).toBeFocused();
  await expect.poll(() => dialog.evaluate((surface) => surface.matches(":modal"))).toBe(true);
  for (let index = 0; index < 6; index += 1) {
    await page.keyboard.press("Tab");
    // Native dialogs allow browser-chrome focus (reported as body), but not background app controls.
    await expect.poll(() => dialog.evaluate((surface) => document.activeElement === document.body || surface.contains(document.activeElement))).toBe(true);
  }
  await dialog.getByRole("radio", { name: /mathematics make-up/i }).check();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(trigger).toBeFocused();
  await expect(page.locator("#sit-in-missed-boundary")).toHaveValue("");
  await trigger.click();
  await expect(dialog.getByRole("button", { name: "Confirm make-up class" })).toBeDisabled();
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  await expectMinimumTapTarget(page, '[data-make-up-trigger]');
  await expectNoActionOverlap(page);
  await page.getByRole("button", { name: "Review absence" }).click();
  await expect(trigger).toBeFocused();
  await expect(trigger).toHaveAttribute("aria-invalid", "true");
  await expect(trigger).toHaveAccessibleDescription(/For Mathematics.*Choose a make-up class for this date\./);
  await trigger.click();
  await page.setViewportSize({ width: 768, height: 1024 });
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole("combobox")).toBeFocused();
});

test("returns Edit reason to the field and resets the actual main scroller", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await completeToReview(page);
  await page.getByRole("button", { name: "Edit reason" }).click();
  await expect(page.getByLabel(/reason for absence/i)).toBeFocused();
  await expect(page.getByLabel(/reason for absence/i)).toHaveValue("Medical appointment");
  await expect(page.locator("#sit-in-missed-boundary")).toHaveValue(boundarySitInSession.id);
  await page.getByRole("button", { name: "Review absence" }).click();
  await expect.poll(() => page.locator("#absence-form-content").evaluate((main) => main.scrollTop)).toBe(0);
  expect(submitted).toHaveLength(0);
});

test("keeps long course names and a long make-up list usable with keyboard resize and larger text", async ({ page }, testInfo) => {
  await page.addInitScript(() => {
    const viewport = new EventTarget();
    Object.assign(viewport, { height: window.innerHeight, offsetTop: 0, width: window.innerWidth });
    Object.defineProperty(window, "visualViewport", { configurable: true, value: viewport });
  });
  await installAbsenceRoutes(page, []);
  const name = "Mathematics — Advanced problem solving and examination preparation";
  await page.route("**/api/v1/absence-self-service/me", (route) => route.fulfill({ json: {
    ...studentProfile,
    subjects: [{ ...studentProfile.subjects[0], name, teacher_name: "AJ. Alexandra and AJ. Christopher" }, { id: "subject-physics", code: "PHYS", name: "Physics" }],
  } }));
  const math = sessionsInRange.subjects[0];
  await page.route("**/api/v1/absence-self-service/sessions**", (route) => route.fulfill({ json: { subjects: [
    { ...math, subject_name: name, teacher_name: "AJ. Alexandra and AJ. Christopher", sit_in: { ...math.sit_in, available_sessions: Array.from({ length: 24 }, (_, index) => ({
      ...boundarySitInSession,
      id: `long-option-${index}`,
      start_at: new Date(Date.UTC(2026, 0, 17 + index, 6)).toISOString(),
      end_at: new Date(Date.UTC(2026, 0, 17 + index, 8)).toISOString(),
    })) } },
    { ...math, subject_id: "subject-physics", course_id: "course-physics", subject_name: "Physics", sessions: math.sessions.map((session) => ({ ...session, id: "physics-date" })) },
  ] } }));
  await completeToClasses(page);
  await page.getByRole("checkbox", { name: /advanced problem solving/i }).check();
  await page.getByRole("checkbox", { name: "Physics", exact: true }).check();
  const mathDisclosure = page.getByRole("button", { name: /^Mathematics — Advanced problem solving/i });
  await expect(mathDisclosure).toHaveAttribute("aria-expanded", "false");
  await mathDisclosure.click();
  await expect(mathDisclosure).toHaveAttribute("aria-expanded", "true");
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  await page.getByRole("button", { name: /choose a make-up class/i }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox").focus();
  await page.evaluate(() => {
    Object.assign(window.visualViewport!, { height: 420, offsetTop: 20 });
    window.visualViewport!.dispatchEvent(new Event("resize"));
  });
  await expectInsideVisualViewport(page, ".absence-mobile-sheet__footer");
  const lastOption = dialog.locator('input[value="long-option-23"]');
  await lastOption.check();
  await expect(lastOption).toBeChecked();
  await expectInsideVisualViewport(page, ".absence-mobile-sheet__footer");
  await expectAccessiblePage(page);
  await page.evaluate(() => { document.documentElement.style.fontSize = "200%"; });
  await expectNoHorizontalOverflow(page);
  await dialog.getByRole("button", { name: "Confirm make-up class" }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.locator("#sit-in-missed-boundary")).toHaveValue("long-option-23");
  await expectNoHorizontalOverflow(page);
  await page.getByLabel(/reason for absence/i).focus();
  await expectFocusedElementVisible(page);
  await expectNoActionOverlap(page);
  await expectInsideVisualViewport(page, ".absence-action-bar");
  const path = testInfo.outputPath("classes-large-text.png");
  await page.screenshot({ path });
  await testInfo.attach("classes-large-text", { path, contentType: "image/png" });
  await page.emulateMedia({ forcedColors: "active", reducedMotion: "reduce" });
  await expect(page.getByRole("checkbox", { name: /advanced problem solving/i })).toBeChecked();
  await expectNoHorizontalOverflow(page);
  const contrastPath = testInfo.outputPath("classes-forced-colors.png");
  await page.screenshot({ path: contrastPath });
  await testInfo.attach("classes-forced-colors", { path: contrastPath, contentType: "image/png" });
});
