import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
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
  { id: "c2", code: "MATH-201", name: "Algebra II", subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy2025b", cycle_label: "Cycle 2025-02", level: 2, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c3", code: "PHYS-101", name: "Physics I", subject_id: "subj-2", subject_code: "PHYS", subject_name: "Physics", cycle_id: "cy2025b", cycle_label: "Cycle 2025-02", level: 1, root_course_group_id: "g2", root_course_group_name: "SAT Physics" },
  { id: "c4", code: "PHYS-201", name: "Physics II", subject_id: "subj-2", subject_code: "PHYS", subject_name: "Physics", cycle_id: "cy2025b", cycle_label: "Cycle 2025-02", level: null, root_course_group_id: "g2", root_course_group_name: "SAT Physics" },
];

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

function activeCoursesResponse(opts?: { physActive?: boolean }) {
  return {
    subjects: [
      {
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        courses: [
          { course_id: "c1", course_code: "MATH-101", course_name: "Algebra I", cycle_id: "cy2025a", cycle_label: "Cycle 2025-01", is_active: true },
          { course_id: "c2", course_code: "MATH-201", course_name: "Algebra II", cycle_id: "cy2025b", cycle_label: "Cycle 2025-02", is_active: false },
        ],
      },
      {
        subject_id: "subj-2",
        subject_code: "PHYS",
        subject_name: "Physics",
        courses: [
          { course_id: "c3", course_code: "PHYS-101", course_name: "Physics I", cycle_id: "cy2025b", cycle_label: "Cycle 2025-02", is_active: opts?.physActive ?? true },
          { course_id: "c4", course_code: "PHYS-201", course_name: "Physics II", cycle_id: "cy2025b", cycle_label: "Cycle 2025-02", is_active: false },
        ],
      },
    ],
  };
}

function renderWithProviders(ui: React.ReactElement) {
  return render(<MemoryRouter><ToastProvider>{ui}</ToastProvider></MemoryRouter>);
}

function setupDefault(activeCourses = activeCoursesResponse()) {
  mockApiJson
    .mockResolvedValueOnce(BASE_SIT_IN_RULES)
    .mockResolvedValueOnce(BASE_COURSES)
    .mockResolvedValueOnce(BASE_POLICIES)
    .mockResolvedValueOnce(BASE_ROOT_COURSE_GROUPS)
    .mockResolvedValueOnce(activeCourses);
}

async function openActiveView() {
  await screen.findAllByText("MATH-101");
  await screen.findByRole("region", { name: "Active courses" });
}

describe("CourseLevels - Active Courses view", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiJson.mockReset();
  });

  it("lists every subject with its current active course", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);
    await openActiveView();

    expect(screen.getByRole("button", { name: "Active courses" })).toHaveAttribute("aria-pressed", "true");
    const mathRow = screen.getAllByText("MATH-101")[0].closest("tr")!;
    expect(within(mathRow).getByText("MATH")).toBeInTheDocument();
    expect(within(mathRow).getByText("Cycle 2025-01")).toBeInTheDocument();
    expect(screen.getByText("2 of 2 subjects have an active course")).toBeInTheDocument();
  });

  it("flags subjects without an active course and offers a select-all-unset helper", async () => {
    setupDefault(activeCoursesResponse({ physActive: false }));
    renderWithProviders(<CourseLevels />);
    await openActiveView();

    expect(screen.getByText("1 not set")).toBeInTheDocument();
    expect(screen.queryByText("PHYS-101")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Select subjects without an active course" }));

    const physCheck = screen.getByRole("checkbox", { name: "Select PHYS" }) as HTMLInputElement;
    expect(physCheck.checked).toBe(true);
    const mathCheck = screen.getByRole("checkbox", { name: "Select MATH" }) as HTMLInputElement;
    expect(mathCheck.checked).toBe(false);
  });

  it("quick change saves immediately via PUT", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);
    await openActiveView();

    mockApiJson.mockResolvedValueOnce({ status: "ok" });
    fireEvent.change(screen.getByRole("combobox", { name: "Active course for MATH" }), {
      target: { value: "c2" },
    });

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/active-courses",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ subject_id: "subj-1", course_id: "c2" }),
        }),
      );
    });
  });

  it("bulk change applies one PUT per selected subject with preselected courses", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);
    await openActiveView();

    fireEvent.click(screen.getByRole("button", { name: "Bulk change" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Select MATH" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Select PHYS" }));
    fireEvent.click(screen.getByRole("button", { name: "Change active courses…" }));

    const dialog = screen.getByRole("dialog", { name: "Change active courses" });
    // Defaults keep the current active course where one exists.
    expect(within(dialog).getByRole("combobox", { name: "New active course for MATH" })).toHaveValue("c1");
    expect(within(dialog).getByRole("combobox", { name: "New active course for PHYS" })).toHaveValue("c3");

    fireEvent.change(screen.getByRole("combobox", { name: "New active course for MATH" }), {
      target: { value: "c2" },
    });

    mockApiJson.mockResolvedValue({ status: "ok" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Apply to 2 subjects" }));

    await waitFor(() => {
      const putCalls = mockApiJson.mock.calls.filter(
        (call) =>
          call[0] === "/api/v1/admin/active-courses" &&
          (call[1] as RequestInit | undefined)?.method === "PUT",
      );
      expect(putCalls).toHaveLength(2);
      expect(putCalls[0][1].body).toBe(JSON.stringify({ subject_id: "subj-1", course_id: "c2" }));
      expect(putCalls[1][1].body).toBe(JSON.stringify({ subject_id: "subj-2", course_id: "c3" }));
    });
    expect(await within(dialog).findByText("2 updated")).toBeInTheDocument();
  });

  it("bulk change reports per-subject failures", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);
    await openActiveView();

    fireEvent.click(screen.getByRole("button", { name: "Bulk change" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Select MATH" }));
    fireEvent.click(screen.getByRole("button", { name: "Change active course…" }));

    const dialog = screen.getByRole("dialog", { name: "Change active course" });
    mockApiJson.mockRejectedValueOnce(new Error("boom"));
    fireEvent.click(within(dialog).getByRole("button", { name: "Apply to 1 subject" }));

    expect(await within(dialog).findByText(/1 failed/)).toBeInTheDocument();
    expect(within(dialog).getByText(/Failed to set active course for MATH/)).toBeInTheDocument();
  });

  it("classic view shows a read-only active course chip linking to the view", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);
    fireEvent.click(await screen.findByRole("button", { name: "All levels" }));
    await screen.findAllByText("SAT Math");

    expect(screen.getAllByText("Active:").length).toBeGreaterThan(0);
    expect(screen.getAllByText("MATH-101").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: /Active course selector/i })).not.toBeInTheDocument();

    fireEvent.click(screen.getAllByRole("button", { name: "Change" })[0]);
    await screen.findByRole("region", { name: "Active courses" });
  });
});
