import { useEffect, useMemo, useRef, useState } from "react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import LoadingSkeleton from "../../components/ui/LoadingSkeleton";
import Button from "../../components/ui/Button";
import EmptyState from "../../components/ui/EmptyState";
import SearchInput from "../../components/ui/SearchInput";
import { Switch } from "../../components/ui/Switch";
import type { ActiveCoursePayload, ActiveCourseSubject } from "../../types";

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

type StatusFilter = "all" | "configured" | "hidden_active" | "missing_active";

const STATUS_FILTERS: Array<{ value: StatusFilter; label: string; title: string }> = [
  { value: "all", label: "All", title: "Every subject" },
  { value: "configured", label: "Active", title: "Active course set and visible — students can book" },
  { value: "hidden_active", label: "Active, hidden", title: "Active course is hidden — students cannot book this subject" },
  { value: "missing_active", label: "No active", title: "No active course set — absence forms cannot auto-assign" },
];

type SubjectDraft = {
  subjectId: string;
  pendingCourseId: string | null;
};

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

/** One course row: the active-course choice (radio — it is a single pick per
 *  subject) and the absence-form visibility switch live together, because
 *  both only matter in combination: what the student form shows is
 *  "active course, when visible". This page is the single management surface
 *  for both. */
function CourseRow({
  subject,
  courseId,
  isSelected,
  dirty,
  disabled,
  selected,
  onSelect,
  onToggleSelect,
  onToggleVisibility,
  visibilitySaving,
}: {
  subject: ActiveCourseSubject;
  courseId: string;
  isSelected: boolean;
  dirty: boolean;
  disabled: boolean;
  selected: boolean;
  onSelect: (courseId: string) => void;
  onToggleSelect: (courseId: string) => void;
  onToggleVisibility: (courseId: string, next: boolean) => void;
  visibilitySaving: boolean;
}) {
  const course = subject.courses.find((c) => c.course_id === courseId);
  if (!course) return null;
  const visible = course.absence_form_visible !== false;
  const isActiveSaved = course.is_active;

  return (
    <div
      className={`flex flex-wrap items-center gap-x-3 gap-y-1.5 px-4 py-2.5 text-sm transition-colors hover:bg-[var(--color-wi-row-alt)]/50 ${
        dirty && isSelected ? "bg-blue-50/50" : ""
      } ${selected ? "bg-blue-50/40" : ""}`}
    >
      <TriStateCheckbox
        checked={selected}
        onChange={() => onToggleSelect(course.course_id)}
        label={`Select ${course.course_code}`}
      />
      <label className="flex cursor-pointer items-center gap-3">
        <input
          type="radio"
          name={`active-course-${subject.subject_id}`}
          checked={isSelected}
          onChange={() => onSelect(course.course_id)}
          disabled={disabled}
          className="accent-[var(--color-wi-primary)]"
        />
        <span className="min-w-0">
          <span className="font-mono text-xs text-[var(--color-wi-text-light)]">{course.course_code}</span>
          <span className="ml-2 text-[var(--color-wi-text-light)]">{subject.subject_name}</span>
          <span className="ml-2 text-xs text-[var(--color-wi-text-light)]">({course.cycle_label || "no cycle"})</span>
        </span>
      </label>

      <span className="ml-auto flex min-w-0 items-center gap-2">
        {!dirty && isActiveSaved && visible ? (
          <span className="inline-flex items-center rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
            Active
          </span>
        ) : null}
        {!dirty && isActiveSaved && !visible ? (
          <span
            className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800"
            title="Students cannot select this class in the absence form while it is hidden"
          >
            Active — hidden from form
          </span>
        ) : null}
        {dirty && isSelected ? <span className="text-xs font-medium text-blue-600">Selected</span> : null}
        <Switch
          checked={visible}
          onCheckedChange={(next) => onToggleVisibility(course.course_id, next)}
          disabled={visibilitySaving}
          aria-label={`Show ${course.course_code} in the student absence form`}
        />
        {visible ? (
          <span className="text-xs text-[var(--color-wi-text-light)]">In student form</span>
        ) : (
          <span className="text-xs font-medium text-amber-700">Hidden from students</span>
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
  const [drafts, setDrafts] = useState<Record<string, SubjectDraft>>({});
  const [originals, setOriginals] = useState<Record<string, string | null>>({});
  const [savingSubjects, setSavingSubjects] = useState<Set<string>>(new Set());
  const [savingVisibility, setSavingVisibility] = useState<Set<string>>(new Set());
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
      const initDrafts: Record<string, SubjectDraft> = {};
      const initOriginals: Record<string, string | null> = {};
      for (const subject of data.subjects) {
        const active = subject.courses.find((c) => c.is_active);
        const activeId = active?.course_id ?? null;
        initOriginals[subject.subject_id] = activeId;
        initDrafts[subject.subject_id] = { subjectId: subject.subject_id, pendingCourseId: activeId };
      }
      setOriginals(initOriginals);
      setDrafts(initDrafts);
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
  const loadedCourseIds = useMemo(
    () => new Set(loadedCourses.map((x) => x.course.course_id)),
    [loadedCourses],
  );
  const selectedLoaded = useMemo(
    () => loadedCourses.filter((x) => selection.has(x.course.course_id)),
    [loadedCourses, selection],
  );
  const selectedNotOnPage = selection.size - selectedLoaded.length;

  const pageCourseIds = useMemo(() => loadedCourses.map((x) => x.course.course_id), [loadedCourses]);
  const allPageSelected = pageCourseIds.length > 0 && pageCourseIds.every((id) => selection.has(id));
  const somePageSelected = pageCourseIds.some((id) => selection.has(id));

  /** Only ids whose loaded state differs from the target are sent (plus
   *  unknown ids from other pages) so the bulk statement never carries no-ops. */
  const bulkTargets = (target: boolean) =>
    selectedLoaded.filter((x) => (x.course.absence_form_visible !== false) !== target);
  const canApply = (target: boolean) => bulkTargets(target).length > 0 || selectedNotOnPage > 0;
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

  function patchSubjectsVisible(courseIds: string[], visible: boolean) {
    const idSet = new Set(courseIds);
    setSubjects((prev) =>
      prev.map((s) => ({
        ...s,
        courses: s.courses.map((c) =>
          idSet.has(c.course_id) ? { ...c, absence_form_visible: visible } : c,
        ),
      })),
    );
  }

  /** The audit counts treat "active course hidden" as its own state, so a
   *  visibility change to an active course must move the counter. Courses not
   *  on the loaded page are corrected by the next full load. */
  function bumpHiddenActive(delta: number) {
    setStats((prev) => (prev ? { ...prev, hidden_active: Math.max(0, prev.hidden_active + delta) } : prev));
  }

  async function applyBulk(target: boolean) {
    const flips = bulkTargets(target);
    const ids = [
      ...flips.map((x) => x.course.course_id),
      ...[...selection].filter((id) => !loadedCourseIds.has(id)),
    ];
    if (ids.length === 0) return;

    const activeFlips = flips.filter((x) => x.course.is_active).length;
    if (!target && activeFlips > 0) {
      const confirmed = window.confirm(
        `Hide ${ids.length} classes from the student absence form?\n\n` +
          `${activeFlips} of them are active courses — those subjects will not be ` +
          `bookable by students until they are shown again. Sit-ins and staff booking are unaffected.`,
      );
      if (!confirmed) return;
    }

    setBulkBusy(true);
    try {
      const res = await apiJson<{ updated: number }>(
        "/api/v1/admin/active-courses/visibility/bulk",
        {
          method: "PUT",
          body: JSON.stringify({ course_ids: ids, absence_form_visible: target }),
        },
      );
      patchSubjectsVisible(ids, target);
      bumpHiddenActive(target ? -activeFlips : activeFlips);
      setSelection(new Set());
      addToast(
        "success",
        target
          ? `${res.updated} class${res.updated === 1 ? "" : "es"} shown in the student absence form.`
          : `${res.updated} class${res.updated === 1 ? "" : "es"} hidden — students can no longer select them. Sit-ins and staff booking are unaffected.`,
      );
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Bulk update failed");
    } finally {
      setBulkBusy(false);
    }
  }

  // ----- Draft (active-course choice) machinery -----

  function handleCourseChange(subjectId: string, courseId: string) {
    setDrafts((prev) => ({
      ...prev,
      [subjectId]: { subjectId, pendingCourseId: courseId },
    }));
  }

  function isDirty(subjectId: string): boolean {
    const draft = drafts[subjectId];
    if (!draft) return false;
    return draft.pendingCourseId !== originals[subjectId];
  }

  const dirtySubjectIds = useMemo(() => {
    const dirty: string[] = [];
    for (const subject of subjects) {
      if (isDirty(subject.subject_id)) dirty.push(subject.subject_id);
    }
    return dirty;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [drafts, originals, subjects]);

  const hasDirtySubjects = dirtySubjectIds.length >= 2;

  /** Visibility is independent of the active-course draft: it saves
   *  immediately through the dedicated endpoint and updates the loaded
   *  subject rows in place — no refetch, so the page never jumps. The switch
   *  locks per course while its save is in flight, so the visible state can
   *  never drift from the server. */
  async function toggleVisibility(subjectId: string, courseId: string, next: boolean) {
    const subject = subjects.find((s) => s.subject_id === subjectId);
    const course = subject?.courses.find((c) => c.course_id === courseId);
    if (!course) return;
    setSavingVisibility((prev) => new Set(prev).add(courseId));
    try {
      await apiJson("/api/v1/admin/active-courses/visibility", {
        method: "PUT",
        body: JSON.stringify({ course_id: courseId, absence_form_visible: next }),
      });
      patchSubjectsVisible([courseId], next);
      if (course.is_active) bumpHiddenActive(next ? -1 : 1);
      addToast(
        "success",
        next
          ? `${course.course_code} is now visible in the student absence form`
          : `${course.course_code} hidden — students can no longer select it. Sit-ins and staff booking are unaffected.`,
      );
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to update visibility");
    } finally {
      setSavingVisibility((prev) => {
        const nextSet = new Set(prev);
        nextSet.delete(courseId);
        return nextSet;
      });
    }
  }

  async function saveSubject(subjectId: string, silent = false): Promise<boolean> {
    const draft = drafts[subjectId];
    if (!draft) return false;
    const subject = subjects.find((s) => s.subject_id === subjectId);
    if (!subject) return false;
    const courseId = draft.pendingCourseId;
    if (courseId === null) return false;

    setSavingSubjects((prev) => {
      const next = new Set(prev);
      next.add(subjectId);
      return next;
    });

    try {
      const payload: ActiveCoursePayload = { subject_id: subjectId, course_id: courseId };
      await apiJson("/api/v1/admin/active-courses", {
        method: "PUT",
        body: JSON.stringify(payload),
      });
      const course = subject.courses.find((c) => c.course_id === courseId);
      const courseCode = course?.course_code ?? courseId;
      if (!silent) {
        addToast(
          "success",
          `Active course set to ${courseCode} for ${subject.subject_code}. Absence forms will auto-assign this course.`,
        );
      }
      setOriginals((prev) => ({ ...prev, [subjectId]: courseId }));
      setDrafts((prev) => ({
        ...prev,
        [subjectId]: { subjectId, pendingCourseId: courseId },
      }));
      setSubjects((prev) =>
        prev.map((s) =>
          s.subject_id === subjectId
            ? {
                ...s,
                courses: s.courses.map((c) => ({
                  ...c,
                  is_active: c.course_id === courseId,
                })),
              }
            : s,
        ),
      );
      return true;
    } catch (err) {
      if (!silent) {
        addToast("error", `Failed to update ${subject.subject_code}`);
      }
      setDrafts((prev) => ({
        ...prev,
        [subjectId]: { subjectId, pendingCourseId: originals[subjectId] },
      }));
      return false;
    } finally {
      setSavingSubjects((prev) => {
        const next = new Set(prev);
        next.delete(subjectId);
        return next;
      });
    }
  }

  function cancelSubject(subjectId: string) {
    setDrafts((prev) => ({
      ...prev,
      [subjectId]: { subjectId, pendingCourseId: originals[subjectId] },
    }));
  }

  async function saveAllDirty() {
    const ids = dirtySubjectIds;
    let succeeded = 0;
    for (const subjectId of ids) {
      const ok = await saveSubject(subjectId, true);
      if (ok) succeeded++;
    }
    if (succeeded === 0) {
      addToast("error", "Failed to save changes. Please try again.");
    } else if (succeeded < ids.length) {
      addToast("warning", `Saved ${succeeded} of ${ids.length} subjects. ${ids.length - succeeded} failed.`);
    } else {
      addToast("success", `Updates saved for ${ids.length} subjects.`);
    }
  }

  function discardAllDirty() {
    setDrafts((prev) => {
      const next = { ...prev };
      for (const subjectId of dirtySubjectIds) {
        next[subjectId] = { subjectId, pendingCourseId: originals[subjectId] };
      }
      return next;
    });
  }

  // ----- Audit banner (server stats when available, page fallback) -----

  const missingCount = useMemo(() => {
    if (stats) return stats.missing_active;
    return subjects.filter((s) => !s.courses.some((c) => c.is_active)).length;
  }, [stats, subjects]);

  const hiddenActiveCount = useMemo(() => {
    if (stats) return stats.hidden_active;
    return subjects.filter((s) => {
      const active = s.courses.find((c) => c.is_active);
      return !!active && active.absence_form_visible === false;
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
            <strong className="text-[var(--color-wi-text)]">Active course</strong> — the class the form
            auto-assigns when a student reports an absence for the subject.
          </li>
          <li>
            <strong className="text-[var(--color-wi-text)]">In student form</strong> — whether students can
            select the class in the absence form. New classes start visible.
          </li>
          <li>
            Hidden classes can&apos;t be picked by students, but still accept <strong className="text-[var(--color-wi-text)]">sit-in</strong> students —
            and staff can always book any class.
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
            ? "All subjects configured ✓"
            : `${coveredCount}/${totalSubjectCount} subjects have a bookable active course`}
        </span>
        {hiddenActiveCount > 0 ? (
          <span>
            {hiddenActiveCount} subject{hiddenActiveCount === 1 ? "" : "s"}{" "}
            {hiddenActiveCount === 1 ? "has" : "have"} an active course that is hidden — students
            can&apos;t book that class until it is made visible again.
          </span>
        ) : null}
      </div>

      <div className="flex flex-col gap-2">
        <SearchInput
          value={searchInput}
          onChange={setSearchInput}
          placeholder="Search subjects across all pages..."
        />
        <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label="Filter subjects by active-course state">
          {STATUS_FILTERS.map((f) => {
            const count =
              f.value === "all"
                ? totalSubjectCount
                : f.value === "configured"
                  ? coveredCount
                  : f.value === "hidden_active"
                    ? hiddenActiveCount
                    : missingCount;
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
        const draft = drafts[subject.subject_id];
        const dirty = isDirty(subject.subject_id);
        const isSaving = savingSubjects.has(subject.subject_id);
        const pendingCourseId = draft?.pendingCourseId ?? null;
        const savedActive = subject.courses.find((c) => c.is_active);
        const savedActiveHidden = !!savedActive && savedActive.absence_form_visible === false;

        const subjectIds = subject.courses.map((c) => c.course_id);
        const subjectAll = subjectIds.length > 0 && subjectIds.every((id) => selection.has(id));
        const subjectSome = subjectIds.some((id) => selection.has(id));

        return (
          <div
            key={subject.subject_id}
            className={`rounded-sm border bg-white shadow-sm ${
              dirty ? "border-l-2 border-l-blue-500 border-wi-line" : "border-wi-line"
            }`}
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
                {!dirty && savedActive && !savedActiveHidden ? (
                  <span className="inline-flex items-center rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
                    Active
                  </span>
                ) : null}
                {!dirty && savedActiveHidden ? (
                  <span
                    className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800"
                    title="Make the active course visible again so students can book this subject"
                  >
                    Active course hidden
                  </span>
                ) : null}
                {!dirty && !savedActive ? (
                  <span className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700">
                    Not set
                  </span>
                ) : null}
              </div>
              {dirty ? (
                <div className="flex items-center gap-2">
                  <Button variant="ghost" size="sm" onClick={() => cancelSubject(subject.subject_id)} disabled={isSaving}>
                    Cancel
                  </Button>
                  <Button variant="primary" size="sm" loading={isSaving} disabled={isSaving} onClick={() => void saveSubject(subject.subject_id)}>
                    Save
                  </Button>
                </div>
              ) : null}
            </div>

            {subject.courses.length > 0 ? (
              <div className="divide-y divide-gray-50">
                {subject.courses.map((course) => (
                  <CourseRow
                    key={course.course_id}
                    subject={subject}
                    courseId={course.course_id}
                    isSelected={pendingCourseId === course.course_id}
                    dirty={dirty}
                    disabled={isSaving}
                    selected={selection.has(course.course_id)}
                    onSelect={(courseId) => handleCourseChange(subject.subject_id, courseId)}
                    onToggleSelect={toggleCourseSelected}
                    onToggleVisibility={(courseId, next) => void toggleVisibility(subject.subject_id, courseId, next)}
                    visibilitySaving={savingVisibility.has(course.course_id)}
                  />
                ))}
              </div>
            ) : (
              <div className="flex items-center gap-3 px-4 py-2.5">
                <input
                  type="radio"
                  disabled
                  aria-label="No courses available"
                  className="accent-[var(--color-wi-primary)] opacity-50"
                />
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
                Show in form
              </Button>
              <Button
                variant="primary"
                size="sm"
                disabled={!canApply(false) || bulkBusy}
                loading={bulkBusy}
                onClick={() => void applyBulk(false)}
              >
                Hide from form
              </Button>
            </div>
          </div>
          {activeInSelection.length > 0 ? (
            <p className="text-xs text-amber-700">
              {activeInSelection.length} active course{activeInSelection.length === 1 ? "" : "s"} in this
              selection — hiding them makes those subjects unbookable for students until shown again.
            </p>
          ) : null}
        </div>
      ) : null}

      {hasDirtySubjects ? (
        <div className="sticky bottom-0 flex items-center justify-between rounded-sm border border-blue-200 bg-blue-50 px-4 py-3 shadow-md">
          <span className="text-sm text-blue-800">
            <strong>{dirtySubjectIds.length}</strong> subjects have unsaved changes
          </span>
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={discardAllDirty}>
              Discard All
            </Button>
            <Button variant="primary" size="sm" onClick={() => void saveAllDirty()}>
              Save All
            </Button>
          </div>
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
