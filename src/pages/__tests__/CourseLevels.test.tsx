import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import CourseLevels from "../CourseLevels";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

// Mock localStorage for hooks that use it
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
  {
    id: "c1",
    code: "MATH-101",
    name: "Algebra I",
    subject_id: "subj-1",
    subject_code: "MATH",
    subject_name: "Mathematics",
    cycle_id: "cy2025a",
    cycle_label: "Cycle 2025-01",
    level: 1,
    root_course_group_id: null,
    root_course_group_name: null,
  },
  {
    id: "c2",
    code: "MATH-201",
    name: "Algebra II",
    subject_id: "subj-1",
    subject_code: "MATH",
    subject_name: "Mathematics",
    cycle_id: "cy2025a",
    cycle_label: "Cycle 2025-01",
    level: 2,
    root_course_group_id: "g1",
    root_course_group_name: "SAT Math",
  },
  {
    id: "c3",
    code: "PHYS-101",
    name: "Physics I",
    subject_id: "subj-2",
    subject_code: "PHYS",
    subject_name: "Physics",
    cycle_id: "cy2025b",
    cycle_label: "Cycle 2025-02",
    level: 1,
    root_course_group_id: null,
    root_course_group_name: null,
  },
  {
    id: "c4",
    code: "PHYS-201",
    name: "Physics II",
    subject_id: "subj-2",
    subject_code: "PHYS",
    subject_name: "Physics",
    cycle_id: "cy2025b",
    cycle_label: "Cycle 2025-02",
    level: null,
    root_course_group_id: null,
    root_course_group_name: null,
  },
];

const BASE_ROOT_COURSE_GROUPS = [
  {
    id: "g1",
    name: "SAT Math",
    course_count: 1,
  },
];

const BASE_ACTIVE_COURSES = {
  subjects: [
    {
      subject_id: "subj-1",
      subject_code: "MATH",
      subject_name: "Mathematics",
      courses: [
        {
          course_id: "c1",
          course_code: "MATH-101",
          course_name: "Algebra I",
          cycle_id: "cy2025a",
          cycle_label: "Cycle 2025-01",
          is_active: true,
        },
        {
          course_id: "c2",
          course_code: "MATH-201",
          course_name: "Algebra II",
          cycle_id: "cy2025a",
          cycle_label: "Cycle 2025-01",
          is_active: false,
        },
      ],
    },
    {
      subject_id: "subj-2",
      subject_code: "PHYS",
      subject_name: "Physics",
      courses: [],
    },
  ],
};

const BASE_POLICIES = {
  absence_policies: {
    root_course_groups: {
      g1: { auto_sit_in_enabled: true },
    },
  },
};

function renderWithProviders(ui: React.ReactElement) {
  return render(<MemoryRouter><ToastProvider>{ui}</ToastProvider></MemoryRouter>);
}

