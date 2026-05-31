import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import BatchToolbar from "../BatchToolbar";

const defaultProps = {
  onSelectAll: vi.fn(),
  onDeselectAll: vi.fn(),
  onSelectAllCovers: vi.fn(),
  onDeselectAllCovers: vi.fn(),
  onSelectMornings: vi.fn(),
  onSelectAfternoons: vi.fn(),
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe("BatchToolbar", () => {
  it("renders all six action buttons", () => {
    render(<BatchToolbar {...defaultProps} />);
    expect(screen.getByRole("button", { name: /all absent/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /none absent/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /all cover/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /none cover/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /all mornings/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /all afternoons/i })).toBeInTheDocument();
  });

  it("calls onSelectAll when 'All absent' is clicked", async () => {
    const user = userEvent.setup();
    const onSelectAll = vi.fn();
    render(<BatchToolbar {...defaultProps} onSelectAll={onSelectAll} />);
    await user.click(screen.getByRole("button", { name: /all absent/i }));
    expect(onSelectAll).toHaveBeenCalledTimes(1);
  });

  it("calls onDeselectAll when 'None absent' is clicked", async () => {
    const user = userEvent.setup();
    const onDeselectAll = vi.fn();
    render(<BatchToolbar {...defaultProps} onDeselectAll={onDeselectAll} />);
    await user.click(screen.getByRole("button", { name: /none absent/i }));
    expect(onDeselectAll).toHaveBeenCalledTimes(1);
  });

  it("calls onSelectAllCovers when 'All cover' is clicked", async () => {
    const user = userEvent.setup();
    const onSelectAllCovers = vi.fn();
    render(<BatchToolbar {...defaultProps} onSelectAllCovers={onSelectAllCovers} />);
    await user.click(screen.getByRole("button", { name: /all cover/i }));
    expect(onSelectAllCovers).toHaveBeenCalledTimes(1);
  });

  it("calls onDeselectAllCovers when 'None cover' is clicked", async () => {
    const user = userEvent.setup();
    const onDeselectAllCovers = vi.fn();
    render(<BatchToolbar {...defaultProps} onDeselectAllCovers={onDeselectAllCovers} />);
    await user.click(screen.getByRole("button", { name: /none cover/i }));
    expect(onDeselectAllCovers).toHaveBeenCalledTimes(1);
  });

  it("calls onSelectMornings when 'All mornings' is clicked", async () => {
    const user = userEvent.setup();
    const onSelectMornings = vi.fn();
    render(<BatchToolbar {...defaultProps} onSelectMornings={onSelectMornings} />);
    await user.click(screen.getByRole("button", { name: /all mornings/i }));
    expect(onSelectMornings).toHaveBeenCalledTimes(1);
  });

  it("calls onSelectAfternoons when 'All afternoons' is clicked", async () => {
    const user = userEvent.setup();
    const onSelectAfternoons = vi.fn();
    render(<BatchToolbar {...defaultProps} onSelectAfternoons={onSelectAfternoons} />);
    await user.click(screen.getByRole("button", { name: /all afternoons/i }));
    expect(onSelectAfternoons).toHaveBeenCalledTimes(1);
  });

  it("disables all buttons when disabled prop is true", () => {
    render(<BatchToolbar {...defaultProps} disabled={true} />);
    expect(screen.getByRole("button", { name: /all absent/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /none absent/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /all cover/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /none cover/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /all mornings/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /all afternoons/i })).toBeDisabled();
  });

  it("has correct accessibility attributes", () => {
    render(<BatchToolbar {...defaultProps} />);
    const group = screen.getByRole("group", { name: /batch actions/i });
    expect(group).toBeInTheDocument();
    expect(group).not.toHaveAttribute("role", "toolbar");
  });

  it("does not call callbacks when disabled", async () => {
    const user = userEvent.setup();
    const onSelectAll = vi.fn();
    const onDeselectAll = vi.fn();
    const onSelectAllCovers = vi.fn();
    const onDeselectAllCovers = vi.fn();
    const onSelectMornings = vi.fn();
    const onSelectAfternoons = vi.fn();

    render(
      <BatchToolbar
        {...defaultProps}
        disabled={true}
        onSelectAll={onSelectAll}
        onDeselectAll={onDeselectAll}
        onSelectAllCovers={onSelectAllCovers}
        onDeselectAllCovers={onDeselectAllCovers}
        onSelectMornings={onSelectMornings}
        onSelectAfternoons={onSelectAfternoons}
      />,
    );

    await user.click(screen.getByRole("button", { name: /all absent/i }));
    await user.click(screen.getByRole("button", { name: /none absent/i }));
    await user.click(screen.getByRole("button", { name: /all cover/i }));
    await user.click(screen.getByRole("button", { name: /none cover/i }));
    await user.click(screen.getByRole("button", { name: /all mornings/i }));
    await user.click(screen.getByRole("button", { name: /all afternoons/i }));

    expect(onSelectAll).not.toHaveBeenCalled();
    expect(onDeselectAll).not.toHaveBeenCalled();
    expect(onSelectAllCovers).not.toHaveBeenCalled();
    expect(onDeselectAllCovers).not.toHaveBeenCalled();
    expect(onSelectMornings).not.toHaveBeenCalled();
    expect(onSelectAfternoons).not.toHaveBeenCalled();
  });
});
