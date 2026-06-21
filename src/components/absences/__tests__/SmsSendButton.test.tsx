import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import SmsSendButton from "../SmsSendButton";

it("uses the backend-aligned 60 second resend cooldown by default", async () => {
  render(<SmsSendButton isSending={false} sendCount={1} onClick={vi.fn()} />);

  expect(await screen.findByRole("button", { name: "Resend in 60s" })).toBeDisabled();
});
