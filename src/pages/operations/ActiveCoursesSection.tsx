import { useEffect, useState, useMemo } from "react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import LoadingSkeleton from "../../components/ui/LoadingSkeleton";
import Button from "../../components/ui/Button";
import EmptyState from "../../components/ui/EmptyState";
import SearchInput from "../../components/ui/SearchInput";
import { Switch } from "../../components/ui/Switch";
import type { ActiveCoursePayload, ActiveCourseSubject } from "../../types";

type ActiveCoursesResponse = {
  subjects: ActiveCourseSubject[];
  total_subjects?: number;
  total_courses?: number;
  limit?: number;
  offset?: number;
};

const SUBJECT_PAGE_SIZE = 50;

type SubjectDraft = {
  subjectId: string;
  pendingCourseId: string | null;
};

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
  onSelect,
  onToggleVisibility,
  visibilitySaving,
}: {
  subject: ActiveCourseSubject;
  courseId: string;
  isSelected: boolean;
  dirty: boolean;
  disabled: boolean;
  onSelect: (courseId: string) => void;
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
      }`}
    >
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
  const [searchQuery, setSearchQuery] = useState("");
  const [totalSubjects, setTotalSubjects] = useState(0);
  const [subjectOffset, setSubjectOffset] = useState(0);
  const [pageLoading, setPageLoading] = useState(false);

  const filteredSubjects = useMemo(() => {
    if (!searchQuery.trim()) return subjects;
    const q = searchQuery.toLowerCase();
    return subjects.filter(
      (s) =>
        s.subject_code.toLowerCase().includes(q) ||
        s.subject_name.toLowerCase().includes(q),
    );
  }, [subjects, searchQuery]);

  /** A subject only counts as covered when its active course is actually
   *  bookable: an active course hidden from the form is a trap state students
   *  experience as "this subject is gone", so it must surface as a warning,
   *  never as green. */
  const coveredCount = useMemo(
    () =>
      subjects.filter((s) => {
        const active = s.courses.find((c) => c.is_active);
        return !!active && active.absence_form_visible !== false;
      }).length,
    [subjects],
  );

  const hiddenActiveCount = useMemo(
    () =>
      subjects.filter((s) => {
        const active = s.courses.find((c) => c.is_active);
        return !!active && active.absence_form_visible === false;
      }).length,
    [subjects],
  );

  const dirtySubjectIds = useMemo(() => {
    const dirty: string[] = [];
    for (const subject of subjects) {
      const draft = drafts[subject.subject_id];
      if (!draft) continue;
      if (draft.pendingCourseId !== originals[subject.subject_id]) {
        dirty.push(subject.subject_id);
      }
    }
    return dirty;
  }, [drafts, originals, subjects]);

  const hasBulkDirty = dirtySubjectIds.length >= 2;

  async function loadSubjects(offset = subjectOffset) {
    setLoading(true);
    setLoadError(null);
    try {
      const data = await apiJson<ActiveCoursesResponse>(
        `/api/v1/admin/active-courses?limit=${SUBJECT_PAGE_SIZE}&offset=${offset}`,
        { method: "GET" },
      );
      setSubjects(data.subjects);
      setTotalSubjects(data.total_subjects ?? data.subjects.length);
      setSubjectOffset(data.offset ?? offset);
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

  async function loadPage(offset: number) {
    setPageLoading(true);
    try {
      await loadSubjects(offset);
    } finally {
      setPageLoading(false);
    }
  }

  useEffect(() => {
    void loadSubjects();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
      setSubjects((prev) =>
        prev.map((s) =>
          s.subject_id === subjectId
            ? {
                ...s,
                courses: s.courses.map((c) =>
                  c.course_id === courseId ? { ...c, absence_form_visible: next } : c,
                ),
              }
            : s,
        ),
      );
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

  if (loading) {
    return <LoadingSkeleton type="card" lines={5} />;
  }

  if (loadError) {
    return (
      <EmptyState
        message={loadError}
        action={(
          <Button variant="secondary" size="sm" onClick={() => void loadSubjects()}>
            Retry
          </Button>
        )}
      />
    );
  }

  if (subjects.length === 0) {
    return <EmptyState message="No subjects configured yet" />;
  }

  const allCovered = coveredCount === subjects.length;

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
          allCovered && hiddenActiveCount === 0
            ? "border-green-200 bg-green-50 text-green-700"
            : "border-amber-200 bg-amber-50 text-amber-700"
        }`}
      >
        <span className="font-medium">
          {allCovered
            ? "All subjects configured ✓"
            : `${coveredCount}/${subjects.length} subjects have a bookable active course`}
        </span>
        {hiddenActiveCount > 0 ? (
          <span>
            {hiddenActiveCount} subject{hiddenActiveCount === 1 ? "" : "s"}{" "}
            {hiddenActiveCount === 1 ? "has" : "have"} an active course that is hidden — students
            can&apos;t book that class until it is made visible again.
          </span>
        ) : null}
      </div>

      <SearchInput
        value={searchQuery}
        onChange={setSearchQuery}
        placeholder="Search subjects..."
      />
      {searchQuery.trim() && (
        <p className="text-xs text-[var(--color-wi-text-light)]">
          Showing {filteredSubjects.length} of {subjects.length} loaded subjects
        </p>
      )}

      {filteredSubjects.map((subject) => {
        const draft = drafts[subject.subject_id];
        const dirty = isDirty(subject.subject_id);
        const isSaving = savingSubjects.has(subject.subject_id);
        const pendingCourseId = draft?.pendingCourseId ?? null;
        const savedActive = subject.courses.find((c) => c.is_active);
        const savedActiveHidden = !!savedActive && savedActive.absence_form_visible === false;

        return (
          <div
            key={subject.subject_id}
            className={`rounded-sm border bg-white shadow-sm ${
              dirty ? "border-l-2 border-l-blue-500 border-wi-line" : "border-wi-line"
            }`}
          >
            <div className="flex items-center justify-between border-b border-wi-line-soft bg-[var(--color-wi-row-alt)]/70 px-4 py-3">
              <div className="flex items-center gap-2">
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
                    onSelect={(courseId) => handleCourseChange(subject.subject_id, courseId)}
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

      {hasBulkDirty ? (
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
