import type { SitInRuleCreateInput } from "../types";

type RuleExampleSectionProps = {
  form: SitInRuleCreateInput;
};

export function buildExampleScenario(form: SitInRuleCreateInput): string {
  const p = form.predicate;

  switch (form.type) {
    case "level_ladder": {
      const action =
        p.level_1_action === "zoom" ? "attend a Zoom session" : "attend a physical class";
      return `If a Level 1 student is absent, they ${action}. Non-top students sit in the next higher level. Top-level students sit in the level below.`;
    }
    case "cross_section": {
      const section =
        p.section_match === "cross_section" ? "a different section's" : "any section's";
      const day = p.day_match === "same_day" ? "on the same day" : "on any day";
      const exclude = p.last_class_excluded ? " (except the last session)" : "";
      return `Students can sit in ${section} session ${day}${exclude}.`;
    }
    case "any_day_except_last": {
      const day = p.day_match === "same_day" ? "on the same day" : "on any day";
      const exclude = p.last_class_excluded !== false;
      return exclude
        ? `Students can sit in any session ${day}, except the final class of the course.`
        : `Students can sit in any session ${day}, including the last class.`;
    }
    case "rank_chain": {
      const chains = (p.chains as Array<{ from: number; to: number }>) ?? [];
      if (chains.length === 0) return "No rank chains configured yet.";
      const chainText = chains.map((c) => `Rank ${c.from} → Rank ${c.to}`).join(", ");
      return `Rank chain allowlist: ${chainText}.`;
    }
    case "teacher_case_by_case": {
      const auto = p.auto_assign ? "automatically" : "manually";
      const approval = p.requires_teacher_approval
        ? "requires teacher approval"
        : "does not require approval";
      return `Sit-in requests are assigned ${auto} and ${approval}.`;
    }
    default:
      return "Configure this rule to define sit-in behavior.";
  }
}

export function RuleExampleSection({ form }: RuleExampleSectionProps) {
  const scenario = buildExampleScenario(form);

  return (
    <div className="border-t border-gray-100 pt-3">
      <p className="text-sm font-medium text-gray-600 mb-1">Example Scenario</p>
      <p className="text-sm text-gray-600 pl-0">{scenario}</p>
    </div>
  );
}

export default RuleExampleSection;
