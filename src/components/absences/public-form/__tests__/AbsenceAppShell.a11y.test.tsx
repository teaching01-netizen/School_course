import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import AbsenceAppShell from "../AbsenceAppShell";

function ShellHarness({ children }: { children: React.ReactNode }) {
  return (
    <AbsenceAppShell header={<div>Header</div>} footer={<button type="button">Continue</button>}>
      {children}
    </AbsenceAppShell>
  );
}

describe("R1 T — AbsenceAppShell.a11y: focus() + aria-live when step changes", () => {
  it("exposes a single aria-live polite region and focuses the step heading on mount", async () => {
    const { container, rerender } = render(
      <ShellHarness>
        <h1>Find your profile</h1>
        <p>content</p>
      </ShellHarness>,
    );
    const live = container.querySelector('[aria-live="polite"].sr-only') as HTMLElement | null;
    expect(live).toBeTruthy();
    // After mount the heading is focused — activeElement is the h1 (tabIndex=-1 is set by the shell)
    await screen.findByRole("heading", { name: "Find your profile" });
    expect(document.activeElement).toBe(screen.getByRole("heading", { name: "Find your profile" }));
    expect(live!.textContent).toMatch(/Find your profile/);

    rerender(
      <ShellHarness>
        <h1>Parent verification</h1>
      </ShellHarness>,
    );
    await screen.findByRole("heading", { name: "Parent verification" });
    expect(document.activeElement).toBe(screen.getByRole("heading", { name: "Parent verification" }));
    expect(live!.textContent).toMatch(/Parent verification/);
  });

  it("falls back to focusing the main landmark when no heading is present", () => {
    const { container } = render(
      <ShellHarness>
        <p>No heading here</p>
      </ShellHarness>,
    );
    const main = screen.getByRole("main");
    expect(document.activeElement).toBe(main);
    expect(container.querySelectorAll('[aria-live="polite"]')).toHaveLength(1);
  });
});
