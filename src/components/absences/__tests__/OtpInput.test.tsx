import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import OtpInput from "../OtpInput";

function ControlledOtp({ onComplete = vi.fn() }: { onComplete?: (code: string) => void }) {
  const [value, setValue] = useState("");

  return (
    <OtpInput
      value={value}
      onChange={setValue}
      onComplete={onComplete}
      label="Verification code"
    />
  );
}

it("normalizes typed input to six digits", async () => {
  const user = userEvent.setup();
  render(<ControlledOtp />);

  const input = screen.getByRole("textbox", { name: "Verification code" });
  await user.type(input, "a1b2c3d4e5f6g7");

  expect(input).toHaveValue("123456");
});

it("normalizes a formatted paste and completes with the first six digits", () => {
  const onComplete = vi.fn();
  render(<ControlledOtp onComplete={onComplete} />);

  const input = screen.getByRole("textbox", { name: "Verification code" });
  fireEvent.paste(input, {
    clipboardData: { getData: () => "+66 12-34-56-78" },
  });

  expect(input).toHaveValue("661234");
  expect(onComplete).toHaveBeenCalledOnce();
  expect(onComplete).toHaveBeenCalledWith("661234");
});

it("invokes completion when a controlled value reaches six normalized digits", () => {
  const onComplete = vi.fn();
  const { rerender } = render(
    <OtpInput value="12-34" onChange={vi.fn()} onComplete={onComplete} label="Verification code" />,
  );

  expect(onComplete).not.toHaveBeenCalled();

  rerender(
    <OtpInput value="12-34-56" onChange={vi.fn()} onComplete={onComplete} label="Verification code" />,
  );

  expect(onComplete).toHaveBeenCalledOnce();
  expect(onComplete).toHaveBeenCalledWith("123456");
});
