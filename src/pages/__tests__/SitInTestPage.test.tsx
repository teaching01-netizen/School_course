import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SitInTestPage from "../SitInTestPage";
import { renderWithProviders } from "./helpers";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", () => ({
  apiJson: mockApiJson,
}));

const FORM_CONFIG = {
  form: {
    max_date_range_days: 30,
    min_hours_before_session: 0,
    max_hours_after_session: 48,
    require_reason: false,
    reason_categories: [],
    allow_free_text_reason: true,
    intro_text: "",
    confirmation_text: "",
  },
  sit_in: {
    auto_resolve_enabled: true,
    zoom_description: "",
    max_sessions_per_absence: 10,
  },
};

const STUDENT = {
  student_id: "student-1",
  wcode: "W250389",
  full_name: "John Smith",
  parent_phone: null,
  subjects: [{ id: "subject-1", code: "MATH", name: "Mathematics" }],
};

function localDate(date: Date): string {
  return [
    date.getFullYear(),
    String(date.getMonth() + 1).padStart(2, "0"),
    String(date.getDate()).padStart(2, "0"),
  ].join("-");
}

describe("SitInTestPage", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation(async (path: string) => {
      if (path.includes("absence-form-config")) return FORM_CONFIG;
      if (path.includes("student-lookup")) return STUDENT;
      if (path.includes("sessions-in-range")) return { subjects: [] };
      throw new Error(`Unmocked API call: ${path}`);
    });
  });

  it("disables Search and shows progress while the student lookup is pending", async () => {
    const user = userEvent.setup();
    let resolveLookup: () => void = () => {};
    mockApiJson.mockImplementation(async (path: string) => {
      if (path.includes("absence-form-config")) return FORM_CONFIG;
      if (path.includes("student-lookup")) {
        return new Promise((resolve) => {
          resolveLookup = () => resolve(STUDENT);
        });
      }
      if (path.includes("sessions-in-range")) return { subjects: [] };
      throw new Error(`Unmocked API call: ${path}`);
    });

    renderWithProviders(<SitInTestPage />);

    const searchButton = screen.getByRole("button", { name: "Search" });
    await user.type(screen.getByPlaceholderText("e.g. W250389"), "W250389");
    await user.click(searchButton);

    // While the request is in flight the button must be disabled and show progress.
    expect(screen.getByRole("button", { name: "..." })).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Search" })).toBeNull();

    await act(async () => { resolveLookup(); });

    expect(await screen.findByRole("button", { name: "Search" })).toBeEnabled();
  });

  it("includes the configured 48-hour post-session window in session lookup", async () => {
    const user = userEvent.setup();
    const expectedFrom = new Date();
    expectedFrom.setDate(expectedFrom.getDate() - 2);

    renderWithProviders(<SitInTestPage />);

    await user.type(screen.getByPlaceholderText("e.g. W250389"), "W250389");
    await user.click(screen.getByRole("button", { name: "Search" }));
    await user.click(await screen.findByRole("checkbox", { name: /mathematics/i }));

    await waitFor(() => {
      const call = mockApiJson.mock.calls.find(([path]) => String(path).includes("sessions-in-range"));
      expect(call).toBeDefined();
      const url = new URL(String(call?.[0]), "https://example.test");
      expect(url.searchParams.get("date_from")).toBe(localDate(expectedFrom));
    });
  });
});
