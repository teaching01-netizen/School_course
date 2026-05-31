import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { CheckCircle, Clock, RotateCcw, XCircle, PenLine } from "lucide-react";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import type { Course, ManagedAbsence } from "../types";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";
import Modal from "../components/Modal";

type OverrideMethod = "auto" | "zoom" | "physical";
type CandidateSession = {
  id: string;
  start_at: string;
  end_at: string;
  room_name?: string;
  capacity_warning?: boolean;
};

function displayDate(value: string): string {
  return new Date(value + "T00:00:00").toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
}

function displayDateTime(value: string): string {
  return new Date(value).toLocaleString("en-GB", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
}

function displayDateFromDateTime(value: string): string {
  return new Date(value).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
}

function titleCase(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1).replace(/_/g, " ");
}

function daysBetween(a: string, b: string): number {
  const d1 = new Date(a + "T00:00:00");
  const d2 = new Date(b + "T00:00:00");
  return Math.round((d2.getTime() - d1.getTime()) / (1000 * 60 * 60 * 24)) + 1;
}

function displayAbsenceDates(absence: ManagedAbsence): string {
  const sitInDateLabels = [...new Set((absence.sit_ins ?? []).map((session) => displayDateFromDateTime(session.start_at)))];
  if (sitInDateLabels.length === 1) {
    return sitInDateLabels[0];
  }
  return `${displayDate(absence.date_from)} - ${displayDate(absence.date_to)} (${daysBetween(absence.date_from, absence.date_to)} days)`;
}

function displaySitInPlanLabel(absence: ManagedAbsence): string {
  if (absence.sit_in_method === "zoom") {
    return "Zoom";
  }
  return absence.sit_in_subject_name ?? absence.subject_name ?? absence.subject_code ?? absence.sit_in_course_name ?? absence.sit_in_course_code ?? "Not assigned";
}

function displayAbsenceReason(absence: ManagedAbsence): string {
  const category = absence.reason_category ? titleCase(absence.reason_category) : "";
  const reason = absence.reason?.trim() ?? "";
  if (category && reason) {
    return `${category} - ${reason}`;
  }
  return category || reason || "-";
}

function TimelineIcon({ action }: { action: string }) {
  switch (action) {
    case "submitted":
    case "created":
      return <Clock className="h-4 w-4 text-blue-500" />;
    case "reviewed":
      return <CheckCircle className="h-4 w-4 text-emerald-500" />;
    case "actioned":
      return <CheckCircle className="h-4 w-4 text-slate-500" />;
    case "cancelled":
      return <XCircle className="h-4 w-4 text-red-500" />;
    case "overridden":
      return <RotateCcw className="h-4 w-4 text-amber-500" />;
    default:
      return <Clock className="h-4 w-4 text-gray-400" />;
  }
}

