import { expect, test, type Page } from "@playwright/test";

const crmRows = [
  {
    snapshot_id: "11111111-1111-1111-1111-111111111111",
    row_hash: "row-a",
    xlsx_row_number: 12,
    course_name: "SAT Verbal Beginner Section 1",
    course_id: "source-a",
    extra_note: "Tuesday writing section",
    imported_at: "2026-08-26T02:00:00Z",
  },
  {
    snapshot_id: "11111111-1111-1111-1111-111111111111",
    row_hash: "row-b",
    xlsx_row_number: 18,
    course_name: "SAT Verbal Beginner Section 2",
    course_id: "source-b",
    extra_note: "Saturday reading section",
    imported_at: "2026-08-26T02:00:00Z",
  },
];

async function installRoutes(page: Page) {
  await page.route("**/api/v1/me", (route) => route.fulfill({ json: { id: "admin-id", username: "admin", role: "Admin" } }));
  await page.route("**/api/v1/absences/stats", (route) => route.fulfill({ json: { pending_count: 0 } }));
  await page.route("**/api/v1/courses", (route) => route.fulfill({ json: [
    { id: "course-a", code: "A", name: "Writing Destination", subject_name: "Writing" },
    { id: "course-b", code: "B", name: "Reading Destination", subject_name: "Reading" },
  ] }));
  await page.route("**/api/v1/cross-study/assignments?**", (route) => route.fulfill({
    json: { assignments: [], total: 0, review_count: 0 },
  }));
  await page.route("**/api/v1/cross-study/students/W260001", (route) => route.fulfill({ json: {
    student: { id: "student-id", wcode: "w260001", full_name: "Test Student" },
    crm_rows: crmRows,
    current_assignment: null,
  } }));
}

test("staff can inspect every CRM row and choose the assignment source", async ({ page }, testInfo) => {
  // Given: the student lookup API returns two active-snapshot CRM rows.
  await installRoutes(page);
  await page.goto("/crm/cross-study");

  // When: staff searches for the student and chooses the second row.
  await page.getByPlaceholder("e.g. W12345").fill("W260001");
  await page.getByRole("button", { name: "Search" }).click();
  await expect(page.getByText("SAT Verbal Beginner Section 1", { exact: true })).toBeVisible();
  await expect(page.getByText("SAT Verbal Beginner Section 2", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Select SAT Verbal Beginner Section 2" }).click();

  // Then: the chosen row is visibly selected and the responsive page has no horizontal overflow.
  await expect(page.getByRole("button", { name: "Selected" })).toHaveAttribute("aria-pressed", "true");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  await page.screenshot({
    path: `.omo/evidence/cross-study-crm-rows/${testInfo.project.name}.png`,
    fullPage: true,
  });
});
