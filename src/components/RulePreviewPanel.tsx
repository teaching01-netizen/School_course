import { Lightbulb } from "lucide-react";
import type { SitInRuleType, SitInRuleCreateInput } from "../types";
import { LevelLadderVisual } from "./LevelLadderVisual";

export type RuleTypeDescription = {
  label: string;
  description: string;
  example: string;
};

export const RULE_TYPE_DESCRIPTIONS: Record<SitInRuleType, RuleTypeDescription> = {
  level_ladder: {
    label: "Level Ladder",
    description: "Students at different levels sit in lower-level classes",
    example: "A Level 3 student can sit in a Level 2 class when absent",
  },
  cross_section: {
    label: "Cross-Section",
    description: "Students can attend sessions in other sections",
    example: "A student from Section A can sit in Section B's session",
  },
  any_day_except_last: {
    label: "Any Day Except Last",
    description: "Students can sit in any session except the final class",
    example: "A student can sit in any earlier session but not the last one",
  },
  rank_chain: {
    label: "Rank Chain",
    description: "Define specific rank-to-rank allowlists",
    example: "Rank 1 can sit in Rank 2, Rank 2 can sit in Rank 3",
  },
  teacher_case_by_case: {
    label: "Teacher Case by Case",
    description: "Requires teacher approval for each sit-in",
    example: "Each sit-in request must be approved by the teacher",
  },
};

export const FIELD_TOOLTIPS: Record<string, string> = {
  level_1_action:
    "What happens to Level 1 students when absent? Zoom = attend remotely, Physical = attend a class in person",
  min_level:
    "The lowest level allowed for sit-down. Students below this level get the Level 1 Action instead. For example, min_level=3 means only Level 3+ students can sit down; Level 1-2 students get Zoom/Physical.",
  section_match:
    "Cross-section = students attend a different section's session, Any = any section",
  occurrence_match:
    "Same occurrence number = match by lesson number, Any = any lesson",
  day_match:
    "Any day = any session regardless of day, Same day = only sessions on the same day",
  last_class_excluded:
    "When checked, students cannot sit in the final session of a course",
  chains: "Define which ranks can sit in which other ranks (from → to)",
  auto_assign:
    "Automatically assign students to sit-in sessions without admin intervention",
  requires_teacher_approval:
    "Each sit-in requires the receiving teacher's approval",
};

type RulePreviewPanelProps = {
  form: SitInRuleCreateInput;
};

export function buildSummary(form: SitInRuleCreateInput): string {
  const typeDesc = RULE_TYPE_DESCRIPTIONS[form.type];
  if (!form.name) {
    return typeDesc.description;
  }
  return `${form.name}: ${typeDesc.description}`;
}

export function buildConditionBadge(form: SitInRuleCreateInput): string {
  const p = form.predicate;
  const parts: string[] = [];

  switch (form.type) {
    case "level_ladder":
      parts.push(`Level 1: ${p.level_1_action === "zoom" ? "Zoom" : "Physical"}`);
      parts.push(`Min Level: ${p.min_level_for_sit_lower ?? 2}`);
      break;
    case "cross_section":
      parts.push(
        `Section: ${p.section_match === "cross_section" ? "Cross" : "Any"}`
      );
      parts.push(
        `Occurrence: ${p.occurrence_match === "same_occurrence_number" ? "Same#" : "Any"}`
      );
      parts.push(`Day: ${p.day_match === "same_day" ? "Same" : "Any"}`);
      if (p.last_class_excluded) parts.push("No Last");
      break;
    case "any_day_except_last":
      parts.push(`Day: ${p.day_match === "same_day" ? "Same" : "Any"}`);
      if (p.last_class_excluded) parts.push("No Last");
      break;
    case "rank_chain": {
      const chains = (p.chains as Array<{ from: number; to: number }>) ?? [];
      parts.push(
        chains.length > 0
          ? chains.map((c) => `${c.from}→${c.to}`).join(", ")
          : "No chains"
      );
      break;
    }
    case "teacher_case_by_case":
      parts.push(p.auto_assign ? "Auto" : "Manual");
      parts.push(p.requires_teacher_approval ? "Approval" : "No Approval");
      break;
  }

  return parts.join(" | ");
}

export function buildCalendarHighlights(
  form: SitInRuleCreateInput
): number[] {
  switch (form.type) {
    case "any_day_except_last":
    case "rank_chain":
      return form.predicate.last_class_excluded !== false
        ? [0, 1, 2, 3]
        : [0, 1, 2, 3, 4];
    case "level_ladder":
      return [0, 1, 2, 3, 4];
    case "cross_section":
      return form.predicate.day_match === "same_day" ? [0, 1, 2, 3, 4] : [0, 1, 2, 3, 4];
    case "teacher_case_by_case":
      return [0, 1, 2, 3, 4];
    default:
      return [0, 1, 2, 3, 4];
  }
}

export function RulePreviewPanel({ form }: RulePreviewPanelProps) {
  const summary = buildSummary(form);
  const badge = buildConditionBadge(form);
  const highlights = buildCalendarHighlights(form);
  const dayLabels = ["Mon", "Tue", "Wed", "Thu", "Fri"];

  return (
    <div className="rounded-sm border border-gray-200 bg-gray-50/70 p-3">
      <div className="flex items-center gap-2 mb-2">
        <Lightbulb className="w-4 h-4 text-amber-500" />
        <span className="text-xs font-medium text-gray-600 uppercase tracking-wide">
          Rule Preview
        </span>
      </div>

      <p className="text-sm text-gray-700 mb-2">{summary}</p>

      {form.type === "level_ladder" ? (
        <div className="mb-2">
          <LevelLadderVisual
            level1Action={(form.predicate.level_1_action as "zoom" | "physical") ?? "zoom"}
            minLevelForSitLower={Number(form.predicate.min_level_for_sit_lower ?? 2)}
          />
        </div>
      ) : (
        <div className="flex items-center gap-1 mb-2">
          {dayLabels.map((day, i) => (
            <div
              key={day}
              className={`flex-1 h-6 rounded-sm text-[10px] flex items-center justify-center font-medium ${
                highlights.includes(i)
                  ? "bg-emerald-100 text-emerald-700 border border-emerald-200"
                  : "bg-gray-100 text-gray-400 border border-gray-200"
              }`}
            >
              {day}
            </div>
          ))}
        </div>
      )}

      <span className="inline-block rounded-full bg-white border border-gray-200 px-2 py-0.5 text-[11px] text-gray-600">
        {badge}
      </span>
    </div>
  );
}

export default RulePreviewPanel;
