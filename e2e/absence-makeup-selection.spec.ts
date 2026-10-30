import { expect, test } from "@playwright/test";
import { boundarySitInSession, installAbsenceRoutes, sessionsInRange, studentProfile, type SubmittedPayload } from "./fixtures/absence";
import { completeToClasses, expectMakeUpValue, selectAbsenceCheckbox } from "./helpers/absenceFlow";
import { expectAccessiblePage, expectInsideVisualViewport, expectNoHorizontalOverflow } from "./helpers/absenceAssertions";

test.use({ timezoneId: "Asia/Bangkok" });

test("browses, retries, cancels, and confirms alternatives without losing the saved choice", async ({ page }, testInfo) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  const math = sessionsInRange.subjects[0];
  const initial = { ...math, sit_in: { sit_in_method: "physical", current_priority_level: 1, has_next_priority: true,
    priorities: [{ level: 1, label: "First", sit_in_course: math.sit_in.sit_in_course, available_sessions: [boundarySitInSession] }],
  } };
  const alternative = { ...boundarySitInSession, id: "alternative", course_id: "alternative-course", start_at: "2026-01-18T03:00:00Z", end_at: "2026-01-18T04:00:00Z" };
  const next = { ...math, sit_in: { sit_in_method: "physical", current_priority_level: 3, has_next_priority: false,
    priorities: [{ level: 3, label: "Third", sit_in_course: { id: "alternative-course", code: "ALT", name: "Alternative Mathematics" }, available_sessions: [alternative] }],
  } };
  let failMore = true;
  await page.route("**/api/v1/absence-self-service/sessions**", (route) => {
    const more = new URL(route.request().url()).searchParams.has("sat_verbal_after_priority");
    return more && failMore
      ? route.fulfill({ status: 503, json: { message: "Temporary connection failure" } })
      : route.fulfill({ json: { subjects: [more ? next : initial] } });
  });
  await completeToClasses(page);
  await page.getByRole("checkbox", { name: /mathematics/i }).check();
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  const field = page.locator('[id="sit-in-missed-boundary-field"]');
  const trigger = field.locator("[data-make-up-trigger]");
  await trigger.click();
  const dialog = page.getByRole("dialog");
  // One make-up day only: a static date caption, no chip row to tap.
  await expect(dialog.locator(".absence-makeup-days__single")).toHaveText("Sat, 17 Jan 2026");
  await expect(dialog.getByRole("radiogroup")).toHaveCount(0);
  await dialog.locator(`input[value="${boundarySitInSession.id}"]`).check();
  await dialog.getByRole("button", { name: "Confirm make-up class" }).click();
  await page.evaluate(() => { document.documentElement.style.fontSize = "200%"; });
  await trigger.click();
  await expect(dialog.getByRole("button", { name: "Cancel", exact: true })).toBeVisible();
  const largeTextScreenshot = testInfo.outputPath("make-up-large-text.png");
  await page.screenshot({ path: largeTextScreenshot });
  await testInfo.attach("make-up-large-text", { path: largeTextScreenshot, contentType: "image/png" });
  await expectInsideVisualViewport(page, ".absence-mobile-sheet__footer");
  await expectNoHorizontalOverflow(page);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.evaluate(() => { document.documentElement.style.fontSize = ""; });
  await trigger.click();
  await dialog.getByRole("button", { name: "Show more times" }).click();
  await expect(dialog.getByText(/Couldn’t load more times/)).toBeVisible();
  await expect(dialog.locator(`input[value="${boundarySitInSession.id}"]`)).toBeChecked();
  failMore = false;
  await dialog.getByRole("button", { name: "Try loading more times again" }).click();
  // The reveal adds a second make-up day; the strip follows it so the new time
  // is visible while the staged choice stays summarised above the list.
  await expect(dialog.getByRole("radio", { name: "Sunday 18 January" })).toBeChecked();
  await expect(dialog.locator('input[value="alternative"]')).toBeVisible();
  await expect(dialog.locator(".absence-makeup-current")).toContainText("17 Jan · 00:30–01:30");
  await expect(dialog.getByRole("radiogroup", { name: "Make-up date" }).getByRole("radio")).toHaveCount(2);
  await expect(dialog.locator(".absence-makeup-days__caption")).toHaveText("January 2026");
  await expect(dialog.getByText("No more times to show.")).toBeVisible();
  await expect(dialog.locator(".absence-makeup-current")).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "Cancel", exact: true })).toBeVisible();
  await expectInsideVisualViewport(page, ".absence-mobile-sheet__header");
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await expectInsideVisualViewport(page, ".absence-mobile-sheet__footer");
  const screenshot = testInfo.outputPath("make-up-alternatives.png");
  await page.screenshot({ path: screenshot });
  await testInfo.attach("make-up-alternatives", { path: screenshot, contentType: "image/png" });
  await dialog.locator('input[value="alternative"]').check();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expectMakeUpValue(page, boundarySitInSession.id);
  await expect(trigger).toBeFocused();
  await trigger.click();
  await expect(dialog.getByRole("radio", { name: "Saturday 17 January" })).toBeChecked();
  await expect(dialog.locator(`input[value="${boundarySitInSession.id}"]`)).toBeChecked();
  await dialog.getByRole("radio", { name: "Sunday 18 January" }).check();
  await dialog.locator('input[value="alternative"]').check();
  await dialog.getByRole("button", { name: "Confirm make-up class" }).click();
  await page.getByLabel(/reason for absence/i).fill("Family appointment");
  await page.getByRole("button", { name: "Review absence" }).click();
  await expect(page.getByText(/Alternative Mathematics/)).toBeVisible();
  await page.getByRole("button", { name: "Submit absence" }).click();
  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  expect(submitted[0].items[0]).toMatchObject({ sit_in_course_id: "alternative-course", sit_in_session_ids: ["alternative"] });
});

