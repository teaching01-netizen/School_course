import { useState, useEffect, useCallback } from "react";
import type { SitInRuleType } from "../types";
import Select from "./ui/Select";
import Input from "./ui/Input";
import Button from "./ui/Button";
import { Tooltip } from "./ui/Tooltip";
import { FIELD_TOOLTIPS } from "./RulePreviewPanel";

type RankChain = { from: number; to: number };

type RulePredicateFormProps = {
  ruleType: SitInRuleType;
  predicate: Record<string, unknown>;
  onChange: (predicate: Record<string, unknown>) => void;
};

function buildDefaults(ruleType: SitInRuleType): Record<string, unknown> {
  switch (ruleType) {
    case "level_ladder":
      return { level_1_action: "zoom", min_level_for_sit_lower: 2 };
    case "cross_section":
      return {
        section_match: "cross_section",
        occurrence_match: "same_occurrence_number",
        day_match: "any",
        last_class_excluded: true,
      };
    case "any_day_except_last":
      return { day_match: "any_day", last_class_excluded: true };
    case "rank_chain":
      return { chains: [] as RankChain[], last_class_excluded: true, day_match: "any_day" };
    case "teacher_case_by_case":
      return { auto_assign: false, requires_teacher_approval: true };
    default:
      return {};
  }
}

function mergeDefaults(ruleType: SitInRuleType, predicate: Record<string, unknown>): Record<string, unknown> {
  const defaults = buildDefaults(ruleType);
  return { ...defaults, ...predicate };
}

export function RulePredicateForm({ ruleType, predicate, onChange }: RulePredicateFormProps) {
  const [local, setLocal] = useState<Record<string, unknown>>(() => mergeDefaults(ruleType, predicate));
  const [showJson, setShowJson] = useState(false);
  const [jsonText, setJsonText] = useState(() => JSON.stringify(local, null, 2));

  useEffect(() => {
    const merged = mergeDefaults(ruleType, predicate);
    setLocal(merged);
    setJsonText(JSON.stringify(merged, null, 2));
  }, [ruleType, predicate]);

  const update = useCallback(
    (patch: Record<string, unknown>) => {
      const next = { ...local, ...patch };
      setLocal(next);
      setJsonText(JSON.stringify(next, null, 2));
      onChange(next);
    },
    [local, onChange],
  );

  function handleJsonCommit(text: string) {
    setJsonText(text);
    try {
      const parsed = JSON.parse(text);
      if (typeof parsed === "object" && parsed !== null && !Array.isArray(parsed)) {
        setLocal(parsed);
        onChange(parsed);
      }
    } catch {
      // invalid JSON — don't propagate
    }
  }

  return (
    <div className="space-y-3">
      {ruleType === "level_ladder" && (
        <LevelLadderFields predicate={local} onChange={update} />
      )}
      {ruleType === "cross_section" && (
        <CrossSectionFields predicate={local} onChange={update} />
      )}
      {ruleType === "any_day_except_last" && (
        <AnyDayExceptLastFields predicate={local} onChange={update} />
      )}
      {ruleType === "rank_chain" && (
        <RankChainFields predicate={local} onChange={update} />
      )}
      {ruleType === "teacher_case_by_case" && (
        <TeacherCaseByCaseFields predicate={local} onChange={update} />
      )}

      <div className="border-t border-gray-100 pt-3">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setShowJson(!showJson)}
        >
          {showJson ? "Hide" : "Advanced: Edit JSON"}
        </Button>
        {showJson && (
          <textarea
            aria-label="JSON predicate"
            value={jsonText}
            onChange={(e) => handleJsonCommit(e.target.value)}
            className="mt-2 w-full rounded-sm border border-gray-300 px-3 py-2 text-sm font-mono focus:border-[var(--color-wi-primary)] focus:outline-none"
            rows={6}
            placeholder='{"key": "value"}'
          />
        )}
      </div>
    </div>
  );
}

