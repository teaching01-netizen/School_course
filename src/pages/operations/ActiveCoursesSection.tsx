import { useEffect, useMemo, useRef, useState } from "react";
import { ApiRequestError, apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import LoadingSkeleton from "../../components/ui/LoadingSkeleton";
import Button from "../../components/ui/Button";
import EmptyState from "../../components/ui/EmptyState";
import SearchInput from "../../components/ui/SearchInput";
import SessionDateFilter from "../../components/SessionDateFilter";
import { Switch } from "../../components/ui/Switch";
import type { ActiveCourseSubject } from "../../types";
import { queryClient, queryKeys } from "../../query/cache";
import {
  EMPTY_SESSION_DATE_FILTER,
  isSessionDateFilterActive,
  validateSessionDateFilter,
  type SessionDateFilter as SessionDateFilterValue,
} from "../../features/scheduling/domain/sessionDateRange";

type ActiveCoursesStats = {
  total_subjects: number;
  missing_active: number;
};

type ActiveCoursesResponse = {
  subjects: ActiveCourseSubject[];
  total_subjects?: number;
  total_courses?: number;
  limit?: number;
  offset?: number;
  stats?: ActiveCoursesStats;
};

const SUBJECT_PAGE_SIZE = 50;

type StatusFilter = "all" | "configured" | "missing_active";

const STATUS_FILTERS: Array<{ value: StatusFilter; label: string; title: string }> = [
  { value: "all", label: "All", title: "Every subject" },
  { value: "configured", label: "Active", title: "Subject has at least one active class — students can book and sit in" },
  { value: "missing_active", label: "No active", title: "No active class — students cannot book this subject" },
];

/** Checkbox that can render the third, indeterminate state used by
 *  select-all controls. The DOM property is imperative, so it is synced from
 *  React state in an effect rather than through JSX. */
function TriStateCheckbox({
  checked,
  indeterminate = false,
  onChange,
  label,
  disabled = false,
}: {
  checked: boolean;
  indeterminate?: boolean;
  onChange: () => void;
  label: string;
  disabled?: boolean;
}) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = indeterminate;
  }, [indeterminate, checked]);
  return (
    <input
      ref={ref}
      type="checkbox"
      checked={checked}
      onChange={onChange}
      disabled={disabled}
      aria-label={label}
      className="accent-[var(--color-wi-primary)]"
    />
  );
}

/** One course row, one control: the Active switch. Active means the class is
 *  open to students — visible in the absence form and eligible for sit-ins.
 *  Switches are independent: a subject may run several active classes at once,
 *  and students only ever see classes they are enrolled in. Everything applies
 *  immediately; there is no draft/save step. */
function CourseRow({
  subject,
  courseId,
  selected,
  onToggleSelect,
  onToggleActive,
  saving,
}: {
  subject: ActiveCourseSubject;
  courseId: string;
  selected: boolean;
  onToggleSelect: (courseId: string) => void;
  onToggleActive: (courseId: string, next: boolean) => void;
  saving: boolean;
}) {
  const course = subject.courses.find((c) => c.course_id === courseId);
  if (!course) return null;
  const isActive = course.is_active;
  const followsSource = course.rule_course_id && course.rule_course_id !== course.course_id;

  return (
    <div
      className={`flex flex-wrap items-center gap-x-3 gap-y-1.5 px-4 py-2.5 text-sm transition-colors hover:bg-[var(--color-wi-row-alt)]/50 ${
        selected ? "bg-blue-50/40" : ""
      }`}
    >
      <TriStateCheckbox
        checked={selected}
        onChange={() => onToggleSelect(course.course_id)}
        label={`Select ${course.course_code}`}
      />
      <span className="min-w-0">
        <span className="font-mono text-xs text-[var(--color-wi-text-light)]">{course.course_code}</span>
        <span className="ml-2 text-[var(--color-wi-text-light)]">{subject.subject_name}</span>
        <span className="ml-2 text-xs text-[var(--color-wi-text-light)]">({course.cycle_label || "no cycle"})</span>
        {course.merge_group_name ? (
          <span
            className="ml-2 inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-700"
            title={`One of the two source courses of the merged course "${course.merge_group_name}"`}
          >
            Merged · {course.merge_group_name}
          </span>
        ) : null}
      </span>

      <span className="ml-auto flex min-w-0 flex-wrap items-center gap-2">
        {followsSource ? (
          <span className="text-xs text-[var(--color-wi-text-light)]">Follows {course.rule_course_code}</span>
        ) : (
          <Switch
            checked={isActive}
            onCheckedChange={(next) => onToggleActive(course.course_id, next)}
            disabled={saving}
            aria-label={`${course.course_code} active`}
          />
        )}
        {isActive ? (
          <span className="text-xs font-semibold text-green-700">Active</span>
        ) : (
          <span className="text-xs text-[var(--color-wi-text-light)]">Off</span>
        )}
      </span>
    </div>
  );
}

