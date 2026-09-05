import { expect, test, type Page } from "@playwright/test";
import { installAbsenceRoutes, type SubmittedPayload } from "./fixtures/absence";
import { completeToClasses, selectClass } from "./helpers/absenceFlow";
import {
  expectFocusedElementVisible,
  expectInsideVisualViewport,
  expectNoActionOverlap,
} from "./helpers/absenceAssertions";

async function setConnectivity(page: Page, online: boolean) {
  await page.context().setOffline(!online);
  await page.evaluate((eventName) => window.dispatchEvent(new Event(eventName)), online ? "online" : "offline");
}

test("restored draft step cannot bypass student and parent verification", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  let sessionRequests = 0;
  await page.addInitScript((draft) => {
    window.sessionStorage.setItem("warwick.absence.draft.v1", JSON.stringify(draft));
  }, {
    schemaVersion: 1,
    updatedAt: Date.now(),
    wcode: "W250389",
    collectedEmail: "student@example.com",
    step: 3,
    selectedSubjectIds: ["subject-math"],
    selectedSessionIds: ["missed-boundary"],
    sitInSelections: { "missed-boundary": "sit-boundary" },
    sitInPriorityLevels: {},
    reason: "Saved reason",
  });
  await installAbsenceRoutes(page, submitted);
  await page.route("**/api/v1/absence-self-service/sessions**", (route) => {
    sessionRequests += 1;
    return route.fallback();
  });

  await page.goto("/absence");

  // The saved report is never revealed before identity + verification — no
  // resume prompt, no restored classes, and no schedule request.
  await expect(page.getByRole("heading", { name: "Is this you?" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Continue your absence report?" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Review your absence" })).toHaveCount(0);
  expect(sessionRequests).toBe(0);

  await page.getByRole("button", { name: "Yes, continue" }).click();
  await expect(page.getByRole("heading", { name: /confirm with a parent/i })).toBeVisible();
  expect(sessionRequests).toBe(0);

  // Only a completed verification unlocks the saved report for resume.
  await page.getByRole("button", { name: /^(send code|resend code)$/i }).click();
  await page.locator('input[aria-label="Confirmation code"]').fill("123456", { force: true });
  // Verification completes on the student's own tap (WCAG 3.2.1): the form
  // never advances on code input alone.
  await expect(page.getByRole("heading", { name: "Confirmed" })).toBeVisible();
  await page.getByRole("status").getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Continue your absence report?" })).toBeVisible();
});

test("connectivity changes do not render an offline state", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);

  await page.goto("/absence");
  await expect(page.getByPlaceholder("e.g. W250389")).toBeVisible();
  await setConnectivity(page, false);

  await expect(page.getByRole("status").filter({ hasText: /offline/i })).toHaveCount(0);
  await expect(page.getByText(/back online.*rechecking/i)).toHaveCount(0);
});

test("connectivity changes do not block the student form", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);

  await page.goto("/absence");
  await expect(page.getByPlaceholder("e.g. W250389")).toBeVisible();
  await setConnectivity(page, false);

  await expect(page.getByRole("button", { name: "Continue" })).toBeDisabled();
  await expect(page.getByRole("status").filter({ hasText: /offline/i })).toHaveCount(0);
});

test("visual viewport resize keeps the focused reason and action bar reachable", async ({ page }) => {
  await page.addInitScript(() => {
    const viewport = new EventTarget() as EventTarget & { height: number; offsetTop: number; width: number };
    Object.defineProperties(viewport, {
      height: { configurable: true, writable: true, value: window.innerHeight },
      offsetTop: { configurable: true, writable: true, value: 0 },
      width: { configurable: true, writable: true, value: window.innerWidth },
    });
    Object.defineProperty(window, "visualViewport", { configurable: true, value: viewport });
  });
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await completeToClasses(page);
  await selectClass(page, "Mathematics");
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("button", { name: /choose a time|change time/i }).click();
  await page.getByRole("dialog").getByRole("radio").first().click();
  await page.getByRole("button", { name: "Use this time" }).click();
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Why will you be away?" })).toBeVisible();
  await page.getByRole("radio", { name: "Other" }).click();
  const reason = page.getByLabel(/tell us a little more/i);
  await expect(reason).toBeVisible();
  await reason.focus();

  await page.evaluate(() => {
    const viewport = window.visualViewport as (EventTarget & { height: number });
    viewport.height = 320;
    viewport.dispatchEvent(new Event("resize"));
  });
  await expectFocusedElementVisible(page);
  await expectInsideVisualViewport(page, ".absence-action-bar");
  await expectNoActionOverlap(page);
});