describe("CourseLevels", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  function setupDefault() {
    mockApiJson
      .mockResolvedValueOnce({ sit_in_rules: [] })
      .mockResolvedValueOnce(BASE_COURSES)
      .mockResolvedValueOnce(BASE_POLICIES)
      .mockResolvedValueOnce(BASE_ROOT_COURSE_GROUPS)
      .mockResolvedValueOnce(BASE_ACTIVE_COURSES);
  }

  it("renders root groups with subject and cycle sub-headers", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getAllByText("MATH \u2014 Mathematics").length).toBeGreaterThanOrEqual(2);
    });
    expect(screen.getByText("PHYS \u2014 Physics")).toBeInTheDocument();
    expect(screen.getAllByText("Cycle 2025-01").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Cycle 2025-02")).toBeInTheDocument();
  });

  it("shows level input per course row", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeInTheDocument();
    });
    // LevelStepper shows the level value as text, not input
    const stepperValues = screen.getAllByText("1");
    expect(stepperValues.length).toBeGreaterThanOrEqual(2);
  });

  it("shows auto-computed status badge (Zoom for L1, Eligible for L2+)", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getAllByText(/Zoom/, { selector: "span" }).length).toBe(2);
    });
    expect(screen.getByText(/Eligible/)).toBeInTheDocument();
    expect(screen.getByText(/Not set/)).toBeInTheDocument();
  });

  it("shows gap warning when levels non-consecutive", async () => {
    const gapCourses = [
      BASE_COURSES[0],
      {
        ...BASE_COURSES[1],
        id: "c2-gap",
        code: "MATH-301",
        name: "Algebra III",
        level: 3,
        root_course_group_id: null,
        root_course_group_name: null,
      },
      ...BASE_COURSES.slice(2),
    ];

    mockApiJson
      .mockResolvedValueOnce({ sit_in_rules: [] })
      .mockResolvedValueOnce(gapCourses)
      .mockResolvedValueOnce(BASE_POLICIES)
      .mockResolvedValueOnce(BASE_ROOT_COURSE_GROUPS)
      .mockResolvedValueOnce(BASE_ACTIVE_COURSES);

    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      const gapRow = screen.getByText("MATH-301").closest("tr")!;
      expect(within(gapRow).getByText(/No Level 2/)).toBeInTheDocument();
    });
  });

  it("save level calls API with correct body", async () => {
    setupDefault();

    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeInTheDocument();
    });

    const math101Row = screen.getByText("MATH-101").closest("tr")!;
    const user = userEvent.setup();

    // Click the increase button on the LevelStepper to change level from 1 to 2
    const increaseBtn = within(math101Row).getByRole("button", { name: /increase level/i });
    await user.click(increaseBtn);

    const saveBtn = math101Row.querySelector("button[class*='primary']")!;
    await user.click(saveBtn);

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/courses/c1/level",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ level: 2, cycle_id: "cy2025a" }),
        }),
      );
    });
  });

  it("renders root course group headers", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(
        screen.getAllByText("SAT Math").length,
      ).toBeGreaterThanOrEqual(1);
    });
  });

  it("renders (none) group for ungrouped courses", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(
        screen.getAllByText("(none \u2014 ungrouped)").length,
      ).toBeGreaterThanOrEqual(1);
    });
  });

  it("group typeahead column changes root course group", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getByText("MATH-101")).toBeInTheDocument();
    });

    const math101Row = screen.getByText("MATH-101").closest("tr")!;
    const typeaheadInput = within(math101Row).getByRole("combobox");
    const user = userEvent.setup();

    await user.click(typeaheadInput);
    await user.type(typeaheadInput, "SAT Math");

    const option = await screen.findByRole("option", { name: "SAT Math" });
    await user.click(option);

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/courses/c1/root-course-group",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ root_course_group_id: "g1" }),
        }),
      );
    });
  });

  it("auto-sit-in toggle per root course group saves with root_course_groups key", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(
        screen.getAllByText("SAT Math").length,
      ).toBeGreaterThanOrEqual(1);
    });

    const user = userEvent.setup();
    const checkbox = screen.getByRole("checkbox", { name: /auto sit-in for sat math/i });
    expect(checkbox).toBeChecked();
    await user.click(checkbox);

    await user.click(screen.getByRole("button", { name: /save auto sit-in for sat math/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/absence-policies",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({
            absence_policies: {
              root_course_groups: {
                g1: { auto_sit_in_enabled: false },
              },
            },
          }),
        }),
      );
    });
  });

  it("bulk edits levels in a root course group", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /bulk edit levels for sat math/i }));

    // The bulk edit modal uses LevelStepper; find the stepper for MATH-201 and increase
    const modal = screen.getByRole("dialog");
    const math201Row = within(modal).getByText("MATH-201").closest("tr")!;
    const increaseBtn = within(math201Row).getByRole("button", { name: /increase level/i });
    // Click increase twice: level 2 → 3 → 4
    await user.click(increaseBtn);
    await user.click(increaseBtn);

    await user.click(screen.getByRole("button", { name: /apply all changes/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/courses/c2/level",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ level: 4, cycle_id: "cy2025a" }),
        }),
      );
    });
  });

  it("verify all identifies missing level configuration", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /verify all/i }));

    expect(screen.getByText(/1 course has no level set/i)).toBeInTheDocument();
  });
});
