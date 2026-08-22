import { useEffect, useState, useMemo } from "react";
import SearchableSelect from "@/components/ui/SearchableSelect";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import LoadingSkeleton from "../../components/ui/LoadingSkeleton";
import Button from "../../components/ui/Button";
import Modal from "../../components/Modal";
import LevelStepper from "../../components/LevelStepper";
import LevelBadge from "../../components/LevelBadge";
import useReturnsDesk from "../../hooks/useReturnsDesk";
import ReturnsDeskPanel from "../../components/ReturnsDeskPanel";
import { useSitInRules } from "../../features/absences/hooks/useSitInRules";
import { getGapWarning } from "../../utils/levels";
import type { CourseLevelItem, CourseMergeGroupConfig, PolicyResponse, RootCourseGroupInfo } from "../../utils/levels";
import type { SitInRule } from "../../types";

type CardCourse = {
  id: string;
  code: string;
  name: string;
  subject_name: string;
  level: number | null;
  cycle_label: string;
  cycle_id: string;
};

type CardGroup = {
  groupId: string;
  rootCourseGroupId: string | null;
  label: string;
  subjectCode: string;
  courses: CardCourse[];
  sitInRuleId: string | null;
};

type CourseLevelsPage = {
  items: CourseLevelItem[];
  total: number;
  limit: number;
  offset: number;
};

function parseCourseLevelsPage(value: CourseLevelItem[] | CourseLevelsPage): CourseLevelsPage {
  return Array.isArray(value)
    ? { items: value, total: value.length, limit: value.length, offset: 0 }
    : value;
}

const COURSE_PAGE_SIZE = 100;

