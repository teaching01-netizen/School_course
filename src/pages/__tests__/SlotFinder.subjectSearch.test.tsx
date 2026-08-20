import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import SlotFinder, { resolveSubjectCourse } from "../SlotFinder";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());
const mockFindAvailableSlots = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson, findAvailableSlots: mockFindAvailableSlots };
});

const STUDENTS = [
  { id: "st1", wcode: "W10245", full_name: "Ariya S." },
  { id: "st2", wcode: "W11002", full_name: "Boon M." },
];
const SUBJECTS = [
  { id: "sub-math", code: "MATH", name: "Calculus" },
  { id: "sub-phy", code: "PHYS", name: "Physics" },
];
const COURSES = [
  { id: "course-1", version: 1, code: "MATH-101", name: "Calculus", primary_teacher_id: "teacher-1", subject_id: "sub-math", subject_code: "MATH", subject_name: "Calculus" },
  { id: "course-3", version: 1, code: "PHYS-201", name: "Waves", primary_teacher_id: "teacher-2", subject_id: "sub-phy", subject_code: "PHYS", subject_name: "Physics" },
];
const ST1_COURSES = [
  { id: "course-1", code: "MATH-101", name: "Calculus", subject_code: "MATH", subject_name: "Calculus" },
  { id: "course-3", code: "PHYS-201", name: "Waves", subject_code: "PHYS", subject_name: "Physics" },
];
const ST2_COURSES = [
  { id: "course-9", code: "CHEM-110", name: "Molecules", subject_code: "CHEM", subject_name: "Chemistry" },
];

function renderSlotFinder() {
  render(
    <MemoryRouter initialEntries={["/slot-finder"]}>
      <ToastProvider>
        <Routes>
          <Route path="/slot-finder" element={<SlotFinder />} />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  mockApiJson.mockReset();
  mockFindAvailableSlots.mockReset();
  mockApiJson.mockImplementation(async (path: string) => {
    if (path.startsWith("/api/v1/students?limit=")) return { items: STUDENTS, total_count: STUDENTS.length, offset: 0, limit: 200 };
    if (path === "/api/v1/students/st1/courses") return ST1_COURSES;
    if (path === "/api/v1/students/st2/courses") return ST2_COURSES;
    if (path === "/api/v1/courses") return COURSES;
    if (path === "/api/v1/subjects") return SUBJECTS;
    return [];
  });
  mockFindAvailableSlots.mockImplementation(async () => ({ slots: [] }));
});

async function pickStudent(user: ReturnType<typeof userEvent.setup>, label: string) {
  await user.click(screen.getByRole("combobox", { name: "Student" }));
  await user.click(screen.getByRole("option", { name: label }));
}

async function pickSubject(user: ReturnType<typeof userEvent.setup>, label: string) {
  await user.click(screen.getByRole("combobox", { name: "Subject" }));
  await user.click(screen.getByRole("option", { name: label }));
}

describe("resolveSubjectCourse", () => {
  it("matches by subject code", () => {
    const subject = { id: "sub-math", code: "MATH", name: "Calculus" };
    expect(resolveSubjectCourse(ST1_COURSES, subject)?.id).toBe("course-1");
  });

  it("matches by subject name when the code differs", () => {
    const subject = { id: "sub-calc", code: "X", name: "Calculus" };
    expect(resolveSubjectCourse(ST1_COURSES, subject)?.id).toBe("course-1");
  });

  it("does not match an empty subject code field", () => {
    const courses = [{ id: "c0", code: "MISC-1", name: "Misc", subject_code: "", subject_name: "" }];
    expect(resolveSubjectCourse(courses, { id: "sub-math", code: "MATH", name: "Calculus" })).toBeNull();
  });

  it("returns the first matching course when a subject has several", () => {
    const withSecond = [...ST1_COURSES, { id: "course-2", code: "MATH-102", name: "Calculus II", subject_code: "MATH", subject_name: "Calculus" }];
    expect(resolveSubjectCourse(withSecond, { id: "sub-math", code: "MATH", name: "Calculus" })?.id).toBe("course-1");
  });

  it("returns null without a subject or without a match", () => {
    expect(resolveSubjectCourse(ST1_COURSES, null)).toBeNull();
    expect(resolveSubjectCourse(ST1_COURSES, { id: "sub-phy", code: "PHYS", name: "Physics" })?.id).toBe("course-3");
    expect(resolveSubjectCourse([], { id: "sub-math", code: "MATH", name: "Calculus" })).toBeNull();
  });
});

describe("SlotFinder subject search", () => {
  it("searches by subject instead of course, with searchable student and subject fields", async () => {
    const user = userEvent.setup();
    renderSlotFinder();

    expect(screen.queryByLabelText("Course")).not.toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Subject" })).toBeInTheDocument();
    const studentInput = screen.getByRole("combobox", { name: "Student" });

    await user.click(studentInput);
    expect(screen.getByRole("option", { name: "W10245 — Ariya S." })).toBeInTheDocument();
    await user.click(screen.getByRole("option", { name: "W11002 — Boon M." }));
    expect(studentInput).toHaveValue("W11002 — Boon M.");
  });

  it("filters subject options as the user types", async () => {
    const user = userEvent.setup();
    renderSlotFinder();

    const subjectInput = screen.getByRole("combobox", { name: "Subject" });
    await user.click(subjectInput);
    await user.type(subjectInput, "phys");

    expect(screen.getByRole("option", { name: "PHYS — Physics" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "MATH — Calculus" })).not.toBeInTheDocument();
  });

  it("resolves the student's course in the chosen subject and searches with it", async () => {
    const user = userEvent.setup();
    renderSlotFinder();
    await pickStudent(user, "W10245 — Ariya S.");
    await pickSubject(user, "MATH — Calculus");

    await screen.findByText(/Searching MATH-101 · Calculus/);
    await user.click(screen.getByRole("button", { name: /Find Slots/i }));

    await waitFor(() =>
      expect(mockFindAvailableSlots).toHaveBeenCalledWith(
        expect.objectContaining({ student_id: "st1", course_id: "course-1" }),
      ),
    );
    expect(await screen.findByText(/No slots found in this range/)).toBeInTheDocument();
  });

  it("explains when the student has no course in the chosen subject", async () => {
    const user = userEvent.setup();
    renderSlotFinder();
    await pickStudent(user, "W11002 — Boon M.");
    await pickSubject(user, "MATH — Calculus");

    await screen.findByText(/No MATH course on this student's list/);
    expect(screen.getByRole("button", { name: /Find Slots/i })).toBeDisabled();
  });

  it("asks for a student before resolving the subject", async () => {
    const user = userEvent.setup();
    renderSlotFinder();
    await pickSubject(user, "MATH — Calculus");

    await screen.findByText(/Pick a student to find their course in this subject/);
    expect(screen.getByRole("button", { name: /Find Slots/i })).toBeDisabled();
  });

  it("re-resolves when the student changes", async () => {
    const user = userEvent.setup();
    renderSlotFinder();
    await pickStudent(user, "W10245 — Ariya S.");
    await pickSubject(user, "MATH — Calculus");
    await screen.findByText(/Searching MATH-101 · Calculus/);

    await pickStudent(user, "W11002 — Boon M.");
    await screen.findByText(/No MATH course on this student's list/);
    expect(screen.queryByText(/Searching MATH-101/)).not.toBeInTheDocument();
  });
});