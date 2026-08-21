import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import ReasonField from "../ReasonField";

function ControlledReason() {
  const [value, setValue] = useState("");
  return <ReasonField value={value} onChange={setValue} />;
}

function getLiveEl(container: HTMLElement) {
  return container.querySelector('[aria-live="polite"][aria-atomic="true"].sr-only') as HTMLElement | null;
}

describe("R1 T3 — ReasonField.a11y: aria-live throttling (10 rapid keystrokes ≤2 live updates)", () => {
  it("keeps exactly one aria-live region (sr-only) and the visible count is aria-hidden", () => {
    const { container } = render(<ControlledReason />);
    const liveEls = container.querySelectorAll('[aria-live="polite"]');
    expect(liveEls).toHaveLength(1);
    expect(liveEls[0].className).toMatch(/sr-only/);
    const hiddenCount = [...container.querySelectorAll('[aria-hidden="true"]')].find((el) =>
      (el.textContent ?? "").includes("/500 characters"),
    );
    expect(hiddenCount?.textContent).toMatch(/\/500 characters/);
  });

  it("throttles live announcements: 10 rapid keystrokes produce at most 2 live-region mutations", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const { container } = render(<ControlledReason />);
    const liveEl = getLiveEl(container)!;
    expect(liveEl).toBeTruthy();
    let liveMutations = 0;
    let lastText = liveEl.textContent;
    const observer = new MutationObserver((records) => {
      for (const r of records) {
        if (r.type === "childList" || r.type === "characterData") {
          const now = (liveEl.textContent ?? "").trim();
          if (now !== (lastText ?? "").trim()) {
            liveMutations += 1;
            lastText = liveEl.textContent;
          }
        }
      }
    });
    observer.observe(liveEl, { childList: true, characterData: true, subtree: true });

    // 10 rapid keystrokes without advancing timers in between (simulates a burst)
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime.bind(vi) } as never);
    const textarea = screen.getByRole("textbox", { name: /reason for absence/i });
    for (const ch of "abcdefghij") {
      await user.type(textarea, ch, { delay: null } as never);
    }
    // No tick yet — debounced, so at most 0 or 1 mutation so far
    // Now let the debounce window resolve progressively
    await vi.advanceTimersByTimeAsync(400);
    // Allow one trailing coalesced announcement; 10 keystrokes must never exceed 2 live updates in total
    await vi.advanceTimersByTimeAsync(700);
    observer.disconnect();
    expect(liveMutations).toBeLessThanOrEqual(2);
    expect(liveEl.textContent).toMatch(/10\/500 characters/);
    vi.useRealTimers();
  });
});
