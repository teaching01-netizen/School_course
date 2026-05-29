// === Shared types for Course Level Ladder ===

export type CourseLevelItem = {
  id: string;
  code: string;
  name: string;
  subject_id: string;
  subject_code: string;
  subject_name: string;
  cycle_id: string;
  cycle_label: string;
  level: number | null;
  root_course_group_id: string | null;
  root_course_group_name: string | null;
};

export type SubjectPolicy = {
  auto_sit_in_enabled: boolean;
};

export type PolicyResponse = {
  absence_policies: {
    root_course_groups: Record<string, SubjectPolicy>;
  };
};

export type RootCourseGroupInfo = {
  id: string;
  name: string;
  course_count: number;
  sit_in_rule_id: string | null;
};

export type GroupWithCount = {
  id: string;
  name: string;
  course_count: number;
};

export type CycleGroup = {
  cycleId: string;
  cycleLabel: string;
  courses: CourseLevelItem[];
};

export type BulkEditTarget = {
  label: string;
  courses: CourseLevelItem[];
};

export type SubjectInRoot = {
  subjectId: string;
  subjectCode: string;
  subjectName: string;
  cycles: Record<string, CycleGroup>;
};

export type RootGroupHierarchy = {
  rootCourseGroupId: string | null;
  label: string;
  subjects: SubjectInRoot[];
};

export type GapInfo = {
  level: number;
  message: string;
};

export const UNGROUPED_KEY = "__none__";

// === Pure utility functions ===

export function getGapWarning(courses: { level: number | null }[]): string | null {
  const levels = courses
    .map((c) => c.level)
    .filter((l): l is number => l !== null)
    .sort((a, b) => a - b);

  if (levels.length < 2) return null;

  const gaps: number[] = [];
  for (let i = levels[0]; i < levels[levels.length - 1]; i++) {
    if (!levels.includes(i)) gaps.push(i);
  }

  if (gaps.length === 0) return null;

  return `No Level ${gaps[0]} — Level ${levels[0]} students will skip to Level ${levels[levels.length - 1]}`;
}

export function detectGaps(courses: CourseLevelItem[]): GapInfo[] {
  const levels = courses
    .map((c) => c.level)
    .filter((l): l is number => l !== null)
    .sort((a, b) => a - b);

  if (levels.length < 2) return [];

  const gaps: GapInfo[] = [];
  for (let i = levels[0]; i < levels[levels.length - 1]; i++) {
    if (!levels.includes(i)) {
      gaps.push({ level: i, message: `Level ${i} is missing` });
    }
  }
  return gaps;
}

export function computeLevelCompletion(subjectCourses: CourseLevelItem[]): { assigned: number; total: number } {
  const total = subjectCourses.length;
  const assigned = subjectCourses.filter((c) => c.level !== null).length;
  return { assigned, total };
}

export function buildRootGroupHierarchy(
  courses: CourseLevelItem[],
): Record<string, RootGroupHierarchy> {
  const roots: Record<string, RootGroupHierarchy> = {};

  for (const c of courses) {
    const rootKey = c.root_course_group_id ?? UNGROUPED_KEY;
    if (!roots[rootKey]) {
      roots[rootKey] = {
        rootCourseGroupId: c.root_course_group_id,
        label: rootKey === UNGROUPED_KEY ? "(none — ungrouped)" : (c.root_course_group_name ?? rootKey),
        subjects: [],
      };
    }

    const root = roots[rootKey];
    let subject = root.subjects.find((s) => s.subjectId === c.subject_id);
    if (!subject) {
      subject = {
        subjectId: c.subject_id,
        subjectCode: c.subject_code,
        subjectName: c.subject_name,
        cycles: {},
      };
      root.subjects.push(subject);
    }

    if (!subject.cycles[c.cycle_id]) {
      subject.cycles[c.cycle_id] = { cycleId: c.cycle_id, cycleLabel: c.cycle_label, courses: [] };
    }
    subject.cycles[c.cycle_id].courses.push(c);
  }

  for (const root of Object.values(roots)) {
    root.subjects.sort((a, b) => a.subjectCode.localeCompare(b.subjectCode));
  }

  return roots;
}
