import { useEffect, useMemo, useState } from "react";
import { apiJson, findAvailableSlots, type SlotFinderSlot } from "../api/client";
import { useToast } from "../hooks/useToast";
import PageHeading from "../components/ui/PageHeading";
import EmptyState from "../components/ui/EmptyState";
import Button from "../components/ui/Button";

type Student = { id: string; wcode: string; full_name: string };
type Course = { id: string; code: string; name: string };

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
      return { label: kind?.replace(/_/g, " ") ?? "Unknown conflict", icon: "⚠️", color: "text-gray-700" };
  }
}

function yyyyMmDd(d: Date) {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export default function SlotFinder() {
  const { addToast } = useToast();
  const today = useMemo(() => new Date(), []);

  const [students, setStudents] = useState<Student[]>([]);
  const [courses, setCourses] = useState<Course[]>([]);
  const [studentId, setStudentId] = useState("");
  const [courseId, setCourseId] = useState("");
  const [startDate, setStartDate] = useState(yyyyMmDd(today));
  const [endDate, setEndDate] = useState(yyyyMmDd(new Date(today.getTime() + 7 * 24 * 60 * 60 * 1000)));
  const [slots, setSlots] = useState<SlotFinderSlot[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);
  const [expandedSlots, setExpandedSlots] = useState<Set<string>>(new Set());

  const selectedStudent = useMemo(
    () => students.find((s) => s.id === studentId),
    [students, studentId]
  );
  const selectedCourse = useMemo(
    () => courses.find((c) => c.id === courseId),
    [courses, courseId]
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

  const loadLookups = async () => {
    try {
      const [s, c] = await Promise.all([
        apiJson<Student[]>("/api/v1/students", { method: "GET" }),
        apiJson<Course[]>("/api/v1/courses", { method: "GET" }),
      ]);
      setStudents(s);
      setCourses(c);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load lookup data");
    }
  };

  useEffect(() => {
    void loadLookups();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const doSearch = async () => {
    if (!studentId || !courseId) {
      addToast("error", "Select a student and a course");
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
        course_id: courseId,
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
      <p className="text-sm text-gray-500 mb-4">
        Find available time slots for adding a student to a course. Slots show whether the student, course roster,
        and teacher are all free.
      </p>

      {/* Search Form */}
      <div className="bg-gray-50 border border-gray-200 rounded-sm p-4 mb-6">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-3 items-end">
          <div>
            <label className="block text-xs text-gray-500 mb-1">Student</label>
            <select
              value={studentId}
              onChange={(e) => setStudentId(e.target.value)}
              className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
            >
              <option value="">Select student…</option>
              {students.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.wcode} — {s.full_name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Course</label>
            <select
              value={courseId}
              onChange={(e) => setCourseId(e.target.value)}
              className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
            >
              <option value="">Select course…</option>
              {courses.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.code} — {c.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Start date</label>
            <input
              type="date"
              value={startDate}
              onChange={(e) => setStartDate(e.target.value)}
              className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
            />
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">End date</label>
            <input
              type="date"
              value={endDate}
              onChange={(e) => setEndDate(e.target.value)}
              className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
            />
          </div>
        </div>
        <div className="mt-3 flex justify-end">
          <Button variant="primary" size="md" onClick={doSearch} loading={loading}>
            {loading ? "Scanning…" : "Find Slots"}
          </Button>
        </div>
      </div>

      {/* Results */}
      {loading && (
        <div className="py-8 text-center text-gray-400 text-sm">Scanning available slots…</div>
      )}

      {!loading && searched && slots.length === 0 && (
        <EmptyState message="No slots found in this range. Try a wider date range or different student/course." />
      )}

      {!loading && searched && slots.length > 0 && (
        <div>
          {/* Summary bar */}
          <div className="flex items-center gap-4 mb-4 text-sm">
            <span className="text-gray-600">
              {slots.filter((s) => s.status === "provisional").length} provisional
            </span>
            <span className="text-gray-600">
              {slots.filter((s) => s.status === "blocked").length} blocked
            </span>
            {selectedStudent && selectedCourse && (
              <span className="text-gray-500 ml-auto text-xs">
                {selectedStudent.wcode} → {selectedCourse.code}
              </span>
            )}
          </div>

          {/* Legend */}
          <div className="flex items-center gap-4 mb-4 text-xs text-gray-600">
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
                <div key={dateStr} className="border border-gray-200 rounded-sm overflow-hidden">
                  <div className="bg-gray-50 border-b border-gray-200 px-4 py-2 flex items-center justify-between">
                    <div>
                      <span className="font-semibold text-gray-800">{dateStr}</span>
                      <span className="text-gray-500 text-sm ml-2">{dayName}</span>
                    </div>
                    <div className="text-xs text-gray-500">
                      {provCount > 0 && <span className="text-amber-700 mr-2">{provCount} provisional</span>}
                      {blockedCount > 0 && <span className="text-red-600">{blockedCount} blocked</span>}
                      {provCount === 0 && blockedCount === 0 && <span>No slots</span>}
                    </div>
                  </div>
                  <div className="divide-y divide-gray-100">
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
                              <span className="font-mono text-sm font-medium text-gray-800 min-w-[100px]">
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
                                className="text-xs text-gray-500 hover:text-gray-700 underline underline-offset-2"
                              >
                                {isExpanded ? "Hide details" : "Details"}
                              </button>
                            )}
                          </div>

                          {/* Expanded conflict details */}
                          {isExpanded && isBlocked && slot.conflicts && slot.conflicts.length > 0 && (
                            <div className="mt-2 ml-[100px] bg-red-50 border border-red-200 rounded-sm p-3 text-xs space-y-2">
                              <div className="font-medium text-red-800">
                                {meta.icon} {meta.label}
                              </div>
                              {slot.message && (
                                <div className="text-red-700">{slot.message}</div>
                              )}
                              <div>
                                <div className="font-semibold text-gray-600 mb-1">
                                  Conflicting {slot.conflicts.length === 1 ? "session" : "sessions"}:
                                </div>
                                <ul className="space-y-1.5">
                                  {slot.conflicts.map((c) => {
                                    const startLocal = new Date(c.start_at).toLocaleString("en-GB", {
                                      day: "numeric",
                                      month: "short",
                                      hour: "2-digit",
                                      minute: "2-digit",
                                    });
                                    const endLocal = new Date(c.end_at).toLocaleString("en-GB", {
                                      hour: "2-digit",
                                      minute: "2-digit",
                                    });
                                    return (
                                      <li key={c.session_id} className="text-gray-700 flex items-start gap-2">
                                        <span className="text-gray-400 mt-0.5">•</span>
                                        <span>
                                          <span className="font-medium text-red-700">
                                            {c.course_id.slice(0, 8)}…
                                          </span>
                                          <span className="text-gray-500">
                                            {" "}— {startLocal}–{endLocal}
                                          </span>
                                        </span>
                                      </li>
                                    );
                                  })}
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
