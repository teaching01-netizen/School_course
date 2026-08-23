import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { apiJson } from "@/api/client";
import CrossStudyAssignmentList from "../CrossStudyAssignmentList";
import type { AssignmentListResponse } from "@/types/crossStudy";

vi.mock("@/api/client", () => ({
  apiJson: vi.fn(),
}));

const mockedApiJson = vi.mocked(apiJson);

const assignment = (wcode: string, overrides: Partial<AssignmentListResponse["assignments"][number]> = {}) => ({
  id: `id-${wcode}`,
  wcode,
  full_name: `Student ${wcode}`,
  dest_course_a_name: "Course A",
  dest_course_a_id: "course-a",
  dest_course_b_name: "Course B",
  dest_course_b_id: "course-b",
  status: "active",
  updated_at: "2026-08-23T03:00:00Z",
  ...overrides,
});

const makeResponse = (assignments: ReturnType<typeof assignment>[], total: number): AssignmentListResponse => ({
  assignments,
  total,
  review_count: 0,
});

beforeEach(() => {
  mockedApiJson.mockReset();
});

function urlFor(call: unknown): string {
  const args = (call as [string, RequestInit?])[0];
  return typeof args === "string" ? args : "";
}

describe("CrossStudyAssignmentList", () => {
  it("sends the query only after typing settles (debounce)", async () => {
    const user = userEvent.setup();
    mockedApiJson.mockResolvedValue(makeResponse([assignment("w1")], 1));

    render(<CrossStudyAssignmentList refreshKey={0} onSelectWCode={() => {}} />);

    // initial full-list load
    await waitFor(() => expect(mockedApiJson).toHaveBeenCalledTimes(1));
    expect(urlFor(mockedApiJson.mock.calls[0])).toContain("/api/v1/cross-study/assignments?");

    mockedApiJson.mockClear();

    const input = screen.getByLabelText("Search");
    await user.type(input, "w260010");

    // typing fires no requests immediately
    expect(mockedApiJson).not.toHaveBeenCalled();

    // one request after the debounce window, with the full query and no stray calls
    await waitFor(() => expect(mockedApiJson).toHaveBeenCalledTimes(1), { timeout: 1000 });
    const url = urlFor(mockedApiJson.mock.calls[0]);
    expect(url).toContain("q=w260010");
  });

  it("fires no request for short queries (min length), keeping the full list", async () => {
    const user = userEvent.setup();
    mockedApiJson.mockResolvedValue(makeResponse([assignment("w1")], 1));

    render(<CrossStudyAssignmentList refreshKey={0} onSelectWCode={() => {}} />);
    await waitFor(() => expect(mockedApiJson).toHaveBeenCalledTimes(1));

    mockedApiJson.mockClear();

    const input = screen.getByLabelText("Search");
    await user.type(input, "w");

    // short input never triggers a request — not even a full-list refetch
    await new Promise((resolve) => setTimeout(resolve, 700));
    expect(mockedApiJson).not.toHaveBeenCalled();
  });

  it("renders an error state (not 'no assignments') when the request fails", async () => {
    mockedApiJson.mockRejectedValueOnce(new Error("network down"));

    render(<CrossStudyAssignmentList refreshKey={0} onSelectWCode={() => {}} />);

    await screen.findByRole("alert");
    expect(screen.getByText(/could not load assignments/i)).toBeTruthy();
    expect(screen.queryByText(/no cross-study assignments yet/i)).toBeNull();
  });

  it("keeps newer results when an older request resolves later (abort + request identity)", async () => {
    const user = userEvent.setup();

    let resolveFirst: (v: AssignmentListResponse) => void = () => {};
    const firstRequest = new Promise<AssignmentListResponse>((resolve) => {
      resolveFirst = resolve;
    });

    // Request 1 (full list) is slow and will be aborted when the query fires.
    mockedApiJson.mockImplementationOnce((_url, init?: RequestInit) =>
      new Promise((resolve, reject) => {
        if (init?.signal) {
          init.signal.addEventListener("abort", () => {
            reject(new DOMException("Aborted", "AbortError"));
          });
        }
        resolveFirst = resolve;
        // keep a reference so the test can resolve it later
        (firstRequest as unknown as { resolve?: (value: AssignmentListResponse) => void }).resolve = resolve;
      }),
    );

    render(<CrossStudyAssignmentList refreshKey={0} onSelectWCode={() => {}} />);

    // Request 2 (after typing) resolves fast with results for the query.
    mockedApiJson.mockResolvedValueOnce(makeResponse([assignment("w999999", { full_name: "Newest Result" })], 1));

    const input = screen.getByLabelText("Search");
    await user.type(input, "w999999");

    await waitFor(() => expect(mockedApiJson).toHaveBeenCalledTimes(2), { timeout: 1000 });

    // Now the stale first request tries to resolve late.
    resolveFirst(makeResponse([assignment("w1", { full_name: "Stale Result" })], 1));

    // The rendered list must show the query results, not the stale full list.
    await waitFor(() => expect(screen.getByText("Newest Result")).toBeTruthy());
    expect(screen.queryByText("Stale Result")).toBeNull();
  });

  it("paginates with Previous/Next and a total count", async () => {
    const user = userEvent.setup();
    mockedApiJson.mockResolvedValue(makeResponse([assignment("w1"), assignment("w2")], 3));

    render(<CrossStudyAssignmentList refreshKey={0} onSelectWCode={() => {}} />);

    await waitFor(() => expect(mockedApiJson).toHaveBeenCalledTimes(1));
    const firstUrl = urlFor(mockedApiJson.mock.calls[0]);
    expect(firstUrl).toContain("limit=25");
    expect(firstUrl).toContain("offset=0");
    expect(screen.getByText("3 assignments")).toBeTruthy();

    const next = screen.getByRole("button", { name: "Next" });
    expect((next as HTMLButtonElement).disabled).toBe(false);

    await user.click(next);
    await waitFor(() => expect(mockedApiJson).toHaveBeenCalledTimes(2));
    expect(urlFor(mockedApiJson.mock.calls[1])).toContain("offset=25");
  });
});
