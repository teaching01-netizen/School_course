import { act, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import AbsenceActionBar from "../public-form/AbsenceActionBar";

function renderBar(loading: boolean) {
  render(
    <AbsenceActionBar
      showPrimary
      canProceed
      loading={loading}
      onBack={vi.fn()}
      onPrimary={vi.fn()}
      primaryLabel="Submit absence"
    />,
  );
}

function spinnerInButton(): HTMLElement | null {
  return screen.getByRole("button", { name: "Submitting…" }).querySelector(".animate-spin");
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

it("acknowledges a submission instantly: label flips to Submitting… as a status, no spinner flash", () => {
  renderBar(true);

  const button = screen.getByRole("button", { name: "Submitting…" });
  expect(button).toBeDisabled();
  const status = within(button).getByRole("status");
  expect(status).toHaveTextContent("Submitting…");
  expect(spinnerInButton()).not.toBeInTheDocument();
});

it("reveals the spinner only once the operation runs past ~350ms", () => {
  renderBar(true);
  expect(spinnerInButton()).not.toBeInTheDocument();

  act(() => {
    vi.advanceTimersByTime(349);
  });
  expect(spinnerInButton()).not.toBeInTheDocument();

  act(() => {
    vi.advanceTimersByTime(1);
  });
  expect(spinnerInButton()).toBeInTheDocument();
});

it("switches to the slow label at ~4s", () => {
  renderBar(true);

  act(() => {
    vi.advanceTimersByTime(4000);
  });
  expect(screen.getByRole("button", { name: "Still submitting…" })).toBeInTheDocument();
});