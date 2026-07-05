import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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
    min_hours_before_session: 0,
    max_hours_after_session: 0,
    require_reason: true,
    reason_categories: [{ value: "medical", label: "Medical" }],
    allow_free_text_reason: true,
    intro_text: "Tell us what happened.",
    confirmation_text: "Submission received.",
  },
  sit_in: { auto_resolve_enabled: true, zoom_description: "Zoom class", max_sessions_per_absence: 10 },
  notifications: {
    sms_parent_enabled: true,
    sms_parent_template: "OTP {{code}}",
    sms_success_template: "Saved {{class_name}} {{sit_in_class}}",
    allow_submit_without_otp: false,
  },
  student_self_service: { can_view_own: false, can_cancel_own: false },
};

describe("Absence settings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiJson.mockReset();
  });

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

  it("saves session request timing windows", async () => {
    mockApiJson.mockResolvedValueOnce(SETTINGS).mockResolvedValueOnce(SETTINGS);
    render(<ToastProvider><AbsenceSettings /></ToastProvider>);
    const user = userEvent.setup();

    const minHours = await screen.findByLabelText(/minimum hours before session/i);
    await user.clear(minHours);
    await user.type(minHours, "2");
    const maxHours = screen.getByLabelText(/maximum hours after session/i);
    await user.clear(maxHours);
    await user.type(maxHours, "24");
    await user.click(screen.getByRole("button", { name: /save settings/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/absence-settings",
        expect.objectContaining({
          method: "PUT",
          body: expect.stringContaining('"min_hours_before_session":2'),
        }),
      );
    });
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/absence-settings",
      expect.objectContaining({
        method: "PUT",
        body: expect.stringContaining('"max_hours_after_session":24'),
      }),
    );
  });

  it("keeps the success SMS template after saving settings", async () => {
    mockApiJson.mockResolvedValueOnce(SETTINGS).mockResolvedValueOnce({
      ...SETTINGS,
      notifications: {
        ...SETTINGS.notifications,
        sms_success_template: "Updated {{class_name}} {{sit_in_class}}",
      },
    });
    render(<ToastProvider><AbsenceSettings /></ToastProvider>);
    const user = userEvent.setup();

    const successTemplate = await screen.findByLabelText(/success sms template/i);
    fireEvent.change(successTemplate, { target: { value: "Updated {{class_name}} {{sit_in_class}}" } });
    await user.click(screen.getByRole("button", { name: /save settings/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/absence-settings",
        expect.objectContaining({ method: "PUT", body: expect.stringContaining('"sms_success_template":"Updated {{class_name}} {{sit_in_class}}"') }),
      );
    });
    expect(await screen.findByDisplayValue("Updated {{class_name}} {{sit_in_class}}")).toBeInTheDocument();
  });

  it("renders and saves sms_special_approved_template", async () => {
    const initialSettings = {
      ...SETTINGS,
      notifications: {
        ...SETTINGS.notifications,
        sms_special_approved_template: "",
      },
    };
    const savedSettings = {
      ...SETTINGS,
      notifications: {
        ...SETTINGS.notifications,
        sms_special_approved_template: "Special approved {{nickname}}",
      },
    };
    mockApiJson.mockResolvedValueOnce(initialSettings).mockResolvedValueOnce(savedSettings);
    render(<ToastProvider><AbsenceSettings /></ToastProvider>);
    const user = userEvent.setup();

    const specialTemplate = await screen.findByLabelText(/special approved sms template/i);
    expect(specialTemplate).toBeInTheDocument();
    fireEvent.change(specialTemplate, { target: { value: "Special approved {{nickname}}" } });
    await user.click(screen.getByRole("button", { name: /save settings/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/absence-settings",
        expect.objectContaining({
          method: "PUT",
          body: expect.stringContaining('"sms_special_approved_template":"Special approved {{nickname}}"'),
        }),
      );
    });
  });

  it("does not drop special template when saving parent SMS checkbox toggle", async () => {
    const settingsWithSpecial = {
      ...SETTINGS,
      notifications: {
        ...SETTINGS.notifications,
        sms_special_approved_template: "Keep me please",
      },
    };
    mockApiJson
      .mockResolvedValueOnce(settingsWithSpecial)
      .mockResolvedValueOnce(settingsWithSpecial);
    render(<ToastProvider><AbsenceSettings /></ToastProvider>);
    const user = userEvent.setup();

    // Wait for form to load, then toggle parent SMS checkbox
    await screen.findByLabelText(/maximum date range/i);
    const parentCheckbox = screen.getByRole("checkbox", { name: /enable parent sms/i });
    await user.click(parentCheckbox);
    await user.click(screen.getByRole("button", { name: /save settings/i }));

    await waitFor(() => {
      const putCall = mockApiJson.mock.calls.find(
        (c: unknown[]) => c[0] === "/api/v1/admin/absence-settings" && (c[1] as RequestInit).method === "PUT",
      );
      expect(putCall).toBeTruthy();
      const body = JSON.parse((putCall![1] as RequestInit).body as string);
      expect(body.notifications.sms_special_approved_template).toBe("Keep me please");
    });
  });

  it("does not drop special template when saving normal success template", async () => {
    const settingsWithSpecial = {
      ...SETTINGS,
      notifications: {
        ...SETTINGS.notifications,
        sms_special_approved_template: "Do not drop me",
      },
    };
    mockApiJson
      .mockResolvedValueOnce(settingsWithSpecial)
      .mockResolvedValueOnce(settingsWithSpecial);
    render(<ToastProvider><AbsenceSettings /></ToastProvider>);
    const user = userEvent.setup();

    const successTemplate = await screen.findByLabelText(/success sms template/i);
    fireEvent.change(successTemplate, { target: { value: "Updated normal" } });
    await user.click(screen.getByRole("button", { name: /save settings/i }));

    await waitFor(() => {
      const putCall = mockApiJson.mock.calls.find(
        (c: unknown[]) => c[0] === "/api/v1/admin/absence-settings" && (c[1] as RequestInit).method === "PUT",
      );
      expect(putCall).toBeTruthy();
      const body = JSON.parse((putCall![1] as RequestInit).body as string);
      expect(body.notifications.sms_special_approved_template).toBe("Do not drop me");
    });
  });
});
