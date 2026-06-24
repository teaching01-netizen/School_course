import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SmsConfirmModal from "../SmsConfirmModal";

function renderModal(props?: Partial<React.ComponentProps<typeof SmsConfirmModal>>) {
  return render(
    <SmsConfirmModal
      phones={["+66812345678", "+66898765432"]}
      message="Warwick Institute: John ได้แจ้งลาเรียน Math (3 Jun 2026)"
      onSend={vi.fn()}
      onSkip={vi.fn()}
      sending={false}
      {...props}
    />,
  );
}

describe("SmsConfirmModal", () => {
  it("renders phone numbers", () => {
    renderModal();
    expect(screen.getByText("+66812345678")).toBeInTheDocument();
    expect(screen.getByText("+66898765432")).toBeInTheDocument();
  });

  it("renders message preview", () => {
    renderModal();
    expect(screen.getByText(/ได้แจ้งลาเรียน Math/)).toBeInTheDocument();
  });

  it("calls onSend when Send SMS button clicked", async () => {
    const onSend = vi.fn();
    const user = userEvent.setup();
    renderModal({ onSend });

    await user.click(screen.getByRole("button", { name: /send sms/i }));
    expect(onSend).toHaveBeenCalled();
  });

  it("calls onSkip when Skip button clicked", async () => {
    const onSkip = vi.fn();
    const user = userEvent.setup();
    renderModal({ onSkip });

    await user.click(screen.getByRole("button", { name: /skip/i }));
    expect(onSkip).toHaveBeenCalled();
  });

  it("disables buttons when sending", () => {
    renderModal({ sending: true });
    expect(screen.getByRole("button", { name: /skip/i })).toBeDisabled();
  });
});
