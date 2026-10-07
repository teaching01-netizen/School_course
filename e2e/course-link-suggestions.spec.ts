import { expect, test, type Page, type Request } from "@playwright/test";

type SuggestionCourse = {
  id: string;
  code: string;
  name: string;
  subject_id: string;
  subject_code: string;
  subject_name: string;
  level: number | null;
  cycle_id: string | null;
  cycle_label: string | null;
  cycle_start_date: string | null;
  cycle_end_date: string | null;
  root_course_group_id: string | null;
  root_course_group_name: string | null;
  sit_in_rule_id?: string | null;
  sit_in_rule_name?: string | null;
  sit_in_rule_type?: string | null;
  sit_in_rule_description?: string | null;
  absence_form_visible?: boolean | null;
  absence_form_active?: boolean | null;
  session_count: number;
  session_date_from: string | null;
  session_date_to: string | null;
  slots: string[];
  teachers: string[];
};

type Suggestion = {
  configured_source: SuggestionCourse;
  unconfigured_course: SuggestionCourse;
  confidence: "high" | "review";
  reason_codes: string[];
  ambiguity_count: number;
  evidence_fingerprint: string;
};

type PageOfSuggestions = {
  items: Suggestion[];
  has_more?: boolean;
  next_cursor?: string | null;
};

type CreateBody = {
  name: string;
  course_ids: string[];
  kind: string;
  rule_source_course_id: string;
  suggestion_precondition: {
    configured_course_id: string;
    unconfigured_course_id: string;
    evidence_fingerprint: string;
    detector_version: string;
  };
};

type DismissBody = {
  configured_course_id: string;
  unconfigured_course_id: string;
  evidence_fingerprint: string;
  detector_version: string;
};

type LinkHarnessOptions = {
  mode?: "discovery" | "confirmation";
  getSuggestions: (params: { includeReview: boolean; cursor: string | null }, call: number) => Promise<PageOfSuggestions> | PageOfSuggestions;
  createGroup?: (body: CreateBody, key: string, call: number) => Promise<{ status: number; body: unknown }> | { status: number; body: unknown };
  dismiss?: (body: DismissBody, key: string, call: number) => Promise<{ status: number; body: unknown }> | { status: number; body: unknown };
};

type LinkHarness = {
  listRequests: URL[];
  createRequests: Array<{ body: CreateBody; key: string }>;
  dismissRequests: Array<{ body: DismissBody; key: string }>;
};

const DETECTOR_VERSION = "course-link-suggestions-v1";

function course(overrides: Partial<SuggestionCourse> & Pick<SuggestionCourse, "id" | "code">): SuggestionCourse {
  const { id, code, ...values } = overrides;
  return {
    id,
    code,
    name: "English Foundations",
    subject_id: "subject-english",
    subject_code: "ENG",
    subject_name: "English",
    level: 2,
    cycle_id: "cycle-2026-a",
    cycle_label: "2026 Term A",
    cycle_start_date: "2026-01-01",
    cycle_end_date: "2026-03-31",
    root_course_group_id: "root-english",
    root_course_group_name: "English Levels",
    session_count: 3,
    session_date_from: "2026-01-05",
    session_date_to: "2026-01-19",
    slots: ["1|09:00|3600"],
    teachers: [],
    ...values,
  };
}

function highSuggestion(overrides: Partial<Suggestion> = {}): Suggestion {
  return {
    configured_source: course({
      id: "source-course-id",
      code: "ENG-A",
      teachers: ["Dr Ada Lovelace"],
      sit_in_rule_id: "rule-english",
      sit_in_rule_name: "Level ladder",
      sit_in_rule_type: "level_ladder",
      sit_in_rule_description: "Use the next eligible English level.",
      absence_form_visible: true,
      absence_form_active: true,
    }),
    unconfigured_course: course({
      id: "partial-course-id",
      code: "ENG-B",
      level: null,
      teachers: ["Mr Alan Turing"],
      session_date_from: "2026-01-12",
      session_date_to: "2026-01-26",
    }),
    confidence: "high",
    reason_codes: ["same_subject", "normalized_name_match", "same_teaching_period", "compatible_slots", "equal_slot_durations", "complementary_dates", "unique_partner"],
    ambiguity_count: 1,
    evidence_fingerprint: "a".repeat(64),
    ...overrides,
  };
}

