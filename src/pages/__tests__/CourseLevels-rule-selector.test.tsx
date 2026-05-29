import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
    root_course_group_id: "g1",
    root_course_group_name: "SAT Math",
  },
];

const BASE_ROOT_COURSE_GROUPS = [
  {
    id: "g1",
    name: "SAT Math",
    course_count: 1,
    sit_in_rule_id: "rule-1",
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
      ],
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

const BASE_SIT_IN_RULES = {
  sit_in_rules: [
    { id: "rule-1", name: "Level Ladder Default", type: "level_ladder", description: "Default level ladder" },
    { id: "rule-2", name: "Cross-Section Rule", type: "cross_section", description: "Cross-section rule" },
  ],
};

function renderWithProviders(ui: React.ReactElement) {
  return render(<MemoryRouter><ToastProvider>{ui}</ToastProvider></MemoryRouter>);
}

describe("CourseLevels - RuleSelector", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  function setupDefault() {
    mockApiJson
      .mockResolvedValueOnce(BASE_SIT_IN_RULES)
      .mockResolvedValueOnce(BASE_COURSES)
      .mockResolvedValueOnce(BASE_POLICIES)
      .mockResolvedValueOnce(BASE_ROOT_COURSE_GROUPS)
      .mockResolvedValueOnce(BASE_ACTIVE_COURSES);
  }

  it("renders RuleSelector in action bar with assigned rule", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getAllByText("MATH-101").length).toBeGreaterThanOrEqual(1);
    });

    const select = screen.getByRole("combobox", { name: /sit-in rule/i });
    expect(select).toBeInTheDocument();
    // Should show the currently assigned rule (rule-1)
    expect(select).toHaveValue("rule-1");
  });

  it("shows 'No rule assigned' when no rule is set", async () => {
    const noRuleGroups = [
      { ...BASE_ROOT_COURSE_GROUPS[0], sit_in_rule_id: null },
    ];
    mockApiJson
      .mockResolvedValueOnce(BASE_SIT_IN_RULES)
      .mockResolvedValueOnce(BASE_COURSES)
      .mockResolvedValueOnce(BASE_POLICIES)
      .mockResolvedValueOnce(noRuleGroups)
      .mockResolvedValueOnce(BASE_ACTIVE_COURSES);

    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getAllByText("MATH-101").length).toBeGreaterThanOrEqual(1);
    });

    const select = screen.getByRole("combobox", { name: /sit-in rule/i });
    expect(select).toHaveValue("");
    expect(screen.getByText("No rule assigned")).toBeInTheDocument();
    expect(screen.getByText(/No sit-in rule — students cannot sit into/)).toBeInTheDocument();
  });

  it("shows confirmation dialog when changing rule", async () => {
    setupDefault();
    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getAllByText("MATH-101").length).toBeGreaterThanOrEqual(1);
    });

    const user = userEvent.setup();
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    const select = screen.getByRole("combobox", { name: /sit-in rule/i });

    await user.selectOptions(select, "rule-2");

    expect(confirmSpy).toHaveBeenCalledWith(
      expect.stringContaining("Level Ladder Default"),
    );
    expect(confirmSpy).toHaveBeenCalledWith(
      expect.stringContaining("Cross-Section Rule"),
    );
    // Should not call API when cancelled
    expect(mockApiJson).not.toHaveBeenCalledWith(
      "/api/v1/admin/root-course-groups/g1",
      expect.anything(),
    );
    confirmSpy.mockRestore();
  });

  it("saves rule assignment after confirmation", async () => {
    setupDefault();
    // Add the PUT response
    mockApiJson.mockResolvedValueOnce({ ok: true });

    renderWithProviders(<CourseLevels />);

    await waitFor(() => {
      expect(screen.getAllByText("MATH-101").length).toBeGreaterThanOrEqual(1);
    });

    const user = userEvent.setup();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const select = screen.getByRole("combobox", { name: /sit-in rule/i });

    await user.selectOptions(select, "rule-2");

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/root-course-groups/g1",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ sit_in_rule_id: "rule-2" }),
        }),
      );
    });
  });
});
