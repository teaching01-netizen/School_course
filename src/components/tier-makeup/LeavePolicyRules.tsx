import { useEffect, useMemo, useState } from "react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import LoadingSkeleton from "../ui/LoadingSkeleton";
import EmptyState from "../ui/EmptyState";
import Button from "../ui/Button";
import { LEAVE_POLICY_COURSE_RULES, getRuleTypeBadgeColor, getRuleTypeLabel } from "./leavePolicyData";
import type { SubjectMapping } from "../../types";

type Subject = { id: string; code: string; name: string };

const STORAGE_KEY = "sat-verbal-leave-policy-mappings";

function loadMappings(): SubjectMapping[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

function saveMappings(mappings: SubjectMapping[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(mappings));
}

export default function LeavePolicyRules() {
  const { addToast } = useToast();
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [mappings, setMappings] = useState<SubjectMapping[]>(loadMappings);
  const [loadingSubjects, setLoadingSubjects] = useState(true);
  const [savingRuleId, setSavingRuleId] = useState<string | null>(null);
  const [expandedRuleId, setExpandedRuleId] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const data = await apiJson<Subject[]>("/api/v1/subjects", { method: "GET" });
        setSubjects(data ?? []);
      } catch (err) {
        addToast("error", err instanceof Error ? err.message : "Failed to load subjects");
      } finally {
        setLoadingSubjects(false);
      }
    })();
  }, [addToast]);

  const mappingsByRule = useMemo(() => {
    const map = new Map<string, SubjectMapping>();
    for (const m of mappings) {
      map.set(m.ruleId, m);
    }
    return map;
  }, [mappings]);

  function handleMappingChange(ruleId: string, subjectId: string) {
    const subject = subjects.find((s) => s.id === subjectId);
    const next = mappings.filter((m) => m.ruleId !== ruleId);
    if (subjectId && subject) {
      next.push({
        ruleId,
        subjectId: subject.id,
        subjectCode: subject.code,
        subjectName: subject.name,
      });
    }
    setMappings(next);
    saveMappings(next);
  }

  async function persistMapping(ruleId: string) {
    const mapping = mappings.find((m) => m.ruleId === ruleId);
    if (!mapping) {
      addToast("warning", "No subject selected for this rule");
      return;
    }
    setSavingRuleId(ruleId);
    try {
      // Find root course group for this subject and save the sit-in rule assignment
      const rootGroups = await apiJson<Array<{ id: string; name: string; sit_in_rule_id: string | null }>>(
        "/api/v1/admin/root-course-groups",
        { method: "GET" }
      );
      // Match by subject code in group name
      const matchingGroup = rootGroups.find(
        (g) => g.name.toUpperCase().includes(mapping.subjectCode.toUpperCase())
      );
      if (matchingGroup) {
        await apiJson(`/api/v1/admin/root-course-groups/${matchingGroup.id}`, {
          method: "PUT",
          body: JSON.stringify({ sit_in_rule_id: ruleId }),
        });
        addToast("success", `Mapping saved for ${mapping.subjectCode}`);
      } else {
        addToast("warning", `No root course group found for subject ${mapping.subjectCode}`);
      }
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to save mapping");
    } finally {
      setSavingRuleId(null);
    }
  }

  const mappedCount = mappings.filter((m) => m.subjectId).length;
  const totalRules = LEAVE_POLICY_COURSE_RULES.length;

  if (loadingSubjects) {
    return <LoadingSkeleton type="table" lines={10} />;
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <div>
          <p className="text-sm text-gray-500">
            Map each SAT Verbal course rule to a subject in your system. Mappings are used by the absence form
            to determine sit-in options.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-gray-400">
            {mappedCount}/{totalRules} mapped
          </span>
          <Button variant="secondary" size="sm" onClick={() => { setMappings([]); saveMappings([]); }}>
            Reset All
          </Button>
        </div>
      </div>

      {subjects.length === 0 ? (
        <EmptyState message="No subjects found. Create subjects first." />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-[13px]">
            <thead>
              <tr className="border-b-2 border-gray-200">
                <th className="text-left py-2 px-2 font-semibold text-gray-700 w-[200px]">Course Rule</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700 w-[100px]">Subject</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700 w-[120px]">Rule Type</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700 w-[80px]">Priorities</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700">Makeup Rules</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700 w-[220px]">Map to Subject</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700 w-[80px]"></th>
              </tr>
            </thead>
            <tbody>
              {LEAVE_POLICY_COURSE_RULES.map((rule, idx) => {
                const badge = getRuleTypeBadgeColor(rule.ruleType);
                const mapping = mappingsByRule.get(rule.id);
                const isExpanded = expandedRuleId === rule.id;

                return (
                  <tr
                    key={rule.id}
                    className={`border-b border-gray-100 hover:bg-gray-50 ${idx % 2 === 1 ? "bg-gray-50/40" : ""}`}
                  >
                    <td className="py-2 px-2">
                      <button
                        type="button"
                        onClick={() => setExpandedRuleId(isExpanded ? null : rule.id)}
                        className="font-medium text-gray-800 hover:text-[var(--color-wi-primary)] text-left"
                      >
                        {rule.courseName}
                      </button>
                    </td>
                    <td className="py-2 px-2">
                      <span className="text-xs font-medium text-gray-600">{rule.subject}</span>
                    </td>
                    <td className="py-2 px-2">
                      <span className={`inline-block rounded-sm px-2 py-0.5 text-xs font-medium ${badge.bg} ${badge.text}`}>
                        {getRuleTypeLabel(rule.ruleType)}
                      </span>
                    </td>
                    <td className="py-2 px-2 text-center">
                      <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-gray-100 text-xs font-semibold text-gray-600">
                        {rule.priorityCount}
                      </span>
                    </td>
                    <td className="py-2 px-2 text-gray-600">
                      {rule.description}
                    </td>
                    <td className="py-2 px-2">
                      <select
                        value={mapping?.subjectId ?? ""}
                        onChange={(e) => handleMappingChange(rule.id, e.target.value)}
                        className="w-full text-xs border border-gray-200 rounded-sm px-2 py-1.5 bg-white"
                      >
                        <option value="">-- Select subject --</option>
                        {subjects.map((s) => (
                          <option key={s.id} value={s.id}>
                            {s.code} — {s.name}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td className="py-2 px-2">
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={!mapping?.subjectId}
                        loading={savingRuleId === rule.id}
                        onClick={() => void persistMapping(rule.id)}
                      >
                        Save
                      </Button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Expanded rule detail */}
      {expandedRuleId && (
        <div className="mt-4 rounded-sm border border-gray-200 bg-gray-50 p-4">
          {(() => {
            const rule = LEAVE_POLICY_COURSE_RULES.find((r) => r.id === expandedRuleId);
            if (!rule) return null;
            return (
              <div>
                <h4 className="text-sm font-semibold text-gray-800 mb-2">{rule.courseName}</h4>
                <ul className="list-disc list-inside space-y-1 text-sm text-gray-600">
                  {rule.makeupRules.map((r, i) => (
                    <li key={i}>{r}</li>
                  ))}
                </ul>
                {rule.lastClassExcluded && (
                  <p className="mt-2 text-xs text-amber-600 font-medium">
                    Last class of cycle excluded (End-of-class Meal)
                  </p>
                )}
                <div className="mt-2">
                  <span className="text-xs text-gray-400">Eligible targets: </span>
                  <span className="text-xs text-gray-600">{rule.eligibleTargets.join(", ")}</span>
                </div>
              </div>
            );
          })()}
        </div>
      )}
    </div>
  );
}