function reviewSuggestion(): Suggestion {
  return {
    configured_source: course({ id: "review-source-id", code: "ENG-C", teachers: ["Ms Grace Hopper"] }),
    unconfigured_course: course({ id: "review-partial-id", code: "ENG-D", level: null, slots: ["2|11:00|5400"], teachers: [] }),
    confidence: "review",
    reason_codes: ["same_subject", "normalized_name_match", "period_unknown", "slot_mismatch", "duration_mismatch", "multiple_possible_partners"],
    ambiguity_count: 2,
    evidence_fingerprint: "b".repeat(64),
  };
}

function response(items: Suggestion[], hasMore = false, nextCursor: string | null = null): PageOfSuggestions {
  return { items, has_more: hasMore, next_cursor: nextCursor };
}

async function installCourseLinkRoutes(page: Page, options: LinkHarnessOptions): Promise<LinkHarness> {
  const calls: LinkHarness = { listRequests: [], createRequests: [], dismissRequests: [] };
  let listCount = 0;
  let createCount = 0;
  let dismissCount = 0;
  const mode = options.mode ?? "discovery";

  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;

    if (request.method() === "GET" && path === "/api/v1/me") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ id: "admin-id", username: "Admin", role: "Admin" }) });
      return;
    }
    if (request.method() === "GET" && path === "/api/v1/admin/course-levels") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ items: [], total: 0, limit: 100, offset: 0 }) });
      return;
    }
    if (request.method() === "GET" && path === "/api/v1/admin/root-course-groups") {
      await route.fulfill({ contentType: "application/json", body: "[]" });
      return;
    }
    if (request.method() === "GET" && path === "/api/v1/admin/sit-in-rules") {
      await route.fulfill({ contentType: "application/json", body: "[]" });
      return;
    }
    if (request.method() === "GET" && path === "/api/v1/admin/course-link-suggestions") {
      calls.listRequests.push(url);
      listCount += 1;
      let pageOfSuggestions: PageOfSuggestions;
      try {
        pageOfSuggestions = await options.getSuggestions({
          includeReview: url.searchParams.get("include_review") === "true",
          cursor: url.searchParams.get("cursor"),
        }, listCount);
      } catch {
        await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ code: "internal", message: "Suggestion service unavailable" }) });
        return;
      }
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          enabled: true,
          mode,
          confirmation_enabled: mode === "confirmation",
          items: pageOfSuggestions.items,
          has_more: pageOfSuggestions.has_more ?? false,
          next_cursor: pageOfSuggestions.next_cursor ?? null,
          evaluated_at: "2026-01-01T00:00:00Z",
          detector_version: DETECTOR_VERSION,
          institute_timezone: "Asia/Bangkok",
        }),
      });
      return;
    }
    if (request.method() === "POST" && path === "/api/v1/course-groups") {
      createCount += 1;
      const body = request.postDataJSON() as CreateBody;
      const key = request.headers()["idempotency-key"] ?? "";
      calls.createRequests.push({ body, key });
      const result = await options.createGroup?.(body, key, createCount) ?? {
        status: 201,
        body: { id: "group-id", name: body.name, course_ids: body.course_ids },
      };
      await route.fulfill({ status: result.status, contentType: "application/json", body: JSON.stringify(result.body) });
      return;
    }
    if (request.method() === "POST" && path === "/api/v1/admin/course-link-suggestions/dismiss") {
      dismissCount += 1;
      const body = request.postDataJSON() as DismissBody;
      const key = request.headers()["idempotency-key"] ?? "";
      calls.dismissRequests.push({ body, key });
      const result = await options.dismiss?.(body, key, dismissCount) ?? {
        status: 200,
        body: { dismissed: true, already_dismissed: false },
      };
      await route.fulfill({ status: result.status, contentType: "application/json", body: JSON.stringify(result.body) });
      return;
    }

    await route.fulfill({ contentType: "application/json", body: "[]" });
  });

  return calls;
}