test("explains timetable clashes, previous assignments, and a time chosen for another missed date", async ({ page }, testInfo) => {
  await installAbsenceRoutes(page, []);
  const math = sessionsInRange.subjects[0];
  const overlap = { ...boundarySitInSession, id: "overlap", start_at: "2026-01-18T03:00:00Z", end_at: "2026-01-18T04:00:00Z" };
  const used = { ...boundarySitInSession, id: "used", start_at: "2026-01-19T03:00:00Z", end_at: "2026-01-19T04:00:00Z", conflict: { absence_id: "internal-absence-id", absence_subject_name: "English", absence_date_from: "2026-01-12" } };
  const alternative = { ...boundarySitInSession, id: "free-time", start_at: "2026-01-22T03:00:00Z", end_at: "2026-01-22T04:00:00Z" };
  await page.route("**/api/v1/absence-self-service/me", (route) => route.fulfill({ json: { ...studentProfile, subjects: [...studentProfile.subjects, { id: "chemistry", code: "CHEM", name: "Chemistry" }] } }));
  await page.route("**/api/v1/absence-self-service/sessions**", (route) => route.fulfill({ json: { subjects: [
    { ...math, sessions: [...math.sessions, { ...math.sessions[0], id: "second-missed", date: "2026-01-20", start_at: "2026-01-20T02:00:00Z", end_at: "2026-01-20T03:00:00Z" }],
      sit_in: { ...math.sit_in, available_sessions: [boundarySitInSession, overlap, used, alternative] } },
    { ...math, subject_id: "chemistry", subject_code: "CHEM", subject_name: "Chemistry", course_id: "chemistry-course", sessions: [{ ...overlap, id: "chemistry-class", date: "2026-01-18", already_absent: false }], sit_in: undefined },
  ] } }));
  await completeToClasses(page);
  await page.getByRole("checkbox", { name: /mathematics/i }).check();
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  await page.locator('[id="sit-in-missed-boundary-field"] [data-make-up-trigger]').click();
  const dialog = page.getByRole("dialog");
  await dialog.locator(`input[value="${boundarySitInSession.id}"]`).check();
  await dialog.getByRole("button", { name: "Confirm make-up class" }).click();
  await selectAbsenceCheckbox(page, "session-second-missed");
  await page.locator('[id="sit-in-second-missed-field"] [data-make-up-trigger]').click();
  // Each day keeps its own reason: the strip marks days with no free time.
  await dialog.getByRole("radio", { name: "Saturday 17 January" }).check();
  await expect(dialog.getByRole("radio", { name: /Chosen for another absence in this form/ })).toBeDisabled();
  await dialog.getByRole("radio", { name: "Sunday 18 January" }).check();
  await expect(dialog.getByRole("radio", { name: /Overlaps another scheduled class/ })).toBeDisabled();
  await expect(dialog.getByText(/Chemistry — Sun, 18 Jan 2026 10:00-11:00/)).toBeVisible();
  await dialog.getByRole("radio", { name: "Monday 19 January" }).check();
  await expect(dialog.getByRole("radio", { name: /Already assigned to another absence/ })).toBeDisabled();
  await expect(dialog.getByText(/English.*12 Jan/)).toBeVisible();
  await expect(dialog.getByText("internal-absence-id")).toHaveCount(0);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  const screenshot = testInfo.outputPath("make-up-conflicts.png");
  await page.screenshot({ path: screenshot });
  await testInfo.attach("make-up-conflicts", { path: screenshot, contentType: "image/png" });
  await dialog.getByRole("radio", { name: "Thursday 22 January" }).check();
  await dialog.locator('input[value="free-time"]').check();
  await dialog.getByRole("button", { name: "Confirm make-up class" }).click();
  await expectMakeUpValue(page, boundarySitInSession.id, "sit-in-missed-boundary");
  await expectMakeUpValue(page, "free-time", "sit-in-second-missed");
  await page.route("**/api/v1/absences/batch", (route) => route.fulfill({ status: 409, json: {
    code: "sit_in_session_already_used", message: "This make-up class was already assigned.",
    details: { session_ids: ["free-time"], conflicts: [{ sit_in_subject_name: "Mathematics", absence_subject_name: "English" }] },
  } }));
  await page.getByLabel(/reason for absence/i).fill("Family appointment");
  await page.getByRole("button", { name: "Review absence" }).click();
  await page.getByRole("button", { name: "Submit absence" }).click();
  await expect(page.getByRole("heading", { name: "Courses & classes" })).toBeVisible();
  await expectMakeUpValue(page, boundarySitInSession.id, "sit-in-missed-boundary");
  await expect(page.locator('[id="sit-in-missed-boundary"]')).toHaveAttribute("aria-invalid", "false");
  await expectMakeUpValue(page, "free-time", "sit-in-second-missed");
  await expect(page.locator('[id="sit-in-second-missed"]')).toHaveAttribute("aria-invalid", "true");
  await page.getByRole("button", { name: "Review absence" }).click();
  await expect(page.getByRole("heading", { name: "Courses & classes" })).toBeVisible();
  await page.getByRole("checkbox", { name: /mathematics/i }).uncheck();
  await page.getByRole("checkbox", { name: "Chemistry", exact: true }).check();
  await page.getByRole("button", { name: "Review absence" }).click();
  await expect(page.getByText("Select at least one class you will miss.")).toBeVisible();
});

test("keeps an empty physical make-up picker actionable and explains the required next step", async ({ page }) => {
  await installAbsenceRoutes(page, []);
  const math = sessionsInRange.subjects[0];
  await page.route("**/api/v1/absence-self-service/sessions**", (route) => route.fulfill({ json: { subjects: [{ ...math, sit_in: { ...math.sit_in, available_sessions: [] } }] } }));
  await completeToClasses(page);
  await page.getByRole("checkbox", { name: /mathematics/i }).check();
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  await page.getByRole("button", { name: "Choose a make-up class" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText(/A make-up class is required for this date/)).toBeVisible();
  await expect(dialog.getByRole("link", { name: "Call Warwick" })).toHaveAttribute("href", "tel:+6621234567");
  await expect(dialog.getByRole("button", { name: "Confirm make-up class" })).toBeDisabled();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByLabel(/reason for absence/i).fill("Family appointment");
  await page.getByRole("button", { name: "Review absence" }).click();
  await expect(page.getByRole("heading", { name: "Courses & classes" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Choose a make-up class" })).toBeFocused();
});
