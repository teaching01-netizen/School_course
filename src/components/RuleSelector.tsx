import type { SitInRule, SitInRuleType } from "../types";
import Select from "./ui/Select";

type RuleSelectorProps = {
  rules: SitInRule[];
  value: string | null;
  onChange: (ruleId: string | null) => void;
  disabled?: boolean;
};

const RULE_TYPE_LABELS: Record<SitInRuleType, string> = {
  level_ladder: "Level Ladder",
  cross_section: "Cross-Section",
  any_day_except_last: "Any Day",
  rank_chain: "Rank Chain",
  teacher_case_by_case: "Teacher Case",
};

const TYPE_ORDER: SitInRuleType[] = [
  "level_ladder",
  "cross_section",
  "any_day_except_last",
  "rank_chain",
  "teacher_case_by_case",
];

function groupByType(rules: SitInRule[]): Map<SitInRuleType, SitInRule[]> {
  const map = new Map<SitInRuleType, SitInRule[]>();
  for (const type of TYPE_ORDER) {
    const grouped = rules.filter((r) => r.type === type);
    if (grouped.length > 0) map.set(type, grouped);
  }
  return map;
}

export default function RuleSelector({ rules, value, onChange, disabled = false }: RuleSelectorProps) {
  const grouped = groupByType(rules);
  const selectedRule = rules.find((r) => r.id === value) ?? null;

  return (
    <div className="space-y-1.5">
      <Select
        value={value ?? ""}
        onChange={(e) => {
          onChange(e.target.value === "" ? null : e.target.value);
        }}
        disabled={disabled}
        aria-label="Sit-in rule"
      >
        <option value="">No rule assigned</option>
        {Array.from(grouped.entries()).map(([type, typeRules]) => (
          <optgroup key={type} label={RULE_TYPE_LABELS[type]}>
            {typeRules.map((rule) => (
              <option key={rule.id} value={rule.id}>
                {rule.name} — {RULE_TYPE_LABELS[type]}
              </option>
            ))}
          </optgroup>
        ))}
      </Select>

      {selectedRule?.description && (
        <p className="text-xs text-gray-500">{selectedRule.description}</p>
      )}

      {value === null && (
        <p className="text-xs text-amber-600 flex items-center gap-1">
          <span aria-hidden="true">⚠</span>
          No sit-in rule — students cannot sit into this group&apos;s sessions
        </p>
      )}
    </div>
  );
}
