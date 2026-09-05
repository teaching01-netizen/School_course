import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  renderPublicAbsenceForm,
  completeToClasses,
  selectClass,
  expandCalendar,
} from "./helpers/absenceFormHarness";
import { relativeDateKey } from "./fixtures/absenceFormFixtures";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const mockNavigate = vi.hoisted(() => vi.fn());
vi.mock("react-router-dom", () => ({
  useNavigate: () => mockNavigate,
  useLocation: () => ({ pathname: "/absence" }),
}));

function cellForDateKey(dateKey: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-date-key="${dateKey}"]`);
}

describe("AbsenceForm — schedule calendar keyboard", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockNavigate.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("arrows explore dates without moving the agenda; Enter activates the focused date", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();
    await completeToClasses(user);

    const todayKey = relativeDateKey(0);
    const tomorrowKey = relativeDateKey(1);

    // The calendar opens focused on today — the fixture's Mathematics class.
    expect(screen.getByRole("heading", { name: /^today/i })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: /mathematics/i })).toBeInTheDocument();

    const todayCell = cellForDateKey(todayKey);
    expect(todayCell).not.toBeNull();
    todayCell!.focus();

    await user.keyboard("{ArrowRight}");

    // Focus moved to tomorrow, but the selected day and its agenda are untouched.
    expect(document.activeElement?.getAttribute("data-date-key")).toBe(tomorrowKey);
    expect(screen.getByRole("heading", { name: /^today/i })).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: /physics/i })).not.toBeInTheDocument();

    // Enter is the activation: selection follows, and so does the agenda.
    await user.keyboard("{Enter}");
    await screen.findByRole("heading", { name: /^tomorrow/i });
    expect(screen.getByRole("checkbox", { name: /physics/i })).toBeInTheDocument();
  });

  it("day cells announce the date, the Today marker, and the class count", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();
    await completeToClasses(user);
    await expandCalendar(user);

    const todayKey = relativeDateKey(0);
    const tomorrowKey = relativeDateKey(1);
    const todayLabel = cellForDateKey(todayKey)?.getAttribute("aria-label") ?? "";
    const tomorrowLabel = cellForDateKey(tomorrowKey)?.getAttribute("aria-label") ?? "";

    // The fixture schedules one class today and one tomorrow.
    expect(todayLabel).toMatch(/today\. 1 class$/i);
    expect(tomorrowLabel).toMatch(/1 class$/);
    expect(tomorrowLabel).not.toMatch(/today/i);
    expect(cellForDateKey(todayKey)?.getAttribute("aria-label") ?? "").not.toMatch(/selected/i);
  });

  it("marks a picked class's day as selected with a filled day and a truthful label", async () => {
    renderPublicAbsenceForm(mockApiJson);
    const user = userEvent.setup();
    await completeToClasses(user);

    const todayCell = cellForDateKey(relativeDateKey(0));
    expect(todayCell).not.toBeNull();
    expect(todayCell?.getAttribute("aria-label") ?? "").not.toMatch(/selected/i);
    expect(todayCell).not.toHaveAttribute("aria-pressed");

    await selectClass(user, "Mathematics");

    // The chosen day is announced as selected and carries a filled bubble,
    // independent of which day is currently viewed.
    expect(todayCell?.getAttribute("aria-label") ?? "").toMatch(/selected$/i);
    const bubble = todayCell?.querySelector(".text-white");
    expect(bubble).not.toBeNull();
    expect(screen.getByText(/1 class day selected/i)).toBeInTheDocument();
  });
});
