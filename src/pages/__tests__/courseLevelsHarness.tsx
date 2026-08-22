import type { ReactElement } from "react";
import { MemoryRouter } from "react-router-dom";
import { ToastProvider } from "../../hooks/useToast";

export const COURSES = [
  { id: "c1", code: "MATH-101", name: "Algebra I", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: 1, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c2", code: "MATH-201", name: "Algebra II", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: 2, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c3", code: "MATH-301", name: "Algebra III", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: null, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c4", code: "PHYS-101", name: "Physics I", subject_id: "s2", subject_code: "PHYS", subject_name: "Physics", cycle_id: "cy2", cycle_label: "Cycle 2025-02", level: 1, root_course_group_id: null, root_course_group_name: null },
];

export const GROUPS = [
  { id: "g1", name: "SAT Math", course_count: 3, sit_in_rule_id: null },
  { id: "g2", name: "SAT Verbal", course_count: 0, sit_in_rule_id: null },
];

export function renderWithProviders(ui: ReactElement) {
  return <MemoryRouter><ToastProvider>{ui}</ToastProvider></MemoryRouter>;
}

export function setupCourseLevelsApi(mockApiJson: { mockResolvedValueOnce: (value: unknown) => unknown }, courses = COURSES, groups = GROUPS) {
  mockApiJson.mockResolvedValueOnce(courses).mockResolvedValueOnce(groups);
}
