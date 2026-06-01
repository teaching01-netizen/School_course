import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import StudentProfile from "../StudentProfile";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderStudentProfile(wcode = "W001") {
  render(
    <MemoryRouter initialEntries={[`/students/${wcode}`]}>
      <ToastProvider>
        <Routes>
          <Route path="/students/:wcode" element={<StudentProfile />} />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("StudentProfile", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string) => {
      if (path.includes("/students/by-wcode")) {
        return Promise.resolve({ id: "st-1", wcode: "W001", full_name: "Alice Smith", notes: "Test notes" });
      }
      if (path.includes("/students/st-1/courses")) {
        return Promise.resolve([
          { id: "course-1", code: "MATH-101", name: "Math", teacher_name: "Teacher One", subject_code: "MATH", subject_name: "Mathematics", student_count: 15, course_type: "General" },
        ]);
      }
      if (path === "/api/v1/courses") {
        return Promise.resolve([{ id: "course-1", code: "MATH-101", name: "Math" }]);
      }
      if (path === "/api/v1/rooms") {
        return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
      }
      if (path === "/api/v1/meta/time") {
        return Promise.resolve({ institute_tz: "Asia/Bangkok" });
      }
      if (path.startsWith("/api/v1/sessions?")) {
        return Promise.resolve([]);
      }
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("renders student name and wcode", async () => {
    renderStudentProfile();
    const headings = await screen.findAllByText("Alice Smith");
    expect(headings.length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("W001")).toBeInTheDocument();
  });

  it("renders enrolled courses", async () => {
    renderStudentProfile();
    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeInTheDocument();
      expect(screen.getByText("Math")).toBeInTheDocument();
      expect(screen.getByText("Teacher One")).toBeInTheDocument();
    });
  });

  it("renders 24-hour calendar grid in weekly schedule", async () => {
    renderStudentProfile();
    await screen.findAllByText("Alice Smith");

    // Verify 24hr time slots are present
    expect(screen.getByText("00:00")).toBeInTheDocument();
    expect(screen.getByText("09:00")).toBeInTheDocument();
    expect(screen.getByText("23:00")).toBeInTheDocument();
  });

  it("places session in correct cell using institute timezone", async () => {
    // 2026-06-01T02:00:00Z = 2026-06-01T09:00:00 Bangkok (Monday)
    const session = {
      id: "sess-1",
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-06-01T02:00:00Z",
      end_at: "2026-06-01T04:00:00Z",
    };

    mockApiJson.mockImplementation((path: string) => {
      if (path.includes("/students/by-wcode")) {
        return Promise.resolve({ id: "st-1", wcode: "W001", full_name: "Alice Smith", notes: "" });
      }
      if (path.includes("/students/st-1/courses")) {
        return Promise.resolve([
          { id: "course-1", code: "MATH-101", name: "Math", teacher_name: "Teacher One", subject_code: "MATH", subject_name: "Math", student_count: 15, course_type: "General" },
        ]);
      }
      if (path === "/api/v1/courses") return Promise.resolve([{ id: "course-1", code: "MATH-101", name: "Math" }]);
      if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
      if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok" });
      if (path.startsWith("/api/v1/sessions?")) return Promise.resolve([session]);
      throw new Error(`Unexpected API call: ${path}`);
    });

    renderStudentProfile();
    await screen.findAllByText("Alice Smith");

    await waitFor(() => {
      // Session should appear in the 09:00 row (Bangkok time), not 02:00 (UTC)
      const row09 = screen.getByText("09:00").closest("tr");
      expect(row09).toBeInTheDocument();
      expect(row09!.textContent).toContain("MATH-101");

      // Session should NOT appear in the 02:00 row
      const row02 = screen.getByText("02:00").closest("tr");
      expect(row02!.textContent).not.toContain("MATH-101");
    });
  });

  it("navigates week with Prev/Today/Next buttons", async () => {
    renderStudentProfile();
    await screen.findAllByText("Alice Smith");

    const user = userEvent.setup();

    // Click Prev
    await user.click(screen.getByRole("button", { name: /‹ prev/i }));
    await waitFor(() => {
      expect(screen.getByText(/Weekly Schedule/)).toBeInTheDocument();
    });

    // Click Today
    await user.click(screen.getByRole("button", { name: /today/i }));
    await waitFor(() => {
      expect(screen.getByText(/Weekly Schedule/)).toBeInTheDocument();
    });

    // Click Next
    await user.click(screen.getByRole("button", { name: /next ›/i }));
    await waitFor(() => {
      expect(screen.getByText(/Weekly Schedule/)).toBeInTheDocument();
    });
  });

  it("opens edit modal and saves", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path.includes("/students/by-wcode")) {
        return Promise.resolve({ id: "st-1", wcode: "W001", full_name: "Alice Smith", notes: "old notes" });
      }
      if (path.includes("/students/st-1/courses")) return Promise.resolve([]);
      if (path === "/api/v1/courses") return Promise.resolve([]);
      if (path === "/api/v1/rooms") return Promise.resolve([]);
      if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok" });
      if (path.startsWith("/api/v1/sessions?")) return Promise.resolve([]);
      if (path.includes("/students/st-1") && init?.method === "PUT") {
        return Promise.resolve({ id: "st-1", wcode: "W001", full_name: "Alice Updated", notes: "new notes" });
      }
      throw new Error(`Unexpected API call: ${path}`);
    });

    renderStudentProfile();
    await screen.findAllByText("Alice Smith");

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /edit/i }));

    const nameInput = screen.getByDisplayValue("Alice Smith");
    await user.clear(nameInput);
    await user.type(nameInput, "Alice Updated");

    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(screen.getAllByText("Alice Updated").length).toBeGreaterThanOrEqual(1);
    });
  });
});
