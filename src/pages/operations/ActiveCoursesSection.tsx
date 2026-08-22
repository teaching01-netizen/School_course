import { useEffect, useMemo, useRef, useState } from "react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import LoadingSkeleton from "../../components/ui/LoadingSkeleton";
import Button from "../../components/ui/Button";
import EmptyState from "../../components/ui/EmptyState";
import SearchInput from "../../components/ui/SearchInput";
import { Switch } from "../../components/ui/Switch";
import type { ActiveCourseSubject } from "../../types";

type ActiveCoursesStats = {
  total_subjects: number;
  missing_active: number;
  hidden_active: number;
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
  const legacyHiddenActive = isActive && course.absence_form_visible === false;

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
        {legacyHiddenActive ? (
          <span
            className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800"
            title="Old data: the active class is hidden. Toggle it off and on to fix."
          >
            Active — hidden from form
          </span>
        ) : null}
        <Switch
          checked={isActive}
          onCheckedChange={(next) => onToggleActive(course.course_id, next)}
          disabled={saving}
          aria-label={`${course.course_code} active`}
        />
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

  // The only selection state: course ids. Checkbox states, counts, and bulk
  // previews all derive from it. Selection survives page changes so a bulk
  // action can span pages; ids not on the current page stay in the set.
  const [selection, setSelection] = useState<Set<string>>(new Set());
  const [bulkBusy, setBulkBusy] = useState(false);

  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(searchInput), 300);
    return () => clearTimeout(t);
  }, [searchInput]);

  async function loadSubjects(offset: number, search: string, statusFilter: StatusFilter) {
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
      const data = await apiJson<ActiveCoursesResponse>(
        `/api/v1/admin/active-courses?${params.toString()}`,
        { method: "GET" },
      );
      setSubjects(data.subjects);
      setTotalSubjects(data.total_subjects ?? data.subjects.length);
      setSubjectOffset(data.offset ?? offset);
      if (data.stats) setStats(data.stats);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to load active courses";
      setLoadError(message);
      addToast("error", message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    setSubjectOffset(0);
    void loadSubjects(0, debouncedSearch, status);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedSearch, status]);

  async function loadPage(offset: number) {
    setPageLoading(true);
    try {
      await loadSubjects(offset, debouncedSearch, status);
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
      const next = new Set(prev);
      if (next.has(courseId)) next.delete(courseId);
      else next.add(courseId);
      return next;
    });
  }

  function toggleSubjectCourses(subject: ActiveCourseSubject) {
    const ids = subject.courses.map((c) => c.course_id);
    setSelection((prev) => {
      const next = new Set(prev);
      const allSelected = ids.length > 0 && ids.every((id) => next.has(id));
      for (const id of ids) {
        if (allSelected) next.delete(id);
        else next.add(id);
      }
      return next;
    });
  }

  function togglePageSelection() {
    setSelection((prev) => {
      const next = new Set(prev);
      const target = !allPageSelected;
      for (const id of pageCourseIds) {
        if (target) next.add(id);
        else next.delete(id);
      }
      return next;
    });
  }

  /** Local audit-count adjustments so the banner stays truthful without a
   *  refetch; the next full load reconciles anything not derivable here. */
  function bumpStats(delta: Partial<Pick<ActiveCoursesStats, "missing_active" | "hidden_active">>) {
    setStats((prev) =>
      prev
        ? {
            ...prev,
            missing_active: Math.max(0, prev.missing_active + (delta.missing_active ?? 0)),
            hidden_active: Math.max(0, prev.hidden_active + (delta.hidden_active ?? 0)),
          }
        : prev,
    );
  }

  // ----- Immediate actions -----

  /** A subject's audit state: whether any class is active and whether every
   *  active class is actually visible. Mirrors the backend's per-subject
   *  classification used by the stats and filter chips. */
  function subjectAuditState(courses: ActiveCourseSubject["courses"]) {
    const actives = courses.filter((c) => c.is_active);
    return {
      hasActive: actives.length > 0,
      allVisible: actives.every((c) => c.absence_form_visible !== false),
    };
  }

  async function toggleActive(subjectId: string, courseId: string, next: boolean) {
    const subject = subjects.find((s) => s.subject_id === subjectId);
    const course = subject?.courses.find((c) => c.course_id === courseId);
    if (!subject || !course) return;

    setSavingActive((prev) => new Set(prev).add(courseId));
    try {
      await apiJson("/api/v1/admin/active-courses/set-active", {
        method: "PUT",
        body: JSON.stringify({ course_id: courseId, active: next }),
      });
      // The server re-derives visibility for the whole subject (visible ⇔
      // active), so mirror that locally instead of flipping one flag.
      const before = subjectAuditState(subject.courses);
      const toggled = subject.courses.map((c) => ({
        ...c,
        is_active: c.course_id === courseId ? next : c.is_active,
        absence_form_visible: c.course_id === courseId ? next : c.is_active,
      }));
      const after = subjectAuditState(toggled);
      setSubjects((prev) =>
        prev.map((s) => (s.subject_id === subjectId ? { ...s, courses: toggled } : s)),
      );
      const missingDelta = (before.hasActive ? 0 : 1) - (after.hasActive ? 0 : 1);
      const hiddenDelta = (before.hasActive && !before.allVisible ? 1 : 0) - (after.hasActive && !after.allVisible ? 1 : 0);
      if (missingDelta !== 0 || hiddenDelta !== 0) {
        bumpStats({ missing_active: missingDelta, hidden_active: hiddenDelta });
      }
      addToast(
        "success",
        next
          ? `${course.course_code} is active — students can see it and sit in.`
          : `${course.course_code} turned off — hidden from students and closed to sit-ins.`,
      );
    } catch (err) {
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
    const ids = [...selection];
    if (ids.length === 0) return;

    const activeFlips = selectedLoaded.filter((x) => x.course.is_active !== target && x.course.is_active).length;
    if (!target && activeFlips > 0) {
      const confirmed = window.confirm(
        `Turn off ${ids.length} classes?\n\n` +
          `${activeFlips} of them are active — those classes will be hidden from students ` +
          `and closed to sit-ins until activated again.`,
      );
      if (!confirmed) return;
    }

    setBulkBusy(true);
    try {
      await apiJson("/api/v1/admin/active-courses/set-active/bulk", {
        method: "PUT",
        body: JSON.stringify({ course_ids: ids, active: target }),
      });
      setSelection(new Set());
      await loadSubjects(subjectOffset, debouncedSearch, status);
      addToast(
        "success",
        target
          ? "Activated — the selected classes are now live for students (visible and sit-in-able)."
          : "Turned off — the selected classes are hidden from students and closed to sit-ins.",
      );
    } catch (err) {
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

  const hiddenActiveCount = useMemo(() => {
    if (stats) return stats.hidden_active;
    return subjects.filter((s) => {
      const { hasActive, allVisible } = subjectAuditState(s.courses);
      return hasActive && !allVisible;
    }).length;
  }, [stats, subjects]);

  const totalSubjectCount = stats?.total_subjects ?? subjects.length;
  const coveredCount = Math.max(0, totalSubjectCount - missingCount - hiddenActiveCount);
  const allCovered = coveredCount === totalSubjectCount && hiddenActiveCount === 0;

  if (loading && subjects.length === 0) {
    return <LoadingSkeleton type="card" lines={5} />;
  }

  if (loadError && subjects.length === 0) {
    return (
      <EmptyState
        message={loadError}
        action={(
          <Button variant="secondary" size="sm" onClick={() => void loadSubjects(0, debouncedSearch, status)}>
            Retry
          </Button>
        )}
      />
    );
  }

  if (subjects.length === 0) {
    const filtered = searchInput.trim() !== "" || status !== "all";
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

  const filterActive = status !== "all" || searchInput.trim() !== "";

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
        {hiddenActiveCount > 0 ? (
          <span>
            {hiddenActiveCount} subject{hiddenActiveCount === 1 ? "" : "s"}{" "}
            {hiddenActiveCount === 1 ? "has" : "have"} an active course that is hidden (old data) — toggle
            {" "}the class off and on to fix.
          </span>
        ) : null}
      </div>

      <div className="flex flex-col gap-2">
        <SearchInput
          value={searchInput}
          onChange={setSearchInput}
          placeholder="Search subjects, classes, or merged courses..."
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
        const activeHiddenLegacy = activeCourses.some((c) => c.absence_form_visible === false);

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
                {activeCourses.length > 0 && !activeHiddenLegacy ? (
                  <span
                    className="inline-flex items-center rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700"
                    title={activeCourses.length > 1 ? "Parallel active classes — students only see the ones they're enrolled in" : undefined}
                  >
                    {activeCourses.length > 1 ? `Active (${activeCourses.length})` : "Active"}
                  </span>
                ) : null}
                {activeHiddenLegacy ? (
                  <span
                    className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800"
                    title="Toggle the hidden active class off and on to fix"
                  >
                    Active course hidden
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
                    onToggleActive={(courseId, next) => void toggleActive(subject.subject_id, courseId, next)}
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
              <Button variant="ghost" size="sm" onClick={() => setSelection(new Set())} disabled={bulkBusy}>
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