function LevelLadderFields({
  predicate,
  onChange,
}: {
  predicate: Record<string, unknown>;
  onChange: (patch: Record<string, unknown>) => void;
}) {
  return (
    <>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1 flex items-center gap-1" htmlFor="level_1_action">
          Level 1 Action
          <Tooltip content={FIELD_TOOLTIPS.level_1_action} />
        </label>
        <Select
          id="level_1_action"
          value={(predicate.level_1_action as string) ?? "zoom"}
          onChange={(e) => onChange({ level_1_action: e.target.value })}
        >
          <option value="zoom">Zoom</option>
          <option value="physical">Physical</option>
        </Select>
      </div>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1 flex items-center gap-1" htmlFor="min_level">
          Minimum Level for Sit-Down
            <Tooltip content={FIELD_TOOLTIPS.min_level} />
        </label>
        <Input
          id="min_level"
          type="number"
          min={1}
          value={Number(predicate.min_level_for_sit_lower ?? 2)}
          onChange={(e) => onChange({ min_level_for_sit_lower: Number(e.target.value) })}
        />
      </div>
    </>
  );
}

function CrossSectionFields({
  predicate,
  onChange,
}: {
  predicate: Record<string, unknown>;
  onChange: (patch: Record<string, unknown>) => void;
}) {
  return (
    <>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1 flex items-center gap-1" htmlFor="section_match">
          Section Match
          <Tooltip content={FIELD_TOOLTIPS.section_match} />
        </label>
        <Select
          id="section_match"
          value={(predicate.section_match as string) ?? "cross_section"}
          onChange={(e) => onChange({ section_match: e.target.value })}
        >
          <option value="cross_section">Cross-Section</option>
          <option value="any">Any</option>
        </Select>
      </div>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1 flex items-center gap-1" htmlFor="occurrence_match">
          Occurrence Match
          <Tooltip content={FIELD_TOOLTIPS.occurrence_match} />
        </label>
        <Select
          id="occurrence_match"
          value={(predicate.occurrence_match as string) ?? "same_occurrence_number"}
          onChange={(e) => onChange({ occurrence_match: e.target.value })}
        >
          <option value="same_occurrence_number">Same Occurrence Number</option>
          <option value="any">Any</option>
        </Select>
      </div>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1 flex items-center gap-1" htmlFor="day_match">
          Day Match
          <Tooltip content={FIELD_TOOLTIPS.day_match} />
        </label>
        <Select
          id="day_match"
          value={(predicate.day_match as string) ?? "any"}
          onChange={(e) => onChange({ day_match: e.target.value })}
        >
          <option value="any">Any</option>
          <option value="same_day">Same Day</option>
        </Select>
      </div>
      <label className="flex items-center gap-2 text-sm text-gray-700">
        <input
          type="checkbox"
          checked={(predicate.last_class_excluded as boolean) ?? true}
          onChange={(e) => onChange({ last_class_excluded: e.target.checked })}
          className="rounded-sm border-gray-300"
        />
        Last Class Excluded
        <Tooltip content={FIELD_TOOLTIPS.last_class_excluded} />
      </label>
    </>
  );
}

function AnyDayExceptLastFields({
  predicate,
  onChange,
}: {
  predicate: Record<string, unknown>;
  onChange: (patch: Record<string, unknown>) => void;
}) {
  return (
    <>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1 flex items-center gap-1" htmlFor="day_match">
          Day Match
          <Tooltip content={FIELD_TOOLTIPS.day_match} />
        </label>
        <Select
          id="day_match"
          value={(predicate.day_match as string) ?? "any_day"}
          onChange={(e) => onChange({ day_match: e.target.value })}
        >
          <option value="any_day">Any Day</option>
          <option value="same_day">Same Day</option>
        </Select>
      </div>
      <label className="flex items-center gap-2 text-sm text-gray-700">
        <input
          type="checkbox"
          checked={(predicate.last_class_excluded as boolean) ?? true}
          onChange={(e) => onChange({ last_class_excluded: e.target.checked })}
          className="rounded-sm border-gray-300"
        />
        Last Class Excluded
        <Tooltip content={FIELD_TOOLTIPS.last_class_excluded} />
      </label>
    </>
  );
}

