import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import OperationsCalendar from "../OperationsCalendar";
import { ToastProvider } from "../../hooks/useToast";
import { queryClient } from "../../query/cache";

const mockApiJson = vi.hoisted(() => vi.fn());
let calendarResponse: unknown = { sessions: [], absence_days: [] };

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderPage(initialEntry = "/calendar?view=month") {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <ToastProvider>
        <OperationsCalendar />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("OperationsCalendar institute-zone today badge", () => {
  const originalTimeZone = process.env.TZ;

  beforeAll(() => {
    // Browser-local timezone differs from the institute zone (Bangkok).
    process.env.TZ = "UTC";
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2026-06-02T12:00:00Z"));
  });

  afterAll(() => {
    vi.useRealTimers();
    process.env.TZ = originalTimeZone;
  });

  beforeEach(() => {
    queryClient.clear();
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((url: string) => {
      if (url === "/api/v1/meta/time") {
        // 18:00 UTC June 2 == 01:00 Bangkok June 3, so the institute's today
        // is a different calendar day than the browser's.
        return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-06-02T18:00:00Z" });
      }
      return Promise.resolve(calendarResponse);
    });
  });

  it("marks the institute-zone today, not the browser-local today", async () => {
    renderPage("/calendar?view=month&show=all");

    await screen.findByText("Calendar");
    const june2 = screen.getByRole("button", { name: /open details for tuesday, 2 june 2026/i });
    const june3 = screen.getByRole("button", { name: /open details for wednesday, 3 june 2026/i });

    expect(within(june3).getByText("3")).toHaveClass("bg-[var(--color-wi-primary)]");
    expect(within(june2).getByText("2")).not.toHaveClass("bg-[var(--color-wi-primary)]");
  });

  it("anchors the initial month to the institute-zone month even in a different browser zone", async () => {
    renderPage("/calendar?view=month&show=all");

    await screen.findByText("Calendar");
    expect(screen.getByText("June 2026")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /open details for wednesday, 3 june 2026/i })).toBeInTheDocument();
  });
});