export function SitInRulesSection() {
  const { addToast } = useToast();
  const returnsDesk = useReturnsDesk();
  const { rules: sitInRules } = useSitInRules();
  const [courses, setCourses] = useState<CourseLevelItem[]>([]);
  const [mergeGroups, setMergeGroups] = useState<CourseMergeGroupConfig[]>([]);
  const [courseTotal, setCourseTotal] = useState(0);
  const [courseOffset, setCourseOffset] = useState(0);
  const [coursePageLoading, setCoursePageLoading] = useState(false);
  const [autoToggles, setAutoToggles] = useState<Record<string, boolean>>({});
  const [initialAutoToggles, setInitialAutoToggles] = useState<Record<string, boolean>>({});
  const [windowWeeks, setWindowWeeks] = useState<Record<string, number>>({});
  const [initialWindowWeeks, setInitialWindowWeeks] = useState<Record<string, number>>({});
  const [loading, setLoading] = useState(true);
  const [editLevels, setEditLevels] = useState<Record<string, number | null>>({});
  const [savingId, setSavingId] = useState<string | null>(null);
  const [savingPolicy, setSavingPolicy] = useState<Record<string, boolean>>({});
  const [bulkEditGroup, setBulkEditGroup] = useState<CardGroup | null>(null);
  const [bulkLevels, setBulkLevels] = useState<Record<string, number | null>>({});
  const [savingBulk, setSavingBulk] = useState(false);
  const [verificationReport, setVerificationReport] = useState<string[] | null>(null);
  const [lastVerified, setLastVerified] = useState<Date | null>(null);
  const [selectedCourseIds, setSelectedCourseIds] = useState<Set<string>>(new Set());
  const [rootCourseGroupMap, setRootCourseGroupMap] = useState<Map<string, RootCourseGroupInfo>>(new Map());

  async function loadCoursePage(offset: number) {
    setCoursePageLoading(true);
    try {
      const response = await apiJson<CourseLevelItem[] | CourseLevelsPage>(
        `/api/v1/admin/course-levels?limit=${COURSE_PAGE_SIZE}&offset=${offset}`,
        { method: "GET" },
      );
      const page = parseCourseLevelsPage(response);
      setCourses(page.items);
      setCourseTotal(page.total);
      setCourseOffset(page.offset);
      setEditLevels(Object.fromEntries(page.items.map((course) => [course.id, course.level])));
    } finally {
      setCoursePageLoading(false);
    }
  }

  useEffect(() => {
    (async () => {
      try {
        const [coursesData, policiesResp, rootGroupsData, mergeGroupsData] = await Promise.all([
          apiJson<CourseLevelItem[] | CourseLevelsPage>(`/api/v1/admin/course-levels?limit=${COURSE_PAGE_SIZE}&offset=0`, { method: "GET" }),
          apiJson<PolicyResponse>("/api/v1/admin/absence-policies", { method: "GET" }),
          apiJson<RootCourseGroupInfo[]>("/api/v1/admin/root-course-groups", { method: "GET" }),
          apiJson<CourseMergeGroupConfig[]>("/api/v1/admin/course-merge-groups", { method: "GET" }),
        ]);
        const coursesPage = parseCourseLevelsPage(coursesData);
        setCourses(coursesPage.items);
        setCourseTotal(coursesPage.total);
        setCourseOffset(coursesPage.offset);
        setRootCourseGroupMap(new Map(rootGroupsData.map(g => [g.id, g])));
        setMergeGroups(mergeGroupsData ?? []);
        const rootPolicies = policiesResp.absence_policies?.root_course_groups ?? {};
        const mergePolicies = policiesResp.absence_policies?.merge_groups ?? {};
        const toggles: Record<string, boolean> = {};
        const initial: Record<string, boolean> = {};
        const weeks: Record<string, number> = {};
        const initialWeeks: Record<string, number> = {};
        for (const [id, policy] of Object.entries(rootPolicies)) {
          toggles[id] = policy.auto_sit_in_enabled;
          initial[id] = policy.auto_sit_in_enabled;
          weeks[id] = policy.sit_in_window_weeks ?? 0;
          initialWeeks[id] = policy.sit_in_window_weeks ?? 0;
        }
        for (const [id, policy] of Object.entries(mergePolicies)) {
          toggles[id] = policy.auto_sit_in_enabled;
          initial[id] = policy.auto_sit_in_enabled;
          weeks[id] = policy.sit_in_window_weeks ?? 0;
          initialWeeks[id] = policy.sit_in_window_weeks ?? 0;
        }
        setAutoToggles(toggles);
        setInitialAutoToggles(initial);
        setWindowWeeks(weeks);
        setInitialWindowWeeks(initialWeeks);
        const levels: Record<string, number | null> = {};
        for (const c of coursesPage.items) {
          levels[c.id] = c.level;
        }
        setEditLevels(levels);
      } catch (err) {
        addToast("error", err instanceof Error ? err.message : "Failed to load data");
      } finally {
        setLoading(false);
      }
    })();
  }, [addToast]);

  async function saveLevel(courseId: string) {
    const level = editLevels[courseId] ?? null;
    const course = courses.find((c) => c.id === courseId);
    if (!course) return;
    setSavingId(courseId);
    try {
      await apiJson(`/api/v1/admin/courses/${courseId}/level`, {
        method: "PUT",
        body: JSON.stringify({ level, cycle_id: course.cycle_id }),
      });
      addToast("success", "Level saved");
      setCourses((prev) => prev.map((c) => (c.id === courseId ? { ...c, level } : c)));
    } catch (err) {
      const message = err instanceof Error ? err.message : "Save failed";
      addToast("error", message);
      returnsDesk.addFailure({
        courseId,
        courseCode: course.code,
        attemptedLevel: level,
        cycleId: course.cycle_id,
        error: { code: "save_failed", message },
      });
    } finally {
      setSavingId(null);
    }
  }

  async function saveAutoPolicy(rootCourseId: string) {
    setSavingPolicy((p) => ({ ...p, [rootCourseId]: true }));
    try {
      await apiJson("/api/v1/admin/absence-policies", {
        method: "PUT",
        body: JSON.stringify({
          absence_policies: {
            root_course_groups: {
              [rootCourseId]: {
                auto_sit_in_enabled: autoToggles[rootCourseId],
                sit_in_window_weeks: windowWeeks[rootCourseId] ?? 0,
              },
            },
          },
        }),
      });
      addToast("success", "Policy saved");
      setInitialAutoToggles((prev) => ({ ...prev, [rootCourseId]: autoToggles[rootCourseId] }));
      setInitialWindowWeeks((prev) => ({ ...prev, [rootCourseId]: windowWeeks[rootCourseId] ?? 0 }));
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Save failed");
    } finally {
      setSavingPolicy((p) => ({ ...p, [rootCourseId]: false }));
    }
  }

  async function saveRuleAssignment(rootCourseId: string, ruleId: string | null) {
    try {
      await apiJson(`/api/v1/admin/root-course-groups/${rootCourseId}`, {
        method: "PUT",
        body: JSON.stringify({ sit_in_rule_id: ruleId }),
      });
      setRootCourseGroupMap((prev) => {
        const next = new Map(prev);
        const existing = next.get(rootCourseId);
        if (existing) next.set(rootCourseId, { ...existing, sit_in_rule_id: ruleId });
        return next;
      });
      addToast("success", "Rule assigned");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to assign rule");
    }
  }

  async function saveMergeRuleAssignment(groupId: string, ruleId: string | null) {
    try {
      await apiJson(`/api/v1/admin/course-merge-groups/${groupId}`, {
        method: "PUT",
        body: JSON.stringify({ sit_in_rule_id: ruleId ?? "" }),
      });
      setMergeGroups((previous) => previous.map((group) => group.id === groupId ? { ...group, sit_in_rule_id: ruleId } : group));
      addToast("success", "Merged course rule assigned");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to assign merged course rule");
    }
  }

  async function saveMergeAutoPolicy(groupId: string) {
    setSavingPolicy((previous) => ({ ...previous, [groupId]: true }));
    try {
      await apiJson("/api/v1/admin/absence-policies", {
        method: "PUT",
        body: JSON.stringify({
          absence_policies: {
            merge_groups: {
              [groupId]: {
                auto_sit_in_enabled: autoToggles[groupId] ?? true,
                sit_in_window_weeks: windowWeeks[groupId] ?? 0,
              },
            },
          },
        }),
      });
      setInitialAutoToggles((previous) => ({ ...previous, [groupId]: autoToggles[groupId] ?? true }));
      setInitialWindowWeeks((previous) => ({ ...previous, [groupId]: windowWeeks[groupId] ?? 0 }));
      addToast("success", "Merged course policy saved");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to save merged course policy");
    } finally {
      setSavingPolicy((previous) => ({ ...previous, [groupId]: false }));
    }
  }

  const cardGroups = useMemo(() => {
    const rcMap = new Map<string, { label: string; courses: CardCourse[]; subjectCode: string }>();
    for (const c of courses) {
      const key = c.root_course_group_id ?? c.subject_id;
      const existing = rcMap.get(key);
      const cardCourse: CardCourse = {
        id: c.id,
        code: c.code,
        name: c.name,
        subject_name: c.subject_name,
        level: editLevels[c.id] ?? null,
        cycle_label: c.cycle_label,
        cycle_id: c.cycle_id,
      };
      if (existing) {
        existing.courses.push(cardCourse);
      } else {
        rcMap.set(key, {
          label: c.root_course_group_name ?? c.subject_code,
          courses: [cardCourse],
          subjectCode: c.subject_code,
        });
      }
    }
    return [...rcMap.entries()].map(([groupId, g]) => {
      const rcg = rootCourseGroupMap.get(groupId);
      return {
        groupId,
        rootCourseGroupId: courses.find((c) => c.root_course_group_id === groupId)?.root_course_group_id ?? null,
        ...g,
        sitInRuleId: rcg?.sit_in_rule_id ?? null,
      };
    });
  }, [courses, editLevels, rootCourseGroupMap]);

  function verifyConfiguration() {
    const missingCount = courses.filter((c) => !editLevels[c.id]).length;
    const gapReports: string[] = [];
    for (const group of cardGroups) {
      const g = getGapWarning(group.courses);
      if (g) gapReports.push(`${group.label}: ${g}`);
    }
    const report: string[] = [];
    if (missingCount > 0) report.push(`${missingCount} course${missingCount === 1 ? " has" : "s have"} no level set.`);
    if (gapReports.length > 0) report.push(...gapReports);
    if (report.length === 0) report.push("All course configurations are valid.");
    setVerificationReport(report);
    setLastVerified(new Date());
  }

  function openBulkEdit(group: CardGroup) {
    const bl: Record<string, number | null> = {};
    for (const c of group.courses) {
      bl[c.id] = c.level;
    }
    setBulkLevels(bl);
    setBulkEditGroup(group);
  }

  async function applyBulkEdit() {
    if (!bulkEditGroup) return;
    setSavingBulk(true);
    try {
      const changed = bulkEditGroup.courses.filter((c) => {
        const original = courses.find((oc) => oc.id === c.id);
        return original && bulkLevels[c.id] !== original.level;
      });
      const results = await Promise.allSettled(changed.map((c) => {
        const level = bulkLevels[c.id] ?? null;
        return apiJson(`/api/v1/admin/courses/${c.id}/level`, {
          method: "PUT",
          body: JSON.stringify({ level, cycle_id: c.cycle_id }),
        }).then(() => ({ course: c, level }));
      }));

      const succeeded = results.filter((r): r is PromiseFulfilledResult<{ course: CardCourse; level: number | null }> => r.status === "fulfilled");
      const failed = results.filter((r): r is PromiseRejectedResult => r.status === "rejected");

      if (succeeded.length > 0) {
        setCourses((prev) => prev.map((pc) => {
          if (!succeeded.some((s) => s.value.course.id === pc.id)) return pc;
          return { ...pc, level: bulkLevels[pc.id] ?? null };
        }));
        setEditLevels((prev) => {
          const next = { ...prev };
          for (const c of bulkEditGroup.courses) {
            next[c.id] = bulkLevels[c.id] ?? null;
          }
          return next;
        });
      }

      for (let i = 0; i < failed.length; i++) {
        const course = changed[i];
        const error = failed[i].reason;
        returnsDesk.addFailure({
          courseId: course.id,
          courseCode: course.code,
          attemptedLevel: bulkLevels[course.id] ?? null,
          cycleId: course.cycle_id,
          error: {
            code: "bulk_update_failed",
            message: error instanceof Error ? error.message : "Update failed",
          },
        });
      }

      const successCount = succeeded.length;
      const failCount = failed.length;
      if (failCount === 0) {
        addToast("success", successCount === 0 ? "No changes" : "Levels updated");
      } else {
        addToast("error", `${successCount} saved, ${failCount} failed (see Returns Desk)`);
      }
      setBulkEditGroup(null);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Bulk update failed");
    } finally {
      setSavingBulk(false);
    }
  }

  if (loading) return <LoadingSkeleton type="card" lines={5} />;

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <div>
          <p className="text-sm text-[var(--color-wi-text-light)]">Set level for each course. Levels must be consecutive. Level 1 = Zoom, higher levels sit in next level up.</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="secondary" size="sm" onClick={verifyConfiguration}>Verify All</Button>
          {lastVerified ? (
            <span className="text-xs text-[var(--color-wi-text-light)]">
              Last verified: {lastVerified.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" })}
            </span>
          ) : null}
        </div>
      </div>

      {verificationReport ? (
        <div className="mb-4 rounded-sm border border-wi-line bg-white px-4 py-3 text-sm text-[var(--color-wi-text-light)]" role="status">
          <p className="mb-1 font-medium">Verification results</p>
          {verificationReport.map((msg, i) => <p key={i}>{msg}</p>)}
        </div>
      ) : null}

      {mergeGroups.length > 0 ? (
        <section className="mb-5" aria-labelledby="merged-course-rules-heading">
          <div className="mb-3">
            <h2 id="merged-course-rules-heading" className="text-sm font-semibold text-[var(--color-wi-text)]">Merged course policies</h2>
            <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">These settings apply once to the merged absence scope. Source course sessions remain the physical sit-in destinations.</p>
          </div>
          <div className="space-y-3">
            {mergeGroups.map((group) => {
              const autoEnabled = autoToggles[group.id] ?? true;
              const weeks = windowWeeks[group.id] ?? 0;
              const dirty = autoEnabled !== (initialAutoToggles[group.id] ?? true) || weeks !== (initialWindowWeeks[group.id] ?? 0);
              return (
                <div key={group.id} className="rounded-sm border border-blue-200 bg-blue-50/40 px-4 py-3">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-[var(--color-wi-text)]">{group.name}</span>
                        <span className="rounded-full bg-blue-100 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-blue-700">Merged scope</span>
                      </div>
                      <p className="mt-1 font-mono text-xs text-[var(--color-wi-text-light)]">{group.course_codes.join(" + ")}</p>
                    </div>
                    <div className="flex flex-wrap items-center gap-3">
                      <label className="flex items-center gap-1.5 text-xs text-[var(--color-wi-text-light)]">
                        <span>Rule:</span>
                        <SearchableSelect
                          aria-label={`Sit-in rule for ${group.name}`}
                          value={group.sit_in_rule_id ?? ""}
                          onChange={(event) => saveMergeRuleAssignment(group.id, event.target.value || null)}
                          className="border border-wi-line rounded px-2 py-1 text-xs bg-white"
                        >
                          <option value="">No rule assigned</option>
                          {sitInRules.map((rule: SitInRule) => <option key={rule.id} value={rule.id}>{rule.name}</option>)}
                        </SearchableSelect>
                      </label>
                      <label className="flex items-center gap-1.5 text-xs text-[var(--color-wi-text-light)]">
                        <input type="checkbox" checked={autoEnabled} onChange={(event) => setAutoToggles((previous) => ({ ...previous, [group.id]: event.target.checked }))} />
                        <span>Auto</span>
                      </label>
                      <label className="flex items-center gap-1 text-xs text-[var(--color-wi-text-light)]">
                        <span>Window:</span>
                        <input type="number" min={0} className="w-14 border border-wi-line rounded px-1.5 py-1 text-xs bg-white" value={weeks} onChange={(event) => setWindowWeeks((previous) => ({ ...previous, [group.id]: Math.max(0, parseInt(event.target.value) || 0) }))} />
                        <span>weeks</span>
                      </label>
                      {dirty ? <Button size="sm" loading={savingPolicy[group.id]} onClick={() => saveMergeAutoPolicy(group.id)}>Save policy</Button> : null}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      ) : null}

      <div className="space-y-4">
        {cardGroups.map((group) => {
          const rcId = group.rootCourseGroupId;
          const warning = getGapWarning(group.courses);
          const selectedCount = group.courses.filter(c => selectedCourseIds.has(c.id)).length;
          return (
            <div key={group.groupId} className="rounded-sm border border-wi-line bg-white shadow-sm">
              <div className="flex items-center justify-between border-b border-wi-line-soft bg-[var(--color-wi-row-alt)]/70 px-4 py-3">
                <div className="flex items-center gap-2">
                  {rcId ? (
                    <input
                      type="checkbox"
                      ref={(el) => { if (el) el.indeterminate = selectedCount > 0 && selectedCount < group.courses.length; }}
                      checked={selectedCount === group.courses.length && group.courses.length > 0}
                      onChange={() => {
                        const allSelected = group.courses.every(c => selectedCourseIds.has(c.id));
                        setSelectedCourseIds(prev => {
                          const next = new Set(prev);
                          if (allSelected) {
                            group.courses.forEach(c => next.delete(c.id));
                          } else {
                            group.courses.forEach(c => next.add(c.id));
                          }
                          return next;
                        });
                      }}
                    />
                  ) : null}
                  <span className="text-sm font-semibold text-[var(--color-wi-text)]">
                    {group.subjectCode} &mdash; {group.label}
                  </span>
                  {!group.sitInRuleId ? (
                    <span className="text-xs text-amber-600">No rule assigned</span>
                  ) : null}
                </div>
                {rcId ? (
                  <div className="flex items-center gap-3">
                    {selectedCount > 0 ? (
                      <span className="text-xs text-blue-600">{selectedCount} selected</span>
                    ) : null}
                    <SearchableSelect
                      value={group.sitInRuleId ?? ""}
                      onChange={(e) => {
                        const ruleId = e.target.value || null;
                        saveRuleAssignment(rcId, ruleId);
                      }}
                      className="text-xs border border-wi-line rounded px-2 py-1"
                    >
                      <option value="">No rule assigned</option>
                      {sitInRules.map((rule: SitInRule) => (
                        <option key={rule.id} value={rule.id}>{rule.name}</option>
                      ))}
                    </SearchableSelect>
                    <label className="flex items-center gap-1.5 text-xs text-[var(--color-wi-text-light)]" title="Automatically assign students to next-level course when no sit-in capacity remains">
                      <input
                        type="checkbox"
                        checked={autoToggles[rcId] ?? true}
                        onChange={(e) => setAutoToggles((prev) => ({ ...prev, [rcId]: e.target.checked }))}
                      />
                      <span>Auto</span>
                    </label>
                    <label className="flex items-center gap-1 text-xs text-[var(--color-wi-text-light)]">
                      <span>Window:</span>
                      <input
                        type="number"
                        min={0}
                        className="w-14 border border-wi-line rounded px-1.5 py-1 text-xs"
                        value={windowWeeks[rcId] ?? 0}
                        onChange={(e) => setWindowWeeks((prev) => ({ ...prev, [rcId]: Math.max(0, parseInt(e.target.value) || 0) }))}
                      />
                      <span>weeks</span>
                    </label>
                    {autoToggles[rcId] !== initialAutoToggles[rcId] || (windowWeeks[rcId] ?? 0) !== (initialWindowWeeks[rcId] ?? 0) ? (
                      <Button
                        size="sm"
                        loading={savingPolicy[rcId]}
                        onClick={() => saveAutoPolicy(rcId)}
                      >
                        Save
                      </Button>
                    ) : null}
                    <Button variant="secondary" size="sm" onClick={() => {
                      const groupCourses = selectedCourseIds.size > 0
                        ? group.courses.filter(c => selectedCourseIds.has(c.id))
                        : group.courses;
                      openBulkEdit({ ...group, courses: groupCourses });
                    }}>Bulk Edit</Button>
                  </div>
                ) : null}
              </div>

              <div className="divide-y divide-gray-50">
                {group.courses.map((course) => (
                  <div key={course.id} className="flex items-center gap-4 px-4 py-2.5 text-sm hover:bg-[var(--color-wi-row-alt)]/50">
                    <input
                      type="checkbox"
                      checked={selectedCourseIds.has(course.id)}
                      onChange={() => {
                        setSelectedCourseIds(prev => {
                          const next = new Set(prev);
                          if (next.has(course.id)) next.delete(course.id);
                          else next.add(course.id);
                          return next;
                        });
                      }}
                    />
                    <div className="flex-1 min-w-0">
                      <span className="font-mono text-xs text-[var(--color-wi-text-light)]">{course.code}</span>
                      <span className="ml-2 text-[var(--color-wi-text-light)] truncate">{course.subject_name}</span>
                      <span className="ml-2 text-xs text-[var(--color-wi-text-light)]">({course.cycle_label})</span>
                    </div>
                    <LevelStepper
                      value={editLevels[course.id] ?? null}
                      onChange={(v) => setEditLevels((prev) => ({ ...prev, [course.id]: v }))}
                    />
                    <LevelBadge level={editLevels[course.id] ?? null} />
                    <Button
                      variant="primary"
                      size="sm"
                      disabled={editLevels[course.id] === course.level}
                      loading={savingId === course.id}
                      onClick={() => saveLevel(course.id)}
                    >
                      Save
                    </Button>
                  </div>
                ))}
              </div>

              {warning ? (
                <div className="border-t border-amber-100 bg-amber-50/50 px-4 py-2 text-xs text-amber-700">
                  {warning}
                </div>
              ) : null}
            </div>
          );
        })}
        {cardGroups.length === 0 ? (
          <p className="py-8 text-center text-sm text-[var(--color-wi-text-light)]">No course groups found.</p>
        ) : null}
      </div>

      {courseTotal > COURSE_PAGE_SIZE ? (
        <div className="mt-5 flex items-center justify-between border-t border-wi-line pt-3 text-xs text-[var(--color-wi-text-light)]">
          <span>Showing {courseOffset + 1}–{Math.min(courseOffset + courses.length, courseTotal)} of {courseTotal} courses</span>
          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" disabled={courseOffset === 0 || coursePageLoading} onClick={() => loadCoursePage(Math.max(0, courseOffset - COURSE_PAGE_SIZE))}>Previous</Button>
            <Button variant="secondary" size="sm" disabled={courseOffset + courses.length >= courseTotal || coursePageLoading} onClick={() => loadCoursePage(courseOffset + COURSE_PAGE_SIZE)}>Next</Button>
          </div>
        </div>
      ) : null}

      {bulkEditGroup ? (
        <Modal
          title={`Bulk Edit — ${bulkEditGroup.label}`}
          onClose={() => setBulkEditGroup(null)}
          size="lg"
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => {
                const bl: Record<string, number | null> = {};
                for (const c of bulkEditGroup.courses) {
                  bl[c.id] = c.level;
                }
                setBulkLevels(bl);
              }}>Reset</Button>
              <Button variant="primary" size="sm" loading={savingBulk} onClick={applyBulkEdit}>Apply Changes</Button>
            </>
          }
        >
          <table className="w-full text-sm">
            <caption className="sr-only">Bulk edit course levels</caption>
            <thead>
              <tr className="border-b border-wi-line text-left text-[var(--color-wi-text-light)]">
                <th scope="col" className="py-2 pr-3 font-medium">Course</th>
                <th scope="col" className="py-2 pr-3 font-medium">Cycle</th>
                <th scope="col" className="py-2 pr-3 font-medium">Current</th>
                <th scope="col" className="py-2 font-medium">Level</th>
              </tr>
            </thead>
            <tbody>
              {bulkEditGroup.courses.map((course) => (
                <tr key={course.id} className="border-b border-wi-line-soft">
                  <td className="py-2 pr-3 font-mono text-xs">{course.code}</td>
                  <td className="py-2 pr-3 text-[var(--color-wi-text-light)]">{course.cycle_label}</td>
                  <td className="py-2 pr-3">{course.level ?? <span className="text-[var(--color-wi-text-light)]">&mdash;</span>}</td>
                  <td className="py-2">
                    <LevelStepper
                      value={bulkLevels[course.id] ?? null}
                      onChange={(v) => setBulkLevels((prev) => ({ ...prev, [course.id]: v }))}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Modal>
      ) : null}

      {/* Returns Desk */}
      <ReturnsDeskPanel
        isOpen={false}
        onClose={() => {}}
        entries={returnsDesk.getGrouped()}
        onRetry={returnsDesk.retryFailure}
        onDismiss={returnsDesk.removeFailure}
        totalCount={returnsDesk.totalCount}
      />
    </div>
  );
}
