import { useEffect, useMemo, useState } from "react";
import { BookOpen, Check, ChevronRight, Info, MapPin, Video } from "lucide-react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import type { StudentLookupResponse, SessionsInRangeResponse, SubjectSessions, AbsenceFormConfig, SitInSessionInfo, SmsPreview } from "../../types";
import Button from "../ui/Button";
import Select from "../ui/Select";
import Modal from "../Modal";
import SmsConfirmModal from "./SmsConfirmModal";

type ModalStep = "student" | "sessions" | "sit-in" | "confirm";

type Props = {
  onClose: () => void;
  onCreated: () => void;
};

const INSTITUTE_TIME_ZONE = "Asia/Bangkok";
const STEP_KEYS: ModalStep[] = ["student", "sessions", "sit-in", "confirm"];

function displayDate(value: string): string {
  return new Date(value).toLocaleDateString("en-GB", {
    day: "numeric", month: "short", timeZone: INSTITUTE_TIME_ZONE,
  });
}

function displayTime(value: string): string {
  return new Date(value).toLocaleTimeString("en-GB", {
    hour: "2-digit", minute: "2-digit", timeZone: INSTITUTE_TIME_ZONE,
  });
}

function sessionLabel(s: { start_at: string; end_at: string }): string {
  return `${displayDate(s.start_at)} ${displayTime(s.start_at)}–${displayTime(s.end_at)}`;
}

