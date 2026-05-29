import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AbsenceSettings from "../AbsenceSettings";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const SETTINGS = {
  form: {
    max_date_range_days: 30,
    require_reason: true,
    reason_categories: [{ value: "medical", label: "Medical" }],
    allow_free_text_reason: true,
    intro_text: "Tell us what happened.",
    confirmation_text: "Submission received.",
  },
  sit_in: { auto_resolve_enabled: true, zoom_description: "Zoom class", max_sessions_per_absence: 10 },
  student_self_service: { can_view_own: false, can_cancel_own: false },
};

describe("Absence settings", () => {
  beforeEach(() => vi.clearAllMocks());

  it("loads and saves public form rules without deployment", async () => {
    mockApiJson.mockResolvedValueOnce(SETTINGS).mockResolvedValueOnce(SETTINGS);
    render(<ToastProvider><AbsenceSettings /></ToastProvider>);
    const user = userEvent.setup();

    const maxDays = await screen.findByLabelText(/maximum date range/i);
    await user.clear(maxDays);
    await user.type(maxDays, "45");
    await user.click(screen.getByRole("button", { name: /save settings/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/absence-settings",
        expect.objectContaining({ method: "PUT", body: expect.stringContaining('"max_date_range_days":45') }),
      );
    });
  });
});
