export type LeavePolicyRuleType =
  | "cross_section"
  | "any_day_except_last"
  | "rank_chain"
  | "mastery_wisdom_choice";

export type PriorityCount = 1 | 2 | 3;

export type LeavePolicyCourseRule = {
  id: string;
  courseName: string;
  subject: string;
  ruleType: LeavePolicyRuleType;
  priorityCount: PriorityCount;
  description: string;
  makeupRules: string[];
  lastClassExcluded: boolean;
  makeupTargets?: { section: string; subject: string }[];
  sectionTargets?: Record<string, { section: string; subject: string }[]>;
  eligibleTargets: string[];
  priorities?: RulePriority[];
};

export type RulePriority = {
  level: 1 | 2 | 3;
  ruleType: LeavePolicyRuleType;
  label: string;
  makeupTargets?: { section: string; subject: string }[];
  sectionTargets?: Record<string, { section: string; subject: string }[]>;
  eligibleTargets?: string[];
  anyDay?: boolean;
};

export type SubjectMapping = {
  ruleId: string;
  subjectId: string;
  subjectCode: string;
  subjectName: string;
};

export type LeavePolicyTestInput = {
  courseRuleId: string;
  missedCourseName: string;
  missedSection: string;
  missedOccurrence: number;
  totalSessions: number;
  isLastClass: boolean;
};

export type MakeupOption = {
  label: string;
  available: boolean;
  reason?: string;
};

export type LeavePolicyTestResult = {
  input: LeavePolicyTestInput;
  options: MakeupOption[];
  isBlocked: boolean;
  blockReason?: string;
};
