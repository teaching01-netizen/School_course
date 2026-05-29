import { useEffect, useState, useMemo } from "react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import LoadingSkeleton from "../../components/ui/LoadingSkeleton";
import Button from "../../components/ui/Button";
import EmptyState from "../../components/ui/EmptyState";
import SearchInput from "../../components/ui/SearchInput";
import type { ActiveCoursePayload, ActiveCourseSubject } from "../../types";

type ActiveCoursesResponse = {
  subjects: ActiveCourseSubject[];
};

type SubjectDraft = {
  subjectId: string;
  pendingCourseId: string | null;
};

export function ActiveCoursesSection() {
  const { addToast } = useToast();
  const [subjects, setSubjects] = useState<ActiveCourseSubject[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<string, SubjectDraft>>({});
  const [originals, setOriginals] = useState<Record<string, string | null>>({});
  const [savingSubjects, setSavingSubjects] = useState<Set<string>>(new Set());
  const [searchQuery, setSearchQuery] = useState("");

  const filteredSubjects = useMemo(() => {
    if (!searchQuery.trim()) return subjects;
    const q = searchQuery.toLowerCase();
    return subjects.filter(
      (s) =>
        s.subject_code.toLowerCase().includes(q) ||
        s.subject_name.toLowerCase().includes(q),
    );
  }, [subjects, searchQuery]);

  const coveredCount = useMemo(
    () => subjects.filter((s) => s.courses.some((c) => c.is_active)).length,
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

  async function loadSubjects() {
    setLoading(true);
    setLoadError(null);
    try {
      const data = await apiJson<ActiveCoursesResponse>("/api/v1/admin/active-courses", { method: "GET" });
      setSubjects(data.subjects);
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
    void loadSubjects();
  }, [addToast]);

  function handleCourseChange(subjectId: string, courseId: string | null) {
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
      <p className="text-sm text-gray-500">
        Select which course is active for each subject. The absence form will auto-assign the active course when a student reports an absence.
      </p>

      <div
        className={`flex items-center gap-2 rounded-sm border px-4 py-2.5 text-sm ${
          allCovered
            ? "border-green-200 bg-green-50 text-green-700"
            : "border-amber-200 bg-amber-50 text-amber-700"
        }`}
      >
        {allCovered ? (
          <span className="font-medium text-green-600">All subjects configured ✓</span>
        ) : (
          <>
            <svg className="h-4 w-4 shrink-0 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <span>
              <strong>{coveredCount}/{subjects.length}</strong> subjects have an active course set
            </span>
          </>
        )}
      </div>

      <SearchInput
        value={searchQuery}
        onChange={setSearchQuery}
        placeholder="Search subjects..."
      />
      {searchQuery.trim() && (
        <p className="text-xs text-gray-400">
          Showing {filteredSubjects.length} of {subjects.length} subjects
        </p>
      )}

      {filteredSubjects.map((subject) => {
        const draft = drafts[subject.subject_id];
        const dirty = isDirty(subject.subject_id);
        const isSaving = savingSubjects.has(subject.subject_id);
        const active = subject.courses.find((c) => c.is_active);
        const pendingCourseId = draft?.pendingCourseId ?? null;
        const hasMultipleCourses = subject.courses.length > 1;
        const hasSingleCourse = subject.courses.length === 1;

        return (
          <div
            key={subject.subject_id}
            className={`rounded-sm border bg-white shadow-sm ${
              dirty ? "border-l-2 border-l-blue-500 border-gray-200" : "border-gray-200"
            }`}
          >
            <div className="flex items-center justify-between border-b border-gray-100 bg-gray-50/70 px-4 py-3">
              <div className="flex items-center gap-2">
                <span className="text-sm font-semibold text-gray-800">
                  {subject.subject_code} — {subject.subject_name}
                </span>
                {!dirty && active ? (
                  <span className="inline-flex items-center rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
                    Active
                  </span>
                ) : null}
                {!dirty && !active ? (
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

            {hasMultipleCourses ? (
              <div className="divide-y divide-gray-50">
                {subject.courses.map((course) => {
                  const isSelected = pendingCourseId === course.course_id;
                  const isDisabled = isSaving;
                  return (
                    <label
                      key={course.course_id}
                      className={`flex cursor-pointer items-center gap-3 px-4 py-2.5 text-sm hover:bg-gray-50/50 ${
                        dirty && isSelected ? "bg-blue-50/50" : ""
                      }`}
                    >
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={() => handleCourseChange(subject.subject_id, course.course_id)}
                        disabled={isDisabled}
                        className="accent-[var(--color-wi-primary)]"
                      />
                      <span className="flex-1">
                        <span className="font-mono text-xs text-gray-500">{course.course_code}</span>
                        <span className="ml-2 text-gray-700">{course.course_name}</span>
                        <span className="ml-2 text-xs text-gray-400">({course.cycle_label})</span>
                      </span>
                      {!dirty && course.is_active ? (
                        <span className="inline-flex items-center rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
                          Active
                        </span>
                      ) : null}
                      {dirty && isSelected ? (
                        <span className="text-xs font-medium text-blue-600">Selected</span>
                      ) : null}
                    </label>
                  );
                })}
              </div>
            ) : hasSingleCourse ? (
              <label className="flex items-center gap-3 px-4 py-2.5 cursor-pointer">
                <input
                  type="checkbox"
                  checked={pendingCourseId === subject.courses[0].course_id}
                  onChange={() => handleCourseChange(subject.subject_id, subject.courses[0].course_id)}
                  disabled={isSaving}
                  className="accent-[var(--color-wi-primary)]"
                />
                <span className="text-sm text-gray-500">
                  {pendingCourseId === subject.courses[0].course_id ? "Active" : "Set Active"}
                </span>
                <span className="text-sm text-gray-500">
                  —{" "}
                  <span className="font-medium text-gray-700">{subject.courses[0].course_code}</span>
                  <span className="ml-1 text-xs text-gray-400">({subject.courses[0].cycle_label})</span>
                </span>
              </label>
            ) : (
              <div className="flex items-center gap-3 px-4 py-2.5">
                <input
                  type="checkbox"
                  disabled
                  aria-label="No courses available"
                  className="accent-[var(--color-wi-primary)] opacity-50"
                />
                <span className="text-sm italic text-gray-400">No courses — create one first</span>
                <a
                  href="/courses/create"
                  className="inline-flex items-center justify-center rounded-sm border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-[var(--color-wi-text)] transition-colors hover:bg-gray-50"
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
    </div>
  );
}
