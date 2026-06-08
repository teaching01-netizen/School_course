import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Courses from "../Courses";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => { store[key] = value; }),
    removeItem: vi.fn((key: string) => { delete store[key]; }),
    clear: vi.fn(() => { store = {}; }),
    get length() { return Object.keys(store).length; },
    key: vi.fn((index: number) => Object.keys(store)[index] ?? null),
  };
})();
Object.defineProperty(globalThis, "localStorage", { value: localStorageMock, writable: true });

const TEACHERS = [
  { id: "t1", username: "Alice", role: "Teacher" as const },
  { id: "t2", username: "Bob", role: "Teacher" as const },
  { id: "t3", username: "Charlie", role: "Teacher" as const },
];

const COURSES = [
  { id: "c1", course_no: 1, code: "MATH-101", name: "Algebra I", year: 2025, teacher_id: "t1", teacher_name: "Alice", subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics", hour: 40, student_count: 20, course_type: "Lecture", legacy_course_id: null },
  { id: "c2", course_no: 2, code: "PHYS-101", name: "Physics I", year: 2025, teacher_id: "t2", teacher_name: "Bob", subject_id: "subj-2", subject_code: "PHYS", subject_name: "Physics", hour: 30, student_count: 15, course_type: "Lab", legacy_course_id: null },
  { id: "c3", course_no: 3, code: "CHEM-101", name: "Chemistry I", year: 2025, teacher_id: "t1", teacher_name: "Alice", subject_id: "subj-3", subject_code: "CHEM", subject_name: "Chemistry", hour: 35, student_count: 18, course_type: "Lecture", legacy_course_id: null },
  { id: "c4", course_no: 4, code: "MATH-201", name: "Algebra II", year: 2026, teacher_id: null, teacher_name: "", subject_id: null, subject_code: "", subject_name: "", hour: null, student_count: null, course_type: null, legacy_course_id: null },
];

function renderCourses() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <Courses />
      </ToastProvider>
    </MemoryRouter>
  );
}

