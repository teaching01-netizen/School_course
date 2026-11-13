import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CourseLevels from "../CourseLevels";
import {
  CONTINUATION_GROUP,
  CONTINUATION_GROUP_OVERRIDE,
  EMPTY_SUGGESTIONS,
  GROUPS_WITH_RULE,
  LINKED_COURSES,
  MERGED_GROUP,
  RULES,
  renderWithProviders,
  setupCourseLevelsApi,
} from "./courseLevelsHarness";
import type { CourseLinkSuggestion, CourseLinkSuggestionCourse, CourseLinkSuggestionsResponse } from "@/features/courses/types";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function suggestionCourse(overrides: Partial<CourseLinkSuggestionCourse>): CourseLinkSuggestionCourse {
  return {
    id: "course",
    code: "MATH-101",
    name: "Algebra I",
    subject_id: "s1",
    subject_code: "MATH",
    subject_name: "Mathematics",
    level: 2,
    cycle_id: "cy1",
    cycle_label: "Cycle 2025-01",
    cycle_start_date: null,
    cycle_end_date: null,
    root_course_group_id: "g1",
    root_course_group_name: "SAT Math",
    session_count: 4,
    session_date_from: "2026-01-01",
    session_date_to: "2026-02-01",
    slots: ["1|09:00|5400"],
    teachers: [],
    ...overrides,
  };
}

function linkSuggestion(): CourseLinkSuggestion {
  return {
    configured_source: suggestionCourse({ id: "c1", code: "MATH-101" }),
    unconfigured_course: suggestionCourse({ id: "c9", code: "MATH-901", name: "Algebra I (split)", level: null }),
    confidence: "high",
    reason_codes: ["same_subject"],
    ambiguity_count: 1,
    evidence_fingerprint: "fp-linked",
  };
}

function suggestionPage(items: CourseLinkSuggestion[]): CourseLinkSuggestionsResponse {
  return {
    enabled: true,
    mode: "confirmation",
    confirmation_enabled: true,
    items,
    has_more: false,
    next_cursor: null,
    evaluated_at: "2026-01-01T00:00:00Z",
    detector_version: "v1",
    institute_timezone: "Asia/Bangkok",
  };
}

