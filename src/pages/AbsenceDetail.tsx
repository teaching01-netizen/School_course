import { useCallback, useEffect, useMemo, useState } from "react";
import SearchableSelect from "@/components/ui/SearchableSelect";
import { Link, useParams } from "react-router-dom";
import { CheckCircle, Clock, RotateCcw, XCircle, PenLine } from "lucide-react";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { useRealtime } from "../hooks/useRealtime";
import type { ManagedAbsence } from "../types";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";
import Modal from "../components/Modal";
import { formatSitInLabel } from "../components/absences/sitInLabel";
import OverrideSitInModal from "../components/absences/OverrideSitInModal";

const INSTITUTE_TIME_ZONE = "Asia/Bangkok";

type TimeRanged = { start_at: string; end_at: string };

type ScheduleImpactIssue = {
  id: string;
  issue_type: string;
  severity: "warning" | "critical";
  status: string;
  latest_session_change_id?: string;
};

type DayRangeGroup<T extends TimeRanged> = {
  date: string;
  start_at: string;
  end_at: string;
  items: T[];
};

function displayDate(value: string): string {
  return new Date(value + "T00:00:00").toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
}

function displayDateTime(value: string): string {
  return new Date(value).toLocaleString("en-GB", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", timeZone: INSTITUTE_TIME_ZONE });
}

function displayTime(value: string): string {
  return new Date(value).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit", timeZone: INSTITUTE_TIME_ZONE });
}

function instituteDateKey(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value.slice(0, 10);
  const parts = new Intl.DateTimeFormat("en-GB", {
    timeZone: INSTITUTE_TIME_ZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(date);
  const part = (type: string) => parts.find((p) => p.type === type)?.value ?? "";
  return `${part("year")}-${part("month")}-${part("day")}`;
}

function groupByInstituteDay<T extends TimeRanged>(items: T[]): DayRangeGroup<T>[] {
  const byDay = new Map<string, T[]>();
  for (const item of items.slice().sort((a, b) => new Date(a.start_at).getTime() - new Date(b.start_at).getTime())) {
    const day = instituteDateKey(item.start_at);
    byDay.set(day, [...(byDay.get(day) ?? []), item]);
  }
  return [...byDay.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([date, dayItems]) => {
    let start = dayItems[0].start_at;
    let end = dayItems[0].end_at;
    for (const item of dayItems) {
      if (new Date(item.start_at).getTime() < new Date(start).getTime()) start = item.start_at;
      if (new Date(item.end_at).getTime() > new Date(end).getTime()) end = item.end_at;
    }
    return { date, start_at: start, end_at: end, items: dayItems };
  });
}

function titleCase(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1).replace(/_/g, " ");
}

function initials(name: string): string {
  return name.split(" ").map((part) => part.charAt(0)).join("").toUpperCase().slice(0, 2);
}

function daysBetween(a: string, b: string): number {
  const d1 = new Date(a + "T00:00:00");
  const d2 = new Date(b + "T00:00:00");
  return Math.round((d2.getTime() - d1.getTime()) / (1000 * 60 * 60 * 24)) + 1;
}

function displayAbsenceDates(absence: ManagedAbsence): string {
  const sessions = (absence.missed_sessions ?? [])
    .slice()
    .sort((a, b) => new Date(a.start_at).getTime() - new Date(b.start_at).getTime());
  if (sessions.length > 0) {
    return groupByInstituteDay(sessions)
      .map((group) => `${displayDate(group.date)} ${displayTime(group.start_at)}-${displayTime(group.end_at)}`)
      .join("\n");
  }
  if (absence.date_from === absence.date_to) {
    return displayDate(absence.date_from);
  }
  return `${displayDate(absence.date_from)} - ${displayDate(absence.date_to)} (${daysBetween(absence.date_from, absence.date_to)} days)`;
}

function displaySitInPlanLabel(absence: ManagedAbsence): string {
  return formatSitInLabel(absence);
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
      return <CheckCircle className="h-4 w-4 text-[var(--color-wi-text-light)]" />;
    case "cancelled":
      return <XCircle className="h-4 w-4 text-red-500" />;
    case "overridden":
      return <RotateCcw className="h-4 w-4 text-amber-500" />;
    default:
      return <Clock className="h-4 w-4 text-[var(--color-wi-text-light)]" />;
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
  const [cancelReasonCategory, setCancelReasonCategory] = useState("");
  const [cancelReasonDetail, setCancelReasonDetail] = useState("");
  const [overrideOpen, setOverrideOpen] = useState(false);
  const [impactIssues, setImpactIssues] = useState<ScheduleImpactIssue[]>([]);
  const openImpactIssues = impactIssues.filter((issue) => issue.status === "open");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = await apiJson<ManagedAbsence>(`/api/v1/absences/${id}`, { method: "GET" });
      setAbsence(result);
      setNotes(result.admin_notes ?? "");
      setNotesDirty(false);
      try {
        const impactResult = await apiJson<{ items: ScheduleImpactIssue[] }>(
          `/api/v1/operations/schedule-issues?absence_id=${encodeURIComponent(id ?? "")}`,
          { method: "GET" },
        );
        setImpactIssues(Array.isArray(impactResult?.items) ? impactResult.items : []);
      } catch {
        setImpactIssues([]);
      }
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load absence");
    } finally {
      setLoading(false);
    }
  }, [addToast, id]);

  useEffect(() => { void load(); }, [load]);

  useRealtime(
    ["absent:all"],
    (event) => {
      if (event.id === id) void load();
    },
    { debounceMs: 500, onReconnect: () => { void load(); } }
  );



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

  function openOverride() {
    setOverrideOpen(true);
  }

  const statusClasses = useMemo(() => {
    switch (absence?.status) {
      case "pending": return "bg-blue-50 text-blue-700";
      case "reviewed": return "bg-emerald-50 text-emerald-700";
      case "cancelled": return "bg-red-50 text-red-700";
      default: return "bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]";
    }
  }, [absence?.status]);

  if (loading && !absence) return <LoadingSkeleton type="table" lines={6} />;
  if (!absence) return <p className="text-sm text-[var(--color-wi-text-light)]">Absence could not be loaded.</p>;

  return (
    <div className="mx-auto max-w-6xl">
      <Link className="text-sm text-[var(--color-wi-primary)] hover:underline" to="/absences">Back to Absences</Link>

      <div className="mt-2 flex items-center gap-3">
        <Link className="text-xs text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text-light)]" to="/absences/calendar">View on Calendar</Link>
        <Link className="text-xs text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text-light)]" to="/slot-finder">Find Alternative Slots</Link>
      </div>

      <div className="mt-2 flex flex-wrap items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <span className="mt-1 flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-base font-bold text-white">{initials(absence.student_name ?? absence.wcode)}</span>
          <div>
            <h1 className="text-2xl font-semibold text-[var(--color-wi-text)]">Absence Detail</h1>
            <div className="mt-1 flex items-center gap-2 text-sm text-[var(--color-wi-text-light)]">
              <span className="font-medium">{absence.student_name ?? "Unknown"}</span>
              <span className="font-mono text-xs text-[var(--color-wi-text-light)]">{absence.wcode}</span>
            </div>
          </div>
        </div>
        <div className="hidden md:flex flex-wrap items-center gap-2 rounded-sm border border-wi-line bg-white p-3 shadow-sm md:flex-nowrap">
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

      {openImpactIssues.length > 0 ? (
        <section className="mt-4 rounded-sm border border-amber-200 bg-amber-50 p-4" aria-live="polite">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold text-amber-900">Schedule change needs review</h2>
              <p className="mt-1 text-sm text-amber-800">{openImpactIssues.length} open impact {openImpactIssues.length === 1 ? "issue affects" : "issues affect"} this absence plan.</p>
            </div>
            <Link to="/operations/schedule-impact" className="text-sm font-medium text-amber-900 underline underline-offset-2">Review Schedule Impact</Link>
          </div>
        </section>
      ) : null}

      <div className="mt-4 grid gap-4">
        <section className="rounded-sm border border-wi-line bg-white">
          <h2 className="border-b border-wi-line-soft bg-[var(--color-wi-row-alt)]/70 px-4 py-3 text-sm font-semibold text-[var(--color-wi-text)]">Absence Summary</h2>
          <div className="grid gap-4 p-4 md:grid-cols-2">
            <dl className="grid grid-cols-[120px_1fr] gap-y-2 text-sm">
              <dt className="text-[var(--color-wi-text-light)]">Subject</dt>
              <dd>{absence.subject_code ?? "-"} {absence.subject_name ? `- ${absence.subject_name}` : ""}</dd>
              <dt className="text-[var(--color-wi-text-light)]">Email</dt>
              <dd>{absence.student_email ?? "-"}</dd>
              <dt className="text-[var(--color-wi-text-light)]">Nickname</dt>
              <dd>{absence.student_nickname ?? "-"}</dd>
              <dt className="text-[var(--color-wi-text-light)]">Dates</dt>
              <dd className="whitespace-pre-line">{displayAbsenceDates(absence)}</dd>
              <dt className="text-[var(--color-wi-text-light)]">Reason</dt>
              <dd>{displayAbsenceReason(absence)}</dd>
              <dt className="text-[var(--color-wi-text-light)]">Submitted</dt>
              <dd>{displayDateTime(absence.created_at)}</dd>
            </dl>
          </div>
        </section>

        <section className="rounded-sm border border-wi-line bg-white">
          <h2 className="border-b border-wi-line-soft bg-[var(--color-wi-row-alt)]/70 px-4 py-3 text-sm font-semibold text-[var(--color-wi-text)]">Sit-in Plan</h2>
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
                {(() => {
                  return groupByInstituteDay(absence.sit_ins).map((group) => {
                    if (group.items.length > 1) {
                      const rooms = [...new Set(group.items.map((s) => s.room_name ?? "No room"))].join(", ");
                      return (
                        <div key={group.start_at} className="flex items-center justify-between rounded-sm border border-wi-line-soft bg-[var(--color-wi-row-alt)] px-3 py-2 text-sm">
                          <span>{displayDate(group.date)} {displayTime(group.start_at)} &ndash; {displayTime(group.end_at)}</span>
                          <span className="text-[var(--color-wi-text-light)]">{rooms}</span>
                        </div>
                      );
                    }
                    const s = group.items[0];
                    return (
                      <div key={s.id} className="flex items-center justify-between rounded-sm border border-wi-line-soft bg-[var(--color-wi-row-alt)] px-3 py-2 text-sm">
                        <span>{displayDateTime(s.start_at)} &ndash; {displayTime(s.end_at)}</span>
                        <span className="text-[var(--color-wi-text-light)]">{s.room_name ?? "No room"}</span>
                      </div>
                    );
                  });
                })()}
              </div>
            ) : <p className="text-sm text-[var(--color-wi-text-light)]">No physical sit-in sessions assigned.</p>}
            {absence.sit_in_method === "zoom" ? (
              <p className="mt-2 text-sm text-[var(--color-wi-text-light)]">Student attends via Zoom &mdash; no physical class required.</p>
            ) : null}
            {absence.sit_in_method === "physical" && !absence.sit_ins?.length ? (
              <p className="mt-2 text-sm text-[var(--color-wi-text-light)]">No sessions assigned yet.</p>
            ) : null}
          </div>
        </section>

        <div className="grid gap-4">
          <section className="rounded-sm border border-wi-line bg-white">
            <h2 className="border-b border-wi-line-soft bg-[var(--color-wi-row-alt)]/70 px-4 py-3 text-sm font-semibold text-[var(--color-wi-text)]">Admin Notes</h2>
            <div className="p-4">
              <label className="sr-only" htmlFor="detail-note">Internal note</label>
              <textarea
                id="detail-note"
                value={notes}
                onChange={(e) => { setNotes(e.target.value); setNotesDirty(true); }}
                rows={5}
                className="w-full rounded-sm border border-wi-line p-2 text-sm"
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

          <section className="rounded-sm border border-wi-line bg-white">
            <h2 className="border-b border-wi-line-soft bg-[var(--color-wi-row-alt)]/70 px-4 py-3 text-sm font-semibold text-[var(--color-wi-text)]">Timeline</h2>
            <div className="p-4">
              <ol className="space-y-3">
                {(absence.timeline ?? []).map((entry) => (
                  <li key={entry.id} className="relative flex gap-3 timeline-item">
                    <div className="mt-0.5 shrink-0">{TimelineIcon({ action: entry.action })}</div>
                    <div>
                      <p className="text-sm font-medium text-[var(--color-wi-text)]">{titleCase(entry.action)}</p>
                      <p className="text-xs text-[var(--color-wi-text-light)]">{displayDateTime(entry.created_at)} &mdash; {entry.actor_name ?? entry.actor_role}</p>
                    </div>
                  </li>
                ))}
                {!absence.timeline?.length ? <li className="text-sm text-[var(--color-wi-text-light)]">No activity recorded.</li> : null}
              </ol>
            </div>
          </section>
        </div>
      </div>

      {cancelOpen ? (
        <Modal title="Cancel absence" onClose={() => setCancelOpen(false)}
          footer={<><Button variant="secondary" onClick={() => setCancelOpen(false)}>Back</Button><Button variant="danger" disabled={!cancelReasonCategory} loading={saving} onClick={() => void updateStatus("cancelled", JSON.stringify({ category: cancelReasonCategory, detail: cancelReasonDetail })).then(() => setCancelOpen(false))}>Cancel Absence</Button></>}>
          <p className="mb-3 text-sm text-[var(--color-wi-text-light)]">This action is retained in the audit timeline.</p>
          <label className="block text-sm font-medium text-[var(--color-wi-text-light)]" htmlFor="detail-cancel-category">Cancellation reason</label>
          <SearchableSelect id="detail-cancel-category" className="mt-1 w-full rounded-sm border border-wi-line p-2 text-sm" value={cancelReasonCategory} onChange={(e) => setCancelReasonCategory(e.target.value)}>
            <option value="">Select a reason...</option>
            <option value="duplicate">Duplicate submission</option>
            <option value="student_requested">Student requested cancellation</option>
            <option value="admin_error">Admin error</option>
            <option value="incorrect_dates">Incorrect dates</option>
            <option value="other">Other</option>
          </SearchableSelect>
          <label className="mt-3 block text-sm font-medium text-[var(--color-wi-text-light)]" htmlFor="detail-cancel-detail">Additional details (optional)</label>
          <textarea id="detail-cancel-detail" className="mt-1 w-full rounded-sm border border-wi-line p-2 text-sm" rows={3} value={cancelReasonDetail} onChange={(e) => setCancelReasonDetail(e.target.value)} />
        </Modal>
      ) : null}

      {overrideOpen && absence ? (
        <OverrideSitInModal
          absenceId={absence.id}
          version={absence.version}
          currentMethod={absence.sit_in_method}
          currentCourseId={absence.sit_in_course_id}
          onClose={() => setOverrideOpen(false)}
          onSaved={() => void load()}
        />
      ) : null}
    </div>
  );
}
