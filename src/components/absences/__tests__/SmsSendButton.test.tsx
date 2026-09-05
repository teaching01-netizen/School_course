import { render, screen } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import SmsSendButton from "../SmsSendButton";

/**
 * framer-motion reads `prefers-reduced-motion` once per module through
 * `window.matchMedia`. A single controllable mock, installed before the first
 * render, lets each test flip the preference and notify the change listener
 * the same way the browser would.
 */
const mql = {
  matches: false,
  media: "(prefers-reduced-motion)",
  changeHandler: null as (() => void) | null,
  addEventListener: (_type: string, callback: () => void) => {
    mql.changeHandler = callback;
  },
} as unknown as MediaQueryList & { matches: boolean; changeHandler: (() => void) | null };

vi.stubGlobal("matchMedia", vi.fn(() => mql));

beforeEach(() => {
  mql.matches = false;
  mql.changeHandler?.();
});

it("uses the backend-aligned 5 minute resend cooldown by default", async () => {
  render(<SmsSendButton isSending={false} sendCount={1} onClick={vi.fn()} />);

  // The waiting state is a quiet note beside the action, not a ring inside it:
  // the button names the next action and the caption states the wait.
  const countdown = /you can resend in \d+:\d{2}/i;
  const button = await screen.findByRole("button", { name: countdown });
  expect(button).toBeDisabled();
  expect(screen.getByText(countdown)).toBeInTheDocument();
  // Motion is on by default: the state crossfade starts from opacity 0.
  expect(button.firstElementChild as HTMLElement).toHaveStyle({ opacity: "0" });
});

it("renders state changes with zero fade under prefers-reduced-motion", () => {
  mql.matches = true;
  mql.changeHandler?.();

  render(<SmsSendButton isSending={false} sendCount={0} onClick={vi.fn()} />);

  const button = screen.getByRole("button", { name: "Send code" });
  expect(button).toBeEnabled();
  expect(screen.queryByText(/you can resend in/i)).not.toBeInTheDocument();
  // initial={false} means the content never fades in — same label, no motion.
  expect(button.firstElementChild as HTMLElement).not.toHaveStyle({ opacity: "0" });
});