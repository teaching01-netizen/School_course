import { useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { apiJson, findAvailableSlots, type SlotFinderSlot } from "../api/client";
import { useToast } from "../hooks/useToast";
import PageHeading from "../components/ui/PageHeading";
import EmptyState from "../components/ui/EmptyState";
import Button from "../components/ui/Button";
import TypeaheadSelect from "../components/TypeaheadSelect";
import { ConflictContextCard, parseConflictContext } from "../features/scheduling/components/ConflictContextCard";
import { formatTimeRange } from "../features/scheduling/domain/time";
import type { Course } from "../features/courses/types";

type Student = { id: string; wcode: string; full_name: string };
export type Subject = { id: string; code: string; name: string };
/** A course on the student's own list; carries subject code/name for resolution. */
export type StudentCourse = {
  id: string;
  code: string;
  name: string;
  subject_code?: string | null;
  subject_name?: string | null;
};

/**
 * The course of a student that belongs to the chosen subject — that is the
 * course the slot search runs against. Subject codes are the stable join key;
 * the name is a fallback when the code differs.
 */
export function resolveSubjectCourse(studentCourses: StudentCourse[], subject: Subject | null): StudentCourse | null {
  if (!subject) return null;
  const byCode = studentCourses.find((c) => c.subject_code && c.subject_code === subject.code);
  if (byCode) return byCode;
  return studentCourses.find((c) => c.subject_name === subject.name) ?? null;
}

function conflictKindMeta(kind: string | undefined): { label: string; icon: string; color: string } {
  switch (kind) {
    case "room_overlap":
      return { label: "Room already booked", icon: "🏢", color: "text-purple-700" };
    case "teacher_overlap":
      return { label: "Teacher has another session", icon: "👤", color: "text-orange-700" };
    case "student_overlap":
      return { label: "Student scheduling conflict", icon: "📚", color: "text-red-700" };
    case "teacher_availability":
      return { label: "Teacher not available", icon: "⏰", color: "text-amber-700" };
    case "room_availability":
      return { label: "Room not available", icon: "🚫", color: "text-rose-700" };
    default:
      return { label: kind?.replace(/_/g, " ") ?? "Unknown conflict", icon: "⚠️", color: "text-[var(--color-wi-text-light)]" };
  }
}

function yyyyMmDd(d: Date) {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export default function SlotFinder() {
  const { addToast } = useToast();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const today = useMemo(() => new Date(), []);

  // Carried over from the blocked availability strip ("Find alternative slots →").
  const conflictContext = useMemo(() => parseConflictContext(searchParams), [searchParams]);

  const [students, setStudents] = useState<Student[]>([]);
  const [courses, setCourses] = useState<Course[]>([]);
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [studentId, setStudentId] = useState("");
  const [subjectId, setSubjectId] = useState("");
  const [studentCourses, setStudentCourses] = useState<StudentCourse[]>([]);
  const [studentCoursesLoading, setStudentCoursesLoading] = useState(false);
  const [startDate, setStartDate] = useState(() => {
    const requested = searchParams.get("start_at");
    return requested ? yyyyMmDd(new Date(requested)) : yyyyMmDd(today);
  });
  const [endDate, setEndDate] = useState(() => {
    const requested = searchParams.get("start_at");
    return requested
      ? yyyyMmDd(new Date(new Date(requested).getTime() + 6 * 24 * 60 * 60 * 1000))
      : yyyyMmDd(new Date(today.getTime() + 7 * 24 * 60 * 60 * 1000));
  });
  const [slots, setSlots] = useState<SlotFinderSlot[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);
  const [expandedSlots, setExpandedSlots] = useState<Set<string>>(new Set());

  const selectedStudent = useMemo(
    () => students.find((s) => s.id === studentId) ?? null,
    [students, studentId]
  );
  const selectedSubject = useMemo(
    () => subjects.find((s) => s.id === subjectId) ?? null,
    [subjects, subjectId]
  );
  // The single course the search runs against: the student's course in the
  // chosen subject.
  const resolvedCourse = useMemo(
    () => resolveSubjectCourse(studentCourses, selectedSubject),
    [studentCourses, selectedSubject]
  );

  const slotsByDate = useMemo(() => {
    const map = new Map<string, SlotFinderSlot[]>();
    for (const s of slots) {
      const arr = map.get(s.date) ?? [];
      arr.push(s);
      map.set(s.date, arr);
    }
    for (const arr of map.values()) {
      arr.sort((a, b) => a.start_time.localeCompare(b.start_time));
    }
    return map;
  }, [slots]);

  const coursesById = useMemo(() => new Map(courses.map((c) => [c.id, c])), [courses]);

  // Apply the carried-over context once the lookups have loaded, so the search
  // starts ready for the subject (and student) the user was dealing with.
  useEffect(() => {
    if (!conflictContext) return;
    const requested = conflictContext.details.requested;
    if (!studentId && conflictContext.studentId && students.some((s) => s.id === conflictContext.studentId)) {
      setStudentId(conflictContext.studentId);
    }
    if (!subjectId) {
      const carriedCourse = courses.find((c) => c.id === requested.course_id);
      if (carriedCourse?.subject_id && subjects.some((s) => s.id === carriedCourse.subject_id)) {
        setSubjectId(carriedCourse.subject_id);
      }
    }
  }, [conflictContext, courses, subjects, students, studentId, subjectId]);

  // The student's courses drive the subject → course resolution. A stale
  // response from a previously selected student must not overwrite the new one.
  useEffect(() => {
    if (!studentId) {
      setStudentCourses([]);
      setStudentCoursesLoading(false);
      return;
    }
    let stale = false;
    setStudentCoursesLoading(true);
    apiJson<StudentCourse[]>(`/api/v1/students/${encodeURIComponent(studentId)}/courses`, { method: "GET" })
      .then((items) => {
        if (stale) return;
        setStudentCourses(items);
        setStudentCoursesLoading(false);
      })
      .catch((err) => {
        if (stale) return;
        setStudentCourses([]);
        setStudentCoursesLoading(false);
        addToast("error", err instanceof Error ? err.message : "Failed to load the student's courses");
      });
    return () => {
      stale = true;
    };
  }, [studentId, addToast]);

  const loadLookups = async () => {
    try {
      // The students list endpoint always returns the paginated envelope
      // ({ items, total_count, offset, limit }), never a bare array — ask
      // for more than the default 50 so the searchable dropdown covers
      // realistic rosters.
      const [studentsRes, c, subjectsRes] = await Promise.all([
        apiJson<{ items: Student[] }>("/api/v1/students?limit=200", { method: "GET" }),
        apiJson<Course[]>("/api/v1/courses", { method: "GET" }),
        apiJson<Subject[]>("/api/v1/subjects", { method: "GET" }),
      ]);
      setStudents(studentsRes.items);
      setCourses(c);
      setSubjects(subjectsRes);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load lookup data");
    }
  };

  useEffect(() => {
    void loadLookups();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const doSearch = async () => {
    if (!studentId || !selectedSubject || !resolvedCourse) {
      addToast("error", "Select a student and a subject with a course to search");
      return;
    }
    if (!startDate || !endDate) {
      addToast("error", "Select a date range");
      return;
    }
    if (endDate < startDate) {
      addToast("error", "End date must be on or after start date");
      return;
    }
    setLoading(true);
    setSearched(true);
    setExpandedSlots(new Set());
    try {
      const res = await findAvailableSlots({
        student_id: studentId,
        course_id: resolvedCourse.id,
        start_date: startDate,
        end_date: endDate,
        slot_duration_minutes: 60,
        day_start_hour: 8,
        day_end_hour: 20,
      });
      setSlots(res.slots);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Search failed");
      setSlots([]);
    } finally {
      setLoading(false);
    }
  };

  const toggleExpanded = (slotKey: string) => {
    setExpandedSlots((prev) => {
      const next = new Set(prev);
      if (next.has(slotKey)) {
        next.delete(slotKey);
      } else {
        next.add(slotKey);
      }
      return next;
    });
  };

  const daysInRange = useMemo(() => {
    if (!startDate || !endDate) return [];
    const start = new Date(`${startDate}T00:00:00`);
    const end = new Date(`${endDate}T00:00:00`);
    if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) return [];
    const out: string[] = [];
    for (let d = new Date(start); d <= end && out.length < 14; d = new Date(d.getTime() + 24 * 60 * 60 * 1000)) {
      out.push(yyyyMmDd(d));
    }
    return out;
  }, [startDate, endDate]);

  return (
    <div>
      <PageHeading>Slot Finder</PageHeading>
      <p className="text-sm text-[var(--color-wi-text-light)] mb-4">
        Find available time slots for adding a student to a course. Slots show whether the student, course roster,
        and teacher are all free.
      </p>

      {/* Reason the user arrived: carried from the blocked availability strip. */}
      {conflictContext && (
        <div className="mb-4">
          <ConflictContextCard
            context={conflictContext}
            coursesById={coursesById}
            onDismiss={() => navigate("/slot-finder", { replace: true })}
          />
        </div>
      )}

      {/* Search Form */}
      <div className="bg-[var(--color-wi-row-alt)] border var(--color-wi-line) rounded-sm p-4 mb-6">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-3 items-end">
          <div>
            <label htmlFor="slot-finder-student" className="block text-xs text-[var(--color-wi-text-light)] mb-1">Student</label>
            <TypeaheadSelect
              id="slot-finder-student"
              value={studentId}
              onChange={setStudentId}
              options={students.map((s) => ({
                value: s.id,
                label: `${s.wcode} — ${s.full_name}`,
                keywords: `${s.wcode} ${s.full_name}`,
              }))}
              placeholder="Search student…"
            />
          </div>
          <div>
            <label htmlFor="slot-finder-subject" className="block text-xs text-[var(--color-wi-text-light)] mb-1">Subject</label>
            <TypeaheadSelect
              id="slot-finder-subject"
              value={subjectId}
              onChange={setSubjectId}
              options={subjects.map((s) => ({
                value: s.id,
                label: `${s.code} — ${s.name}`,
                keywords: `${s.code} ${s.name}`,
              }))}
              placeholder="Search subject…"
            />
          </div>
          <div>
            <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">
              Start date
              <input
                type="date"
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
                className="mt-1 w-full px-2 py-1.5 text-sm border var(--color-wi-line) rounded-sm"
              />
            </label>
          </div>
          <div>
            <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">
              End date
              <input
                type="date"
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
                className="mt-1 w-full px-2 py-1.5 text-sm border var(--color-wi-line) rounded-sm"
              />
            </label>
          </div>
        </div>

        {/* The resolved course — what the search actually runs against. */}
        {selectedSubject && (
          <div className="mt-3 text-sm">
            {resolvedCourse ? (
              <p className="text-[var(--color-wi-text-light)]">
                {`Searching ${resolvedCourse.code} · ${resolvedCourse.name}`}
              </p>
            ) : (
              <p className="text-[var(--color-wi-text-light)]">
                {!selectedStudent
                  ? "Pick a student to find their course in this subject"
                  : studentCoursesLoading
                    ? "Loading the student's courses…"
                    : `No ${selectedSubject.code} course on this student's list`}
              </p>
            )}
          </div>
        )}

        <div className="mt-3 flex justify-end">
          <Button
            variant="primary"
            size="md"
            onClick={doSearch}
            loading={loading}
            disabled={!selectedStudent || !selectedSubject || !resolvedCourse || loading}
          >
            {loading ? "Scanning…" : "Find Slots"}
          </Button>
        </div>
      </div>

      {/* Results */}
      {loading && (
        <div className="py-8 text-center text-[var(--color-wi-text-light)] text-sm">Scanning available slots…</div>
      )}

      {!loading && searched && slots.length === 0 && (
        <EmptyState message="No slots found in this range. Try a wider date range or different student/course." />
      )}

      {!loading && searched && slots.length > 0 && (
        <div>
          {/* Summary bar */}
          <div className="flex items-center gap-4 mb-4 text-sm">
            <span className="text-[var(--color-wi-text-light)]">
              {slots.filter((s) => s.status === "provisional").length} provisional
            </span>
            <span className="text-[var(--color-wi-text-light)]">
              {slots.filter((s) => s.status === "blocked").length} blocked
            </span>
            {selectedStudent && resolvedCourse && (
              <span className="text-[var(--color-wi-text-light)] ml-auto text-xs">
                {selectedStudent.wcode} → {resolvedCourse.code}
              </span>
            )}
          </div>

          {/* Legend */}
          <div className="flex items-center gap-4 mb-4 text-xs text-[var(--color-wi-text-light)]">
            <span className="flex items-center gap-1">
              <span className="inline-block w-3 h-3 rounded-sm bg-amber-100 border border-amber-300" />
              Provisional — No room assigned
            </span>
            <span className="flex items-center gap-1">
              <span className="inline-block w-3 h-3 rounded-sm bg-red-100 border border-red-300" />
              Blocked — Conflict
            </span>
          </div>

          {/* Results by day */}
          <div className="space-y-4">
            {daysInRange.map((dateStr) => {
              const daySlots = slotsByDate.get(dateStr) ?? [];
              if (daySlots.length === 0) return null;
              const dayDate = new Date(`${dateStr}T00:00:00`);
              const dayName = dayDate.toLocaleDateString("en-GB", {
                weekday: "long",
              });
              const provCount = daySlots.filter((s) => s.status === "provisional").length;
              const blockedCount = daySlots.filter((s) => s.status === "blocked").length;

              return (
                <div key={dateStr} className="border var(--color-wi-line) rounded-sm overflow-hidden">
                  <div className="bg-[var(--color-wi-row-alt)] border-b var(--color-wi-line) px-4 py-2 flex items-center justify-between">
                    <div>
                      <span className="font-semibold text-[var(--color-wi-text)]">{dateStr}</span>
                      <span className="text-[var(--color-wi-text-light)] text-sm ml-2">{dayName}</span>
                    </div>
                    <div className="text-xs text-[var(--color-wi-text-light)]">
                      {provCount > 0 && <span className="text-amber-700 mr-2">{provCount} provisional</span>}
                      {blockedCount > 0 && <span className="text-red-600">{blockedCount} blocked</span>}
                      {provCount === 0 && blockedCount === 0 && <span>No slots</span>}
                    </div>
                  </div>
                  <div className="divide-y divide-wi-line">
                    {daySlots.map((slot) => {
                      const slotKey = `${slot.date}_${slot.start_time}`;
                      const isBlocked = slot.status === "blocked";
                      const meta = conflictKindMeta(slot.kind);
                      const isExpanded = expandedSlots.has(slotKey);

                      return (
                        <div
                          key={slotKey}
                          className={`px-4 py-2.5 ${isBlocked ? "hover:bg-red-50/40" : "hover:bg-amber-50/40"} transition-colors`}
                        >
                          <div className="flex items-center justify-between">
                            <div className="flex items-center gap-3">
                              {/* Time */}
                              <span className="font-mono text-sm font-medium text-[var(--color-wi-text)] min-w-[100px]">
                                {slot.start_time}–{slot.end_time}
                              </span>
                              {/* Status badge */}
                              {isBlocked ? (
                                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-sm text-xs font-medium bg-red-100 text-red-800 border border-red-200">
                                  🚫 Blocked
                                </span>
                              ) : (
                                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-sm text-xs font-medium bg-amber-100 text-amber-800 border border-amber-200">
                                  ⏳ Provisional
                                </span>
                              )}
                              {/* Conflict kind label */}
                              {isBlocked && slot.kind && (
                                <span className={`text-xs ${meta.color}`}>
                                  {meta.icon} {meta.label}
                                </span>
                              )}
                            </div>
                            {/* Expand button for blocked slots */}
                            {isBlocked && slot.conflicts && slot.conflicts.length > 0 && (
                              <button
                                onClick={() => toggleExpanded(slotKey)}
                                className="text-xs text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text-light)] underline underline-offset-2"
                              >
                                {isExpanded ? "Hide details" : "Details"}
                              </button>
                            )}
                          </div>

                          {/* Expanded conflict details */}
                          {isExpanded && isBlocked && slot.conflicts && slot.conflicts.length > 0 && (
                            <div className="mt-2 ml-[100px] bg-red-50 border border-red-200 rounded-sm p-3 space-y-2">
                              <div className="font-medium text-red-800">
                                {meta.icon} {meta.label}
                              </div>
                              {slot.message && (
                                <div className="text-red-700">{slot.message}</div>
                              )}
                              <div>
                                <div className="font-semibold text-[var(--color-wi-text-light)] mb-1">
                                  Conflicting {slot.conflicts.length === 1 ? "session" : "sessions"}:
                                </div>
                                <ul className="space-y-1.5">
                                  {slot.conflicts.map((c) => (
                                    <li key={c.session_id} className="rounded-[4px] border border-red-100 bg-white/70 px-2 py-1">
                                      <div className="flex items-baseline justify-between gap-2">
                                        <span className="truncate text-xs font-semibold text-[var(--color-wi-red)]">
                                          {coursesById.get(c.course_id)?.code ?? `${c.course_id.slice(0, 8)}…`}
                                        </span>
                                        <span className="shrink-0 font-mono text-[11px] text-[var(--color-wi-text-light)]">
                                          {formatTimeRange(c.start_at, c.end_at)}
                                        </span>
                                      </div>
                                      <p className="mt-0.5 truncate text-xs text-[var(--color-wi-text-light)]">
                                        {c.teacher_id.slice(0, 8)}…{c.room_id ? <> · {c.room_id.slice(0, 8)}…</> : null}
                                      </p>
                                    </li>
                                  ))}
                                </ul>
                              </div>
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