export function ActiveCoursesSection() {
  const { addToast } = useToast();
  const [subjects, setSubjects] = useState<ActiveCourseSubject[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [savingActive, setSavingActive] = useState<Set<string>>(new Set());
  const [totalSubjects, setTotalSubjects] = useState(0);
  const [subjectOffset, setSubjectOffset] = useState(0);
  const [pageLoading, setPageLoading] = useState(false);
  const [stats, setStats] = useState<ActiveCoursesStats | null>(null);

  // Filtering runs server-side so search reaches every subject, not just the
  // loaded page. Input is debounced before it hits the API.
  const [searchInput, setSearchInput] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [status, setStatus] = useState<StatusFilter>("all");
  const [sessionDateFilter, setSessionDateFilter] = useState<SessionDateFilterValue>(EMPTY_SESSION_DATE_FILTER);
  const sessionDateFilterError = validateSessionDateFilter(sessionDateFilter);

  // Retain the rule source seen at selection time, including on other pages.
  const [selection, setSelection] = useState<Map<string, string | undefined>>(new Map());
  const [bulkBusy, setBulkBusy] = useState(false);
  const latestLoad = useRef(0);
  const currentPage = useRef({ offset: 0, search: "", statusFilter: status, dateFilter: sessionDateFilter });

  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(searchInput), 300);
    return () => clearTimeout(t);
  }, [searchInput]);

  async function loadSubjects(offset: number, search: string, statusFilter: StatusFilter, dateFilter: SessionDateFilterValue) {
    currentPage.current = { offset, search, statusFilter, dateFilter };
    const loadID = ++latestLoad.current;
    setLoading(true);
    setLoadError(null);
    try {
      const params = new URLSearchParams({
        limit: String(SUBJECT_PAGE_SIZE),
        offset: String(offset),
      });
      const trimmed = search.trim();
      if (trimmed) params.set("search", trimmed);
      if (statusFilter !== "all") params.set("status", statusFilter);
      if (dateFilter.from) params.set("session_date_from", dateFilter.from);
      if (dateFilter.to) params.set("session_date_to", dateFilter.to);
      const data = await apiJson<ActiveCoursesResponse>(
        `/api/v1/admin/active-courses?${params.toString()}`,
        { method: "GET" },
      );
      if (loadID !== latestLoad.current) return;
      if (offset > 0 && data.subjects.length === 0) {
        await loadSubjects(0, search, statusFilter, dateFilter);
        return;
      }
      setSubjects(data.subjects);
      setTotalSubjects(data.total_subjects ?? data.subjects.length);
      setSubjectOffset(data.offset ?? offset);
      setStats(data.stats ?? null);
    } catch (err) {
      if (loadID !== latestLoad.current) return;
      const message = err instanceof Error ? err.message : "Failed to load active courses";
      setLoadError(message);
      addToast("error", message);
    } finally {
      if (loadID === latestLoad.current) setLoading(false);
    }
  }

  useEffect(() => {
    if (sessionDateFilterError) return;
    setSubjectOffset(0);
    void loadSubjects(0, debouncedSearch, status, sessionDateFilter);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedSearch, status, sessionDateFilter.from, sessionDateFilter.to, sessionDateFilterError]);

  useEffect(() => {
    if (sessionDateFilterError) return;
    function refreshOnFocus() {
      void loadSubjects(subjectOffset, debouncedSearch, status, sessionDateFilter);
    }
    window.addEventListener("focus", refreshOnFocus);
    return () => window.removeEventListener("focus", refreshOnFocus);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [subjectOffset, debouncedSearch, status, sessionDateFilter.from, sessionDateFilter.to, sessionDateFilterError]);

  async function loadPage(offset: number) {
    setPageLoading(true);
    try {
      await loadSubjects(offset, debouncedSearch, status, sessionDateFilter);
    } finally {
      setPageLoading(false);
    }
  }

  // ----- Derived selection model -----

  const loadedCourses = useMemo(
    () => subjects.flatMap((s) => s.courses.map((c) => ({ course: c, subject: s }))),
    [subjects],
  );
  const selectedLoaded = useMemo(
    () => loadedCourses.filter((x) => selection.has(x.course.course_id)),
    [loadedCourses, selection],
  );
  const selectedNotOnPage = selection.size - selectedLoaded.length;
  const pageCourseIds = useMemo(() => loadedCourses.map((x) => x.course.course_id), [loadedCourses]);
  const allPageSelected = pageCourseIds.length > 0 && pageCourseIds.every((id) => selection.has(id));
  const somePageSelected = pageCourseIds.some((id) => selection.has(id));

  const canApply = (target: boolean) =>
    selectedLoaded.some((x) => x.course.is_active !== target) || selectedNotOnPage > 0;
  const activeInSelection = selectedLoaded.filter((x) => x.course.is_active);

  function toggleCourseSelected(courseId: string) {
    setSelection((prev) => {
      const next = new Map(prev);
      if (next.has(courseId)) next.delete(courseId);
      else next.set(courseId, loadedCourses.find(({ course }) => course.course_id === courseId)?.course.rule_course_id);
      return next;
    });
  }

  function toggleSubjectCourses(subject: ActiveCourseSubject) {
    const ids = subject.courses.map((c) => c.course_id);
    setSelection((prev) => {
      const next = new Map(prev);
      const allSelected = ids.length > 0 && ids.every((id) => next.has(id));
      for (const course of subject.courses) {
        if (allSelected) next.delete(course.course_id);
        else next.set(course.course_id, course.rule_course_id);
      }
      return next;
    });
  }

  function togglePageSelection() {
    setSelection((prev) => {
      const next = new Map(prev);
      const target = !allPageSelected;
      for (const { course } of loadedCourses) {
        if (target) next.set(course.course_id, course.rule_course_id);
        else next.delete(course.course_id);
      }
      return next;
    });
  }

  async function refreshAfterMutation() {
    const { offset, search, statusFilter, dateFilter } = currentPage.current;
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.courses.all, refetchType: "active" }),
      loadSubjects(offset, search, statusFilter, dateFilter),
    ]);
  }

  async function toggleActive(courseId: string, next: boolean) {
    const course = loadedCourses.find((entry) => entry.course.course_id === courseId)?.course;
    if (!course) return;

    setSavingActive((prev) => new Set(prev).add(courseId));
    try {
      await apiJson("/api/v1/admin/active-courses/set-active", {
        method: "PUT",
        body: JSON.stringify({
          course_id: courseId,
          active: next,
          ...(course.rule_course_id && { expected_rule_course_ids: { [courseId]: course.rule_course_id } }),
        }),
      });
      await refreshAfterMutation();
      addToast("success", `${course.course_code} ${next ? "is active" : "turned off"}.`);
    } catch (err) {
      if (err instanceof ApiRequestError && err.code === "course_rule_source_changed") {
        await refreshAfterMutation();
      }
      addToast("error", err instanceof Error ? err.message : "Failed to update");
    } finally {
      setSavingActive((prev) => {
        const nextSet = new Set(prev);
        nextSet.delete(courseId);
        return nextSet;
      });
    }
  }

  async function applyBulk(target: boolean) {
    const ids = [...selection.keys()];
    const expectedSources = Object.fromEntries([...selection].filter(([, source]) => source));
    if (ids.length === 0) return;

    const activeFlips = selectedLoaded.filter((x) => x.course.is_active !== target && x.course.is_active).length;
    if (!target && activeFlips > 0) {
      const confirmed = window.confirm(
        `Turn off ${ids.length} classes?\n\n` +
          `${activeFlips} of them are active. Student booking will also close for any courses that share their settings.`,
      );
      if (!confirmed) return;
    }

    setBulkBusy(true);
    try {
      await apiJson("/api/v1/admin/active-courses/set-active/bulk", {
        method: "PUT",
        body: JSON.stringify({
          course_ids: ids,
          active: target,
          ...(Object.keys(expectedSources).length > 0 && { expected_rule_course_ids: expectedSources }),
        }),
      });
      setSelection(new Map());
      await refreshAfterMutation();
      addToast(
        "success",
        target
          ? "Activated — the selected classes are now live for students (visible and sit-in-able)."
          : "Turned off — the selected classes are hidden from students and closed to sit-ins.",
      );
    } catch (err) {
      if (err instanceof ApiRequestError && err.code === "course_rule_source_changed") {
        setSelection(new Map());
        await refreshAfterMutation();
      }
      addToast("error", err instanceof Error ? err.message : "Bulk update failed");
    } finally {
      setBulkBusy(false);
    }
  }

  // ----- Audit banner (server stats when available, page fallback) -----

  const missingCount = useMemo(() => {
    if (stats) return stats.missing_active;
    return subjects.filter((s) => !s.courses.some((c) => c.is_active)).length;
  }, [stats, subjects]);

  const totalSubjectCount = stats?.total_subjects ?? subjects.length;
  const coveredCount = Math.max(0, totalSubjectCount - missingCount);
  const allCovered = coveredCount === totalSubjectCount;

  if (loading && subjects.length === 0) {
    return <LoadingSkeleton type="card" lines={5} />;
  }

  if (loadError && subjects.length === 0) {
    return (
      <EmptyState
        message={loadError}
        action={(
          <Button variant="secondary" size="sm" onClick={() => void loadSubjects(0, debouncedSearch, status, sessionDateFilter)}>
            Retry
          </Button>
        )}
      />
    );
  }

  if (subjects.length === 0) {
    const filtered = searchInput.trim() !== "" || status !== "all" || isSessionDateFilterActive(sessionDateFilter);
    return filtered ? (
      <EmptyState
        message="No subjects match the current filters"
        action={(
          <Button
            variant="secondary"
            size="sm"
            onClick={() => {
              setSearchInput("");
              setStatus("all");
              setSessionDateFilter(EMPTY_SESSION_DATE_FILTER);
            }}
          >
            Clear filters
          </Button>
        )}
      />
    ) : (
      <EmptyState message="No subjects configured yet" />
    );
  }

  const filterActive = status !== "all" || searchInput.trim() !== "" || isSessionDateFilterActive(sessionDateFilter);

  return (
    <div className="space-y-4">
      <section
        className="rounded-sm border border-wi-line bg-[var(--color-wi-callout)] p-3"
        aria-label="How the student absence form uses these settings"
      >
        <h3 className="mb-1.5 text-sm font-semibold text-[var(--color-wi-text)]">
          How the student absence form uses these settings
        </h3>
        <ul className="space-y-1 text-xs text-[var(--color-wi-text-light)]">
          <li>
            <strong className="text-[var(--color-wi-text)]">Active</strong> — the class is open: students can pick it
            in the absence form and sit in. A subject can run several active classes at once — students only ever see
            the classes they are enrolled in.
          </li>
          <li>
            <strong className="text-[var(--color-wi-text)]">Off</strong> — hidden from students and closed to sit-ins.
            Hidden classes can&apos;t be picked by students. Staff can always book any class.
          </li>
        </ul>
      </section>

      <div
        className={`flex flex-col gap-1 rounded-sm border px-4 py-2.5 text-sm ${
          allCovered
            ? "border-green-200 bg-green-50 text-green-700"
            : "border-amber-200 bg-amber-50 text-amber-700"
        }`}
      >
        <span className="font-medium">
          {allCovered
            ? "All subjects have an active class ✓"
            : `${coveredCount}/${totalSubjectCount} subjects have an active class students can book`}
        </span>
        {missingCount > 0 && !allCovered ? (
          <span>{missingCount} subject{missingCount === 1 ? "" : "s"} without an active class — students cannot book {missingCount === 1 ? "it" : "them"}.</span>
        ) : null}

      </div>

      <div className="flex flex-col gap-2">
        <SearchInput
          value={searchInput}
          onChange={setSearchInput}
          placeholder="Search subjects, classes, or merged courses..."
        />
        <SessionDateFilter
          value={sessionDateFilter}
          onChange={setSessionDateFilter}
          onClear={() => setSessionDateFilter(EMPTY_SESSION_DATE_FILTER)}
          idPrefix="active-courses-session-date"
        />
        <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label="Filter subjects by active-class state">
          {STATUS_FILTERS.map((f) => {
            const count =
              f.value === "all" ? totalSubjectCount : f.value === "configured" ? coveredCount : missingCount;
            const isCurrent = status === f.value;
            return (
              <button
                key={f.value}
                type="button"
                title={f.title}
                aria-pressed={isCurrent}
                onClick={() => setStatus(f.value)}
                className={`rounded-full border px-2.5 py-1 text-xs font-medium transition-colors ${
                  isCurrent
                    ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)] text-white"
                    : f.value === "configured"
                      ? "border-wi-line bg-white text-[var(--color-wi-text)] hover:bg-[var(--color-wi-row-alt)]"
                      : "border-wi-line bg-white text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)]"
                }`}
              >
                {f.label} <span className="font-normal opacity-80">({count})</span>
              </button>
            );
          })}
        </div>
      </div>

      <div className="flex items-center justify-between rounded-sm border border-wi-line bg-white px-4 py-2">
        <label className="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-wi-text)]">
          <TriStateCheckbox
            checked={allPageSelected}
            indeterminate={somePageSelected && !allPageSelected}
            onChange={togglePageSelection}
            label="Select all classes on this page"
          />
          Select all classes on this page ({pageCourseIds.length})
        </label>
        {selection.size > 0 ? (
          <span className="text-xs text-blue-700">
            {selection.size} selected{selectedNotOnPage > 0 ? ` · ${selectedNotOnPage} on other pages` : ""}
          </span>
        ) : null}
      </div>

      {filterActive ? (
        <p className="text-xs text-[var(--color-wi-text-light)]">
          Showing {subjects.length} of {totalSubjects} matching subjects
        </p>
      ) : null}

      {subjects.map((subject) => {
        const activeCourses = subject.courses.filter((c) => c.is_active);

        const subjectIds = subject.courses.map((c) => c.course_id);
        const subjectAll = subjectIds.length > 0 && subjectIds.every((id) => selection.has(id));
        const subjectSome = subjectIds.some((id) => selection.has(id));

        return (
          <div
            key={subject.subject_id}
            className="rounded-sm border border-wi-line bg-white shadow-sm"
          >
            <div className="flex items-center justify-between border-b border-wi-line-soft bg-[var(--color-wi-row-alt)]/70 px-4 py-3">
              <div className="flex items-center gap-2">
                {subject.courses.length > 0 ? (
                  <TriStateCheckbox
                    checked={subjectAll}
                    indeterminate={subjectSome && !subjectAll}
                    onChange={() => toggleSubjectCourses(subject)}
                    label={`Select all ${subject.subject_code} classes`}
                  />
                ) : null}
                <span className="text-sm font-semibold text-[var(--color-wi-text)]">
                  {subject.subject_code} — {subject.subject_name}
                </span>
                {activeCourses.length > 0 ? (
                  <span
                    className="inline-flex items-center rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700"
                    title={activeCourses.length > 1 ? "Parallel active classes — students only see the ones they're enrolled in" : undefined}
                  >
                    {activeCourses.length > 1 ? `Active (${activeCourses.length})` : "Active"}
                  </span>
                ) : null}
                {activeCourses.length === 0 ? (
                  <span className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700">
                    Not set
                  </span>
                ) : null}
              </div>
            </div>

            {subject.courses.length > 0 ? (
              <div className="divide-y divide-gray-50">
                {subject.courses.map((course) => (
                  <CourseRow
                    key={course.course_id}
                    subject={subject}
                    courseId={course.course_id}
                    selected={selection.has(course.course_id)}
                    onToggleSelect={toggleCourseSelected}
                    onToggleActive={(courseId, next) => void toggleActive(courseId, next)}
                    saving={savingActive.has(course.course_id)}
                  />
                ))}
              </div>
            ) : (
              <div className="flex items-center gap-3 px-4 py-2.5">
                <span className="text-sm italic text-[var(--color-wi-text-light)]">No courses — create one first</span>
                <a
                  href="/courses/create"
                  className="inline-flex items-center justify-center rounded-sm border border-wi-line bg-white px-2 py-1 text-xs font-medium text-[var(--color-wi-text)] transition-colors hover:bg-[var(--color-wi-row-alt)]"
                >
                  Create Course
                </a>
              </div>
            )}
          </div>
        );
      })}

      {selection.size > 0 ? (
        <div
          className="sticky bottom-0 space-y-1 rounded-sm border border-blue-200 bg-blue-50 px-4 py-3 shadow-md"
          data-testid="bulk-bar"
        >
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-sm font-medium text-blue-800">
              {selection.size} classes selected
              {selectedNotOnPage > 0 ? (
                <span className="ml-1 font-normal text-xs">({selectedNotOnPage} selected on other pages)</span>
              ) : null}
            </span>
            <div className="flex items-center gap-2">
              <Button variant="ghost" size="sm" onClick={() => setSelection(new Map())} disabled={bulkBusy}>
                Clear
              </Button>
              <Button
                variant="secondary"
                size="sm"
                disabled={!canApply(true) || bulkBusy}
                onClick={() => void applyBulk(true)}
              >
                Activate
              </Button>
              <Button
                variant="primary"
                size="sm"
                disabled={!canApply(false) || bulkBusy}
                loading={bulkBusy}
                onClick={() => void applyBulk(false)}
              >
                Turn off
              </Button>
            </div>
          </div>
          {activeInSelection.length > 0 ? (
            <p className="text-xs text-amber-700">
              {activeInSelection.length} active class{activeInSelection.length === 1 ? "" : "es"} in this
              selection — turning off hides {activeInSelection.length === 1 ? "it" : "them"} from students and closes{" "}
              {activeInSelection.length === 1 ? "it" : "them"} to sit-ins.
            </p>
          ) : null}
        </div>
      ) : null}

      {totalSubjects > SUBJECT_PAGE_SIZE ? (
        <div className="flex items-center justify-between border-t border-wi-line pt-3 text-xs text-[var(--color-wi-text-light)]">
          <span>Showing {subjectOffset + 1}–{Math.min(subjectOffset + subjects.length, totalSubjects)} of {totalSubjects} subjects</span>
          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" disabled={subjectOffset === 0 || pageLoading} onClick={() => void loadPage(Math.max(0, subjectOffset - SUBJECT_PAGE_SIZE))}>Previous</Button>
            <Button variant="secondary" size="sm" disabled={subjectOffset + subjects.length >= totalSubjects || pageLoading} onClick={() => void loadPage(subjectOffset + SUBJECT_PAGE_SIZE)}>Next</Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
