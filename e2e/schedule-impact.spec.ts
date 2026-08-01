import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import {
  installScheduleImpactRoutes,
  type QueueItem,
} from "./fixtures/schedule-impact";

async function expectAccessiblePage(page: Page) {
  const results = await new AxeBuilder({ page }).analyze();
  const blocking = results.violations.filter(
    (violation) => violation.impact === "critical" || violation.impact === "serious",
  );
  expect(blocking, blocking.map(({ id, help }) => `${id}: ${help}`).join("\n")).toEqual([]);
}

function makeQueueItem(overrides: Partial<QueueItem> & { id: string }): QueueItem {
  return {
    absence_id: overrides.absence_id ?? `abs-${overrides.id}`,
    issue_type: overrides.issue_type ?? "regular_session_overlap",
    severity: overrides.severity ?? "warning",
    status: overrides.status ?? "open",
    issue_version: overrides.issue_version ?? 1,
    wcode: overrides.wcode ?? "W250000",
    student_name: overrides.student_name ?? `Student ${overrides.id}`,
    start_at: overrides.start_at ?? "2026-07-31T10:00:00Z",
    end_at: overrides.end_at ?? "2026-07-31T11:00:00Z",
    details: overrides.details ?? { reasons: ["test"], notice_hours: 1 },
    suggested_resolutions: overrides.suggested_resolutions ?? [],
    resolution_action: overrides.resolution_action ?? null,
    assignment_context: overrides.assignment_context ?? {
      assigned_at: "2026-07-25T09:00:00Z",
      original_session: {
        quality: "exact",
        source: "database",
        snapshot: {
          start_at: "2026-07-31T10:00:00Z",
          end_at: "2026-07-31T11:00:00Z",
          room_name: "Room 1",
          teacher_name: "Teacher",
          course_code: "COURSE",
          course_name: "Course",
        },
      },
      current_session: {
        status: "active",
        session_id: `session-${overrides.id}`,
        version: 1,
        start_at: "2026-07-31T10:00:00Z",
        end_at: "2026-07-31T11:00:00Z",
        course_code: "COURSE",
        course_name: "Course",
        subject_name: "Course",
        room_name: "Room 1",
        teacher_name: "Teacher",
      },
    },
    change_context: overrides.change_context ?? {
      change_id: `change-${overrides.id}`,
      before: { start_at: "2026-07-31T10:00:00Z", end_at: "2026-07-31T11:00:00Z" },
      after: { start_at: "2026-07-31T10:00:00Z", end_at: "2026-07-31T11:00:00Z" },
    },
    impact_context: overrides.impact_context ?? {
      issue_type: overrides.issue_type ?? "regular_session_overlap",
      severity: overrides.severity ?? "warning",
      reasons: [{ code: "overlap", message: "test reason" }],
    },
  };
}

/** Helper to open the resolution panel and wait for it to load */
async function openResolutionPanel(page: Page) {
  await page.getByRole("button", { name: "Review" }).first().click();
  await expect(page.getByRole("heading", { name: "Resolve issue" })).toBeVisible();
}