function RankChainFields({
  predicate,
  onChange,
}: {
  predicate: Record<string, unknown>;
  onChange: (patch: Record<string, unknown>) => void;
}) {
  const chains = (predicate.chains as RankChain[]) ?? [];

  function updateChain(index: number, field: keyof RankChain, value: number) {
    const next = chains.map((c, i) => (i === index ? { ...c, [field]: value } : c));
    onChange({ chains: next });
  }

  function addChain() {
    onChange({ chains: [...chains, { from: 1, to: 1 }] });
  }

  function removeChain(index: number) {
    onChange({ chains: chains.filter((_, i) => i !== index) });
  }

  return (
    <>
      <div className="space-y-2">
        {chains.map((chain, i) => (
          <div key={i} className="flex items-end gap-2">
            <div className="flex-1">
              <label className="block text-sm font-medium text-gray-700 mb-1 flex items-center gap-1" htmlFor={`chain-from-${i}`}>
                From Rank
                <Tooltip content="The starting rank in this chain" />
              </label>
              <Input
                id={`chain-from-${i}`}
                type="number"
                min={1}
                value={chain.from}
                onChange={(e) => updateChain(i, "from", Number(e.target.value))}
              />
            </div>
            <div className="flex-1">
              <label className="block text-sm font-medium text-gray-700 mb-1 flex items-center gap-1" htmlFor={`chain-to-${i}`}>
                To Rank
                <Tooltip content="The ending rank in this chain" />
              </label>
              <Input
                id={`chain-to-${i}`}
                type="number"
                min={1}
                value={chain.to}
                onChange={(e) => updateChain(i, "to", Number(e.target.value))}
              />
            </div>
            <Button variant="danger" size="sm" onClick={() => removeChain(i)}>
              Remove Chain
            </Button>
          </div>
        ))}
      </div>
      <Button variant="secondary" size="sm" onClick={addChain}>
        Add Chain
      </Button>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1 flex items-center gap-1" htmlFor="day_match">
          Day Match
          <Tooltip content={FIELD_TOOLTIPS.day_match} />
        </label>
        <Select
          id="day_match"
          value={(predicate.day_match as string) ?? "any_day"}
          onChange={(e) => onChange({ day_match: e.target.value })}
        >
          <option value="any_day">Any Day</option>
          <option value="same_day">Same Day</option>
        </Select>
      </div>
      <label className="flex items-center gap-2 text-sm text-gray-700">
        <input
          type="checkbox"
          checked={(predicate.last_class_excluded as boolean) ?? true}
          onChange={(e) => onChange({ last_class_excluded: e.target.checked })}
          className="rounded-sm border-gray-300"
        />
        Last Class Excluded
        <Tooltip content={FIELD_TOOLTIPS.last_class_excluded} />
      </label>
    </>
  );
}

function TeacherCaseByCaseFields({
  predicate,
  onChange,
}: {
  predicate: Record<string, unknown>;
  onChange: (patch: Record<string, unknown>) => void;
}) {
  return (
    <>
      <label className="flex items-center gap-2 text-sm text-gray-700">
        <input
          type="checkbox"
          checked={(predicate.auto_assign as boolean) ?? false}
          onChange={(e) => onChange({ auto_assign: e.target.checked })}
          className="rounded-sm border-gray-300"
        />
        Auto Assign
        <Tooltip content={FIELD_TOOLTIPS.auto_assign} />
      </label>
      <label className="flex items-center gap-2 text-sm text-gray-700">
        <input
          type="checkbox"
          checked={(predicate.requires_teacher_approval as boolean) ?? true}
          onChange={(e) => onChange({ requires_teacher_approval: e.target.checked })}
          className="rounded-sm border-gray-300"
        />
        Requires Teacher Approval
        <Tooltip content={FIELD_TOOLTIPS.requires_teacher_approval} />
      </label>
    </>
  );
}

export default RulePredicateForm;
