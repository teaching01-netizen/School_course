import { useEffect, useMemo, useRef, useState } from "react";
import Button from "./ui/Button";
import Select from "./ui/Select";

export type ActiveCourseEntry = {
  course_id: string;
  course_code: string;
  course_name: string;
  cycle_id: string;
  cycle_label: string;
  is_active: boolean;
};

export type ActiveCourseSubject = {
  subject_id: string;
  subject_code: string;
  subject_name: string;
  courses: ActiveCourseEntry[];
};

interface ActiveCoursesPanelProps {
  subjects: ActiveCourseSubject[];
  activeCourseId: Record<string, string>;
  saving: Record<string, boolean>;
  onSave: (subjectId: string, courseId: string) => Promise<void>;
}

type BulkChoice = Record<string, string>;

const groupedByCycle = (courses: ActiveCourseEntry[]) => {
  const groups = new Map<string, ActiveCourseEntry[]>();
  for (const course of courses) {
    const key = course.cycle_label || course.cycle_id || "No cycle";
    const list = groups.get(key) ?? [];
    list.push(course);
    groups.set(key, list);
  }
  for (const list of groups.values()) {
    list.sort((a, b) => a.course_code.localeCompare(b.course_code));
  }
  return [...groups.entries()].sort((a, b) => a[0].localeCompare(b[0]));
};

// Preselect the newest cycle's last course: labels sort chronologically in
// practice, and the last code inside that cycle is the most recent section.
const defaultChoiceFor = (subject: ActiveCourseSubject, current?: string): string => {
  if (current && subject.courses.some((c) => c.course_id === current)) return current;
  const groups = groupedByCycle(subject.courses);
  const lastGroup = groups[groups.length - 1];
  if (!lastGroup || lastGroup[1].length === 0) return "";
  return lastGroup[1][lastGroup[1].length - 1].course_id;
};

