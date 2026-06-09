import { useEffect, useMemo, useState } from "react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import LoadingSkeleton from "../ui/LoadingSkeleton";
import EmptyState from "../ui/EmptyState";
import Button from "../ui/Button";
import { LEAVE_POLICY_COURSE_RULES, getRuleTypeBadgeColor, getRuleTypeLabel } from "./leavePolicyData";

type Course = {
  id: string;
  code: string;
  name: string;
  subject_code?: string;
  subject_name?: string;
};

type SatVerbalRuleMapping = {
  active: boolean;
  rule_id: string;
  course_id: string;
  course_code?: string;
  course_name?: string;
  subject_code?: string;
  subject_name?: string;
};

type SatVerbalPolicyMapping = {
  active: boolean;
  mappings?: SatVerbalRuleMapping[];
  warnings?: string[];
  matched_courses?: Array<{ policy_course_name: string; course_name: string; root_group_name?: string }>;
  unmatched_policy_rows?: string[];
};

function courseLabel(course: Course) {
  const subject = course.subject_code || course.subject_name;
  return `${course.code} - ${course.name}${subject ? ` (${subject})` : ""}`;
}

export default function LeavePolicyRules() {
  const { addToast } = useToast();
  const [courses, setCourses] = useState<Course[]>([]);
  const [selectedCourseIds, setSelectedCourseIds] = useState<Record<string, string>>({});
  const [mapping, setMapping] = useState<SatVerbalPolicyMapping | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [expandedRuleId, setExpandedRuleId] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [courseData, mappingData] = await Promise.all([
          apiJson<Course[]>("/api/v1/courses", { method: "GET" }),
          apiJson<SatVerbalPolicyMapping>("/api/v1/admin/sat-verbal-policy/mapping", { method: "GET" }),
        ]);
        if (cancelled) return;
        setCourses(courseData ?? []);
        setMapping(mappingData);
        const byRule: Record<string, string> = {};
        for (const item of mappingData.mappings ?? []) {
          byRule[item.rule_id] = item.course_id;
        }
        setSelectedCourseIds(byRule);
      } catch (err) {
        if (!cancelled) addToast("error", err instanceof Error ? err.message : "Failed to load SAT Verbal mappings");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [addToast]);

  const selectedCount = useMemo(
    () => Object.values(selectedCourseIds).filter((courseId) => courseId.trim() !== "").length,
    [selectedCourseIds],
  );

  async function applyPolicy() {
    const mappings = LEAVE_POLICY_COURSE_RULES
      .map((rule) => ({ rule_id: rule.id, course_id: selectedCourseIds[rule.id] || "" }))
      .filter((item) => item.course_id);

    setSaving(true);
    try {
      const updated = await apiJson<SatVerbalPolicyMapping>("/api/v1/admin/sat-verbal-policy/apply", {
        method: "POST",
        body: JSON.stringify({
          policy: LEAVE_POLICY_COURSE_RULES,
          mappings,
        }),
      });
      setMapping(updated);
      const byRule: Record<string, string> = {};
      for (const item of updated.mappings ?? []) {
        byRule[item.rule_id] = item.course_id;
      }
      setSelectedCourseIds(byRule);
      const warningCount = updated.warnings?.length ?? 0;
      if (warningCount > 0) {
        addToast("warning", `SAT Verbal course rules saved with ${warningCount} warning${warningCount === 1 ? "" : "s"}`);
      } else {
        addToast("success", "SAT Verbal course rules saved");
      }
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to save SAT Verbal course rules");
    } finally {
      setSaving(false);
    }
  }

  const totalRules = LEAVE_POLICY_COURSE_RULES.length;
  const warnings = mapping?.warnings ?? [];
  const unmatchedPolicyRows = mapping?.unmatched_policy_rows ?? [];
  const matchedCount = mapping?.mappings?.length ?? selectedCount;

  if (loading) {
    return <LoadingSkeleton type="table" lines={10} />;
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between gap-3">
        <div>
          <p className="text-sm text-gray-500">
            Map each hard-coded SAT Verbal course rule to the exact production course. The absence resolver activates
            from this course mapping, so subject names and production course names do not need to match the policy text.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-gray-400">
            {matchedCount}/{totalRules} mapped
          </span>
          <Button loading={saving} disabled={courses.length === 0} onClick={() => void applyPolicy()}>
            Save SAT Verbal Course Rules
          </Button>
        </div>
      </div>

      {courses.length === 0 ? (
        <EmptyState message="No courses found. Create courses first." />
      ) : (
        <>
          {(warnings.length > 0 || unmatchedPolicyRows.length > 0) && (
            <div className="mb-4 space-y-2 text-xs">
              {warnings.length > 0 && (
                <div className="rounded-sm border border-amber-200 bg-amber-50 p-2 text-amber-800">
                  <div className="font-medium">Warnings</div>
                  <ul className="mt-1 list-disc pl-4">
                    {warnings.map((warning) => <li key={warning}>{warning}</li>)}
                  </ul>
                </div>
              )}
              {unmatchedPolicyRows.length > 0 && (
                <div className="text-gray-600">Unmapped policy rows: {unmatchedPolicyRows.join(", ")}</div>
              )}
            </div>
          )}

          <div className="overflow-x-auto">
            <table className="w-full text-[13px]">
              <thead>
                <tr className="border-b-2 border-gray-200">
                  <th className="w-[220px] px-2 py-2 text-left font-semibold text-gray-700">Course Rule</th>
                  <th className="w-[100px] px-2 py-2 text-left font-semibold text-gray-700">Subject</th>
                  <th className="w-[120px] px-2 py-2 text-left font-semibold text-gray-700">Rule Type</th>
                  <th className="w-[80px] px-2 py-2 text-left font-semibold text-gray-700">Priorities</th>
                  <th className="min-w-[280px] px-2 py-2 text-left font-semibold text-gray-700">Production Course</th>
                  <th className="px-2 py-2 text-left font-semibold text-gray-700">Makeup Rules</th>
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
                      <td className="px-2 py-2">
                        <button
                          type="button"
                          onClick={() => setExpandedRuleId(isExpanded ? null : rule.id)}
                          className="text-left font-medium text-gray-800 hover:text-[var(--color-wi-primary)]"
                        >
                          {rule.courseName}
                        </button>
                      </td>
                      <td className="px-2 py-2">
                        <span className="text-xs font-medium text-gray-600">{rule.subject}</span>
                      </td>
                      <td className="px-2 py-2">
                        <span className={`inline-block rounded-sm px-2 py-0.5 text-xs font-medium ${badge.bg} ${badge.text}`}>
                          {getRuleTypeLabel(rule.ruleType)}
                        </span>
                      </td>
                      <td className="px-2 py-2 text-center">
                        <span className="inline-flex h-6 w-6 items-center justify-center rounded-full bg-gray-100 text-xs font-semibold text-gray-600">
                          {rule.priorityCount}
                        </span>
                      </td>
                      <td className="px-2 py-2">
                        <select
                          aria-label={`${rule.courseName} production course`}
                          value={selectedCourseIds[rule.id] ?? ""}
                          onChange={(event) => {
                            const courseId = event.target.value;
                            setSelectedCourseIds((current) => ({ ...current, [rule.id]: courseId }));
                          }}
                          className="w-full rounded-sm border border-gray-200 bg-white px-2 py-1.5 text-sm"
                        >
                          <option value="">-- Not mapped --</option>
                          {courses.map((course) => (
                            <option key={course.id} value={course.id}>
                              {courseLabel(course)}
                            </option>
                          ))}
                        </select>
                      </td>
                      <td className="px-2 py-2 text-gray-600">
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

      {expandedRuleId && (
        <div className="mt-4 rounded-sm border border-gray-200 bg-gray-50 p-4">
          {(() => {
            const rule = LEAVE_POLICY_COURSE_RULES.find((r) => r.id === expandedRuleId);
            if (!rule) return null;
            return (
              <div>
                <h4 className="mb-2 text-sm font-semibold text-gray-800">{rule.courseName}</h4>
                <ul className="list-inside list-disc space-y-1 text-sm text-gray-600">
                  {rule.makeupRules.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
                {rule.lastClassExcluded && (
                  <p className="mt-2 text-xs font-medium text-amber-600">
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
