import { describe, expect, it } from "vitest";
import { LEAVE_POLICY_COURSE_RULES, evaluateLeavePolicy } from "../leavePolicyData";

function ruleById(id: string) {
  const rule = LEAVE_POLICY_COURSE_RULES.find((item) => item.id === id);
  if (!rule) throw new Error(`Missing rule ${id}`);
  return rule;
}

function optionLabels(ruleId: string, missedCourseName: string, maxPriorityToShow = 1, missedSection = "Section 1") {
  return evaluateLeavePolicy(
    {
      courseRuleId: ruleId,
      missedCourseName,
      missedSection,
      missedOccurrence: 2,
      totalSessions: 8,
      isLastClass: false,
    },
    undefined,
    maxPriorityToShow
  ).options.map((option) => option.label);
}

describe("SAT Verbal hardcoded leave policy", () => {
  it("keeps Beginner courses separate from Rank 3 course rules", () => {
    expect(ruleById("sat-verbal-reading-beginner").courseName).toBe("SAT Verbal Reading Beginner");
    expect(ruleById("sat-verbal-writing-beginner").courseName).toBe("SAT Verbal Writing Beginner");

    const beginnerNames = LEAVE_POLICY_COURSE_RULES
      .filter((rule) => rule.id.includes("beginner"))
      .map((rule) => rule.courseName);

    expect(beginnerNames).not.toContain("SAT Verbal Rank 3-Section 1");
    expect(beginnerNames).not.toContain("SAT Verbal Rank 3-Section 2");
    expect(beginnerNames).not.toContain("SAT Verbal Rank 3-Section 3");
  });

  it("directs Beginner Section 3 to Section 1 for the same lesson", () => {
    expect(optionLabels("sat-verbal-reading-beginner", "SAT Verbal Reading Beginner", 1, "Section 3")).toEqual([
      "Section 1 (Reading Beginner) — same lesson #",
    ]);
  });

  it("does not expose Rank 2 for Rank 3 Section 3", () => {
    expect(optionLabels("rank3-sec3", "SAT Verbal Rank 3-Section 3", 3)).not.toContain("SAT Verbal Rank 2");
  });

  it("labels Rank 3 Section 3 with the Section 3 subject", () => {
    expect(ruleById("rank3-sec3").subject).toBe("Math");
  });

  it("keeps Brush Up Rank 4 and Rank 5 targets tied to Reading or Writing main course", () => {
    expect(optionLabels("sat-verbal-brushup", "SAT Verbal Reading Rank 4")).toEqual([
      "SAT Verbal Reading Rank 5",
    ]);
    expect(optionLabels("sat-verbal-brushup", "SAT Verbal Writing Rank 5")).toEqual([
      "SAT Verbal Writing Rank 4",
    ]);
  });

  it("uses one visible priority for rank-choice courses that choose by current rank", () => {
    expect(ruleById("sat-verbal-realtime-practice").priorityCount).toBe(1);
    expect(ruleById("reading-mastery").priorityCount).toBe(1);
    expect(ruleById("sat-verbal-knockout").priorityCount).toBe(1);
    expect(ruleById("sat-verbal-intensive").priorityCount).toBe(1);
    expect(ruleById("sat-verbal-believe").priorityCount).toBe(1);
  });

  it("still blocks every policy on the final class of a cycle", () => {
    const result = evaluateLeavePolicy({
      courseRuleId: "sat-verbal-reading-rank5",
      missedCourseName: "SAT Verbal Reading Rank 5",
      missedSection: "Section 1",
      missedOccurrence: 8,
      totalSessions: 8,
      isLastClass: true,
    });

    expect(result.isBlocked).toBe(true);
    expect(result.options).toEqual([]);
  });

  it("uses correct ordinal suffixes for legacy cross-section priority labels", () => {
    const result = evaluateLeavePolicy({
      courseRuleId: "legacy-cross-section",
      missedCourseName: "Legacy Course",
      missedSection: "Section 1",
      missedOccurrence: 2,
      totalSessions: 8,
      isLastClass: false,
    }, [
      {
        id: "legacy-cross-section",
        courseName: "Legacy Course",
        subject: "Reading",
        ruleType: "cross_section",
        priorityCount: 3,
        description: "Legacy cross-section rule",
        makeupRules: [],
        lastClassExcluded: true,
        makeupTargets: [
          { section: "Section 2", subject: "Reading" },
          { section: "Section 3", subject: "Reading" },
          { section: "Section 4", subject: "Reading" },
          { section: "Section 5", subject: "Reading" },
        ],
        eligibleTargets: [],
      },
    ]);

    expect(result.options.map((option) => option.reason)).toEqual([
      "1st Priority",
      "2nd Priority",
      "3rd Priority",
      "4th Priority",
    ]);
  });
});
