import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import CourseLevels from "../CourseLevels";
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

const BASE_ROOT_COURSE_GROUPS = [
  { id: "g1", name: "SAT Math", course_count: 2, sit_in_rule_id: "rule-1" },
  { id: "g2", name: "SAT Physics", course_count: 2, sit_in_rule_id: null },
];

const BASE_COURSES = [
  { id: "c1", code: "MATH-101", name: "Algebra I", subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy2025a", cycle_label: "Cycle 2025-01", level: 1, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c2", code: "MATH-201", name: "Algebra II", subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy2025a", cycle_label: "Cycle 2025-01", level: 2, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c3", code: "PHYS-101", name: "Physics I", subject_id: "subj-2", subject_code: "PHYS", subject_name: "Physics", cycle_id: "cy2025b", cycle_label: "Cycle 2025-02", level: 1, root_course_group_id: "g2", root_course_group_name: "SAT Physics" },
  { id: "c4", code: "PHYS-201", name: "Physics II", subject_id: "subj-2", subject_code: "PHYS", subject_name: "Physics", cycle_id: "cy2025b", cycle_label: "Cycle 2025-02", level: null, root_course_group_id: "g2", root_course_group_name: "SAT Physics" },
];

const BASE_ACTIVE_COURSES = {
  subjects: [
    { subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics", courses: [{ course_id: "c1", course_code: "MATH-101", course_name: "Algebra I", cycle_id: "cy2025a", cycle_label: "Cycle 2025-01", is_active: true }] },
    { subject_id: "subj-2", subject_code: "PHYS", subject_name: "Physics", courses: [{ course_id: "c3", course_code: "PHYS-101", course_name: "Physics I", cycle_id: "cy2025b", cycle_label: "Cycle 2025-02", is_active: true }] },
  ],
};

const BASE_POLICIES = {
  absence_policies: {
    root_course_groups: {
      g1: { auto_sit_in_enabled: true },
      g2: { auto_sit_in_enabled: true },
    },
  },
};

const BASE_SIT_IN_RULES = [
  { id: "rule-1", name: "Level Ladder Default", type: "level_ladder", description: "Default level ladder" },
];

function renderWithProviders(ui: React.ReactElement) {
  return render(<MemoryRouter><ToastProvider>{ui}</ToastProvider></MemoryRouter>);
}

function setupDefault() {
  mockApiJson
    .mockResolvedValueOnce(BASE_SIT_IN_RULES)
    .mockResolvedValueOnce(BASE_COURSES)
    .mockResolvedValueOnce(BASE_POLICIES)
    .mockResolvedValueOnce(BASE_ROOT_COURSE_GROUPS)
    .mockResolvedValueOnce(BASE_ACTIVE_COURSES);
}

describe("CourseLevels - Status Overview Bar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders overview bar with configured and missing rule counts", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getByText("1 configured")).toBeInTheDocument();
    });
    expect(screen.getByText("1 missing rule")).toBeInTheDocument();
  });

  it("does not render missing rule badge when all groups have rules", async () => {
    const allConfiguredGroups = BASE_ROOT_COURSE_GROUPS.map(g => ({ ...g, sit_in_rule_id: "rule-1" }));
    mockApiJson
      .mockResolvedValueOnce(BASE_SIT_IN_RULES)
      .mockResolvedValueOnce(BASE_COURSES)
      .mockResolvedValueOnce(BASE_POLICIES)
      .mockResolvedValueOnce(allConfiguredGroups)
      .mockResolvedValueOnce(BASE_ACTIVE_COURSES);

    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getByText("2 configured")).toBeInTheDocument();
    });
    expect(screen.queryByText(/missing rule/)).not.toBeInTheDocument();
  });

  it("shows gap count when groups have level gaps", async () => {
    const gapCourses = [
      { id: "c1", code: "MATH-101", name: "Algebra I", subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy2025a", cycle_label: "Cycle 2025-01", level: 1, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
      { id: "c2", code: "MATH-301", name: "Algebra III", subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy2025a", cycle_label: "Cycle 2025-01", level: 3, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
    ];
    mockApiJson
      .mockResolvedValueOnce(BASE_SIT_IN_RULES)
      .mockResolvedValueOnce(gapCourses)
      .mockResolvedValueOnce(BASE_POLICIES)
      .mockResolvedValueOnce(BASE_ROOT_COURSE_GROUPS.slice(0, 1))
      .mockResolvedValueOnce({ subjects: [{ subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics", courses: [{ course_id: "c1", course_code: "MATH-101", course_name: "Algebra I", cycle_id: "cy2025a", cycle_label: "Cycle 2025-01", is_active: true }] }] });

    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getByText("1 configured")).toBeInTheDocument();
    });
    expect(screen.getByText("1 with gaps")).toBeInTheDocument();
  });

  it("does not render overview bar when there are no groups", async () => {
    mockApiJson
      .mockResolvedValueOnce(BASE_SIT_IN_RULES)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce(BASE_POLICIES)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce({ subjects: [] });

    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getByText("Course Levels")).toBeInTheDocument();
    });
    expect(screen.queryByText("configured")).not.toBeInTheDocument();
    expect(screen.queryByText("missing rule")).not.toBeInTheDocument();
    expect(screen.queryByText("with gaps")).not.toBeInTheDocument();
  });
});
