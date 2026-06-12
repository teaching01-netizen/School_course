import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AttendeeSection } from "../components/AttendeeSection";
import { ToastProvider } from "../hooks/useToast";

// Mock the api client
vi.mock("../api/client", () => ({
  apiJson: vi.fn().mockResolvedValue({}),
}));

// Mock react-router-dom Link
vi.mock("react-router-dom", () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

const mockStudent = (id: string, wcode: string, name: string) => ({
  id,
  wcode,
  full_name: name,
  notes: "",
});

const defaultProps = {
  courseId: "course-1",
  isAdmin: true,
  crmEnabled: false,
  crmLocked: false,
  roster: [mockStudent("s1", "W250389", "Alice"), mockStudent("s2", "W250390", "Bob")],
  rosterLoading: false,
  addingWcode: "",
  adding: false,
  onRosterChanged: vi.fn(),
  onSetAddingWcode: vi.fn(),
  onAddStudentByWcode: vi.fn().mockResolvedValue({ ok: true }),
  onRemoveStudent: vi.fn().mockResolvedValue(undefined),
};

function renderWithProviders(ui: React.ReactElement) {
  return render(<ToastProvider>{ui}</ToastProvider>);
}

describe("AttendeeSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("CRM disabled state", () => {
    it('shows "Add Manual" button enabled and "Add from Sage" button', () => {
      renderWithProviders(<AttendeeSection {...defaultProps} />);

      expect(screen.getByText("Add Manual")).toBeEnabled();
      expect(screen.getByText("Add from Sage")).toBeInTheDocument();
    });

    it('opens a modal with W-code input when "Add Manual" is clicked', async () => {
      const user = userEvent.setup();
      renderWithProviders(<AttendeeSection {...defaultProps} />);

      await user.click(screen.getByText("Add Manual"));

      expect(screen.getByText("Add by W-code")).toBeInTheDocument();
      expect(screen.getByPlaceholderText("e.g. W250389")).toBeInTheDocument();
    });

    it("keeps manual modal open and shows conflict message when add fails", async () => {
      const user = userEvent.setup();
      const onAddStudentByWcode = vi.fn().mockResolvedValue({
        ok: false,
        error: "Student scheduling conflict: Alice conflict at 20 May, 10:30-11:30",
      });
      renderWithProviders(
        <AttendeeSection
          {...defaultProps}
          addingWcode="W250389"
          onAddStudentByWcode={onAddStudentByWcode}
        />,
      );

      await user.click(screen.getByText("Add Manual"));
      await user.click(screen.getByRole("button", { name: "Add" }));

      expect(await screen.findByText(/Student scheduling conflict: Alice/)).toBeInTheDocument();
      expect(screen.getByText("Add Student Manually")).toBeInTheDocument();
    });

    it('opens CRM filter modal when "Add from Sage" is clicked', async () => {
      const user = userEvent.setup();
      renderWithProviders(<AttendeeSection {...defaultProps} />);

      await user.click(screen.getByText("Add from Sage"));

      expect(screen.getByText("CRM Filter")).toBeInTheDocument();
      expect(screen.getByText("Enable CRM management")).toBeInTheDocument();
    });
  });

  describe("CRM enabled state", () => {
    it('disables "Add Manual" button and shows "Edit Sage filter"', () => {
      renderWithProviders(
        <AttendeeSection {...defaultProps} crmEnabled={true} />,
      );

      expect(screen.getByText("Add Manual")).toBeDisabled();
      expect(screen.getByText("Edit Sage filter")).toBeInTheDocument();
    });

    it('shows the roster is managed message when crmEnabled', () => {
      renderWithProviders(
        <AttendeeSection {...defaultProps} crmEnabled={true} />,
      );

      expect(
        screen.getByText(/Roster is managed by CRM filter/),
      ).toBeInTheDocument();
    });
  });

  describe("roster display", () => {
    it("renders roster students in a table", () => {
      renderWithProviders(<AttendeeSection {...defaultProps} />);

      expect(screen.getByText("W250389")).toBeInTheDocument();
      expect(screen.getByText("Alice")).toBeInTheDocument();
      expect(screen.getByText("W250390")).toBeInTheDocument();
      expect(screen.getByText("Bob")).toBeInTheDocument();
    });

    it('shows "No students" when roster is empty', () => {
      renderWithProviders(<AttendeeSection {...defaultProps} roster={[]} />);

      expect(screen.getByText("No students")).toBeInTheDocument();
    });

    it("shows loading state when rosterLoading is true", () => {
      renderWithProviders(
        <AttendeeSection {...defaultProps} rosterLoading={true} />,
      );

      expect(screen.getByText("Loading…")).toBeInTheDocument();
    });
  });
});
