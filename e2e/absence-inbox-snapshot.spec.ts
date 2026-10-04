import { expect, test } from "@playwright/test";

const session = {
  id: "session-1", session_id: "session-1", course_id: "course-1", course_code: "SAT-MATH",
  course_name: "SAT Math : Rank 2 C3", start_at: "2026-09-13T17:00:00+07:00", end_at: "2026-09-13T20:20:00+07:00",
};

for (const removed of [false, true]) {
  test(`inbox groups original and ${removed ? "removed" : "moved"} sit-in sessions`, async ({ page }, testInfo) => {
    await page.route("**/api/v1/**", (route) => route.fulfill({ json: {} }));
    await page.route("**/api/v1/me", (route) => route.fulfill({ json: { id: "admin", username: "admin", role: "Admin" } }));
    await page.route("**/api/v1/absences?**", (route) => route.fulfill({ json: {
      items: [{
        id: "absence-1", wcode: "W260073", student_name: "Nick", course_id: "source", course_name: "SAT Math : Rank 3 C3",
        subject_name: "SAT Math : Rank 3 C3", date_from: "2026-09-06", date_to: "2026-09-06", sit_in_method: "physical",
        status: "pending", version: 1, created_at: "2026-09-06T10:00:00Z", updated_at: "2026-09-06T10:00:00Z",
        open_schedule_issue_count: 1, critical_schedule_issue_count: 1, latest_session_change_id: "change-1", sit_ins: removed ? [{
          ...session, id: "session-2", session_id: "session-2", start_at: "2026-10-18T17:00:00+07:00", end_at: "2026-10-18T20:20:00+07:00",
        }] : [session],
        sit_in_impacts: [{ session_id: "session-1", snapshot_quality: "exact", current_session: removed ? null : session,
          original_snapshot: { schema_version: 1, session_id: "session-1", session_version: 1,
            course: { id: "course-1", code: "SAT-MATH", name: "SAT Math : Rank 2 C3" },
            start_at: "2026-09-06T17:00:00+07:00", end_at: "2026-09-06T20:20:00+07:00", timezone: "Asia/Bangkok",
            captured_at: "2026-09-01T00:00:00Z", room: { id: null, name: null }, teacher: { id: null, name: null },
            series_id: null, occurrence_status: "active" },
        }],
      }], total_count: 1, offset: 0, limit: 25, open_schedule_impact_count: 1, critical_schedule_impact_count: 1,
    } }));
    await page.goto("/absences");
    const cell = page.locator('td[data-label="Sit-in"]');
    await expect(cell.getByText("Original session", { exact: true })).toBeVisible();
    const changed = cell.getByRole("group", { name: "Sit-in session change" });
    const outcome = changed.getByText(removed ? "Session removed" : "Current session", { exact: true });
    await expect(outcome).toBeVisible();
    if (removed) {
      await expect(changed).toContainText(/Original session.*6 Sept.*Session removed/);
      const assigned = cell.getByRole("group", { name: "Assigned sit-in session" });
      await expect(assigned).toContainText(/Assigned session.*18 Oct/);
      await expect(changed).not.toContainText("18 Oct");
    } else {
      await expect(changed).toContainText(/Original session.*6 Sept.*Current session.*13 Sept/);
    }
    await expect(cell).not.toContainText("No session selected");
    const original = await changed.getByText("Original session", { exact: true }).boundingBox();
    const current = await outcome.boundingBox();
    expect(original!.y).toBeLessThan(current!.y);
    const overflowing = await cell.evaluate((element) => element.scrollWidth > element.clientWidth + 1);
    expect(overflowing).toBe(false);
    await page.screenshot({ path: testInfo.outputPath("session-pair.png"), fullPage: true });
  });
}
