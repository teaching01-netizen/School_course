import type { ReactElement } from "react";
import type { Mock } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ToastProvider } from "../../hooks/useToast";
import type { SitInRule } from "../../types";
import type { CourseLinkSuggestionsResponse } from "../../features/courses/types";
import type { CourseLevelItem, CourseMergeGroupConfig, RootCourseGroupInfo } from "../../utils/levels";

export const COURSES: CourseLevelItem[] = [
  { id: "c1", code: "MATH-101", name: "Algebra I", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: 1, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c2", code: "MATH-201", name: "Algebra II", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: 2, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c3", code: "MATH-301", name: "Algebra III", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: null, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c4", code: "PHYS-101", name: "Physics I", subject_id: "s2", subject_code: "PHYS", subject_name: "Physics", cycle_id: "cy2", cycle_label: "Cycle 2025-02", level: 1, root_course_group_id: null, root_course_group_name: null },
];

export const GROUPS: RootCourseGroupInfo[] = [
  { id: "g1", name: "SAT Math", course_count: 3, sit_in_rule_id: null },
  { id: "g2", name: "SAT Verbal", course_count: 0, sit_in_rule_id: null },
];

export const GROUPS_WITH_RULE: RootCourseGroupInfo[] = [
  { ...GROUPS[0], sit_in_rule_id: "rule-level-ladder" },
  GROUPS[1],
];

export const RULES: SitInRule[] = [
  { id: "rule-level-ladder", name: "Level Ladder", type: "level_ladder", predicate: {}, description: "Use the next eligible course level.", created_at: "", updated_at: "" },
];

export const LINKED_COURSES: CourseLevelItem[] = [
  { ...COURSES[0], level: 2 },
  { id: "c9", code: "MATH-901", name: "Algebra I (split)", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: null, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { ...COURSES[1], level: 3 },
];

export const CONTINUATION_GROUP: CourseMergeGroupConfig = {
  id: "mg1",
  name: "Same course: MATH-101 + MATH-901",
  level: 2,
  cycle_id: "cy1",
  cycle_label: "Cycle 2025-01",
  sit_in_rule_id: "rule-level-ladder",
  course_codes: ["MATH-101", "MATH-901"],
  course_names: ["Algebra I", "Algebra I (split)"],
  rule_source_course_code: "MATH-101",
};

export const CONTINUATION_GROUP_OVERRIDE: CourseMergeGroupConfig = {
  ...CONTINUATION_GROUP,
  level: 4,
};

export const MERGED_GROUP: CourseMergeGroupConfig = {
  ...CONTINUATION_GROUP,
  rule_source_course_code: null,
};

const DISABLED_SUGGESTIONS: CourseLinkSuggestionsResponse = {
  enabled: false,
  mode: "disabled",
  items: [],
  has_more: false,
  next_cursor: null,
  evaluated_at: "",
  detector_version: "",
  institute_timezone: "Asia/Bangkok",
};

export const EMPTY_SUGGESTIONS: CourseLinkSuggestionsResponse = {
  enabled: true,
  mode: "discovery",
  confirmation_enabled: false,
  items: [],
  has_more: false,
  next_cursor: null,
  evaluated_at: "2026-01-01T00:00:00Z",
  detector_version: "v1",
  institute_timezone: "Asia/Bangkok",
};

export type CourseLevelsApiOptions = {
  courses?: CourseLevelItem[];
  groups?: RootCourseGroupInfo[];
  rules?: SitInRule[];
  mergeGroups?: CourseMergeGroupConfig[];
  failures?: Partial<Record<"courses" | "groups" | "rules" | "mergeGroups" | "suggestions", string>>;
  suggestions?: CourseLinkSuggestionsResponse;
};

export function renderWithProviders(ui: ReactElement) {
  return <MemoryRouter><ToastProvider>{ui}</ToastProvider></MemoryRouter>;
}

export function setupCourseLevelsApi(mockApiJson: Mock, options: CourseLevelsApiOptions = {}) {
  const {
    courses = COURSES,
    groups = GROUPS,
    rules = RULES,
    mergeGroups = [],
    failures = {},
    suggestions = DISABLED_SUGGESTIONS,
  } = options;

  mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
    const method = init?.method ?? "GET";
    if (method === "GET") {
      if (path.startsWith("/api/v1/admin/course-levels")) {
        return failures.courses ? Promise.reject(new Error(failures.courses)) : Promise.resolve(courses);
      }
      if (path === "/api/v1/admin/root-course-groups") {
        return failures.groups ? Promise.reject(new Error(failures.groups)) : Promise.resolve(groups);
      }
      if (path === "/api/v1/admin/sit-in-rules") {
        return failures.rules ? Promise.reject(new Error(failures.rules)) : Promise.resolve(rules);
      }
      if (path === "/api/v1/admin/course-merge-groups") {
        return failures.mergeGroups ? Promise.reject(new Error(failures.mergeGroups)) : Promise.resolve(mergeGroups);
      }
      if (path.startsWith("/api/v1/admin/course-link-suggestions")) {
        return failures.suggestions ? Promise.reject(new Error(failures.suggestions)) : Promise.resolve(suggestions);
      }
    }
    return Promise.resolve({ ok: true });
  });
}
