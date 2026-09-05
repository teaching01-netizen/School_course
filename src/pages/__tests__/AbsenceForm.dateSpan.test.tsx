import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  renderPublicAbsenceForm,
  completeToClasses,
  selectClass,
  openDayOffset,
} from "./helpers/absenceFormHarness";
import {
  PUBLIC_FORM_CONFIG,
  PUBLIC_FORM_SESSIONS,
  relativeDateKey,
  relativeISO,
} from "./fixtures/absenceFormFixtures";
import type { SessionsInRangeResponse } from "@/types";

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

/** A short report span (7 days) with Mathematics classes 8 days apart. */
const SPAN_CONFIG = {
  ...PUBLIC_FORM_CONFIG,
  form: { ...PUBLIC_FORM_CONFIG.form, max_date_range_days: 7 },
};

const SPAN_SESSIONS: SessionsInRangeResponse = {
  subjects: [
    {
      ...PUBLIC_FORM_SESSIONS.subjects[0],
      sessions: [
        {
          id: "session-math-span-a",
          start_at: relativeISO(16, 0, 0),
          end_at: relativeISO(18, 0, 0),
          date: relativeDateKey(0),
          already_absent: false,
        },
        {
          id: "session-math-span-b",
          start_at: relativeISO(16, 0, 8),
          end_at: relativeISO(18, 0, 8),
          date: relativeDateKey(8),
          already_absent: false,
        },
      ],
    },
    PUBLIC_FORM_SESSIONS.subjects[1],
  ],
};

describe("AbsenceForm — early date-span limit", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockNavigate.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("explains the maximum span when adding a class, and preserves the existing selection", async () => {
    renderPublicAbsenceForm(mockApiJson, { config: SPAN_CONFIG, sessions: SPAN_SESSIONS });
    const user = userEvent.setup();
    await completeToClasses(user);

    // Select the first Mathematics day; the selection summary shows one day.
    await selectClass(user, "Mathematics");
    expect(screen.getByText(/1 class day selected/i)).toBeInTheDocument();

    // The same course eight days later is inside the report span, so the tap
    // is rejected on the spot with a reason — not at submission time.
    await openDayOffset(user, 8);
    const laterRow = screen.getByRole("checkbox", { name: /mathematics/i });
    await user.click(laterRow);
    expect(laterRow).not.toBeChecked();

    const notices = [...screen.queryAllByRole("status"), ...screen.queryAllByRole("alert")];
    const notice = notices.find((node) => node.textContent?.includes("further than 7 days"));
    expect(notice).toBeInTheDocument();
    expect(notice?.textContent).toMatch(/separate absence/i);

    // The earlier selection is untouched: still one selected class day on the
    // original date, and the report can proceed.
    expect(screen.getByText(/1 class day selected/i)).toBeInTheDocument();
    const todayCell = document.querySelector<HTMLElement>(`[data-date-key="${relativeDateKey(0)}"]`);
    expect(todayCell).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: /^continue$/i })).toBeEnabled();
  });
});