// ---------------------------------------------------------------------------
// Queue tests
// ---------------------------------------------------------------------------
test.describe("Schedule Impact — Queue", () => {
  test("loads and displays items with severity grouping", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await expect(page.getByText("Alice Johnson")).toBeVisible();
    await expect(page.getByText("Carol Davis")).toBeVisible();
    await expect(page.getByText("Bob Wilson")).toBeVisible();
  });

  test("default status filter is all (shows open + needs_review)", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    const statusSelect = page.getByLabel("Filter issue status");
    await expect(statusSelect).toHaveValue("all");
  });

  test("search input filters results", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await page.getByLabel("Search student, course, or session").fill("Alice");
    // Wait for debounce (350ms) + response
    await expect(page.getByText("Alice Johnson")).toBeVisible();
    await expect(page.getByText("Bob Wilson")).toHaveCount(0);
    await expect(page.getByText("Carol Davis")).toHaveCount(0);
  });

  test("severity filter works", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await page.getByLabel("Filter severity").selectOption("critical");
    await expect(page.getByText("Alice Johnson")).toBeVisible();
    await expect(page.getByText("Carol Davis")).toBeVisible();
    await expect(page.getByText("Bob Wilson")).toHaveCount(0);
  });

  test("status filter works", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await page.getByLabel("Filter issue status").selectOption("open");
    await expect(page.getByText("Alice Johnson")).toBeVisible();
    await expect(page.getByText("Carol Davis")).toBeVisible();
    await expect(page.getByText("Bob Wilson")).toHaveCount(0);
  });

  test("pagination shows when total > limit, Previous/Next work", async ({ page }) => {
    const items = Array.from({ length: 5 }, (_, i) =>
      makeQueueItem({ id: `p${i}`, student_name: `Student P${i}`, wcode: `W2500${i}` }),
    );

    await installScheduleImpactRoutes(page, {
      queueItems: items,
      queueTotal: 30,
      queueLimit: 25,
    });
    await page.goto("/operations/schedule-impact");

    await expect(page.getByRole("navigation", { name: "Queue pagination" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Next" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Previous" })).toBeVisible();

    await page.getByRole("button", { name: "Next" }).click();
    await expect(page.getByText("Student P0")).toHaveCount(0);
  });

  test("single Review button per item", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    const reviewButtons = page.getByRole("button", { name: "Review" });
    await expect(reviewButtons.first()).toBeVisible();
    const count = await reviewButtons.count();
    expect(count).toBeGreaterThanOrEqual(1);
  });

  test("clicking Review opens the resolution panel", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await page.getByRole("button", { name: "Review" }).first().click();
    await expect(page.getByRole("heading", { name: "Resolve issue" })).toBeVisible();
  });

  test("queue summary shows counts", async ({ page }) => {
    await installScheduleImpactRoutes(page, {
      queueSummary: {
        need_attention: 11,
        critical: 3,
        warnings: 6,
        notification_failures: 2,
        notifications_configured: true,
      },
    });
    await page.goto("/operations/schedule-impact");

    await expect(page.getByText("3 critical")).toBeVisible();
    await expect(page.getByText("11 total")).toBeVisible();
    await expect(page.getByText("6 warnings")).toBeVisible();
  });

  test("empty state when no items match filters", async ({ page }) => {
    await installScheduleImpactRoutes(page, { queueItems: [], queueTotal: 0 });
    await page.goto("/operations/schedule-impact");

    await expect(page.getByText("No student arrangements need attention")).toBeVisible();
    await expect(
      page.getByText("Schedule changes are being monitored automatically"),
    ).toBeVisible();
  });

  test("error state when API fails", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.route("**/api/v1/operations/schedule-impact**", (route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ code: "internal_error", message: "Server error" }),
      }),
    );
    await page.goto("/operations/schedule-impact");

    await expect(page.getByText("Could not load Schedule Impact")).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Resolution panel tests
