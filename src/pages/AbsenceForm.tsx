import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Check, ChevronLeft, ChevronRight } from "lucide-react";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import Button from "../components/ui/Button";
import type { AbsenceSettings, SubjectWithActiveCourse } from "../types";

type StudentLookup = { student_id: string; wcode: string; full_name: string; subjects: SubjectWithActiveCourse[] };
type SessionBrief = { id: string; start_at: string; end_at: string };
type SitInCourseInfo = { id: string; code: string; name: string };
type SitInResult = {
  sit_in_method: "physical" | "zoom" | "pending";
  sit_in_course?: SitInCourseInfo;
  missed_count: number;
  missed_sessions?: SessionBrief[];
  available_sessions?: SessionBrief[];
  pre_selected?: SessionBrief[];
};
type AbsenceRes = { id: string; wcode: string; subject_id: string; course_id: string; date_from: string; date_to: string; sit_in_method?: string };
type AbsenceFormConfig = Pick<AbsenceSettings, "form" | "sit_in">;

const DEFAULT_CONFIG: AbsenceFormConfig = {
  form: {
    max_date_range_days: 30,
    require_reason: false,
    reason_categories: [],
    allow_free_text_reason: true,
    intro_text: "",
    confirmation_text: "",
  },
  sit_in: {
    auto_resolve_enabled: true,
    zoom_description: "Zoom session - no physical class attendance required.",
    max_sessions_per_absence: 10,
  },
};

const STEPS = ["Student", "Subject & Dates", "Sit-in Plan", "Confirm"];

function fmtDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short", year: "numeric" });
}

function fmtDay(iso: string): string {
  return new Date(iso.slice(0, 10) + "T00:00:00").toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short" });
}

function fmtTime(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
}

function daysBetween(a: string, b: string): number {
  return Math.round((new Date(b + "T00:00:00").getTime() - new Date(a + "T00:00:00").getTime()) / (1000 * 60 * 60 * 24));
}

function formatCycleSuffix(label?: string | null): string {
  return label ? ` (${label})` : "";
}

function formatSubjectBadge(subject: SubjectWithActiveCourse): string {
  return `${subject.code}${formatCycleSuffix(subject.active_cycle_label)}`;
}

function formatSubjectOption(subject: SubjectWithActiveCourse): string {
  return `${subject.code} — ${subject.name}${formatCycleSuffix(subject.active_cycle_label)}`;
}

function formatSubjectConfirmation(subject?: SubjectWithActiveCourse | null): string {
  if (!subject) return "";
  const base = subject.name ? `${subject.code} — ${subject.name}` : subject.code;
  if (!subject.active_course_code) return base;
  return `${base} → ${subject.active_course_code}${formatCycleSuffix(subject.active_cycle_label)}`;
}

