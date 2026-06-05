import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import DateRangeSlot from "../DateRangeSlot";

describe("DateRangeSlot", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 4, 31, 12));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("uses native date inputs with mobile-friendly tap targets and date constraints", () => {
    render(
      <DateRangeSlot
        index={0}
        fromDate={undefined}
        toDate={new Date(2026, 5, 8)}
        onFromChange={vi.fn()}
        onToChange={vi.fn()}
        onRemove={vi.fn()}
        canRemove={false}
        maxDays={30}
      />,
    );

    const fromInput = screen.getByLabelText("From date: Pick a date");
    const toInput = screen.getByLabelText("To date: Mon, Jun 8");

    expect(fromInput).toHaveAttribute("type", "date");
    expect(fromInput).toHaveAttribute("min", "2026-05-31");
    expect(fromInput).toHaveAttribute("max", "2026-06-08");
    expect(fromInput).toHaveClass("min-h-[44px]");

    expect(toInput).toHaveAttribute("type", "date");
    expect(toInput).toHaveAttribute("min", "2026-05-31");
    expect(toInput).toHaveAttribute("max", "2026-06-30");
    expect(toInput).toHaveValue("2026-06-08");
  });

  it("converts native date values to local start-of-day Date objects", () => {
    const onFromChange = vi.fn();

    render(
      <DateRangeSlot
        index={0}
        fromDate={undefined}
        toDate={undefined}
        onFromChange={onFromChange}
        onToChange={vi.fn()}
        onRemove={vi.fn()}
        canRemove={false}
        maxDays={30}
      />,
    );

    fireEvent.change(screen.getByLabelText("From date: Pick a date"), {
      target: { value: "2026-06-02" },
    });

    expect(onFromChange).toHaveBeenLastCalledWith(new Date(2026, 5, 2));
  });
});