export default function ActiveCoursesPanel({
  subjects,
  activeCourseId,
  saving,
  onSave,
}: ActiveCoursesPanelProps) {
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [selectionMode, setSelectionMode] = useState(false);
  const [showUnassigned, setShowUnassigned] = useState(false);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [bulkChoices, setBulkChoices] = useState<BulkChoice>({});
  const [bulkState, setBulkState] = useState<"idle" | "running" | "done">("idle");
  const [bulkErrors, setBulkErrors] = useState<Record<string, string>>({});
  const [bulkDone, setBulkDone] = useState<{ applied: number; failed: number } | null>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const firstSelectRef = useRef<HTMLSelectElement>(null);

  const hasActiveCourse = (subject: ActiveCourseSubject) => {
    const active = activeCourseId[subject.subject_id];
    return Boolean(active && subject.courses.some((course) => course.course_id === active));
  };

  const visible = useMemo(() => {
    const needle = search.trim().toLowerCase();
    const searchable = subjects.filter(
      (subject) =>
        subject.subject_code.toLowerCase().includes(needle) ||
        subject.subject_name.toLowerCase().includes(needle) ||
        subject.courses.some((c) => c.course_code.toLowerCase().includes(needle)),
    );
    if (showUnassigned) return searchable;
    return searchable.filter(hasActiveCourse);
  }, [activeCourseId, search, showUnassigned, subjects]);

  const stats = useMemo(() => {
    const unset = subjects.filter((s) => {
      const active = activeCourseId[s.subject_id];
      return !active || !s.courses.some((c) => c.course_id === active);
    });
    return { total: subjects.length, set: subjects.length - unset.length, unset: unset.length };
  }, [subjects, activeCourseId]);

  const allVisibleSelected =
    visible.length > 0 && visible.every((s) => selected.has(s.subject_id));

  const toggleSubject = (subjectId: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(subjectId)) next.delete(subjectId);
      else next.add(subjectId);
      return next;
    });
  };

  const toggleAllVisible = () => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (allVisibleSelected) {
        for (const s of visible) next.delete(s.subject_id);
      } else {
        for (const s of visible) next.add(s.subject_id);
      }
      return next;
    });
  };

  const selectUnset = () => {
    const unset = subjects.filter((s) => {
      const active = activeCourseId[s.subject_id];
      return !active || !s.courses.some((c) => c.course_id === active);
    });
    setShowUnassigned(true);
    setSelectionMode(true);
    setSelected(new Set(unset.map((s) => s.subject_id)));
  };

  const toggleUnassigned = () => {
    if (showUnassigned) setSelected(new Set());
    setShowUnassigned((previous) => !previous);
  };

  const openBulk = () => {
    const targets = subjects.filter((s) => selected.has(s.subject_id) && s.courses.length > 0);
    const choices: BulkChoice = {};
    for (const subject of targets) {
      const choice = defaultChoiceFor(subject, activeCourseId[subject.subject_id]);
      if (choice) choices[subject.subject_id] = choice;
    }
    setBulkChoices(choices);
    setBulkErrors({});
    setBulkDone(null);
    setBulkState("idle");
    setBulkOpen(true);
  };

  useEffect(() => {
    if (!bulkOpen) return;
    firstSelectRef.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setBulkOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [bulkOpen]);

  const runBulk = async () => {
    const entries = Object.entries(bulkChoices).filter(([subjectId]) =>
      selected.has(subjectId),
    );
    if (entries.length === 0) return;
    setBulkState("running");
    setBulkErrors({});
    let applied = 0;
    const errors: Record<string, string> = {};
    for (const [subjectId, courseId] of entries) {
      try {
        await onSave(subjectId, courseId);
        applied += 1;
      } catch {
        const subject = subjects.find((s) => s.subject_id === subjectId);
        errors[subjectId] = `Failed to set active course for ${subject?.subject_code ?? subjectId}`;
      }
    }
    setBulkErrors(errors);
    setBulkDone({ applied, failed: entries.length - applied });
    setBulkState("done");
  };

  const bulkTargets = subjects.filter(
    (s) => selected.has(s.subject_id) && s.courses.length > 0,
  );

  return (
    <section aria-label="Active courses">
      <div className="flex flex-wrap items-end justify-between gap-4 mb-4">
        <div>
          <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-faint)]">Current setup</p>
          <h2 className="text-lg font-semibold text-[var(--color-wi-text)]">Active courses</h2>
          <p className="mt-1 max-w-2xl text-sm text-[var(--color-wi-text-light)]">
            Only the course selected for each subject is shown. This is the course students use
            when they book an absence.
          </p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search active courses"
            aria-label="Search subjects"
            className="w-52 rounded-sm border border-wi-line px-2.5 py-1.5 text-sm bg-white hover:border-wi-line focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15"
          />
          {stats.unset > 0 && (
            <button
              type="button"
              aria-pressed={showUnassigned}
              onClick={toggleUnassigned}
              className="rounded-sm px-2.5 py-1.5 text-xs font-medium text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]/40"
            >
              {showUnassigned ? "Hide unassigned" : `Show unassigned (${stats.unset})`}
            </button>
          )}
          {subjects.some((subject) => subject.courses.length > 0) && (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setSelectionMode((previous) => !previous)}
            >
              {selectionMode ? "Done" : "Bulk change"}
            </Button>
          )}
        </div>
      </div>

      <div className="mb-3 flex flex-wrap items-center gap-3 text-xs text-[var(--color-wi-text-light)]">
        <span className="font-medium text-[var(--color-wi-text)]">
          {stats.set} of {stats.total} subjects have an active course
        </span>
        {stats.unset > 0 && (
          <span className="flex items-center gap-1 text-[var(--color-wi-amber)]">
            <span className="inline-block w-2 h-2 rounded-full bg-[var(--color-wi-amber)]" />
            {stats.unset} not set
          </span>
        )}
        {stats.unset > 0 && (
          <button
            type="button"
            onClick={selectUnset}
            className="font-medium text-[var(--color-wi-primary)] hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]/40 rounded-sm"
          >
            Select subjects without an active course
          </button>
        )}
      </div>

      {selectionMode && selected.size > 0 && (
        <div className="mb-3 flex items-center justify-between rounded-sm border border-[var(--color-wi-border)] bg-white px-3 py-2">
          <span className="text-sm font-medium text-[var(--color-wi-text)]">
            {selected.size} subject{selected.size === 1 ? "" : "s"} selected
          </span>
          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" onClick={() => setSelected(new Set())}>
              Clear selection
            </Button>
            <Button variant="primary" size="sm" onClick={openBulk} disabled={bulkTargets.length === 0}>
              Change active course{selected.size === 1 ? "" : "s"}…
            </Button>
          </div>
        </div>
      )}

      <div className="rounded-lg border border-[var(--color-wi-border)] bg-white overflow-hidden">
        <table className="w-full text-sm border-collapse">
          <caption className="sr-only">Active course per subject</caption>
          <thead>
            <tr className="border-b border-[var(--color-wi-border)] text-left text-xs uppercase tracking-wide text-[var(--color-wi-text-light)]">
              {selectionMode && (
                <th scope="col" className="w-8 px-3 py-2">
                  <input
                    type="checkbox"
                    aria-label="Select all visible subjects"
                    checked={allVisibleSelected}
                    ref={(el) => {
                      if (el) el.indeterminate = !allVisibleSelected && visible.some((s) => selected.has(s.subject_id));
                    }}
                    onChange={toggleAllVisible}
                    className="rounded-sm"
                  />
                </th>
              )}
              <th scope="col" className="px-3 py-2 font-medium">Subject</th>
              <th scope="col" className="px-3 py-2 font-medium">Active course</th>
              <th scope="col" className="px-3 py-2 font-medium w-72">Change to</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--color-wi-border)]">
            {visible.map((subject) => {
              const active = activeCourseId[subject.subject_id];
              const activeCourse = subject.courses.find((c) => c.course_id === active);
              const missingActive = Boolean(active) && !activeCourse;
              const isSaving = saving[subject.subject_id] ?? false;
              const groups = groupedByCycle(subject.courses);
              return (
                <tr key={subject.subject_id} className="hover:bg-[var(--color-wi-row-alt)]/60">
                  {selectionMode && (
                    <td className="px-3 py-2 align-middle">
                      <input
                        type="checkbox"
                        aria-label={`Select ${subject.subject_code}`}
                        checked={selected.has(subject.subject_id)}
                        onChange={() => toggleSubject(subject.subject_id)}
                        className="rounded-sm"
                      />
                    </td>
                  )}
                  <td className="px-3 py-2">
                    <div className="font-medium text-[var(--color-wi-text)]">
                      {subject.subject_code}
                    </div>
                    <div className="text-xs text-[var(--color-wi-text-light)]">
                      {subject.subject_name}
                    </div>
                  </td>
                  <td className="px-3 py-2">
                    {activeCourse ? (
                      <div className="flex items-center gap-2">
                        <span className="w-1.5 h-1.5 rounded-full bg-[var(--color-wi-green)] inline-block shrink-0" aria-hidden="true" />
                        <span className="font-medium text-[var(--color-wi-text)]">{activeCourse.course_code}</span>
                        <span className="text-xs text-[var(--color-wi-text-light)]">{activeCourse.cycle_label}</span>
                      </div>
                    ) : missingActive ? (
                      <span className="text-xs font-medium text-[var(--color-wi-red)]">
                        Active course no longer exists
                      </span>
                    ) : (
                      <span className="text-xs font-medium text-[var(--color-wi-amber)]">Not set</span>
                    )}
                  </td>
                  <td className="px-3 py-2">
                    {subject.courses.length === 0 ? (
                      <span className="text-xs text-[var(--color-wi-text-light)]">No courses</span>
                    ) : (
                      <Select
                        size="sm"
                        value={activeCourse ? active : ""}
                        disabled={isSaving}
                        aria-label={`Active course for ${subject.subject_code}`}
                        onChange={(e) => {
                          const courseId = e.target.value;
                          if (courseId && courseId !== active) {
                            // Toasts come from onSave; swallow the rejection here.
                            onSave(subject.subject_id, courseId).catch(() => undefined);
                          }
                        }}
                      >
                        {!activeCourse && (
                          <option value="">Set active course…</option>
                        )}
                        {groups.map(([label, courses]) => (
                          <optgroup key={label} label={label}>
                            {courses.map((course) => (
                              <option key={course.course_id} value={course.course_id}>
                                {course.course_code}
                              </option>
                            ))}
                          </optgroup>
                        ))}
                      </Select>
                    )}
                  </td>
                </tr>
              );
            })}
            {visible.length === 0 && (
              <tr>
                <td colSpan={selectionMode ? 4 : 3} className="px-3 py-10 text-center text-sm text-[var(--color-wi-text-light)]">
                  {subjects.length === 0
                    ? "No subjects yet. Subjects appear once courses are assigned to them."
                    : !showUnassigned && stats.set === 0
                      ? "No active courses are set yet. Show unassigned subjects to choose the first one."
                      : "No active courses match this search."}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {bulkOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--color-wi-nav)]/40 p-4"
          role="dialog"
          aria-modal="true"
          aria-label={`Change active course${bulkTargets.length === 1 ? "" : "s"}`}
          ref={dialogRef}
          onMouseDown={(e) => {
            if (e.target === dialogRef.current && bulkState !== "running") setBulkOpen(false);
          }}
        >
          <div className="w-full max-w-lg rounded-lg border border-[var(--color-wi-border)] bg-white shadow-lg">
            <div className="border-b border-[var(--color-wi-border)] px-4 py-3">
              <h3 className="text-sm font-semibold text-[var(--color-wi-text)]">
                Change active course{bulkTargets.length === 1 ? "" : "s"}
              </h3>
              <p className="text-xs text-[var(--color-wi-text-light)]">
                Each subject moves to its own active course. Subjects without courses are skipped.
              </p>
            </div>
            <div className="max-h-80 overflow-y-auto px-4 py-3 space-y-3">
              {bulkTargets.map((subject, index) => (
                <div key={subject.subject_id} className="flex items-center gap-3">
                  <div className="w-36 shrink-0">
                    <div className="text-sm font-medium text-[var(--color-wi-text)] truncate">
                      {subject.subject_code}
                    </div>
                    <div className="text-xs text-[var(--color-wi-text-light)] truncate">
                      Currently{" "}
                      {activeCourseId[subject.subject_id]
                        ? subject.courses.find(
                            (c) => c.course_id === activeCourseId[subject.subject_id],
                          )?.course_code ?? "—"
                        : "not set"}
                    </div>
                  </div>
                  <div className="flex-1 min-w-0">
                    <Select
                      size="sm"
                      ref={index === 0 ? firstSelectRef : undefined}
                      value={bulkChoices[subject.subject_id] ?? ""}
                      disabled={bulkState === "running"}
                      aria-label={`New active course for ${subject.subject_code}`}
                      onChange={(e) =>
                        setBulkChoices((prev) => ({
                          ...prev,
                          [subject.subject_id]: e.target.value,
                        }))
                      }
                    >
                      {groupedByCycle(subject.courses).map(([label, courses]) => (
                        <optgroup key={label} label={label}>
                          {courses.map((course) => (
                            <option key={course.course_id} value={course.course_id}>
                              {course.course_code}
                            </option>
                          ))}
                        </optgroup>
                      ))}
                    </Select>
                    {bulkErrors[subject.subject_id] && (
                      <p className="mt-1 text-xs text-[var(--color-wi-red)]" role="alert">
                        {bulkErrors[subject.subject_id]}
                      </p>
                    )}
                  </div>
                </div>
              ))}
              {bulkTargets.length === 0 && (
                <p className="text-sm text-[var(--color-wi-text-light)]">
                  None of the selected subjects have courses to activate.
                </p>
              )}
            </div>
            <div className="flex items-center justify-between gap-3 border-t border-[var(--color-wi-border)] px-4 py-3">
              <div className="text-xs">
                {bulkState === "running" && (
                  <span className="text-[var(--color-wi-text-light)]">Saving…</span>
                )}
                {bulkState === "done" && bulkDone && (
                  <span
                    className={
                      bulkDone.failed > 0
                        ? "text-[var(--color-wi-amber)]"
                        : "text-[var(--color-wi-green)]"
                    }
                  >
                    {bulkDone.applied} updated
                    {bulkDone.failed > 0 ? `, ${bulkDone.failed} failed` : ""}
                  </span>
                )}
                {bulkState === "idle" && (
                  <span className="text-[var(--color-wi-text-light)]">
                    {bulkTargets.length} subject{bulkTargets.length === 1 ? "" : "s"} to update
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setBulkOpen(false)}
                  disabled={bulkState === "running"}
                >
                  Close
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => void runBulk()}
                  loading={bulkState === "running"}
                  disabled={bulkState === "running" || bulkTargets.length === 0}
                >
                  Apply to {bulkTargets.length} subject{bulkTargets.length === 1 ? "" : "s"}
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