export default function AbsenceForm() {
  const navigate = useNavigate();
  const { addToast } = useToast();

  const [step, setStep] = useState(0);
  const [wcode, setWcode] = useState("");
  const [studentName, setStudentName] = useState("");
  const [subjects, setSubjects] = useState<SubjectWithActiveCourse[]>([]);
  const [subjectId, setSubjectId] = useState("");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [reasonCategory, setReasonCategory] = useState("");
  const [reason, setReason] = useState("");
  const [sitInResult, setSitInResult] = useState<SitInResult | null>(null);
  const [selectedSessionIds, setSelectedSessionIds] = useState<Set<string>>(new Set());
  const [submitted, setSubmitted] = useState<AbsenceRes | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [lookupLoading, setLookupLoading] = useState(false);
  const [availabilityLoading, setAvailabilityLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [config, setConfig] = useState<AbsenceFormConfig>(DEFAULT_CONFIG);
  const selectedSubject = subjects.find((s) => s.id === subjectId);

  useEffect(() => {
    let active = true;
    apiJson<AbsenceFormConfig>("/api/v1/absence-form-config", { method: "GET" })
      .then((data) => { if (active) setConfig(data); })
      .catch(() => {});
    return () => { active = false; };
  }, []);

  async function handleLookup() {
    if (!wcode.trim()) { setErrors({ wcode: "Enter your W-Code" }); return; }
    setErrors({});
    setLookupLoading(true);
    try {
      const data = await apiJson<StudentLookup>(`/api/v1/absences/student-lookup?wcode=${encodeURIComponent(wcode.trim())}`, { method: "GET" });
      setStudentName(data.full_name);
      setSubjects(data.subjects);
    } catch {
      setErrors({ wcode: "Student not found or no enrolled subjects" });
    } finally {
      setLookupLoading(false);
    }
  }

  async function handleCheckAvailability(): Promise<boolean> {
    const e: Record<string, string> = {};
    if (!subjectId) e.subject = "Select a subject";
    if (!dateFrom) e.dateFrom = "Select start date";
    if (!dateTo) e.dateTo = "Select end date";
    if (dateFrom && dateTo && dateTo < dateFrom) e.dateTo = "End date must be on or after start date";
    if (dateFrom && dateTo && daysBetween(dateFrom, dateTo) > config.form.max_date_range_days) {
      e.dateRange = `Date range must be ${config.form.max_date_range_days} days or less`;
    }
    if (config.form.require_reason && !reasonCategory) e.reasonCategory = "Select a reason category";
    if (Object.keys(e).length > 0) { setErrors(e); return false; }
    setErrors({});
    if (!config.sit_in.auto_resolve_enabled) {
      setSitInResult({ sit_in_method: "pending", missed_count: 0 });
      setSelectedSessionIds(new Set());
      return true;
    }
    setAvailabilityLoading(true);
    try {
      const data = await apiJson<SitInResult>(
        `/api/v1/absences/sit-in-options?wcode=${encodeURIComponent(wcode.trim())}&subject_id=${subjectId}&date_from=${dateFrom}&date_to=${dateTo}`,
        { method: "GET" }
      );
      setSitInResult(data);
      if (data.sit_in_method === "physical" && data.pre_selected) {
        setSelectedSessionIds(new Set(data.pre_selected.slice(0, config.sit_in.max_sessions_per_absence).map((s) => s.id)));
      }
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to check availability");
    } finally {
      setAvailabilityLoading(false);
    }
    return true;
  }

  function toggleSession(id: string) {
    setSelectedSessionIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) { next.delete(id); return next; }
      if (next.size >= config.sit_in.max_sessions_per_absence) {
        addToast("warning", `Select up to ${config.sit_in.max_sessions_per_absence} sessions`);
        return prev;
      }
      next.add(id);
      return next;
    });
  }

  async function handleSubmit() {
    setSubmitting(true);
    try {
      const body: Record<string, unknown> = {
        wcode: wcode.trim(),
        subject_id: subjectId,
        date_from: dateFrom,
        date_to: dateTo,
      };
      if (selectedSubject?.active_course_id) {
        body.course_id = selectedSubject.active_course_id;
      }
      if (reasonCategory) body.reason_category = reasonCategory;
      if (reason.trim()) body.reason = reason.trim();
      if (sitInResult && sitInResult.sit_in_method !== "pending") {
        body.sit_in_method = sitInResult.sit_in_method;
        if (sitInResult.sit_in_method === "physical") {
          body.sit_in_course_id = sitInResult.sit_in_course?.id;
          body.sit_in_session_ids = Array.from(selectedSessionIds);
        }
      }
      const res = await apiJson<AbsenceRes>("/api/v1/absences", { method: "POST", body: JSON.stringify(body) });
      setSubmitted(res);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Submission failed");
    } finally {
      setSubmitting(false);
    }
  }

  function resetForm() {
    setStep(0);
    setWcode(""); setStudentName(""); setSubjects([]); setSubjectId("");
    setDateFrom(""); setDateTo(""); setReasonCategory(""); setReason("");
    setSitInResult(null); setSelectedSessionIds(new Set()); setErrors({}); setSubmitted(null);
  }

  function canProceedFromStep(s: number): boolean {
    switch (s) {
      case 0: return !!studentName;
      case 1: return !!subjectId && !!dateFrom && !!dateTo;
      case 2: return !!sitInResult;
      default: return true;
    }
  }

  async function handleNext() {
    if (step === 1 && !sitInResult) {
      const ok = await handleCheckAvailability();
      if (!ok) return;
    }
    if (canProceedFromStep(step)) setStep((prev) => Math.min(STEPS.length - 1, prev + 1));
  }

  if (submitted) {
    const subj = subjects.find((s) => s.id === submitted.subject_id);
    return (
      <div className="min-h-screen bg-gray-100 py-8 px-4">
        <div className="mx-auto max-w-2xl">
          <div className="rounded-sm border border-gray-200 bg-white p-6 text-center shadow-sm">
            <div className="mb-3 text-5xl text-[var(--color-wi-green)]">&#10003;</div>
            <h2 className="mb-4 text-lg font-semibold text-gray-800">Absence Submitted</h2>
            {config.form.confirmation_text && <p className="mb-4 text-sm text-gray-600">{config.form.confirmation_text}</p>}
            <div className="mb-6 space-y-1 text-left text-sm text-gray-700">
              <p><span className="font-semibold">W-Code:</span> {submitted.wcode}</p>
              <p><span className="font-semibold">Subject:</span> {formatSubjectConfirmation(subj) || submitted.subject_id}</p>
              <p><span className="font-semibold">Dates:</span> {fmtDate(submitted.date_from)} &ndash; {fmtDate(submitted.date_to)}</p>
              {reasonCategory && <p><span className="font-semibold">Reason:</span> {config.form.reason_categories.find((c) => c.value === reasonCategory)?.label || reasonCategory}</p>}
              {sitInResult?.sit_in_method && <p><span className="font-semibold">Sit-in:</span> {sitInResult.sit_in_method === "zoom" ? "Zoom" : sitInResult.sit_in_course?.code || "Physical"}</p>}
            </div>
            <div className="flex justify-center gap-3">
              <Button variant="secondary" size="lg" onClick={resetForm}>Submit Another</Button>
              <Button variant="primary" size="lg" onClick={() => navigate("/")}>Done</Button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-100 py-8 px-4">
      <div className="mx-auto max-w-2xl">
        <h1 className="mb-6 text-2xl font-bold text-gray-800">Report an Absence</h1>
        {config.form.intro_text && (
          <p className="mb-4 rounded-sm border border-gray-200 bg-white px-4 py-3 text-sm text-gray-700">{config.form.intro_text}</p>
        )}

        <div className="mb-6">
          <nav aria-label="Progress" className="flex items-center gap-0">
            {STEPS.map((label, i) => (
              <div key={label} className="flex items-center flex-1">
                <button
                  onClick={() => { if (i < step) setStep(i); }}
                  disabled={i > step}
                  className={`flex items-center gap-1.5 text-xs font-medium ${
                    i < step ? "text-[var(--color-wi-green)] cursor-pointer hover:underline" :
                    i === step ? "text-[var(--color-wi-primary)]" :
                    "text-gray-400 cursor-default"
                  }`}
                >
                  <span className={`flex h-5 w-5 items-center justify-center rounded-full text-[10px] font-bold ${
                    i < step ? "bg-[var(--color-wi-green)] text-white" :
                    i === step ? "bg-[var(--color-wi-primary)] text-white" :
                    "bg-gray-200 text-gray-500"
                  }`}>
                    {i < step ? <Check className="h-3 w-3" /> : i + 1}
                  </span>
                  <span className="hidden sm:inline">{label}</span>
                </button>
                {i < STEPS.length - 1 ? <div className={`mx-2 h-px flex-1 ${i < step ? "bg-[var(--color-wi-green)]" : "bg-gray-200"}`} /> : null}
              </div>
            ))}
          </nav>
        </div>

        <div className="mb-4 rounded-sm border border-gray-200 bg-white shadow-sm">
          {/* Step 1: Student */}
          {step === 0 ? (
            <div className="p-4">
              <h2 className="mb-3 text-sm font-semibold text-gray-700">Enter your W-Code</h2>
              <div className="flex gap-2">
                <div className="flex-1">
                  <label className="mb-1 block text-xs text-gray-500" htmlFor="wcode-input">W-Code</label>
                  <input
                    id="wcode-input"
                    value={wcode}
                    onChange={(e) => { setWcode(e.target.value); setErrors((p) => ({ ...p, wcode: "" })); }}
                    onBlur={() => { if (wcode.trim() && !studentName) void handleLookup(); }}
                    onKeyDown={(e) => { if (e.key === "Enter" && !studentName) void handleLookup(); }}
                    placeholder="e.g., W250389"
                    className="w-full rounded-sm border border-gray-300 px-2 py-1.5 text-sm"
                  />
                  {errors.wcode && <p className="mt-0.5 text-xs text-red-600">{errors.wcode}</p>}
                </div>
                <div className="pt-5">
                  <Button variant="primary" size="md" loading={lookupLoading} onClick={() => void handleLookup()}>
                    {lookupLoading ? "..." : "Look Up"}
                  </Button>
                </div>
              </div>
            {studentName ? (
                <div className="mt-3 flex flex-wrap items-center gap-1.5 rounded-sm bg-green-50 px-3 py-2 text-sm text-green-800">
                  <Check className="h-4 w-4 shrink-0" />
                  <span className="font-medium">{studentName}</span>
                  <span className="text-green-600">({wcode})</span>
                  <span className="text-green-400">&mdash;</span>
                  {subjects.map((s) => (
                    <span key={s.id} className="rounded-sm bg-green-100 px-1.5 py-0.5 text-xs">{formatSubjectBadge(s)}</span>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}

          {/* Step 2: Subject & Dates */}
          {step === 1 ? (
            <div className="p-4 space-y-4">
              <h2 className="text-sm font-semibold text-gray-700">Subject &amp; Dates</h2>
              <div>
                <label htmlFor="wizard-subject" className="mb-1 block text-xs text-gray-500">Subject</label>
                <select id="wizard-subject" value={subjectId} onChange={(e) => { setSubjectId(e.target.value); setErrors((p) => ({ ...p, subject: "" })); }}
                  className="w-full rounded-sm border border-gray-300 px-2 py-1.5 text-sm">
                  <option value="">Select a subject...</option>
                  {subjects.map((s) => <option key={s.id} value={s.id}>{formatSubjectOption(s)}</option>)}
                </select>
                {errors.subject && <p className="mt-0.5 text-xs text-red-600">{errors.subject}</p>}
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="mb-1 block text-xs text-gray-500" htmlFor="wizard-date-from">From</label>
                  <input id="wizard-date-from" type="date" value={dateFrom} onChange={(e) => { setDateFrom(e.target.value); setErrors((p) => ({ ...p, dateFrom: "" })); }}
                    className="w-full rounded-sm border border-gray-300 px-2 py-1.5 text-sm" />
                  {errors.dateFrom && <p className="mt-0.5 text-xs text-red-600">{errors.dateFrom}</p>}
                </div>
                <div>
                  <label className="mb-1 block text-xs text-gray-500" htmlFor="wizard-date-to">To</label>
                  <input id="wizard-date-to" type="date" value={dateTo} onChange={(e) => { setDateTo(e.target.value); setErrors((p) => ({ ...p, dateTo: "" })); }}
                    className="w-full rounded-sm border border-gray-300 px-2 py-1.5 text-sm" />
                  {errors.dateTo && <p className="mt-0.5 text-xs text-red-600">{errors.dateTo}</p>}
                </div>
              </div>
              {errors.dateRange && <p className="text-xs text-red-600">{errors.dateRange}</p>}
              <div>
                <label htmlFor="wizard-reason-category" className="mb-1 block text-xs text-gray-500">
                  Reason category{config.form.require_reason ? " *" : ""}
                </label>
                <select id="wizard-reason-category" value={reasonCategory} onChange={(e) => { setReasonCategory(e.target.value); setErrors((p) => ({ ...p, reasonCategory: "" })); }}
                  className="w-full rounded-sm border border-gray-300 px-2 py-1.5 text-sm">
                  <option value="">Select a reason...</option>
                  {config.form.reason_categories.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}
                </select>
                {errors.reasonCategory && <p className="mt-0.5 text-xs text-red-600">{errors.reasonCategory}</p>}
              </div>
              {config.form.allow_free_text_reason && (
                <div>
                  <label htmlFor="wizard-reason" className="mb-1 block text-xs text-gray-500">Additional details</label>
                  <textarea id="wizard-reason" value={reason} onChange={(e) => setReason(e.target.value)}
                    placeholder="Optional &mdash; e.g., illness or family event"
                    rows={2} className="w-full rounded-sm border border-gray-300 px-2 py-1.5 text-sm" />
                </div>
              )}
              {availabilityLoading ? (
                <div className="flex items-center gap-2 text-sm text-gray-500">
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-gray-600" />
                  Checking availability...
                </div>
              ) : null}
            </div>
          ) : null}

          {/* Step 3: Sit-in Plan */}
          {step === 2 && sitInResult ? (
            <div className="p-4 space-y-3">
              <h2 className="text-sm font-semibold text-gray-700">Sit-in Plan</h2>
              {sitInResult.sit_in_method === "pending" ? (
                <div className="rounded-sm border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
                  Your sit-in plan will be assigned by staff after review.
                </div>
              ) : sitInResult.sit_in_method === "zoom" ? (
                <div className="rounded-sm border border-blue-200 bg-blue-50 p-3 text-sm text-blue-800">
                  {config.sit_in.zoom_description}
                  {sitInResult.missed_count > 0 && <p className="mt-1 text-xs text-blue-600">You will miss {sitInResult.missed_count} session(s).</p>}
                </div>
              ) : (
                <>
                  <div className="rounded-sm border border-green-200 bg-green-50 p-3 text-sm text-green-800">
                    Sit-in at <strong>{sitInResult.sit_in_course?.code}</strong> &mdash; {sitInResult.sit_in_course?.name}
                    <p className="mt-1 text-xs text-green-600">
                      {sitInResult.missed_count} missed session(s).{sitInResult.available_sessions ? ` ${sitInResult.available_sessions.length} sit-in session(s) available.` : ""}
                    </p>
                  </div>
                  {buildTimeline(sitInResult, selectedSessionIds, toggleSession)}
                </>
              )}
            </div>
          ) : null}

          {/* Step 4: Confirm */}
          {step === 3 ? (
            <div className="p-4 space-y-3">
              <h2 className="text-sm font-semibold text-gray-700">Confirm Your Absence</h2>
              <div className="rounded-sm border border-gray-100 bg-gray-50 p-3 text-sm space-y-2">
                <p><span className="font-medium">Student:</span> {studentName} ({wcode})</p>
                <p><span className="font-medium">Subject:</span> {formatSubjectConfirmation(selectedSubject) || subjectId}</p>
                <p><span className="font-medium">Dates:</span> {fmtDate(dateFrom)} &ndash; {fmtDate(dateTo)} ({daysBetween(dateFrom, dateTo) + 1} days)</p>
                {reasonCategory ? <p><span className="font-medium">Reason:</span> {config.form.reason_categories.find((c) => c.value === reasonCategory)?.label || reasonCategory}{reason ? ` - ${reason}` : ""}</p> : null}
                <p><span className="font-medium">Sit-in:</span> {sitInResult?.sit_in_method === "zoom" ? "Zoom" : sitInResult?.sit_in_course?.code || "To be assigned"}</p>
              </div>
            </div>
          ) : null}

          {/* Navigation */}
          <div className="flex items-center justify-between border-t border-gray-100 px-4 py-3">
            <div>
              {step > 0 ? (
                <Button variant="secondary" size="md" onClick={() => setStep((prev) => prev - 1)}>
                  <ChevronLeft className="mr-1 h-4 w-4" /> Back
                </Button>
              ) : null}
            </div>
            <div>
              {step < STEPS.length - 1 ? (
                <Button variant="primary" size="md" disabled={!canProceedFromStep(step)} onClick={handleNext}>
                  {step === 1 && !sitInResult ? "Check Availability" : "Next"} <ChevronRight className="ml-1 h-4 w-4" />
                </Button>
              ) : (
                <Button variant="primary" size="lg" loading={submitting} onClick={() => void handleSubmit()}>
                  {submitting ? "Submitting..." : "Submit Absence"}
                </Button>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function buildTimeline(
  result: SitInResult,
  selectedSessionIds: Set<string>,
  toggleSession: (id: string) => void,
) {
  const missed = result.missed_sessions ?? [];
  const available = result.available_sessions ?? [];

  if (missed.length === 0 && available.length === 0) {
    return <p className="text-sm text-gray-500">No sessions in this date range.</p>;
  }

  const availByDate = new Map<string, SessionBrief[]>();
  for (const a of available) {
    const date = a.start_at.slice(0, 10);
    if (!availByDate.has(date)) availByDate.set(date, []);
    availByDate.get(date)!.push(a);
  }

  const usedAvail = new Set<string>();
  const allDates = new Set<string>();
  for (const m of missed) allDates.add(m.start_at.slice(0, 10));
  for (const a of available) allDates.add(a.start_at.slice(0, 10));
  const sortedDates = [...allDates].sort();

  return (
    <div className="space-y-3">
      {sortedDates.map((date) => {
        const dayMissed = missed.filter((m) => m.start_at.slice(0, 10) === date);
        const dayAvail = availByDate.get(date) ?? [];
        return (
          <div key={date}>
            <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">{fmtDay(date)}</p>
            <div className="space-y-1">
              {dayMissed.map((m) => {
                const pair = dayAvail.find((a) => !usedAvail.has(a.id));
                if (pair) usedAvail.add(pair.id);
                return (
                  <div key={m.id} className="flex items-center gap-2 rounded-sm px-2 py-1 hover:bg-gray-50">
                    <div className="flex-1 text-sm text-gray-700">
                      Missed: {fmtTime(m.start_at)} &ndash; {fmtTime(m.end_at)}
                    </div>
                    {pair ? (
                      <label className="flex cursor-pointer items-center gap-1.5 text-sm ml-auto shrink-0">
                        <input
                          type="checkbox"
                          checked={selectedSessionIds.has(pair.id)}
                          onChange={() => toggleSession(pair.id)}
                          className="accent-[var(--color-wi-green)]"
                        />
                        <span className="text-green-700">
                          Sit-in: {fmtTime(pair.start_at)} &ndash; {fmtTime(pair.end_at)}
                        </span>
                      </label>
                    ) : (
                      <span className="ml-auto shrink-0 text-xs italic text-gray-400">&mdash; (full)</span>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        );
      })}
    </div>
  );
}