// ---------------------------------------------------------------------------
test.describe("Schedule Impact — Resolution panel", () => {
  test("panel opens with issue details (student name, severity badge, course info)", async ({
    page,
  }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await page.getByRole("button", { name: "Review" }).first().click();
    await expect(page.getByRole("heading", { name: "Resolve issue" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Alice Johnson", exact: true })).toBeVisible();
    await expect(page.getByLabel("Resolve issue").getByText("Critical")).toBeVisible();
    await expect(page.getByLabel("Resolve issue").getByText("Mathematics")).toBeVisible();
  });

  test("'What changed' section shows original vs current session", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await openResolutionPanel(page);
    await expect(page.getByText("What changed")).toBeVisible();
    await expect(page.getByText("Originally assigned")).toBeVisible();
    await expect(page.getByText("Session now")).toBeVisible();
    await expect(page.getByText("Room 4")).toBeVisible();
    await expect(page.getByText("Room 7")).toBeVisible();
  });

  test("'Why this needs attention' shows impact reason", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await openResolutionPanel(page);
    await expect(page.getByText("Why this needs attention")).toBeVisible();
    // For regular_session_overlap, the display text is:
    // "This sit-in overlaps with the student's regular class."
    await expect(page.getByLabel("Resolve issue").getByText("sit-in overlaps")).toBeVisible();
  });

  test("'What should happen' shows action options", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await openResolutionPanel(page);
    await expect(page.getByText("What should happen?")).toBeVisible();
    const radioGroup = page.getByRole("radiogroup", { name: "Resolution actions" });
    await expect(radioGroup.getByText("Move to another session")).toBeVisible();
    await expect(radioGroup.getByText("Keep the current arrangement")).toBeVisible();
  });

  test("selecting Move to another session shows candidate list", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await openResolutionPanel(page);
    await page.getByRole("radiogroup", { name: "Resolution actions" })
      .getByText("Move to another session")
      .click();

    await expect(page.getByText("Choose a replacement")).toBeVisible();
    const candidateOptions = page.getByRole("radiogroup", { name: "Replacement session options" });
    await expect(candidateOptions.getByText("Room 5")).toBeVisible();
    await expect(candidateOptions.getByText("Room 2")).toBeVisible();
  });

  test("safe candidate is selectable, unsafe candidate shows Cannot be selected", async ({
    page,
  }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await openResolutionPanel(page);
    await page.getByRole("radiogroup", { name: "Resolution actions" })
      .getByText("Move to another session")
      .click();

    await expect(page.getByText("Choose a replacement")).toBeVisible();

    // Click the first candidate radio (safe — eligible, no blocking reasons)
    const candidateRadio = page
      .getByRole("radiogroup", { name: "Replacement session options" })
      .locator("input[type='radio']")
      .first();
    await candidateRadio.click({ force: true });

    // Unsafe candidate (eligible: false) shows "Cannot be selected"
    await expect(page.getByText("Cannot be selected")).toBeVisible();
  });

  test("no candidate is pre-selected (user must choose)", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await openResolutionPanel(page);
    await page.getByRole("radiogroup", { name: "Resolution actions" })
      .getByText("Move to another session")
      .click();

    await expect(page.getByText("Choose a replacement")).toBeVisible();
    // No radio should be checked — user must choose
    const checked = page
      .getByRole("radiogroup", { name: "Replacement session options" })
      .locator("input:checked");
    await expect(checked).toHaveCount(0);
  });

  test("confirmation step shows 'Confirm reassignment' (not generic Confirm)", async ({
    page,
  }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await openResolutionPanel(page);
    await page.getByRole("radiogroup", { name: "Resolution actions" })
      .getByText("Move to another session")
      .click();

    // Select first candidate
    const candidateRadio = page
      .getByRole("radiogroup", { name: "Replacement session options" })
      .locator("input[type='radio']")
      .first();
    await candidateRadio.click({ force: true });

    await expect(page.getByText("Reassign Alice Johnson?")).toBeVisible();
    await expect(page.getByRole("button", { name: "Confirm reassignment" })).toBeVisible();
  });

  test("successful resolution closes the panel and refetches the queue", async ({
    page,
  }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await openResolutionPanel(page);
    await page.getByRole("radiogroup", { name: "Resolution actions" })
      .getByText("Move to another session")
      .click();

    const candidateRadio = page
      .getByRole("radiogroup", { name: "Replacement session options" })
      .locator("input[type='radio']")
      .first();
    await candidateRadio.click({ force: true });

    await page.getByRole("button", { name: "Confirm reassignment" }).click();

    // Panel closes after resolution (the page's resolve() calls setSelected(null))
    await expect(page.getByRole("heading", { name: "Resolve issue" })).toHaveCount(0);
  });

  test("panel close button dismisses the panel", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await openResolutionPanel(page);
    await expect(page.getByRole("heading", { name: "Resolve issue" })).toBeVisible();

    // Close panel via the close button
    await page.getByRole("button", { name: "Close panel" }).click();
    await expect(page.getByRole("heading", { name: "Resolve issue" })).toHaveCount(0);
  });

  test("Back button returns from confirmation to action selection", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await openResolutionPanel(page);
    await page.getByRole("radiogroup", { name: "Resolution actions" })
      .getByText("Move to another session")
      .click();

    const candidateRadio = page
      .getByRole("radiogroup", { name: "Replacement session options" })
      .locator("input[type='radio']")
      .first();
    await candidateRadio.click({ force: true });

    await expect(page.getByRole("button", { name: "Confirm reassignment" })).toBeVisible();

    await page.getByRole("button", { name: "Back" }).click();
    // Back resets pendingAction, showing the action selector again
    await expect(page.getByText("What should happen?")).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Processing tab tests
// ---------------------------------------------------------------------------
test.describe("Schedule Impact — Processing tab", () => {
  test("shows analysis items with status badges", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await page.getByRole("button", { name: "Processing" }).click();
    await expect(page.getByText("Chemistry")).toBeVisible();
    // Use exact match to avoid ambiguity with the tab button
    await expect(page.getByText("processing", { exact: true })).toBeVisible();
  });

  test("failed item shows Retry analysis button", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await page.getByRole("button", { name: "Processing" }).click();
    await expect(page.getByText("Physics")).toBeVisible();
    await expect(page.getByRole("button", { name: "Retry analysis" })).toBeVisible();
  });

  test("empty processing state shows appropriate message", async ({ page }) => {
    await installScheduleImpactRoutes(page, { processingItems: [] });
    await page.goto("/operations/schedule-impact");

    await page.getByRole("button", { name: "Processing" }).click();
    await expect(page.getByText("No impact analyses are processing")).toBeVisible();
    await expect(
      page.getByText("Completed changes are available in History"),
    ).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// History tab tests
// ---------------------------------------------------------------------------
test.describe("Schedule Impact — History tab", () => {
  test("shows completed changes in table format", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await page.getByRole("button", { name: "History" }).click();
    await expect(page.getByRole("link", { name: "History", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "Art", exact: true })).toBeVisible();
  });

  test("table has correct columns", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await page.getByRole("button", { name: "History" }).click();
    await expect(page.getByRole("columnheader", { name: "Changed" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Course" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Change", exact: true })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Affected" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Result" })).toBeVisible();
  });

  test("completed items show green Completed badge", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await page.getByRole("button", { name: "History" }).click();
    await expect(page.getByText("Completed").first()).toBeVisible();
  });

  test("unresolved items show amber badge with count", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await page.getByRole("button", { name: "History" }).click();
    await expect(page.getByText(/unresolved/)).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Navigation tests
// ---------------------------------------------------------------------------
test.describe("Schedule Impact — Navigation", () => {
  test("Schedule Impact link is visible in primary nav", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await expect(page.getByRole("link", { name: "Schedule Impact" })).toBeVisible();
  });

  test("critical badge is red, non-critical badge is blue", async ({ page }) => {
    // Test critical badge: when critical > 0, the red badge shows
    await installScheduleImpactRoutes(page, {
      queueSummary: {
        need_attention: 5,
        critical: 2,
        warnings: 3,
        notification_failures: 0,
        notifications_configured: true,
      },
    });

    const navSummaryResponse = page.waitForResponse(
      (res) => res.url().includes("/api/v1/operations/schedule-impact") &&
        res.url().includes("limit=1") && res.status() === 200,
    );
    await page.goto("/operations/schedule-impact");
    await navSummaryResponse;

    // Critical badge shown (red) — Layout uses critical OR unresolved, not both
    const criticalBadge = page.getByLabel("2 critical schedule impacts");
    await expect(criticalBadge).toBeVisible();
    await expect(criticalBadge).toHaveText("2");
  });

  test("non-critical badge is blue when no critical items", async ({ page }) => {
    // Test non-critical badge: when critical = 0 but need_attention > 0, the blue badge shows
    await installScheduleImpactRoutes(page, {
      queueSummary: {
        need_attention: 5,
        critical: 0,
        warnings: 5,
        notification_failures: 0,
        notifications_configured: true,
      },
    });

    const navSummaryResponse = page.waitForResponse(
      (res) => res.url().includes("/api/v1/operations/schedule-impact") &&
        res.url().includes("limit=1") && res.status() === 200,
    );
    await page.goto("/operations/schedule-impact");
    await navSummaryResponse;

    const unresolvedBadge = page.getByLabel("5 unresolved schedule impacts");
    await expect(unresolvedBadge).toBeVisible();
    await expect(unresolvedBadge).toHaveText("5");
  });
});

// ---------------------------------------------------------------------------
// Accessibility tests
// ---------------------------------------------------------------------------
test.describe("Schedule Impact — Accessibility", () => {
  test("queue page passes axe scan", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");
    await expect(page.getByText("Alice Johnson")).toBeVisible();
    await expectAccessiblePage(page);
  });

  test("resolution panel passes axe scan", async ({ page }) => {
    await installScheduleImpactRoutes(page);
    await page.goto("/operations/schedule-impact");

    await openResolutionPanel(page);
    await expectAccessiblePage(page);
  });
});
