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
    expect(ruleById("sat-verbal-reading-beginner-sec1").courseName).toBe("SAT Verbal Reading Beginner Section 1");
    expect(ruleById("sat-verbal-reading-beginner-sec2").courseName).toBe("SAT Verbal Reading Beginner Section 2");
    expect(ruleById("sat-verbal-reading-beginner-sec3").courseName).toBe("SAT Verbal Reading Beginner Section 3");
    expect(ruleById("sat-verbal-writing-beginner-sec1").courseName).toBe("SAT Verbal Writing Beginner Section 1");
    expect(ruleById("sat-verbal-writing-beginner-sec2").courseName).toBe("SAT Verbal Writing Beginner Section 2");
    expect(ruleById("sat-verbal-writing-beginner-sec3").courseName).toBe("SAT Verbal Writing Beginner Section 3");

    const beginnerNames = LEAVE_POLICY_COURSE_RULES
      .filter((rule) => rule.id.includes("beginner"))
      .map((rule) => rule.courseName);

    expect(beginnerNames).not.toContain("SAT Verbal Rank 3-Section 1");
    expect(beginnerNames).not.toContain("SAT Verbal Rank 3-Section 2");
    expect(beginnerNames).not.toContain("SAT Verbal Rank 3-Section 3");
  });

  it("directs Beginner Section 3 to Section 1 for the same lesson", () => {
    expect(optionLabels("sat-verbal-reading-beginner-sec3", "SAT Verbal Reading Beginner Section 3", 1, "Section 3")).toEqual([
      "Section 1 (Reading Beginner) — same lesson #",
    ]);
  });

  it("does not offer a different Beginner lesson after the same-lesson priority", () => {
    expect(optionLabels("sat-verbal-reading-beginner-sec1", "SAT Verbal Reading Beginner Section 1", 3, "Section 1")).toEqual([
      "Section 2 (Reading Beginner) — same lesson #",
      "Section 3 (Reading Beginner) — same lesson #",
      "SAT Verbal Reading Rank 5",
      "SAT Reading Rank 5",
      "SAT Verbal Reading Rank 4",
      "SAT Reading Rank 4",
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

  it("follows the policy priority count for rank-choice courses", () => {
    expect(ruleById("sat-verbal-realtime-practice").priorityCount).toBe(1);
    for (const id of ["reading-mastery", "writing-wisdom", "sat-verbal-knockout", "sat-verbal-intensive", "sat-verbal-believe"]) {
      expect(ruleById(id).priorityCount).toBe(2);
      expect(ruleById(id).priorities).toHaveLength(2);
    }
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

  it("keeps exactly one canonical Rank 4 row per subject", () => {
    const rank4Rows = LEAVE_POLICY_COURSE_RULES.filter((rule) => /Rank 4/.test(rule.courseName));
    expect(rank4Rows.map((rule) => rule.id).sort()).toEqual([
      "sat-verbal-reading-rank4",
      "sat-verbal-writing-rank4",
    ]);
    expect(rank4Rows.map((rule) => rule.courseName).sort()).toEqual([
      "SAT Verbal Reading Rank 4",
      "SAT Verbal Writing Rank 4",
    ]);
    expect(LEAVE_POLICY_COURSE_RULES.some((rule) => rule.id === "sat-reading-rank4")).toBe(false);
    expect(LEAVE_POLICY_COURSE_RULES.some((rule) => rule.id === "sat-writing-rank4")).toBe(false);
  });

  it("preserves canonical Rank 4 rank-chain options and last-class exclusion", () => {
    for (const id of ["sat-verbal-reading-rank4", "sat-verbal-writing-rank4"]) {
      const rule = ruleById(id);
      expect(rule.ruleType).toBe("rank_chain");
      expect(rule.priorityCount).toBe(2);
      expect(rule.priorities).toHaveLength(2);
      expect(rule.lastClassExcluded).toBe(true);
    }
    expect(optionLabels("sat-verbal-reading-rank4", "SAT Verbal Reading Rank 4", 1, "Section 1")).toEqual([
      "SAT Verbal Reading Rank 5",
    ]);
    expect(optionLabels("sat-verbal-writing-rank4", "SAT Verbal Writing Rank 4", 2, "Section 1")).toEqual([
      "SAT Verbal Writing Rank 5",
      "SAT Verbal Rank 3 Section 1",
      "SAT Verbal Rank 3 Section 2",
    ]);
    for (const id of ["sat-verbal-reading-rank4", "sat-verbal-writing-rank4"]) {
      const blocked = evaluateLeavePolicy({
        courseRuleId: id,
        missedCourseName: ruleById(id).courseName,
        missedSection: "Section 1",
        missedOccurrence: 2,
        totalSessions: 8,
        isLastClass: true,
      });
      expect(blocked.isBlocked).toBe(true);
      expect(blocked.options).toEqual([]);
    }
  });

  it("preserves existing beginner dual-alias Rank 4 and Rank 5 targets", () => {
    for (const id of [
      "sat-verbal-reading-beginner-sec1",
      "sat-verbal-reading-beginner-sec2",
      "sat-verbal-reading-beginner-sec3",
    ]) {
      const rule = ruleById(id);
      expect(rule.eligibleTargets).toContain("SAT Verbal Reading Rank 4");
      expect(rule.eligibleTargets).toContain("SAT Reading Rank 4");
      expect(rule.eligibleTargets).toContain("SAT Verbal Reading Rank 5");
      expect(rule.eligibleTargets).toContain("SAT Reading Rank 5");
    }
    for (const id of [
      "sat-verbal-writing-beginner-sec1",
      "sat-verbal-writing-beginner-sec2",
      "sat-verbal-writing-beginner-sec3",
    ]) {
      const rule = ruleById(id);
      expect(rule.eligibleTargets).toContain("SAT Verbal Writing Rank 4");
      expect(rule.eligibleTargets).toContain("SAT Writing Rank 4");
      expect(rule.eligibleTargets).toContain("SAT Verbal Writing Rank 5");
      expect(rule.eligibleTargets).toContain("SAT Writing Rank 5");
    }
  });

  it("leaves all Rank 5 definitions unchanged", () => {
    expect(ruleById("sat-verbal-reading-rank5").priorityCount).toBe(2);
    expect(ruleById("sat-verbal-writing-rank5").priorityCount).toBe(2);
    expect(ruleById("sat-reading-rank5").priorityCount).toBe(2);
    expect(ruleById("sat-writing-rank5").priorityCount).toBe(2);
    expect(ruleById("sat-verbal-reading-rank5").eligibleTargets).toContain("SAT Verbal Reading Rank 4");
    expect(ruleById("sat-verbal-writing-rank5").eligibleTargets).toContain("SAT Verbal Writing Rank 4");
    expect(ruleById("sat-reading-rank5").eligibleTargets).toContain("SAT Reading Rank 4");
    expect(ruleById("sat-writing-rank5").eligibleTargets).toContain("SAT Writing Rank 4");
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
