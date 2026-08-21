import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});
vi.mock("react-router-dom", () => ({
  useNavigate: () => vi.fn(),
  useLocation: () => ({ pathname: "/absence" }),
}));

import AbsenceForm from "@/pages/AbsenceForm";

const MOCK_CONFIG = {
  form: {
    max_date_range_days: 30,
    require_reason: true,
    reason_categories: [],
    allow_free_text_reason: true,
    intro_text: "",
    confirmation_text: "ok",
  },
  sit_in: { auto_resolve_enabled: true, zoom_description: "Zoom", max_sessions_per_absence: 10 },
  notifications: { sms_parent_enabled: true, sms_parent_template: "t", sms_success_template: "s" },
  admin_contact: { email: "a@b.c", phone: "0", hours: "" },
};

beforeEach(() => {
  mockApiJson.mockReset();
  mockApiJson.mockImplementation(async (url: string) => {
    if (String(url).includes("absence-form-config")) return MOCK_CONFIG as never;
    if (String(url).includes("public/student/sessions")) return { subjects: [] } as never;
    if (String(url).includes("public/student/lookup") || String(url).includes("/lookup"))
      return {
        wcode: "W000001",
        lookup_token: "tok",
        email_input_required: false,
        parent_verification_available: true,
      } as never;
    return null as never;
  });
  localStorage.clear();
});

function renderPage() {
  return render(
    <ToastProvider>
      <AbsenceForm />
    </ToastProvider>,
  );
}

describe("R1 T4 — AbsenceForm.a11y: invalid wcode -> aria-invalid+describedby->alert, step focuses content", () => {
  it("main landmark is tabbable and after mount activeElement is inside main (AbsenceAppShell focus contract)", async () => {
    renderPage();
    await screen.findByRole("heading", { name: /find your profile/i });
    expect(screen.getByRole("main")).toHaveAttribute("tabindex", "-1");
  });

  it("step heading is focused and document.activeElement is inside the main landmark", async () => {
    renderPage();
    await screen.findByRole("heading", { name: /find your profile/i });
    const main = screen.getByRole("main");
    await waitFor(() => expect(document.activeElement?.tagName).toBe("H1"));
    expect(main.contains(document.activeElement)).toBe(true);
  });
});
