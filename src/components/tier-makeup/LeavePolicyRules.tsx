import { useEffect, useState } from "react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import LoadingSkeleton from "../ui/LoadingSkeleton";
import EmptyState from "../ui/EmptyState";
import Button from "../ui/Button";
import { LEAVE_POLICY_COURSE_RULES, getRuleTypeBadgeColor, getRuleTypeLabel } from "./leavePolicyData";

type Subject = { id: string; code: string; name: string };

type SatVerbalPolicyMapping = {
  active: boolean;
  subject_id?: string;
  policy_hash?: string;
  warnings?: string[];
  matched_courses?: Array<{ policy_course_name: string; course_name: string; root_group_name?: string }>;
  unmatched_policy_rows?: string[];
  unmatched_courses?: string[];
};

export default function LeavePolicyRules() {
  const { addToast } = useToast();
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [selectedSubjectId, setSelectedSubjectId] = useState("");
  const [mapping, setMapping] = useState<SatVerbalPolicyMapping | null>(null);
  const [loadingSubjects, setLoadingSubjects] = useState(true);
  const [loadingMapping, setLoadingMapping] = useState(false);
  const [saving, setSaving] = useState(false);
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

  useEffect(() => {
    if (!selectedSubjectId) {
      setMapping(null);
      return;
    }
    let cancelled = false;
    setLoadingMapping(true);
    void apiJson<SatVerbalPolicyMapping>(
      `/api/v1/admin/sat-verbal-policy/mapping?subject_id=${encodeURIComponent(selectedSubjectId)}`,
      { method: "GET" },
    )
      .then((data) => {
        if (!cancelled) setMapping(data);
      })
      .catch((err) => {
        if (!cancelled) addToast("error", err instanceof Error ? err.message : "Failed to load SAT Verbal mapping");
      })
      .finally(() => {
        if (!cancelled) setLoadingMapping(false);
      });
    return () => {
      cancelled = true;
    };
  }, [selectedSubjectId, addToast]);

  async function applyPolicy() {
    if (!selectedSubjectId) {
      addToast("warning", "Select the SAT Verbal subject first");
      return;
    }
    setSaving(true);
    try {
      const updated = await apiJson<SatVerbalPolicyMapping>("/api/v1/admin/sat-verbal-policy/apply", {
        method: "POST",
        body: JSON.stringify({
          subject_id: selectedSubjectId,
          policy: LEAVE_POLICY_COURSE_RULES,
        }),
      });
      setMapping(updated);
      const warningCount = updated.warnings?.length ?? 0;
      if (warningCount > 0) {
        addToast("warning", `SAT Verbal policy applied with ${warningCount} warning${warningCount === 1 ? "" : "s"}`);
      } else {
        addToast("success", "SAT Verbal policy applied");
      }
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to apply SAT Verbal policy");
    } finally {
      setSaving(false);
    }
  }

  const totalRules = LEAVE_POLICY_COURSE_RULES.length;
  const selectedSubject = subjects.find((subject) => subject.id === selectedSubjectId);
  const warnings = mapping?.warnings ?? [];
  const unmatchedPolicyRows = mapping?.unmatched_policy_rows ?? [];
  const unmatchedCourses = mapping?.unmatched_courses ?? [];
  const matchedCount = mapping?.matched_courses?.length ?? 0;

  if (loadingSubjects) {
    return <LoadingSkeleton type="table" lines={10} />;
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <div>
          <p className="text-sm text-gray-500">
            Apply the hard-coded SAT Verbal leave policy to one subject. After applying, the absence resolver uses
            this policy automatically for courses under that subject.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-gray-400">
            {totalRules} policy rows
          </span>
        </div>
      </div>

      {subjects.length === 0 ? (
        <EmptyState message="No subjects found. Create subjects first." />
      ) : (
        <>
        <div className="mb-4 rounded-sm border border-gray-200 bg-white p-4">
          <div className="grid gap-3 md:grid-cols-[minmax(240px,360px)_auto_1fr] md:items-end">
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-gray-600">SAT Verbal Subject</span>
              <select
                aria-label="SAT Verbal subject"
                value={selectedSubjectId}
                onChange={(e) => setSelectedSubjectId(e.target.value)}
                className="w-full rounded-sm border border-gray-200 bg-white px-2 py-2 text-sm"
              >
                <option value="">-- Select subject --</option>
                {subjects.map((subject) => (
                  <option key={subject.id} value={subject.id}>
                    {subject.code} — {subject.name}
                  </option>
                ))}
              </select>
            </label>
            <Button loading={saving} disabled={!selectedSubjectId || loadingMapping} onClick={() => void applyPolicy()}>
              Apply SAT Verbal Policy
            </Button>
            <div className="text-xs text-gray-500">
              {loadingMapping ? (
                <span>Loading mapping...</span>
              ) : mapping?.active ? (
                <span className="font-medium text-green-700">
                  Active for {selectedSubject?.code ?? "selected subject"} · {matchedCount} matched course{matchedCount === 1 ? "" : "s"}
                </span>
              ) : selectedSubjectId ? (
                <span className="font-medium text-amber-700">Not active for {selectedSubject?.code ?? "selected subject"}</span>
              ) : (
                <span>Select a subject to view or apply the production mapping.</span>
              )}
            </div>
          </div>
          {(warnings.length > 0 || unmatchedPolicyRows.length > 0 || unmatchedCourses.length > 0) && (
            <div className="mt-3 space-y-2 text-xs">
              {warnings.length > 0 && (
                <div className="rounded-sm border border-amber-200 bg-amber-50 p-2 text-amber-800">
                  <div className="font-medium">Warnings</div>
                  <ul className="mt-1 list-disc pl-4">
                    {warnings.map((warning) => <li key={warning}>{warning}</li>)}
                  </ul>
                </div>
              )}
              {unmatchedCourses.length > 0 && (
                <div className="text-gray-600">Unmatched real courses: {unmatchedCourses.join(", ")}</div>
              )}
              {unmatchedPolicyRows.length > 0 && (
                <div className="text-gray-600">Unmatched policy rows: {unmatchedPolicyRows.join(", ")}</div>
              )}
            </div>
          )}
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-[13px]">
            <thead>
              <tr className="border-b-2 border-gray-200">
                <th className="text-left py-2 px-2 font-semibold text-gray-700 w-[200px]">Course Rule</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700 w-[100px]">Subject</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700 w-[120px]">Rule Type</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700 w-[80px]">Priorities</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700">Makeup Rules</th>
              </tr>
            </thead>
            <tbody>
              {LEAVE_POLICY_COURSE_RULES.map((rule, idx) => {
                const badge = getRuleTypeBadgeColor(rule.ruleType);
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
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        </>
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
