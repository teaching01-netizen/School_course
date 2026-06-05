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

const BASE_COURSES = [
  { id: "c1", code: "MATH-101", name: "Algebra I", subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy2025a", cycle_label: "Cycle 2025-01", level: 1, root_course_group_id: null, root_course_group_name: null },
];

const BASE_ROOT_COURSE_GROUPS = [{ id: "g1", name: "SAT Math", course_count: 1 }];

const BASE_ACTIVE_COURSES = { subjects: [{ subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics", courses: [{ course_id: "c1", course_code: "MATH-101", course_name: "Algebra I", cycle_id: "cy2025a", cycle_label: "Cycle 2025-01", is_active: true }] }] };

const BASE_POLICIES = { absence_policies: { root_course_groups: { g1: { auto_sit_in_enabled: true } } } };

function renderWithProviders(ui: React.ReactElement) {
  return render(<MemoryRouter><ToastProvider>{ui}</ToastProvider></MemoryRouter>);
}

function setupDefault() {
  mockApiJson
    .mockResolvedValueOnce([])
    .mockResolvedValueOnce(BASE_COURSES)
    .mockResolvedValueOnce(BASE_POLICIES)
    .mockResolvedValueOnce(BASE_ROOT_COURSE_GROUPS)
    .mockResolvedValueOnce(BASE_ACTIVE_COURSES);
}

describe("CourseLevels - Cross-links and Info Banner", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders "Manage Rules" link pointing to OperationsHub rule-inventory tab', async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getByText("Course Levels")).toBeInTheDocument();
    });

    const manageRulesLink = screen.getByRole("link", { name: "Manage Rules" });
    expect(manageRulesLink).toBeInTheDocument();
    expect(manageRulesLink).toHaveAttribute("href", "/operations?tab=rule-inventory");
  });

  it('info banner explains sit-in rules with "Manage Rules" reference', async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getByText("How sit-in rules work")).toBeInTheDocument();
    });

    expect(screen.getByText(/Each root course group needs a sit-in rule/)).toBeInTheDocument();
    expect(screen.getByText(/Level 1 students attend via Zoom/)).toBeInTheDocument();
  });
});