async function openCourseLevels(page: Page) {
  await page.goto("/course-levels");
  await expect(page.getByRole("heading", { name: "Possible same course" })).toBeVisible();
}

test.describe("Course link suggestions", () => {
  test("shows high-confidence evidence read-only by default, including different teachers and source consequences", async ({ page }) => {
    await installCourseLinkRoutes(page, { getSuggestions: () => response([highSuggestion()]) });
    await openCourseLevels(page);

    const card = page.getByRole("article", { name: "High confidence course link: ENG-A and ENG-B" });
    await expect(card).toBeVisible();
    await expect(card.getByText("Teachers: Dr Ada Lovelace")).toBeVisible();
    await expect(card.getByText("Teachers: Mr Alan Turing")).toBeVisible();
    await expect(card.getByText(/Effective rules: Sit-in rule: Level ladder/)).toBeVisible();
    await expect(card.getByText("Use the next eligible English level.")).toBeVisible();
    await expect(page.getByText("Discovery is read only.")).toBeVisible();
    await expect(card.getByRole("button", { name: "Link as same course" })).toHaveCount(0);
    await expect(card.getByRole("button", { name: "Not the same" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Manage levels" })).toBeVisible();
    await expect(page.getByText("Course level management is available in the manager.")).toBeVisible();
  });

  test("review toggle exposes review evidence and manual linking without one-click confirmation", async ({ page }) => {
    const high = highSuggestion();
    const review = reviewSuggestion();
    await installCourseLinkRoutes(page, {
      mode: "confirmation",
      getSuggestions: ({ includeReview }) => response(includeReview ? [high, review] : [high]),
    });
    await openCourseLevels(page);

    await page.getByRole("checkbox", { name: "Show review candidates" }).check();
    const card = page.getByRole("article", { name: "Review course link: ENG-C and ENG-D" });
    await expect(card).toBeVisible();
    await expect(card.getByText("Teaching period is not confirmed")).toBeVisible();
    await expect(card.getByRole("link", { name: "Review with manual linking" })).toHaveAttribute(
      "href",
      "/courses/create?mode=merge&continuation=1&courses=review-source-id%2Creview-partial-id&source=review-source-id&name=Same+course%3A+ENG-C+%2B+ENG-D",
    );
    await expect(card.getByRole("button", { name: "Link as same course" })).toHaveCount(0);
  });

  test("confirmation submits the configured source and evidence precondition, then removes the row", async ({ page }) => {
    let linked = false;
    let createCall: CreateBody | null = null;
    await installCourseLinkRoutes(page, {
      mode: "confirmation",
      getSuggestions: () => response(linked ? [] : [highSuggestion()]),
      createGroup: (body) => {
        linked = true;
        createCall = body;
        return { status: 201, body: { id: "group-id", name: body.name, course_ids: body.course_ids } };
      },
    });
    await openCourseLevels(page);

    const card = page.getByRole("article", { name: "High confidence course link: ENG-A and ENG-B" });
    await expect(card.getByText(/Linking shares enrollment and absence rules/)).toBeVisible();
    await card.getByRole("button", { name: "Link as same course" }).focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("status").filter({ hasText: "ENG-A and ENG-B were linked" })).toBeVisible();
    await expect(page.getByText("No high-confidence course links found.")).toBeVisible();
    expect(createCall).not.toBeNull();
    expect(createCall).toMatchObject({
      kind: "continuation",
      course_ids: ["source-course-id", "partial-course-id"],
      rule_source_course_id: "source-course-id",
      suggestion_precondition: {
        configured_course_id: "source-course-id",
        unconfigured_course_id: "partial-course-id",
        evidence_fingerprint: "a".repeat(64),
        detector_version: DETECTOR_VERSION,
      },
    });
    await expect.poll(async () => page.evaluate(() => document.activeElement?.id)).toBe("course-link-suggestions-heading");
  });

  test("dismissal sends the evidence and idempotency key", async ({ page }) => {
    let dismissed = false;
    const calls = await installCourseLinkRoutes(page, {
      mode: "confirmation",
      getSuggestions: () => response(dismissed ? [] : [highSuggestion()]),
      dismiss: (_body) => {
        dismissed = true;
        return { status: 200, body: { dismissed: true, already_dismissed: false } };
      },
    });
    await openCourseLevels(page);

    await page.getByRole("article", { name: "High confidence course link: ENG-A and ENG-B" })
      .getByRole("button", { name: "Not the same" }).click();
    await expect(page.getByRole("status").filter({ hasText: "Suggestion for ENG-A and ENG-B dismissed" })).toBeVisible();
    await expect(page.getByText("No high-confidence course links found.")).toBeVisible();
    expect(calls.dismissRequests).toHaveLength(1);
    expect(calls.dismissRequests[0].key).toMatch(/^[0-9a-f-]{36}$/i);
    expect(calls.dismissRequests[0].body).toEqual({
      configured_course_id: "source-course-id",
      unconfigured_course_id: "partial-course-id",
      evidence_fingerprint: "a".repeat(64),
      detector_version: DETECTOR_VERSION,
    });
  });

  test("stale evidence reloads and requires a fresh confirmation", async ({ page }) => {
    let createCall = 0;
    let suggestion = highSuggestion();
    const calls = await installCourseLinkRoutes(page, {
      mode: "confirmation",
      getSuggestions: () => response([suggestion]),
      createGroup: (body) => {
        createCall += 1;
        if (createCall === 1) {
          suggestion = highSuggestion({
            evidence_fingerprint: "c".repeat(64),
            reason_codes: ["same_subject", "normalized_name_match", "period_unknown", "compatible_slots", "equal_slot_durations", "unique_partner"],
          });
          return { status: 409, body: { code: "suggestion_changed", message: "This suggestion changed." } };
        }
        return { status: 201, body: { id: "group-id", name: body.name, course_ids: body.course_ids } };
      },
    });
    await openCourseLevels(page);

    await page.getByRole("article", { name: "High confidence course link: ENG-A and ENG-B" })
      .getByRole("button", { name: "Link as same course" }).click();
    await expect(page.getByText("This suggestion changed. The latest evidence is being loaded; review it before deciding again.")).toBeVisible();
    await expect(page.getByText("Teaching period is not confirmed")).toBeVisible();

    await page.getByRole("article", { name: "High confidence course link: ENG-A and ENG-B" })
      .getByRole("button", { name: "Link as same course" }).click();
    await expect.poll(() => calls.createRequests.length).toBe(2);
    expect(calls.createRequests[0].body.suggestion_precondition.evidence_fingerprint).toBe("a".repeat(64));
    expect(calls.createRequests[1].body.suggestion_precondition.evidence_fingerprint).toBe("c".repeat(64));
  });

  test("retries a failed confirmation with the same idempotency key", async ({ page }) => {
    let createCount = 0;
    const calls = await installCourseLinkRoutes(page, {
      mode: "confirmation",
      getSuggestions: () => response(createCount > 1 ? [] : [highSuggestion()]),
      createGroup: (body) => {
        createCount += 1;
        if (createCount === 1) return { status: 503, body: { code: "unavailable", message: "Temporary failure" } };
        return { status: 201, body: { id: "group-id", name: body.name, course_ids: body.course_ids } };
      },
    });
    await openCourseLevels(page);

    const confirm = page.getByRole("article", { name: "High confidence course link: ENG-A and ENG-B" })
      .getByRole("button", { name: "Link as same course" });
    await confirm.click();
    await expect(page.getByRole("article", { name: "High confidence course link: ENG-A and ENG-B" })).toBeVisible();
    await confirm.click();

    await expect(page.getByText("No high-confidence course links found.")).toBeVisible();
    expect(calls.createRequests).toHaveLength(2);
    expect(calls.createRequests[0].key).not.toBe("");
    expect(calls.createRequests[1].key).toBe(calls.createRequests[0].key);
    expect(calls.createRequests[1].body).toEqual(calls.createRequests[0].body);
  });

  test("reports refresh failure separately after a successful confirmation", async ({ page }) => {
    let created = false;
    let listCount = 0;
    await installCourseLinkRoutes(page, {
      mode: "confirmation",
      getSuggestions: () => {
        listCount += 1;
        if (listCount > 1) throw new Error("temporary list failure");
        return response([highSuggestion()]);
      },
      createGroup: (body) => {
        created = true;
        return { status: 201, body: { id: "group-id", name: body.name, course_ids: body.course_ids } };
      },
    });
    await openCourseLevels(page);

    await page.getByRole("article", { name: "High confidence course link: ENG-A and ENG-B" })
      .getByRole("button", { name: "Link as same course" }).click();
    await expect(page.getByRole("status").filter({ hasText: "ENG-A and ENG-B were linked" })).toBeVisible();
    await expect(page.getByRole("alert").filter({ hasText: "Your decision was saved, but the suggestions could not be refreshed." })).toBeVisible();
    expect(created).toBe(true);
    await expect(page.getByText("No high-confidence course links found.")).toBeVisible();
  });

  test("paginates and does not append an old-filter page after the review toggle changes", async ({ page }) => {
    const high = highSuggestion();
    const review = reviewSuggestion();
    const stalePage = highSuggestion({
      configured_source: course({ id: "stale-source", code: "OLD-A" }),
      unconfigured_course: course({ id: "stale-partial", code: "OLD-B", level: null }),
    });
    const calls = await installCourseLinkRoutes(page, {
      getSuggestions: async ({ includeReview, cursor }) => {
        if (cursor) {
          await page.waitForTimeout(250);
          return response([stalePage]);
        }
        return includeReview
          ? response([high, review])
          : response([high], true, "cursor-first-page");
      },
    });
    await openCourseLevels(page);

    const cursorRequest = page.waitForRequest((request: Request) => new URL(request.url()).searchParams.has("cursor"));
    await page.getByRole("button", { name: "Load more suggestions" }).click();
    await cursorRequest;
    await page.getByRole("checkbox", { name: "Show review candidates" }).check();
    await expect(page.getByRole("article", { name: "Review course link: ENG-C and ENG-D" })).toBeVisible();
    await page.waitForTimeout(400);

    expect(calls.listRequests.some((url) => url.searchParams.get("cursor") === "cursor-first-page")).toBe(true);
    await expect(page.getByRole("article", { name: "High confidence course link: OLD-A and OLD-B" })).toHaveCount(0);
    await expect(page.getByRole("article", { name: "High confidence course link: ENG-A and ENG-B" })).toHaveCount(1);
  });

  test("suggestion failure leaves Course Levels usable", async ({ page }) => {
    await installCourseLinkRoutes(page, {
      getSuggestions: () => response([]),
    });
    await page.unroute("**/api/v1/**");
    await page.route("**/api/v1/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (route.request().method() === "GET" && path === "/api/v1/admin/course-link-suggestions") {
        await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ code: "internal", message: "Suggestion service unavailable" }) });
        return;
      }
      if (path === "/api/v1/me") {
        await route.fulfill({ contentType: "application/json", body: JSON.stringify({ id: "admin-id", username: "Admin", role: "Admin" }) });
        return;
      }
      if (path === "/api/v1/admin/course-levels") {
        await route.fulfill({ contentType: "application/json", body: JSON.stringify({ items: [], total: 0, limit: 100, offset: 0 }) });
        return;
      }
      await route.fulfill({ contentType: "application/json", body: "[]" });
    });
    await page.goto("/course-levels");

    await expect(page.getByText(/Suggestions are unavailable/)).toBeVisible();
    await expect(page.getByText("Course level management is available in the manager.")).toBeVisible();
  });
});
