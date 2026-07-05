import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { AbsenceFormEditor } from "../AbsenceFormEditor";
import type { AbsenceSettings } from "../../../types";

const baseSettings: AbsenceSettings = {
  form: {
    max_date_range_days: 30,
    min_hours_before_session: 0,
    max_hours_after_session: 0,
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
  notifications: {
    sms_parent_enabled: true,
    sms_parent_template: "",
    sms_success_template: "",
    sms_special_approved_template: "",
    allow_submit_without_otp: false,
    email_success_enabled: false,
    email_success_subject: "",
    email_success_body: "",
  },
  admin_contact: { email: "", phone: "", hours: "" },
  student_self_service: { can_view_own: false, can_cancel_own: false },
};

describe("AbsenceFormEditor email section", () => {
  it("shows email section when showTextEditors is true", () => {
    render(
      <AbsenceFormEditor
        settings={baseSettings}
        onChange={vi.fn()}
        onSave={vi.fn()}
        saving={false}
        showTextEditors={true}
      />,
    );
    expect(screen.getByText("Email Notifications")).toBeInTheDocument();
  });

  it("hides email section when showTextEditors is false", () => {
    render(
      <AbsenceFormEditor
        settings={baseSettings}
        onChange={vi.fn()}
        onSave={vi.fn()}
        saving={false}
        showTextEditors={false}
      />,
    );
    expect(screen.queryByText("Email Notifications")).not.toBeInTheDocument();
  });

  it("checkbox reflects email_success_enabled", () => {
    render(
      <AbsenceFormEditor
        settings={{ ...baseSettings, notifications: { ...baseSettings.notifications!, email_success_enabled: true } }}
        onChange={vi.fn()}
        onSave={vi.fn()}
        saving={false}
      />,
    );
    const checkbox = screen.getByRole("checkbox", { name: /enable success email/i });
    expect(checkbox).toBeChecked();
  });

  it("checkbox is unchecked when disabled", () => {
    render(
      <AbsenceFormEditor
        settings={baseSettings}
        onChange={vi.fn()}
        onSave={vi.fn()}
        saving={false}
      />,
    );
    const checkbox = screen.getByRole("checkbox", { name: /enable success email/i });
    expect(checkbox).not.toBeChecked();
  });

  it("checkbox toggle fires onChange with email_success_enabled", () => {
    const onChange = vi.fn();
    render(
      <AbsenceFormEditor
        settings={baseSettings}
        onChange={onChange}
        onSave={vi.fn()}
        saving={false}
      />,
    );
    const checkbox = screen.getByRole("checkbox", { name: /enable success email/i });
    fireEvent.click(checkbox);
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        notifications: expect.objectContaining({ email_success_enabled: true }),
      }),
    );
  });

  it("subject input reflects value", () => {
    render(
      <AbsenceFormEditor
        settings={{ ...baseSettings, notifications: { ...baseSettings.notifications!, email_success_subject: "Hi {{student_name}}" } }}
        onChange={vi.fn()}
        onSave={vi.fn()}
        saving={false}
      />,
    );
    const input = screen.getByLabelText(/email subject/i);
    expect(input).toHaveValue("Hi {{student_name}}");
  });

  it("subject input onChange fires", () => {
    const onChange = vi.fn();
    render(
      <AbsenceFormEditor
        settings={baseSettings}
        onChange={onChange}
        onSave={vi.fn()}
        saving={false}
      />,
    );
    const input = screen.getByLabelText(/email subject/i);
    fireEvent.change(input, { target: { value: "New Subject" } });
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        notifications: expect.objectContaining({ email_success_subject: "New Subject" }),
      }),
    );
  });

  it("body textarea reflects value", () => {
    render(
      <AbsenceFormEditor
        settings={{ ...baseSettings, notifications: { ...baseSettings.notifications!, email_success_body: "<p>Hello</p>" } }}
        onChange={vi.fn()}
        onSave={vi.fn()}
        saving={false}
      />,
    );
    const textarea = screen.getByLabelText(/email body/i);
    expect(textarea).toHaveValue("<p>Hello</p>");
  });

  it("body textarea onChange fires", () => {
    const onChange = vi.fn();
    render(
      <AbsenceFormEditor
        settings={baseSettings}
        onChange={onChange}
        onSave={vi.fn()}
        saving={false}
      />,
    );
    const textarea = screen.getByLabelText(/email body/i);
    fireEvent.change(textarea, { target: { value: "<p>Updated</p>" } });
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        notifications: expect.objectContaining({ email_success_body: "<p>Updated</p>" }),
      }),
    );
  });

  it("notificationsWith preserves SMS fields when changing email", () => {
    const onChange = vi.fn();
    render(
      <AbsenceFormEditor
        settings={{
          ...baseSettings,
          notifications: {
            ...baseSettings.notifications!,
            sms_success_template: "SMS template",
          },
        }}
        onChange={onChange}
        onSave={vi.fn()}
        saving={false}
      />,
    );
    const checkbox = screen.getByRole("checkbox", { name: /enable success email/i });
    fireEvent.click(checkbox);
    const call = onChange.mock.calls[0][0];
    expect(call.notifications.sms_success_template).toBe("SMS template");
    expect(call.notifications.email_success_enabled).toBe(true);
  });

  it("defaults when notifications is undefined", () => {
    render(
      <AbsenceFormEditor
        settings={{ ...baseSettings, notifications: undefined }}
        onChange={vi.fn()}
        onSave={vi.fn()}
        saving={false}
      />,
    );
    const checkbox = screen.getByRole("checkbox", { name: /enable success email/i });
    expect(checkbox).not.toBeChecked();
  });

  it("textarea maxLength matches backend limit of 15000", () => {
    render(
      <AbsenceFormEditor
        settings={baseSettings}
        onChange={vi.fn()}
        onSave={vi.fn()}
        saving={false}
      />,
    );
    const textarea = screen.getByLabelText(/email body/i);
    expect(textarea).toHaveAttribute("maxlength", "15000");
  });

  it("body placeholder hints show only supported placeholders", () => {
    render(
      <AbsenceFormEditor
        settings={baseSettings}
        onChange={vi.fn()}
        onSave={vi.fn()}
        saving={false}
      />,
    );
    const emailSection = screen.getByText("Email Notifications").closest("section")!;
    const supportedPlaceholders = ["{{student_name}}", "{{wcode}}", "{{institute_name}}", "{{submitted_at}}", "{{absence_count}}", "{{absence_rows}}"];
    const unsupportedPlaceholders = ["{{subject_name}}", "{{course_name}}", "{{absence_dates}}", "{{missed_sessions}}", "{{sit_in_plan}}", "{{reason}}", "{{status}}"];
    for (const ph of supportedPlaceholders) {
      expect(emailSection.querySelector(`span`)).toBeTruthy();
      expect(emailSection.textContent).toContain(ph);
    }
    for (const ph of unsupportedPlaceholders) {
      expect(emailSection.textContent).not.toContain(ph);
    }
  });
});
