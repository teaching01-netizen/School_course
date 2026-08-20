import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import SlotFinder from "../SlotFinder";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());
const mockFindAvailableSlots = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson, findAvailableSlots: mockFindAvailableSlots };
});

const STUDENTS = [{ id: "s1", wcode: "W001", full_name: "Test Student" }];
const SUBJECTS = [{ id: "sub-1", code: "MATH", name: "Math" }];
const COURSES = [
  { id: "c1", version: 1, code: "MATH-101", name: "Math", primary_teacher_id: "t1", subject_id: "sub-1", subject_code: "MATH", subject_name: "Math" },
];
const S1_COURSES = [
  { id: "c1", code: "MATH-101", name: "Math", subject_code: "MATH", subject_name: "Math" },
];

beforeEach(() => {
  mockApiJson.mockReset();
  mockFindAvailableSlots.mockReset();
  mockApiJson.mockImplementation(async (path: string) => {
    if (path.startsWith("/api/v1/students?limit=")) return { items: STUDENTS, total_count: STUDENTS.length, offset: 0, limit: 200 };
    if (path === "/api/v1/students/s1/courses") return S1_COURSES;
    if (path === "/api/v1/courses") return COURSES;
    if (path === "/api/v1/subjects") return SUBJECTS;
    return [];
  });
  mockFindAvailableSlots.mockImplementation(async () => ({ slots: [] }));
});

describe("SlotFinder empty state", () => {
  it("shows EmptyState after search returns no slots", async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <ToastProvider>
          <SlotFinder />
        </ToastProvider>
      </MemoryRouter>,
    );

    // Wait for the student lookup to load, then pick the student.
    await user.click(screen.getByRole("combobox", { name: "Student" }));
    await user.click(await screen.findByRole("option", { name: "W001 — Test Student" }));

    // Pick the subject; the search resolves to the student's MATH course.
    await user.click(screen.getByRole("combobox", { name: "Subject" }));
    await user.click(await screen.findByRole("option", { name: "MATH — Math" }));
    await screen.findByText(/Searching MATH-101 · Math/);

    await user.click(screen.getByRole("button", { name: /find slots/i }));
    expect(await screen.findByText(/No slots found in this range/)).toBeInTheDocument();
  });
});