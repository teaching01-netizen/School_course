import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "../../hooks/useToast";
import { ActiveCoursesSection } from "../operations/ActiveCoursesSection";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderWithProviders(ui: React.ReactElement) {
  return render(<ToastProvider>{ui}</ToastProvider>);
}

const MOCK_SUBJECTS = {
  subjects: [
    {
      subject_id: "s1",
      subject_code: "MATH",
      subject_name: "Mathematics",
      courses: [
        {
          course_id: "c1",
          course_code: "MATH101",
          course_name: "Math I",
          cycle_id: "cyc-1",
          cycle_label: "Cycle 1",
          is_active: true,
        },
        {
          course_id: "c2",
          course_code: "MATH201",
          course_name: "Math II",
          cycle_id: "cyc-2",
          cycle_label: "Cycle 2",
          is_active: false,
        },
      ],
    },
    {
      subject_id: "s2",
      subject_code: "PHYS",
      subject_name: "Physics",
      courses: [
        {
          course_id: "c3",
          course_code: "PHYS101",
          course_name: "Physics I",
          cycle_id: "cyc-1",
          cycle_label: "Cycle 1",
          is_active: false,
        },
      ],
    },
  ],
};

describe("ActiveCoursesSection", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("renders the loading skeleton initially", () => {
    mockApiJson.mockReturnValue(new Promise(() => {}));
    renderWithProviders(<ActiveCoursesSection />);

    expect(screen.getByRole("status", { name: /loading/i })).toBeInTheDocument();
  });

  it("renders loaded subjects and flags unconfigured subjects", async () => {
    mockApiJson.mockResolvedValueOnce(MOCK_SUBJECTS);
    renderWithProviders(<ActiveCoursesSection />);

    expect(await screen.findByText("MATH — Mathematics")).toBeInTheDocument();
    expect(screen.getByText("PHYS — Physics")).toBeInTheDocument();
    expect(screen.getByText("Not set")).toBeInTheDocument();
  });

  it("shows an error state and allows retrying the load", async () => {
    mockApiJson.mockRejectedValueOnce(new Error("Server down")).mockResolvedValueOnce(MOCK_SUBJECTS);
    const user = userEvent.setup();
    renderWithProviders(<ActiveCoursesSection />);

    const retryButton = await screen.findByRole("button", { name: /retry/i });
    expect(retryButton).toBeInTheDocument();

    await user.click(retryButton);

    await waitFor(() => {
      expect(screen.getByText("MATH — Mathematics")).toBeInTheDocument();
    });
  });

  it("saves active course on checkbox click and updates optimistically", async () => {
    mockApiJson.mockResolvedValueOnce(MOCK_SUBJECTS).mockResolvedValueOnce(undefined);
    const user = userEvent.setup();
    renderWithProviders(<ActiveCoursesSection />);

    await waitFor(() => {
      expect(screen.getByText("MATH — Mathematics")).toBeInTheDocument();
    });

    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes).toHaveLength(3);
    expect(checkboxes[0]).toBeChecked();
    expect(checkboxes[1]).not.toBeChecked();

    await user.click(checkboxes[1]);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/active-courses",
        expect.objectContaining({
          method: "PUT",
          body: expect.stringContaining("c2"),
        }),
      );
    });

    await waitFor(() => {
      const updatedCheckboxes = screen.getAllByRole("checkbox");
      expect(updatedCheckboxes[0]).not.toBeChecked();
      expect(updatedCheckboxes[1]).toBeChecked();
    });
  });

  it("rolls back the optimistic update and shows a toast when save fails", async () => {
    mockApiJson.mockResolvedValueOnce(MOCK_SUBJECTS).mockRejectedValueOnce(new Error("Network error"));
    const user = userEvent.setup();
    renderWithProviders(<ActiveCoursesSection />);

    await waitFor(() => {
      expect(screen.getByText("MATH — Mathematics")).toBeInTheDocument();
    });

    const checkboxes = screen.getAllByRole("checkbox");
    await user.click(checkboxes[1]);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.getByText("Failed to update MATH")).toBeInTheDocument();
    });

    const updatedCheckboxes = screen.getAllByRole("checkbox");
    expect(updatedCheckboxes[0]).toBeChecked();
    expect(updatedCheckboxes[1]).not.toBeChecked();
  });

  it("renders single-course subjects as informational rows", async () => {
    mockApiJson.mockResolvedValueOnce(MOCK_SUBJECTS);
    renderWithProviders(<ActiveCoursesSection />);

    expect(await screen.findByText("PHYS — Physics")).toBeInTheDocument();
    expect(screen.queryAllByRole("checkbox")).toHaveLength(3);

    const physCheckbox = screen.getAllByRole("checkbox")[2];
    expect(physCheckbox).not.toBeChecked();
  });

  it("clicking checkbox on single-course subject marks it dirty and shows Save", async () => {
    mockApiJson.mockResolvedValueOnce(MOCK_SUBJECTS);
    const user = userEvent.setup();
    renderWithProviders(<ActiveCoursesSection />);

    expect(await screen.findByText("PHYS — Physics")).toBeInTheDocument();

    const checkboxes = screen.getAllByRole("checkbox");
    const physCheckbox = checkboxes[2];
    expect(physCheckbox).not.toBeChecked();

    await user.click(physCheckbox);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
    });
  });

  it("single-course subject with active course shows checked checkbox", async () => {
    const singleActiveSubject = {
      subjects: [
        {
          subject_id: "s3",
          subject_code: "BIO",
          subject_name: "Biology",
          courses: [
            {
              course_id: "c4",
              course_code: "BIO101",
              course_name: "Biology I",
              cycle_id: "cyc-1",
              cycle_label: "Cycle 1",
              is_active: true,
            },
          ],
        },
      ],
    };
    mockApiJson.mockResolvedValueOnce(singleActiveSubject);
    renderWithProviders(<ActiveCoursesSection />);

    expect(await screen.findByText("BIO — Biology")).toBeInTheDocument();

    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes).toHaveLength(1);
    expect(checkboxes[0]).toBeChecked();
  });

  it("shows disabled checkbox and create link for subjects with no courses", async () => {
    const zeroCourseSubject = {
      subjects: [
        {
          subject_id: "s4",
          subject_code: "CHEM",
          subject_name: "Chemistry",
          courses: [],
        },
      ],
    };
    mockApiJson.mockResolvedValueOnce(zeroCourseSubject);
    renderWithProviders(<ActiveCoursesSection />);

    expect(await screen.findByText("CHEM — Chemistry")).toBeInTheDocument();

    const disabledCheckbox = screen.getByRole("checkbox", { name: "No courses available" });
    expect(disabledCheckbox).toBeInTheDocument();
    expect(disabledCheckbox).toBeDisabled();

    expect(screen.getByText("No courses — create one first")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Create Course" })).toBeInTheDocument();
  });
});