function StepIndicator({ step }: { step: ModalStep }) {
  const currentIdx = STEP_KEYS.indexOf(step);
  return (
    <div className="mb-6 flex items-center gap-1.5">
      {STEP_KEYS.map((s, i) => {
        const isActive = s === step;
        const isComplete = i < currentIdx;
        return (
          <div key={s} className="flex items-center gap-1.5">
            <div className={`flex h-6 w-6 items-center justify-center rounded-full text-xs font-bold transition-colors ${
              isActive ? "bg-[var(--color-wi-primary)] text-white" :
              isComplete ? "bg-emerald-500 text-white" :
              "bg-gray-200 text-gray-400"
            }`}>
              {isComplete ? <Check className="h-3.5 w-3.5" /> : i + 1}
            </div>
            {i < STEP_KEYS.length - 1 ? (
              <div className={`h-px w-6 transition-colors ${i < currentIdx ? "bg-emerald-300" : "bg-gray-200"}`} />
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

function SessionCard({
  session,
  selected,
  onToggle,
}: {
  session: { id: string; start_at: string; end_at: string; room_name?: string };
  selected: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className={`flex w-full items-center gap-3 rounded-lg border px-4 py-3 text-left transition-all duration-150 ${
        selected
          ? "border-blue-400 bg-blue-50/30 ring-1 ring-blue-500/20"
          : "border-gray-200 bg-white hover:border-gray-300 hover:bg-gray-50/50"
      }`}
    >
      <span
        className={`flex h-5 w-5 shrink-0 items-center justify-center rounded border transition-colors ${
          selected
            ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)] text-white"
            : "border-gray-300 bg-white"
        }`}
      >
        {selected ? <Check className="h-3 w-3" strokeWidth={3} /> : null}
      </span>
      <div className="min-w-0 flex-1">
        <span className="text-sm font-medium text-gray-900">{sessionLabel(session)}</span>
        {session.room_name ? (
          <span className="ml-2 inline-flex items-center gap-1 text-xs text-gray-500">
            <MapPin className="h-3 w-3" /> {session.room_name}
          </span>
        ) : null}
      </div>
    </button>
  );
}

function SkeletonRows({ count = 2 }: { count?: number }) {
  return (
    <div className="space-y-2">
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="h-12 animate-pulse rounded-lg bg-gray-100" />
      ))}
    </div>
  );
}

export default function StaffCreateAbsenceModal({ onClose, onCreated }: Props) {
  const { addToast } = useToast();
  const [step, setStep] = useState<ModalStep>("student");

  // Step 1: Student
  const [wcode, setWcode] = useState("");
  const [student, setStudent] = useState<StudentLookupResponse | null>(null);
  const [lookingUp, setLookingUp] = useState(false);
  const [selectedSubjectId, setSelectedSubjectId] = useState("");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");

  // Step 2: Sessions
  const [subjectSessions, setSubjectSessions] = useState<SubjectSessions[]>([]);
  const [selectedMissed, setSelectedMissed] = useState<Set<string>>(new Set());
  const [loadingSessions, setLoadingSessions] = useState(false);

  // Step 3: Sit-in
  const [sitInMethod, setSitInMethod] = useState<"zoom" | "physical">("physical");
  const [sitInCourseId, setSitInCourseId] = useState("");
  const [sitInCourseName, setSitInCourseName] = useState("");
  const [sitInCandidates, setSitInCandidates] = useState<Array<{ id: string; start_at: string; end_at: string; room_name?: string }>>([]);
  const [selectedSitIn, setSelectedSitIn] = useState<Set<string>>(new Set());
  const [loadingSitIn, setLoadingSitIn] = useState(false);

  // Step 4: Confirm
  const [formConfig, setFormConfig] = useState<AbsenceFormConfig | null>(null);
  const [reasonCategory, setReasonCategory] = useState("");
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [smsPreview, setSmsPreview] = useState<SmsPreview | null>(null);
  const [createdAbsenceId, setCreatedAbsenceId] = useState<string | null>(null);
  const [sendingSms, setSendingSms] = useState(false);

  const selectedSubject = useMemo(
    () => student?.subjects.find((s) => s.id === selectedSubjectId) ?? null,
    [student, selectedSubjectId],
  );

  const selectedSubjectSessions = useMemo(
    () => subjectSessions.find((s) => s.subject_id === selectedSubjectId) ?? null,
    [subjectSessions, selectedSubjectId],
  );

  const missedSessionList = useMemo(() => {
    if (!selectedSubjectSessions) return [];
    return selectedSubjectSessions.sessions.filter((s) => !s.already_absent);
  }, [selectedSubjectSessions]);

  useEffect(() => {
    if (step !== "sessions" || !student || !selectedSubjectId || !dateFrom || !dateTo) return;
    setLoadingSessions(true);
    void apiJson<SessionsInRangeResponse>(
      `/api/v1/absences/sessions-in-range?wcode=${encodeURIComponent(student.wcode)}&subject_id=${encodeURIComponent(selectedSubjectId)}&date_from=${dateFrom}&date_to=${dateTo}`,
      { method: "GET" },
    )
      .then((res) => setSubjectSessions(res.subjects ?? []))
      .catch(() => setSubjectSessions([]))
      .finally(() => setLoadingSessions(false));
  }, [step, student, selectedSubjectId, dateFrom, dateTo]);

  useEffect(() => {
    if (step !== "sit-in" || sitInMethod !== "physical" || !student || !selectedSubjectId || !dateFrom || !dateTo) return;
    setLoadingSitIn(true);
    void apiJson<SitInSessionInfo>(
      `/api/v1/absences/sit-in-options?wcode=${encodeURIComponent(student.wcode)}&subject_id=${encodeURIComponent(selectedSubjectId)}&date_from=${dateFrom}&date_to=${dateTo}`,
      { method: "GET" },
    )
      .then((res) => {
        const course = res.sit_in_course;
        if (course) {
          setSitInCourseId(course.id);
          setSitInCourseName(course.subject_name || course.name || course.code);
        }
        const sessions = (res.available_sessions ?? []).map((s) => ({
          id: s.id,
          start_at: s.start_at,
          end_at: s.end_at,
        }));
        setSitInCandidates(sessions);
      })
      .catch(() => { setSitInCandidates([]); setSitInCourseId(""); setSitInCourseName(""); })
      .finally(() => setLoadingSitIn(false));
  }, [step, sitInMethod, student, selectedSubjectId, dateFrom, dateTo]);

  useEffect(() => {
    if (step !== "confirm" || formConfig) return;
    void apiJson<AbsenceFormConfig>("/api/v1/absence-form-config", { method: "GET" })
      .then((config) => setFormConfig(config))
      .catch(() => { addToast("error", "Failed to load form settings"); setFormConfig(null); });
  }, [step, formConfig, addToast]);

  function toggleMissed(id: string) {
    setSelectedMissed((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleSitIn(id: string) {
    setSelectedSitIn((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function canAdvanceFromStudent(): boolean {
    if (!student || !selectedSubjectId || !dateFrom || !dateTo) return false;
    if (dateTo < dateFrom) return false;
    return true;
  }

  function canAdvanceFromSessions(): boolean {
    return selectedMissed.size > 0;
  }

  function canAdvanceFromSitIn(): boolean {
    if (sitInMethod === "zoom") return true;
    return !!sitInCourseId && selectedSitIn.size > 0;
  }

  function handleNext() {
    if (step === "student") {
      if (!canAdvanceFromStudent()) {
        if (dateFrom && dateTo && dateTo < dateFrom) {
          addToast("error", "End date must be on or after start date");
        } else {
          addToast("error", "Select a student, subject, and date range");
        }
        return;
      }
      setStep("sessions");
    } else if (step === "sessions") {
      if (!canAdvanceFromSessions()) {
        addToast("error", "Select at least one missed session");
        return;
      }
      setStep("sit-in");
    } else if (step === "sit-in") {
      if (!canAdvanceFromSitIn()) {
        addToast("error", sitInMethod === "physical"
          ? (sitInCandidates.length === 0 ? "No sit-in sessions available" : "Select at least one sit-in session")
          : "Select sit-in method");
        return;
      }
      setStep("confirm");
    }
  }

  function handleBack() {
    if (step === "sessions") setStep("student");
    else if (step === "sit-in") setStep("sessions");
    else if (step === "confirm") setStep("sit-in");
  }

  async function lookupStudent() {
    if (!wcode.trim()) return;
    setLookingUp(true);
    try {
      const data = await apiJson<StudentLookupResponse>(`/api/v1/absences/student-lookup?wcode=${encodeURIComponent(wcode.trim())}`, { method: "GET" });
      setStudent(data);
      setSelectedSubjectId("");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Student not found");
      setStudent(null);
    } finally {
      setLookingUp(false);
    }
  }

  async function handleSubmit() {
    if (!student || !selectedSubjectId || !dateFrom || !dateTo) return;
    setSubmitting(true);
    try {
      const res = await apiJson<{ id: string; sms_preview?: SmsPreview }>("/api/v1/absences/staff-create", {
        method: "POST",
        body: JSON.stringify({
          wcode: student.wcode,
          subject_id: selectedSubjectId,
          course_id: selectedSubjectSessions?.course_id,
          date_from: dateFrom,
          date_to: dateTo,
          missed_session_ids: [...selectedMissed],
          sit_in_method: sitInMethod,
          sit_in_course_id: sitInMethod === "physical" ? sitInCourseId : undefined,
          sit_in_session_ids: [...selectedSitIn],
          reason_category: reasonCategory || undefined,
          reason: reason || undefined,
        }),
      });
      if (res.sms_preview && res.sms_preview.phones.length > 0) {
        setCreatedAbsenceId(res.id);
        setSmsPreview(res.sms_preview);
      } else {
        addToast("success", "Absence created successfully");
        onCreated();
      }
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to create absence");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleSendSms() {
    if (!createdAbsenceId) {
      addToast("error", "Missing absence ID, cannot send SMS");
      return;
    }
    setSendingSms(true);
    try {
      const res = await apiJson<{ sent: boolean; recipient_count: number }>(`/api/v1/absences/${createdAbsenceId}/send-success-sms`, { method: "POST" });
      if (!res.sent) {
        addToast("error", "SMS was not sent");
        return;
      }
      addToast("success", `SMS notification sent to ${res.recipient_count} recipient${res.recipient_count !== 1 ? "s" : ""}`);
      onCreated();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to send SMS");
    } finally {
      setSendingSms(false);
    }
  }

  function handleSkipSms() {
    if (sendingSms) return;
    addToast("success", "Absence created successfully (SMS skipped)");
    onCreated();
  }

  if (smsPreview && createdAbsenceId) {
    return (
      <SmsConfirmModal
        phones={smsPreview.phones}
        message={smsPreview.message}
        onSend={() => void handleSendSms()}
        onSkip={handleSkipSms}
        sending={sendingSms}
      />
    );
  }

  return (
    <Modal
      title="Create Absence"
      onClose={onClose}
      size="xl"
      footer={
        step === "confirm" ? (
          <>
            <div className="flex-1" />
            <Button variant="secondary" onClick={onClose}>Cancel</Button>
            <Button loading={submitting} onClick={() => void handleSubmit()}>Create Absence</Button>
          </>
        ) : (
          <>
            <div className="flex-1" />
            {step !== "student" ? (
              <Button variant="secondary" onClick={handleBack}>Back</Button>
            ) : null}
            <Button onClick={handleNext}>
              Next <ChevronRight className="ml-1 inline h-4 w-4" />
            </Button>
          </>
        )
      }
    >
      <StepIndicator step={step} />

      {/* Step 1: Student */}
      {step === "student" && (
        <div className="space-y-5">
          <div>
            <label htmlFor="staff-wcode" className="mb-1.5 block text-sm font-medium text-gray-700">Student W-Code</label>
            <div className="flex gap-2">
              <input
                id="staff-wcode"
                type="text"
                className="flex-1 rounded-sm border border-gray-300 px-3 py-2 text-sm"
                placeholder="e.g. W001234"
                value={wcode}
                onChange={(e) => { setWcode(e.target.value); setStudent(null); setSelectedSubjectId(""); }}
                onKeyDown={(e) => { if (e.key === "Enter" && wcode.trim()) void lookupStudent(); }}
              />
              <Button variant="secondary" onClick={() => void lookupStudent()} loading={lookingUp}>Look up</Button>
            </div>
          </div>

          {student ? (
            <>
              <div className="rounded-lg border border-emerald-200 bg-emerald-50/50 px-4 py-3">
                <p className="text-sm font-medium text-emerald-800">{student.full_name}</p>
                <p className="text-xs text-emerald-600">{student.wcode}</p>
              </div>

              <div>
                <label htmlFor="staff-subject" className="mb-1.5 block text-sm font-medium text-gray-700">Subject</label>
                <Select id="staff-subject" placeholder="Select a subject" value={selectedSubjectId} onChange={(e) => setSelectedSubjectId(e.target.value)}>
                  {student.subjects.map((s) => (
                    <option key={s.id} value={s.id}>{s.code} — {s.name}</option>
                  ))}
                </Select>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label htmlFor="staff-date-from" className="mb-1.5 block text-sm font-medium text-gray-700">From</label>
                  <input id="staff-date-from" type="date" className="w-full rounded-sm border border-gray-300 px-3 py-2 text-sm" value={dateFrom} onChange={(e) => setDateFrom(e.target.value)} />
                </div>
                <div>
                  <label htmlFor="staff-date-to" className="mb-1.5 block text-sm font-medium text-gray-700">To</label>
                  <input id="staff-date-to" type="date" className="w-full rounded-sm border border-gray-300 px-3 py-2 text-sm" value={dateTo} onChange={(e) => setDateTo(e.target.value)} />
                </div>
              </div>
            </>
          ) : null}
        </div>
      )}

      {/* Step 2: Sessions */}
      {step === "sessions" && (
        <div className="space-y-4">
          <p className="text-sm text-gray-600">Select the sessions the student missed.</p>
          {loadingSessions ? (
            <SkeletonRows />
          ) : missedSessionList.length === 0 ? (
            <p className="text-sm text-gray-400">No sessions found in this date range.</p>
          ) : (
            <div className="space-y-2 max-h-64 overflow-y-auto">
              {missedSessionList.map((session) => (
                <SessionCard
                  key={session.id}
                  session={session}
                  selected={selectedMissed.has(session.id)}
                  onToggle={() => toggleMissed(session.id)}
                />
              ))}
            </div>
          )}
          {selectedMissed.size > 0 ? (
            <p className="text-xs text-gray-500">{selectedMissed.size} session{selectedMissed.size !== 1 ? "s" : ""} selected</p>
          ) : null}
        </div>
      )}

      {/* Step 3: Sit-in */}
      {step === "sit-in" && (
        <div className="space-y-5">
          <div className="grid grid-cols-2 gap-3">
            {(["zoom", "physical"] as const).map((m) => {
              const selected = sitInMethod === m;
              const Icon = m === "zoom" ? Video : BookOpen;
              return (
                <button
                  key={m}
                  type="button"
                  onClick={() => { setSitInMethod(m); setSitInCourseId(""); setSelectedSitIn(new Set()); }}
                  className={`relative flex flex-col items-start gap-1 rounded-xl border-2 p-4 text-left transition-all duration-200 ${
                    selected
                      ? "border-[var(--color-wi-primary)] bg-blue-50/40 shadow-sm"
                      : "border-gray-200 bg-white hover:border-gray-300 hover:bg-gray-50/50"
                  }`}
                >
                  {selected ? (
                    <span className="absolute right-3 top-3 flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-white">
                      <Check className="h-3 w-3" strokeWidth={3} />
                    </span>
                  ) : null}
                  <div className={`rounded-lg p-2 ${selected ? "bg-blue-100/70" : "bg-gray-100"}`}>
                    <Icon className={`h-5 w-5 ${selected ? "text-[var(--color-wi-primary)]" : "text-gray-500"}`} />
                  </div>
                  <span className={`mt-1 text-sm font-semibold ${selected ? "text-[var(--color-wi-primary)]" : "text-gray-900"}`}>
                    {m === "zoom" ? "Zoom" : "Physical course"}
                  </span>
                  <span className="text-xs leading-relaxed text-gray-500">
                    {m === "zoom" ? "Student attends via video call." : "Assign to a specific class session."}
                  </span>
                  {m === "physical" ? (
                    <span className="mt-1 text-[11px] font-medium text-amber-600">Requires session selection</span>
                  ) : (
                    <span className="mt-1 text-[11px] font-medium text-emerald-600">No session selection needed</span>
                  )}
                </button>
              );
            })}
          </div>

          {sitInMethod === "physical" && (
            <div className="space-y-4">
              {loadingSitIn ? (
                <SkeletonRows />
              ) : sitInCourseName ? (
                <>
                  <div className="rounded-lg border border-gray-200 bg-gray-50/50 px-4 py-3 text-sm">
                    <span className="text-gray-500">Sit-in course:</span>{" "}
                    <span className="font-medium text-gray-900">{sitInCourseName}</span>
                  </div>

                  {sitInCandidates.length > 0 && (
                    <div>
                      <p className="mb-2 text-sm font-medium text-gray-700">Available Sessions ({sitInCandidates.length})</p>
                      <div className="space-y-2 max-h-48 overflow-y-auto">
                        {sitInCandidates.map((c) => (
                          <SessionCard
                            key={c.id}
                            session={c}
                            selected={selectedSitIn.has(c.id)}
                            onToggle={() => toggleSitIn(c.id)}
                          />
                        ))}
                      </div>
                    </div>
                  )}

                  {sitInCandidates.length === 0 ? (
                    <p className="text-sm text-gray-400">No available sessions for sit-in.</p>
                  ) : null}
                </>
              ) : (
                <p className="text-sm text-gray-400">No sit-in resolution available for this student/subject.</p>
              )}
            </div>
          )}

          {sitInMethod === "zoom" && (
            <div className="flex items-start gap-3 rounded-lg border border-gray-100 bg-gray-50/30 px-4 py-3">
              <Info className="mt-0.5 h-4 w-4 shrink-0 text-gray-400" />
              <p className="text-sm leading-relaxed text-gray-600">
                The student will attend via Zoom. No physical sit-in sessions needed.
              </p>
            </div>
          )}
        </div>
      )}

      {/* Step 4: Confirm */}
      {step === "confirm" && (
        <div className="space-y-5">
          <div className="rounded-lg border border-gray-200 bg-gray-50/50 p-4 space-y-3">
            <div className="grid grid-cols-2 gap-3 text-sm">
              <div>
                <span className="text-gray-500">Student</span>
                <p className="font-medium text-gray-900">{student?.full_name} ({student?.wcode})</p>
              </div>
              <div>
                <span className="text-gray-500">Subject</span>
                <p className="font-medium text-gray-900">{selectedSubject?.code} — {selectedSubject?.name}</p>
              </div>
              <div>
                <span className="text-gray-500">Dates</span>
                <p className="font-medium text-gray-900">{dateFrom} to {dateTo}</p>
              </div>
              <div>
                <span className="text-gray-500">Sit-in</span>
                <p className="font-medium text-gray-900">
                  {sitInMethod === "zoom" ? "Zoom" : (
                    sitInCourseName ? `${sitInCourseName} (${selectedSitIn.size} session${selectedSitIn.size !== 1 ? "s" : ""})` : "Not assigned"
                  )}
                </p>
              </div>
            </div>
            <div>
              <span className="text-sm text-gray-500">Missed sessions ({selectedMissed.size})</span>
              <ul className="mt-1 space-y-0.5">
                {[...selectedMissed].map((id) => {
                  const session = missedSessionList.find((s) => s.id === id);
                  return session ? <li key={id} className="text-sm text-gray-700">{sessionLabel(session)}</li> : null;
                })}
              </ul>
            </div>
          </div>

          <div>
            <label htmlFor="staff-reason-category" className="mb-1.5 block text-sm font-medium text-gray-700">Reason Category</label>
            <Select id="staff-reason-category" placeholder="Select a reason..." value={reasonCategory} onChange={(e) => setReasonCategory(e.target.value)}>
              {(formConfig?.form.reason_categories ?? []).map((cat) => (
                <option key={cat.value} value={cat.value}>{cat.label}</option>
              ))}
            </Select>
          </div>

          <div>
            <label htmlFor="staff-reason" className="mb-1.5 block text-sm font-medium text-gray-700">Additional details (optional)</label>
            <textarea id="staff-reason" className="w-full rounded-sm border border-gray-300 px-3 py-2 text-sm" rows={3} value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Optional note..." />
          </div>
        </div>
      )}
    </Modal>
  );
}