describe("CourseLevels linked courses", () => {
  beforeEach(() => { mockApiJson.mockReset(); });

  it("lets continuation members edit one shared level", async () => {
    setupCourseLevelsApi(mockApiJson, { courses: LINKED_COURSES, groups: GROUPS_WITH_RULE, mergeGroups: [CONTINUATION_GROUP] });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });

    const sourceRow = within(manager).getByText("MATH-101").closest("tr") as HTMLElement;
    const splitRow = within(manager).getByText("MATH-901").closest("tr") as HTMLElement;

    const sourceInput = within(sourceRow).getByRole("spinbutton", { name: "Level for MATH-101" });
    const splitInput = within(splitRow).getByRole("spinbutton", { name: "Level for MATH-901" });
    expect(sourceInput).toBeEnabled();
    expect(sourceInput).toHaveValue(2);
    expect(within(sourceRow).getByText(/Shared level for both linked course IDs/)).toBeInTheDocument();
    expect(within(sourceRow).getByText("Rule source (linked)")).toBeInTheDocument();
    expect(splitInput).toBeEnabled();
    expect(splitInput).toHaveValue(2);
    expect(within(splitRow).getByText(/Shared level for both linked course IDs/)).toBeInTheDocument();
    expect(within(splitRow).getByRole("link", { name: "Manage link in Sit-in Rules" })).toHaveAttribute("href", "/admin/operations?tab=sit-in-rules");
    expect(within(splitRow).getByText("Follows MATH-101")).toBeInTheDocument();
    expect(within(manager).getByText("2 linked courses in this group")).toBeInTheDocument();
  });

  it("allows editing a continuation level when the link overrides its source", async () => {
    setupCourseLevelsApi(mockApiJson, { courses: LINKED_COURSES, groups: GROUPS_WITH_RULE, mergeGroups: [CONTINUATION_GROUP_OVERRIDE] });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });

    const sourceRow = within(manager).getByText("MATH-101").closest("tr") as HTMLElement;
    const sourceInput = within(sourceRow).getByRole("spinbutton", { name: "Level for MATH-101" });
    expect(sourceInput).toBeEnabled();
    expect(sourceInput).toHaveValue(4);
    expect(within(sourceRow).getByText(/Saving overrides the source course's level for this link/)).toBeInTheDocument();
    expect(within(sourceRow).getByRole("button", { name: "Save level for MATH-101" })).toBeDisabled();
  });

  it("treats a merged course without a rule source as config-controlled and read-only", async () => {
    setupCourseLevelsApi(mockApiJson, { courses: LINKED_COURSES, groups: GROUPS_WITH_RULE, mergeGroups: [MERGED_GROUP] });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });

    const firstRow = within(manager).getByText("MATH-101").closest("tr") as HTMLElement;
    const splitRow = within(manager).getByText("MATH-901").closest("tr") as HTMLElement;
    expect(within(firstRow).getByRole("spinbutton", { name: "Level for MATH-101" })).toBeDisabled();
    expect(within(firstRow).getByRole("spinbutton", { name: "Level for MATH-101" })).toHaveValue(2);
    expect(within(firstRow).getByText(/managed by the merged course configuration/)).toBeInTheDocument();
    expect(within(splitRow).getByRole("spinbutton", { name: "Level for MATH-901" })).toBeDisabled();
  });

  it("keeps the source cycle authoritative when the link source has no cycle", async () => {
    setupCourseLevelsApi(mockApiJson, {
      courses: LINKED_COURSES,
      groups: GROUPS_WITH_RULE,
      mergeGroups: [{ ...CONTINUATION_GROUP, cycle_id: null, cycle_label: null }],
    });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });

    const sourceRow = within(manager).getByText("MATH-101").closest("tr") as HTMLElement;
    expect(within(sourceRow).getByRole("spinbutton", { name: "Level for MATH-101" })).toHaveValue(2);
    expect(within(sourceRow).getByText("No cycle")).toBeInTheDocument();
    expect(within(sourceRow).queryByText("Cycle 2025-01")).not.toBeInTheDocument();
    expect(within(manager).getAllByText("Needs attention").length).toBeGreaterThan(0);
    expect(within(manager).queryByText("Ready")).not.toBeInTheDocument();
    expect(within(manager).getByText("A level is set without a cycle")).toBeInTheDocument();
    const setupLinks = within(manager).getAllByRole("link", { name: /Open course setup for MATH-101/ });
    expect(setupLinks.length).toBeGreaterThan(0);
    for (const link of setupLinks) expect(link).toHaveAttribute("href", "/courses/c1");
  });

  it("names the canonical linked course when a duplicate level is rejected", async () => {
    setupCourseLevelsApi(mockApiJson, { courses: LINKED_COURSES, groups: GROUPS_WITH_RULE, mergeGroups: [CONTINUATION_GROUP] });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });

    const row = within(manager).getByText("MATH-201").closest("tr") as HTMLElement;
    const input = within(row).getByRole("spinbutton", { name: "Level for MATH-201" });
    fireEvent.change(input, { target: { value: "2" } });
    fireEvent.click(within(row).getByRole("button", { name: "Save level for MATH-201" }));

    expect(await within(row).findByText(/Level 2 is already used by MATH-101/)).toBeInTheDocument();
    expect(within(row).queryByText(/MATH-901/)).not.toBeInTheDocument();
    expect(mockApiJson).not.toHaveBeenCalledWith(
      "/api/v1/admin/courses/c2/level",
      expect.objectContaining({ method: "PUT" }),
    );
  });

  it("disables editing when merge metadata fails and recovers through retry", async () => {
    setupCourseLevelsApi(mockApiJson, { courses: LINKED_COURSES, groups: GROUPS_WITH_RULE, failures: { mergeGroups: "Merge service unavailable" } });
    const user = userEvent.setup();
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });

    expect(within(manager).getByText("Linked course settings could not be loaded.")).toBeInTheDocument();
    expect(within(manager).getByText(/Level editing is disabled and readiness is not shown/)).toBeInTheDocument();
    expect(within(manager).queryByText("Ready")).not.toBeInTheDocument();
    const input = within(manager).getByRole("spinbutton", { name: "Level for MATH-101" });
    expect(input).toBeDisabled();
    expect(within(manager).getByRole("button", { name: "Add level" })).toBeDisabled();

    setupCourseLevelsApi(mockApiJson, { courses: LINKED_COURSES, groups: GROUPS_WITH_RULE });
    await user.click(within(manager).getByRole("button", { name: "Retry linked course settings" }));
    await waitFor(() => expect(input).toBeEnabled());
    expect(within(manager).queryByText("Linked course settings could not be loaded.")).not.toBeInTheDocument();
  });

  it("refreshes linked metadata after a suggestion link and stops asserting the draft as the effective level", async () => {
    let linked = false;
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      if (method === "GET" && path.startsWith("/api/v1/admin/course-levels")) return Promise.resolve(LINKED_COURSES);
      if (method === "GET" && path === "/api/v1/admin/root-course-groups") return Promise.resolve(GROUPS_WITH_RULE);
      if (method === "GET" && path === "/api/v1/admin/sit-in-rules") return Promise.resolve(RULES);
      if (method === "GET" && path === "/api/v1/admin/course-merge-groups") return Promise.resolve(linked ? [CONTINUATION_GROUP] : []);
      if (method === "GET" && path.startsWith("/api/v1/admin/course-link-suggestions")) {
        return Promise.resolve(linked ? EMPTY_SUGGESTIONS : suggestionPage([linkSuggestion()]));
      }
      if (method === "POST" && path === "/api/v1/course-groups") {
        linked = true;
        return Promise.resolve({ id: "mg1", name: "Same course: MATH-101 + MATH-901", course_ids: ["c1", "c9"] });
      }
      return Promise.resolve({ ok: true });
    });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const user = userEvent.setup();

    const sourceInput = within(manager).getByRole("spinbutton", { name: "Level for MATH-101" });
    fireEvent.change(sourceInput, { target: { value: "5" } });

    const card = await screen.findByRole("article", { name: "High confidence course link: MATH-101 and MATH-901" });
    await user.click(within(card).getByRole("button", { name: "Link as same course" }));

    await waitFor(() => expect(within(manager).getByText("Follows MATH-101")).toBeInTheDocument());
    expect(sourceInput).toBeEnabled();
    expect(sourceInput).toHaveValue(2);
    expect(mockApiJson.mock.calls.filter(([path]) => path === "/api/v1/admin/course-merge-groups").length).toBeGreaterThanOrEqual(2);
    expect(mockApiJson.mock.calls.filter(([path]) => String(path).startsWith("/api/v1/admin/course-levels")).length).toBeGreaterThanOrEqual(2);
  });
});