describe("Courses bulk editor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiJson.mockImplementation(async (url: string) => {
      if (url === "/api/v1/users?role=Teacher") return TEACHERS;
      if (url === "/api/v1/courses") return COURSES;
      throw new Error(`Unmocked: ${url}`);
    });
  });

  it("renders course table with all rows", async () => {
    renderCourses();
    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeTruthy();
    });
    expect(screen.getByText("PHYS-101")).toBeTruthy();
    expect(screen.getByText("CHEM-101")).toBeTruthy();
    expect(screen.getByText("MATH-201")).toBeTruthy();
  });

  it("shows teacher filter dropdown populated from API", async () => {
    renderCourses();
    await waitFor(() => {
      expect(screen.getByLabelText("Teacher filter")).toBeTruthy();
    });
    const select = screen.getByLabelText("Teacher filter") as HTMLSelectElement;
    expect(select.options.length).toBe(5); // All + No teacher + 3 teachers
    expect(select.options[0].text).toBe("All teachers");
    expect(select.options[1].text).toBe("No teacher");
    expect(select.options[2].text).toBe("Alice");
    expect(select.options[3].text).toBe("Bob");
    expect(select.options[4].text).toBe("Charlie");
  });

  it("filters courses by selected teacher", async () => {
    renderCourses();
    await waitFor(() => {
      expect(screen.getByLabelText("Teacher filter")).toBeTruthy();
    });
    const select = screen.getByLabelText("Teacher filter");
    await userEvent.selectOptions(select, "t1");
    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeTruthy();
      expect(screen.getByText("CHEM-101")).toBeTruthy();
    });
    expect(screen.queryByText("PHYS-101")).toBeNull();
    expect(screen.queryByText("MATH-201")).toBeNull();
  });

  it("filters courses by 'No teacher' option", async () => {
    renderCourses();
    await waitFor(() => {
      expect(screen.getByLabelText("Teacher filter")).toBeTruthy();
    });
    const select = screen.getByLabelText("Teacher filter");
    await userEvent.selectOptions(select, "__none__");
    await waitFor(() => {
      expect(screen.getByText("MATH-201")).toBeTruthy();
    });
    expect(screen.queryByText("MATH-101")).toBeNull();
  });

  it("renders row checkboxes and select-all checkbox", async () => {
    renderCourses();
    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeTruthy();
    });
    const selectAll = screen.getByLabelText("Select all courses");
    expect(selectAll).toBeTruthy();
    const rowCheckboxes = screen.getAllByRole("checkbox", { name: /select/i });
    expect(rowCheckboxes.length).toBeGreaterThanOrEqual(5); // select-all + 4 rows
  });

  it("select-all checkbox selects all visible rows", async () => {
    renderCourses();
    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeTruthy();
    });
    const selectAll = screen.getByLabelText("Select all courses") as HTMLInputElement;
    await userEvent.click(selectAll);
    expect(selectAll.checked).toBe(true);
    const rowCheckboxes = screen.getAllByRole("checkbox").filter(cb => cb !== selectAll);
    for (const cb of rowCheckboxes) {
      expect((cb as HTMLInputElement).checked).toBe(true);
    }
  });

  it("shows bulk action bar when courses are selected", async () => {
    renderCourses();
    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeTruthy();
    });
    expect(screen.queryByText(/selected/)).toBeNull();
    const rowCheckbox = screen.getAllByRole("checkbox").find(
      cb => cb.closest("tr")?.textContent?.includes("MATH-101")
    );
    if (!rowCheckbox) throw new Error("Row checkbox not found");
    await userEvent.click(rowCheckbox);
    await waitFor(() => {
      expect(screen.getByText(/1 selected/)).toBeTruthy();
    });
    expect(screen.getByText("Delete Selected")).toBeTruthy();
  });

  it("opens confirmation modal when Delete Selected is clicked", async () => {
    renderCourses();
    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeTruthy();
    });
    const rowCheckbox = screen.getAllByRole("checkbox").find(
      cb => cb.closest("tr")?.textContent?.includes("MATH-101")
    );
    if (!rowCheckbox) throw new Error("Row checkbox not found");
    await userEvent.click(rowCheckbox);
    await waitFor(() => {
      expect(screen.getByText("Delete Selected")).toBeTruthy();
    });
    await userEvent.click(screen.getByText("Delete Selected"));
    await waitFor(() => {
      expect(screen.getByText(/Delete 1 course/i)).toBeTruthy();
    });
  });

  it("calls batch-delete API on confirm and shows success toast", async () => {
    mockApiJson.mockImplementation(async (url: string) => {
      if (url === "/api/v1/users?role=Teacher") return TEACHERS;
      if (url === "/api/v1/courses") return COURSES;
      if (url === "/api/v1/courses/batch-delete") {
        return { succeeded: ["c1"], failed: [], total_processed: 1 };
      }
      throw new Error(`Unmocked: ${url}`);
    });
    renderCourses();
    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeTruthy();
    });
    const rowCheckbox = screen.getAllByRole("checkbox").find(
      cb => cb.closest("tr")?.textContent?.includes("MATH-101")
    );
    if (!rowCheckbox) throw new Error("Row checkbox not found");
    await userEvent.click(rowCheckbox);
    await waitFor(() => {
      expect(screen.getByText("Delete Selected")).toBeTruthy();
    });
    await userEvent.click(screen.getByText("Delete Selected"));
    await waitFor(() => {
      expect(screen.getByText(/Delete 1 course/i)).toBeTruthy();
    });
    await userEvent.click(screen.getByText("Delete", { selector: "button" }));
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/courses/batch-delete",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ ids: ["c1"] }),
        })
      );
    });
  });

  it("shows partial failure details in toast", async () => {
    mockApiJson.mockImplementation(async (url: string) => {
      if (url === "/api/v1/users?role=Teacher") return TEACHERS;
      if (url === "/api/v1/courses") return COURSES;
      if (url === "/api/v1/courses/batch-delete") {
        return { succeeded: ["c1"], failed: [{ id: "c2", error: "not found" }], total_processed: 2 };
      }
      throw new Error(`Unmocked: ${url}`);
    });
    renderCourses();
    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeTruthy();
    });
    const cb1 = screen.getAllByRole("checkbox").find(
      cb => cb.closest("tr")?.textContent?.includes("MATH-101")
    );
    const cb2 = screen.getAllByRole("checkbox").find(
      cb => cb.closest("tr")?.textContent?.includes("PHYS-101")
    );
    if (!cb1 || !cb2) throw new Error("Row checkboxes not found");
    await userEvent.click(cb1);
    await userEvent.click(cb2);
    await waitFor(() => {
      expect(screen.getByText(/2 selected/)).toBeTruthy();
    });
    await userEvent.click(screen.getByText("Delete Selected"));
    await waitFor(() => {
      expect(screen.getByText(/Delete 2 course/i)).toBeTruthy();
    });
    await userEvent.click(screen.getByText("Delete", { selector: "button" }));
    await waitFor(() => {
      expect(screen.getByText(/1 succeeded.*1 failed/)).toBeTruthy();
    });
  });

  it("clears selection after successful delete", async () => {
    mockApiJson.mockImplementation(async (url: string) => {
      if (url === "/api/v1/users?role=Teacher") return TEACHERS;
      if (url === "/api/v1/courses") return COURSES;
      if (url === "/api/v1/courses/batch-delete") {
        return { succeeded: ["c1"], failed: [], total_processed: 1 };
      }
      throw new Error(`Unmocked: ${url}`);
    });
    renderCourses();
    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeTruthy();
    });
    const rowCheckbox = screen.getAllByRole("checkbox").find(
      cb => cb.closest("tr")?.textContent?.includes("MATH-101")
    );
    if (!rowCheckbox) throw new Error("Row checkbox not found");
    await userEvent.click(rowCheckbox);
    await waitFor(() => {
      expect(screen.getByText(/1 selected/)).toBeTruthy();
    });
    await userEvent.click(screen.getByText("Delete Selected"));
    await waitFor(() => {
      expect(screen.getByText(/Delete 1 course/i)).toBeTruthy();
    });
    await userEvent.click(screen.getByText("Delete", { selector: "button" }));
    await waitFor(() => {
      expect(screen.queryByText(/1 selected/)).toBeNull();
    });
  });
});
