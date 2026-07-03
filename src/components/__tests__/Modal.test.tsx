import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Modal from "../Modal";

beforeEach(() => {
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  vi.restoreAllMocks();
});

it("renders dialog element with role", () => {
  render(
    <Modal title="Test" onClose={() => {}}>
      <p>content</p>
    </Modal>,
  );
  expect(screen.getByRole("dialog")).toBeInTheDocument();
});

it("renders title and children", () => {
  render(
    <Modal title="My Title" onClose={() => {}}>
      <p>child content</p>
    </Modal>,
  );
  expect(screen.getByText("My Title")).toBeInTheDocument();
  expect(screen.getByText("child content")).toBeInTheDocument();
});

it("calls onClose when close button clicked", async () => {
  const onClose = vi.fn();
  render(
    <Modal title="Test" onClose={onClose}>
      <p>content</p>
    </Modal>,
  );
  await userEvent.click(screen.getByLabelText("Close dialog"));
  expect(onClose).toHaveBeenCalledTimes(1);
});

it("renders footer when provided", () => {
  render(
    <Modal title="Test" onClose={() => {}} footer={<button>Save</button>}>
      <p>content</p>
    </Modal>,
  );
  expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
});

it("calls onClose when dialog native close event fires", () => {
  const onClose = vi.fn();
  const { container } = render(
    <Modal title="Test" onClose={onClose}>
      <p>content</p>
    </Modal>,
  );
  const dialog = container.querySelector("dialog")!;
  dialog.dispatchEvent(new Event("close"));
  expect(onClose).toHaveBeenCalledTimes(1);
});

it("showModal does not throw on mount", () => {
  const onClose = vi.fn();
  expect(() => {
    render(
      <Modal title="Test" onClose={onClose}>
        <p>content</p>
      </Modal>,
    );
  }).not.toThrow();
});

it("responds to escape key via native dialog behavior", async () => {
  const onClose = vi.fn();
  render(
    <Modal title="Test" onClose={onClose}>
      <p>content</p>
    </Modal>,
  );
  const dialog = screen.getByRole("dialog");
  dialog.dispatchEvent(new Event("close"));
  expect(onClose).toHaveBeenCalled();
});

it("renders with sm size", () => {
  render(
    <Modal title="Small" onClose={() => {}} size="sm">
      <p>sm</p>
    </Modal>,
  );
  expect(screen.getByRole("dialog")).toBeInTheDocument();
});

it("renders with xl size", () => {
  render(
    <Modal title="Xl" onClose={() => {}} size="xl">
      <p>xl</p>
    </Modal>,
  );
  expect(screen.getByRole("dialog")).toBeInTheDocument();
});

it("preserves open attribute after parent re-render (React 19 regression)", () => {
  const onClose = vi.fn();
  const { rerender } = render(
    <Modal title="Test" onClose={onClose}>
      <p>content</p>
    </Modal>,
  );
  const dialog = screen.getByRole("dialog");
  expect(dialog).toHaveAttribute("open");

  rerender(
    <Modal title="Test (updated)" onClose={onClose}>
      <p>updated content</p>
    </Modal>,
  );

  expect(dialog).toHaveAttribute("open");
});

it("does not call onClose when cleanup triggers dialog.close()", () => {
  const onClose = vi.fn();
  const { unmount } = render(
    <Modal title="Test" onClose={onClose}>
      <p>content</p>
    </Modal>,
  );
  expect(onClose).not.toHaveBeenCalled();

  unmount();
  expect(onClose).not.toHaveBeenCalled();
});