export default function AbsenceDetail() {
  const { id = "" } = useParams();
  const { addToast } = useToast();
  const [absence, setAbsence] = useState<ManagedAbsence | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [notes, setNotes] = useState("");
  const [notesDirty, setNotesDirty] = useState(false);
  const [cancelOpen, setCancelOpen] = useState(false);
  const [cancelReason, setCancelReason] = useState("");
  const [overrideOpen, setOverrideOpen] = useState(false);
  const [overrideMethod, setOverrideMethod] = useState<OverrideMethod>("auto");
  const [overrideReason, setOverrideReason] = useState("");
  const [courses, setCourses] = useState<Course[]>([]);
  const [courseID, setCourseID] = useState("");
  const [candidates, setCandidates] = useState<CandidateSession[]>([]);
  const [selectedSessions, setSelectedSessions] = useState<Set<string>>(() => new Set());

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = await apiJson<ManagedAbsence>(`/api/v1/absences/${id}`, { method: "GET" });
      setAbsence(result);
      setNotes(result.admin_notes ?? "");
      setNotesDirty(false);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load absence");
    } finally {
      setLoading(false);
    }
  }, [addToast, id]);

  useEffect(() => { void load(); }, [load]);

  useEffect(() => {
    if (!overrideOpen || overrideMethod !== "physical" || !courseID) return;
    void apiJson<CandidateSession[]>(`/api/v1/absences/${id}/sit-in-candidates?course_id=${encodeURIComponent(courseID)}`, { method: "GET" })
      .then((rows) => { setCandidates(rows); setSelectedSessions(new Set(rows.map((r) => r.id))); })
      .catch((err: unknown) => addToast("error", err instanceof Error ? err.message : "Failed to load sessions"));
  }, [addToast, courseID, id, overrideMethod, overrideOpen]);

  async function updateStatus(status: "reviewed" | "actioned" | "pending" | "cancelled", reason?: string) {
    if (!absence) return;
    setSaving(true);
    try {
      await apiJson(`/api/v1/absences/${absence.id}/status`, {
        method: "PUT",
        body: JSON.stringify({ status, expected_version: absence.version, ...(reason ? { reason } : {}) }),
      });
      addToast("success", `Absence ${status === "pending" ? "reopened" : status}`);
      await load();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Status update failed");
    } finally {
      setSaving(false);
    }
  }

  async function saveNote() {
    if (!absence) return;
    setSaving(true);
    try {
      await apiJson(`/api/v1/absences/${absence.id}/notes`, {
        method: "PUT",
        body: JSON.stringify({ notes: notes.trim(), expected_version: absence.version }),
      });
      addToast("success", "Note saved");
      setNotesDirty(false);
      await load();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Note update failed");
    } finally {
      setSaving(false);
    }
  }

  async function openOverride() {
    setOverrideMethod(absence?.sit_in_method === "zoom" ? "zoom" : absence?.sit_in_method === "physical" ? "physical" : "auto");
    setOverrideReason("");
    setCourseID(absence?.sit_in_course_id ?? "");
    setCandidates([]);
    setSelectedSessions(new Set());
    setOverrideOpen(true);
    try {
      setCourses(await apiJson<Course[]>("/api/v1/courses/public", { method: "GET" }));
    } catch { setCourses([]); }
  }

  async function saveOverride() {
    if (!absence || !overrideReason.trim()) return;
    setSaving(true);
    try {
      await apiJson(`/api/v1/absences/${absence.id}/sit-in`, {
        method: "PUT",
        body: JSON.stringify({
          method: overrideMethod,
          expected_version: absence.version,
          reason: overrideReason.trim(),
          ...(overrideMethod === "physical" ? { sit_in_course_id: courseID, sit_in_session_ids: [...selectedSessions] } : {}),
        }),
      });
      setOverrideOpen(false);
      addToast("success", "Sit-in updated");
      await load();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Override failed");
    } finally {
      setSaving(false);
    }
  }

  const statusClasses = useMemo(() => {
    switch (absence?.status) {
      case "pending": return "bg-blue-50 text-blue-700";
      case "reviewed": return "bg-emerald-50 text-emerald-700";
      case "cancelled": return "bg-red-50 text-red-700";
      default: return "bg-slate-100 text-slate-700";
    }
  }, [absence?.status]);

  if (loading && !absence) return <LoadingSkeleton type="table" lines={6} />;
  if (!absence) return <p className="text-sm text-gray-500">Absence could not be loaded.</p>;

  return (
    <div className="mx-auto max-w-4xl">
      <Link className="text-sm text-[var(--color-wi-primary)] hover:underline" to="/absences">Back to Absences</Link>

      <div className="mt-2 flex items-center gap-3">
        <Link className="text-xs text-gray-500 hover:text-gray-700" to="/absences/calendar">View on Calendar</Link>
        <Link className="text-xs text-gray-500 hover:text-gray-700" to="/slot-finder">Find Alternative Slots</Link>
      </div>

      <div className="mt-2 flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-gray-900">Absence Detail</h1>
          <div className="mt-1 flex items-center gap-2 text-sm text-gray-600">
            <span className="font-medium">{absence.student_name ?? "Unknown"}</span>
            <span className="font-mono text-xs text-gray-400">{absence.wcode}</span>
          </div>
        </div>
        <div className="sticky top-4 z-20 hidden md:flex flex-wrap items-center gap-2 rounded-sm border border-gray-200 bg-white p-3 shadow-sm md:flex-nowrap">
          <span className={`rounded-full px-3 py-1 text-xs font-medium ${statusClasses}`}>{titleCase(absence.status)}</span>
          {absence.status === "pending" ? <Button size="sm" loading={saving} onClick={() => void updateStatus("reviewed")}>Mark Reviewed</Button> : null}
          {absence.status === "reviewed" ? (
            <>
              <Button size="sm" loading={saving} onClick={() => void updateStatus("actioned")}>Actioned</Button>
              <Button size="sm" variant="secondary" loading={saving} onClick={() => void updateStatus("pending")}>Reopen</Button>
            </>
          ) : null}
          {absence.status !== "cancelled" && absence.status !== "actioned" ? (
            <Button size="sm" variant="danger" onClick={() => setCancelOpen(true)}>Cancel</Button>
          ) : null}
          <Button size="sm" variant="secondary" onClick={() => void openOverride()}>Override Sit-in</Button>
        </div>
      </div>

      <div className="fixed bottom-0 left-0 right-0 z-20 border-t border-gray-200 bg-white p-3 shadow-lg md:hidden">
        <div className="flex flex-wrap items-center gap-2">
          <span className={`rounded-full px-3 py-1 text-xs font-medium ${statusClasses}`}>{titleCase(absence.status)}</span>
          {absence.status === "pending" ? <Button size="sm" loading={saving} onClick={() => void updateStatus("reviewed")}>Mark Reviewed</Button> : null}
          {absence.status === "reviewed" ? (
            <>
              <Button size="sm" loading={saving} onClick={() => void updateStatus("actioned")}>Actioned</Button>
              <Button size="sm" variant="secondary" loading={saving} onClick={() => void updateStatus("pending")}>Reopen</Button>
            </>
          ) : null}
          {absence.status !== "cancelled" && absence.status !== "actioned" ? (
            <Button size="sm" variant="danger" onClick={() => setCancelOpen(true)}>Cancel</Button>
          ) : null}
          <Button size="sm" variant="secondary" onClick={() => void openOverride()}>Override Sit-in</Button>
        </div>
      </div>

      <div className="mt-4 grid gap-4 pb-16 md:pb-0">
        <section className="rounded-sm border border-gray-200 bg-white">
          <h2 className="border-b border-gray-100 bg-gray-50/70 px-4 py-3 text-sm font-semibold text-gray-800">Absence Summary</h2>
          <div className="grid gap-4 p-4 md:grid-cols-2">
            <dl className="grid grid-cols-[120px_1fr] gap-y-2 text-sm">
              <dt className="text-gray-500">Subject</dt>
              <dd>{absence.subject_code ?? "-"} {absence.subject_name ? `- ${absence.subject_name}` : ""}</dd>
              <dt className="text-gray-500">Dates</dt>
              <dd>{displayAbsenceDates(absence)}</dd>
              <dt className="text-gray-500">Reason</dt>
              <dd>{displayAbsenceReason(absence)}</dd>
              <dt className="text-gray-500">Submitted</dt>
              <dd>{displayDateTime(absence.created_at)}</dd>
            </dl>
          </div>
        </section>

        <section className="rounded-sm border border-gray-200 bg-white">
          <h2 className="border-b border-gray-100 bg-gray-50/70 px-4 py-3 text-sm font-semibold text-gray-800">Sit-in Plan</h2>
          <div className="p-4">
            <div className="mb-3 flex items-center gap-2">
              <span className="rounded-sm bg-blue-50 px-2 py-1 text-sm font-medium text-blue-700">
                {displaySitInPlanLabel(absence)}
              </span>
              {absence.sit_in_rule_name ? <span className="rounded-sm bg-blue-50 px-2 py-0.5 text-xs text-blue-700">{absence.sit_in_rule_name}</span> : null}
              {absence.sit_in_overridden ? <span className="rounded-sm bg-amber-50 px-2 py-0.5 text-xs text-amber-700">Overridden</span> : null}
            </div>
            {absence.sit_ins?.length ? (
              <div className="space-y-2">
                {absence.sit_ins.map((session) => (
                  <div key={session.id} className="flex items-center justify-between rounded-sm border border-gray-100 bg-gray-50 px-3 py-2 text-sm">
                    <span>{displayDateTime(session.start_at)} &ndash; {new Date(session.end_at).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" })}</span>
                    <span className="text-gray-500">{session.room_name ?? "No room"}</span>
                  </div>
                ))}
              </div>
            ) : <p className="text-sm text-gray-500">No physical sit-in sessions assigned.</p>}
            {absence.sit_in_method === "zoom" ? (
              <p className="mt-2 text-sm text-gray-500">Student attends via Zoom &mdash; no physical class required.</p>
            ) : null}
            {absence.sit_in_method === "physical" && !absence.sit_ins?.length ? (
              <p className="mt-2 text-sm text-gray-500">No sessions assigned yet.</p>
            ) : null}
          </div>
        </section>

        <div className="grid gap-4 md:grid-cols-2">
          <section className="rounded-sm border border-gray-200 bg-white">
            <h2 className="border-b border-gray-100 bg-gray-50/70 px-4 py-3 text-sm font-semibold text-gray-800">Admin Notes</h2>
            <div className="p-4">
              <label className="sr-only" htmlFor="detail-note">Internal note</label>
              <textarea
                id="detail-note"
                value={notes}
                onChange={(e) => { setNotes(e.target.value); setNotesDirty(true); }}
                rows={5}
                className="w-full rounded-sm border border-gray-300 p-2 text-sm"
                placeholder="Visible to staff only..."
              />
              <div className="mt-3 flex items-center justify-between">
                {notesDirty ? <span className="text-xs text-amber-600">Unsaved changes</span> : <span />}
                <Button size="sm" disabled={!notesDirty} loading={saving} onClick={() => void saveNote()}>
                  <PenLine className="mr-1 h-3.5 w-3.5" /> Save Note
                </Button>
              </div>
            </div>
          </section>

          <section className="rounded-sm border border-gray-200 bg-white">
            <h2 className="border-b border-gray-100 bg-gray-50/70 px-4 py-3 text-sm font-semibold text-gray-800">Timeline</h2>
            <div className="p-4">
              <ol className="space-y-3">
                {(absence.timeline ?? []).map((entry) => (
                  <li key={entry.id} className="flex gap-3">
                    <div className="mt-0.5 shrink-0">{TimelineIcon({ action: entry.action })}</div>
                    <div>
                      <p className="text-sm font-medium text-gray-800">{titleCase(entry.action)}</p>
                      <p className="text-xs text-gray-500">{displayDateTime(entry.created_at)} &mdash; {entry.actor_name ?? entry.actor_role}</p>
                    </div>
                  </li>
                ))}
                {!absence.timeline?.length ? <li className="text-sm text-gray-500">No activity recorded.</li> : null}
              </ol>
            </div>
          </section>
        </div>
      </div>

      {cancelOpen ? (
        <Modal title="Cancel absence" onClose={() => setCancelOpen(false)}
          footer={<><Button variant="secondary" onClick={() => setCancelOpen(false)}>Back</Button><Button variant="danger" disabled={!cancelReason.trim()} loading={saving} onClick={() => void updateStatus("cancelled", cancelReason.trim()).then(() => setCancelOpen(false))}>Cancel Absence</Button></>}>
          <label className="block text-sm font-medium text-gray-700" htmlFor="detail-cancel-reason">Reason</label>
          <textarea id="detail-cancel-reason" className="mt-2 w-full rounded-sm border border-gray-300 p-2" rows={3} value={cancelReason} onChange={(e) => setCancelReason(e.target.value)} />
        </Modal>
      ) : null}

      {overrideOpen ? (
        <Modal title="Override Sit-in" onClose={() => setOverrideOpen(false)} size="lg"
          footer={<><Button variant="secondary" onClick={() => setOverrideOpen(false)}>Cancel</Button><Button disabled={!overrideReason.trim() || (overrideMethod === "physical" && (!courseID || selectedSessions.size === 0))} loading={saving} onClick={() => void saveOverride()}>Save Override</Button></>}>
          <p className="mb-4 text-sm text-gray-600">Current: {absence.sit_in_course_code ?? absence.sit_in_method ?? "Not assigned"}</p>
          <div className="border-b border-gray-200">
            <div className="flex gap-0">
              {(["auto", "zoom", "physical"] as OverrideMethod[]).map((method) => (
                <button
                  key={method}
                  onClick={() => setOverrideMethod(method)}
                  className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px ${
                    overrideMethod === method
                      ? "border-[var(--color-wi-primary)] text-[var(--color-wi-primary)]"
                      : "border-transparent text-gray-500 hover:text-gray-700"
                  }`}
                >
                  {method === "auto" ? "Auto-resolve" : method === "zoom" ? "Zoom" : "Manual course"}
                </button>
              ))}
            </div>
          </div>
          <div className="mt-4">
            {overrideMethod === "physical" ? (
              <>
                <label className="block text-sm font-medium" htmlFor="sit-in-course">Course</label>
                <select id="sit-in-course" className="mt-1 w-full" value={courseID} onChange={(e) => setCourseID(e.target.value)}>
                  <option value="">Select a course</option>
                  {courses.map((course) => <option key={course.id} value={course.id}>{course.code} - {course.name}</option>)}
                </select>
                <div className="mt-3 space-y-2">
                  {candidates.map((candidate) => (
                    <label key={candidate.id} className="flex items-center gap-2 text-sm">
                      <input type="checkbox" checked={selectedSessions.has(candidate.id)} onChange={(e) => setSelectedSessions((prev) => {
                        const next = new Set(prev);
                        if (e.target.checked) next.add(candidate.id); else next.delete(candidate.id);
                        return next;
                      })} />
                      <span>{displayDateTime(candidate.start_at)}{candidate.room_name ? ` - ${candidate.room_name}` : ""}</span>
                      {candidate.capacity_warning ? <span className="text-xs font-medium text-amber-700">Near capacity</span> : null}
                    </label>
                  ))}
                </div>
              </>
            ) : overrideMethod === "zoom" ? (
              <p className="text-sm text-gray-600">Student will attend a Zoom session instead of physical class.</p>
            ) : (
              <p className="text-sm text-gray-600">Automatically resolve based on course level and availability.</p>
            )}
          </div>
          <label className="mt-4 block text-sm font-medium" htmlFor="override-reason">Reason</label>
          <textarea id="override-reason" className="mt-1 w-full rounded-sm border border-gray-300 p-2" rows={2} value={overrideReason} onChange={(e) => setOverrideReason(e.target.value)} />
        </Modal>
      ) : null}
    </div>
  );
}
