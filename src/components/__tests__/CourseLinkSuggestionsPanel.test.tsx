import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import CourseLinkSuggestionsPanel from "../CourseLinkSuggestionsPanel";
import { ToastProvider } from "../../hooks/useToast";
import type { CourseLinkSuggestion, CourseLinkSuggestionCourse, CourseLinkSuggestionsResponse } from "@/features/courses/types";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function course(overrides: Partial<CourseLinkSuggestionCourse>): CourseLinkSuggestionCourse {
  return {
    id: "course",
    code: "ENG",
    name: "English",
    subject_id: "subject",
    subject_code: "ENG",
    subject_name: "English",
    level: 1,
    cycle_id: "cycle",
    cycle_label: "C1",
    cycle_start_date: null,
    cycle_end_date: null,
    root_course_group_id: "root",
    root_course_group_name: "English",
    session_count: 4,
    session_date_from: "2026-01-01",
    session_date_to: "2026-02-01",
    slots: ["1|09:00|5400"],
    teachers: [],
    ...overrides,
  } as CourseLinkSuggestionCourse;
}

function suggestion(code: string, confidence: "high" | "review", reasons: string[]): CourseLinkSuggestion {
  return {
    configured_source: course({ id: `${code}-src`, code: `${code}-A` }),
    unconfigured_course: course({ id: `${code}-part`, code: `${code}-B`, level: null }),
    confidence,
    reason_codes: reasons,
    ambiguity_count: 1,
    evidence_fingerprint: `fp-${code}`,
  };
}

function page(items: CourseLinkSuggestion[]): CourseLinkSuggestionsResponse {
  return {
    enabled: true,
    mode: "confirmation",
    confirmation_enabled: true,
    items,
    has_more: false,
    next_cursor: null,
    evaluated_at: "2026-10-06T00:00:00Z",
    detector_version: "v1",
    institute_timezone: "Asia/Bangkok",
  };
}

function renderPanel() {
  render(
    <MemoryRouter>
      <ToastProvider>
        <CourseLinkSuggestionsPanel />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("CourseLinkSuggestionsPanel", () => {
  let items: CourseLinkSuggestion[];

  beforeEach(() => {
    items = [
      suggestion("ONE", "high", ["same_subject", "compatible_slots"]),
      suggestion("TWO", "high", ["same_subject", "period_unknown"]),
    ];
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string) => {
      if (path.startsWith("/api/v1/admin/course-link-suggestions?")) {
        const search = new URL(path, "http://localhost").searchParams.get("q")?.toLowerCase() ?? "";
        return Promise.resolve(page(items.filter((item) => `${item.configured_source.code} ${item.unconfigured_course.code}`.toLowerCase().includes(search))));
      }
      if (path === "/api/v1/admin/course-link-suggestions/dismiss") {
        items = items.filter((item) => item.configured_source.code !== "ONE-A");
        return new Promise((resolve) => setTimeout(() => resolve({ dismissed: true, already_dismissed: false }), 20));
      }
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("separates supporting evidence from evidence that needs review", async () => {
    renderPanel();
    const card = await screen.findByRole("article", { name: "High confidence course link: TWO-A and TWO-B" });
    expect(within(within(card).getByRole("list", { name: "Supports a match" })).getByText("Same subject")).toBeInTheDocument();
    expect(within(within(card).getByRole("list", { name: "Needs review" })).getByText("Teaching period is not confirmed")).toBeInTheDocument();
    expect(within(card).getByRole("button", { name: "Link as same course" })).toHaveAccessibleDescription(/Linking shares enrollment and absence rules/);
  });

  it("disables every action while one runs, announces the result, and focuses the next suggestion", async () => {
    const user = userEvent.setup();
    renderPanel();
    const first = await screen.findByRole("article", { name: "High confidence course link: ONE-A and ONE-B" });
    // The polite live region is mounted (empty) before any decision is made.
    const status = screen.getByRole("status");
    expect(status).toBeEmptyDOMElement();

    await user.click(within(first).getByRole("button", { name: "Not the same" }));
    expect(within(first).getByRole("button", { name: "Not the same" })).toHaveAttribute("aria-busy", "true");
    expect(within(first).getByRole("button", { name: "Link as same course" })).not.toHaveAttribute("aria-busy", "true");
    expect(within(first).getByRole("button", { name: "Link as same course" })).toBeDisabled();

    await waitFor(() => expect(status).toHaveTextContent("Suggestion for ONE-A and ONE-B dismissed."));
    expect(screen.queryByRole("article", { name: /ONE-A/ })).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("article", { name: "High confidence course link: TWO-A and TWO-B" })).toHaveFocus());
  });

  it("links review candidates to prefilled manual linking", async () => {
    items = [suggestion("REV", "review", ["same_subject", "period_unknown"])];
    renderPanel();
    const link = await screen.findByRole("link", { name: "Review with manual linking" });
    const url = new URL(link.getAttribute("href") ?? "", "http://localhost");
    expect(url.pathname).toBe("/courses/create");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      mode: "merge",
      continuation: "1",
      courses: "REV-src,REV-part",
      source: "REV-src",
      name: "Same course: REV-A + REV-B",
    });
  });

  it("searches suggestions on the server and explains an empty result", async () => {
    const user = userEvent.setup();
    renderPanel();
    await screen.findByRole("article", { name: /ONE-A/ });
    const search = screen.getByRole("searchbox", { name: "Search course link suggestions" });

    await user.type(search, " two-b ");
    await waitFor(() => expect(screen.queryByRole("article", { name: /ONE-A/ })).not.toBeInTheDocument());
    expect(screen.getByRole("article", { name: "High confidence course link: TWO-A and TWO-B" })).toBeInTheDocument();
    expect(mockApiJson).toHaveBeenCalledWith(expect.stringContaining("q=two-b"), expect.anything());

    await user.clear(search);
    await user.type(search, "nothing");
    expect(await screen.findByText("No suggestions match “nothing”.")).toBeInTheDocument();
  });
});
