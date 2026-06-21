import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, BookOpen, Check, Info, MapPin, Phone, Video } from "lucide-react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import type { Course } from "../../types";
import Button from "../ui/Button";
import Select from "../ui/Select";
import Modal from "../Modal";

type SmsPreview = {
  student_phone: string;
  parent_phone: string;
  student_sms: string;
  parent_sms: string;
};

type ModalStep = "form" | "confirmation";

type OverrideMethod = "zoom" | "physical";

type CandidateSession = {
  id: string;
  start_at: string;
  end_at: string;
  room_name?: string;
  capacity_warning?: boolean;
};

type Props = {
  absenceId: string;
  version: number;
  currentMethod?: string | null;
  currentCourseId?: string | null;
  onClose: () => void;
  onSaved: () => void;
};

const INSTITUTE_TIME_ZONE = "Asia/Bangkok";

function displayDate(value: string): string {
  return new Date(value).toLocaleDateString("en-GB", {
    day: "numeric", month: "short", timeZone: INSTITUTE_TIME_ZONE,
  });
}

function displayTime(value: string): string {
  return new Date(value).toLocaleTimeString("en-GB", {
    hour: "2-digit", minute: "2-digit",
    timeZone: INSTITUTE_TIME_ZONE,
  });
}

export default function OverrideSitInModal({
  absenceId,
  version,
  currentMethod,
  currentCourseId,
  onClose,
  onSaved,
}: Props) {
  const { addToast } = useToast();
  const [method, setMethod] = useState<OverrideMethod>(
    currentMethod === "zoom" ? "zoom" : "physical",
  );
  const [courseID, setCourseID] = useState(currentCourseId ?? "");
  const [candidates, setCandidates] = useState<CandidateSession[]>([]);
  const [selectedSessions, setSelectedSessions] = useState<Set<string>>(new Set());
  const [courses, setCourses] = useState<Course[]>([]);
  const [coursesLoading, setCoursesLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [step, setStep] = useState<ModalStep>("form");
  const [smsPreview, setSmsPreview] = useState<SmsPreview | null>(null);
  const [smsSending, setSmsSending] = useState(false);

  useEffect(() => {
    setCoursesLoading(true);
    void apiJson<Course[]>("/api/v1/courses/public", { method: "GET" })
      .then((fetched) => {
        setCourses(fetched);
        if (currentCourseId && !fetched.some((c) => c.id === currentCourseId)) {
          setCourseID("");
        }
      })
      .catch(() => setCourses([]))
      .finally(() => setCoursesLoading(false));
  }, []);

  useEffect(() => {
    if (method !== "physical" || !courseID) {
      setCandidates([]);
      return;
    }
    void apiJson<CandidateSession[]>(
      `/api/v1/absences/${absenceId}/sit-in-candidates?course_id=${encodeURIComponent(courseID)}`,
      { method: "GET" },
    )
      .then((rows) => { setCandidates(rows); })
      .catch(() => setCandidates([]));
  }, [courseID, method, absenceId]);

  const selectedCourseName = useMemo(() => {
    const c = courses.find((c) => c.id === courseID);
    return c?.subject_name || (c ? `${c.code} - ${c.name}` : null);
  }, [courses, courseID]);

  const hasCurrentState = !!(currentMethod || currentCourseId);
  const currentLabel = currentMethod === "zoom" ? "Zoom" : currentMethod === "physical" ? "Manual course" : null;

  function handleClose() {
    if (step === "confirmation") {
      onSaved();
    }
    onClose();
  }

  function toggleSession(id: string) {
    setSelectedSessions((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function save() {
    setSaving(true);
    try {
      await apiJson(`/api/v1/absences/${absenceId}/sit-in`, {
        method: "PUT",
        body: JSON.stringify({
          method,
          expected_version: version,
          reason: "Override by admin",
          ...(method === "physical"
            ? { sit_in_course_id: courseID, sit_in_session_ids: [...selectedSessions] }
            : {}),
        }),
      });

      let preview: SmsPreview | null = null;
      try {
        preview = await apiJson<SmsPreview>(
          `/api/v1/absences/${absenceId}/sms-preview`,
          { method: "GET" },
        );
      } catch {
        preview = null;
      }
      setSmsPreview(preview);
      setStep("confirmation");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Override failed");
    } finally {
      setSaving(false);
    }
  }

  async function sendSms() {
    setSmsSending(true);
    try {
      await apiJson(`/api/v1/absences/${absenceId}/sms-notify`, { method: "POST" });
      addToast("success", "Sit-in updated · SMS sent");
      onSaved();
      onClose();
    } catch {
      addToast("success", "Sit-in updated");
      onSaved();
      onClose();
    }
  }

  function skipSms() {
    addToast("success", "Sit-in updated");
    onSaved();
    onClose();
  }

  const canSave = method !== "physical" || (courseID && selectedSessions.size > 0);

  return (
    <Modal
      title="Override Sit-in"
      onClose={handleClose}
      size="xl"
      footer={
        step === "confirmation" ? (
          <>
            <div className="flex-1" />
            <Button variant="secondary" onClick={skipSms}>Skip</Button>
            <Button loading={smsSending} onClick={() => void sendSms()}>Send SMS</Button>
          </>
        ) : (
          <>
            <div className="flex-1 text-sm text-gray-500">
              {method === "zoom" ? (
                <span className="inline-flex items-center gap-1.5">
                  <Video className="h-4 w-4" /> Zoom
                </span>
              ) : (
                <span className="inline-flex items-center gap-1.5">
                  <BookOpen className="h-4 w-4" />
                  {selectedCourseName ?? "No course selected"}
                  {selectedCourseName ? ` · ${selectedSessions.size} session${selectedSessions.size !== 1 ? "s" : ""}` : ""}
                </span>
              )}
            </div>
            <Button variant="secondary" onClick={onClose}>Cancel</Button>
            <Button disabled={!canSave} loading={saving} onClick={() => void save()}>Save Override</Button>
          </>
        )
      }
    >
      {step === "confirmation" ? (
        <div className="space-y-6">
          <div className="flex items-center gap-3 rounded-xl border border-emerald-200 bg-emerald-50/50 px-5 py-4">
            <span className="flex h-8 w-8 items-center justify-center rounded-full bg-emerald-500 text-white">
              <Check className="h-5 w-5" strokeWidth={3} />
            </span>
            <div>
              <p className="text-sm font-semibold text-emerald-800">Override saved</p>
              <p className="text-sm text-emerald-600">Send an SMS notification to the student and parent?</p>
            </div>
          </div>

          {smsPreview ? (
            <>
              <div className="space-y-3">
                <p className="text-xs font-semibold uppercase tracking-wider text-gray-500">Recipients</p>
                <div className="rounded-lg border border-gray-200">
                  <div className="flex items-center gap-3 border-b border-gray-100 px-4 py-3">
                    <Phone className="h-4 w-4 shrink-0 text-gray-400" />
                    <div className="min-w-0 flex-1">
                      <p className="text-xs text-gray-500">Student</p>
                      <p className="text-sm font-medium text-gray-900">{smsPreview.student_phone}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-3 px-4 py-3">
                    <Phone className="h-4 w-4 shrink-0 text-gray-400" />
                    <div className="min-w-0 flex-1">
                      <p className="text-xs text-gray-500">Parent</p>
                      <p className="text-sm font-medium text-gray-900">{smsPreview.parent_phone}</p>
                    </div>
                  </div>
                </div>
              </div>

              <div className="space-y-2">
                <p className="text-xs font-semibold uppercase tracking-wider text-gray-500">Message preview</p>
                <div className="rounded-lg border border-gray-200 bg-gray-50/50 px-4 py-3">
                  <p className="whitespace-pre-wrap text-sm leading-relaxed text-gray-700">{smsPreview.student_sms}</p>
                </div>
              </div>
            </>
          ) : (
            <div className="flex items-start gap-3 rounded-lg border border-gray-200 px-4 py-3">
              <Info className="mt-0.5 h-4 w-4 shrink-0 text-gray-400" />
              <p className="text-sm text-gray-500">SMS preview unavailable. You can still send the notification.</p>
            </div>
          )}
        </div>
      ) : (
        <div className="space-y-6">
          {hasCurrentState ? (
            <div className="rounded-lg border border-gray-200 bg-gray-50/50 px-4 py-3 text-sm text-gray-600">
              <span className="font-medium text-gray-700">Current:</span>{" "}
              {currentLabel ?? "None"}
              {currentCourseId && selectedCourseName ? ` — ${selectedCourseName}` : ""}
            </div>
          ) : null}

          <div className="grid grid-cols-2 gap-3">
            {(["zoom", "physical"] as OverrideMethod[]).map((m) => {
              const selected = method === m;
              const Icon = m === "zoom" ? Video : BookOpen;
              return (
                <button
                  key={m}
                  type="button"
                  onClick={() => { setMethod(m); setSelectedSessions(new Set()); }}
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
                    {m === "zoom" ? "Zoom" : "Manual course"}
                  </span>
                  <span className="text-xs leading-relaxed text-gray-500">
                    {m === "zoom"
                      ? "Student attends via video call. No physical class required."
                      : "Assign the student to a specific class session."}
                  </span>
                  {m === "physical" ? (
                    <span className="mt-1 text-[11px] font-medium text-amber-600">Requires course + session selection</span>
                  ) : (
                    <span className="mt-1 text-[11px] font-medium text-emerald-600">No session selection needed</span>
                  )}
                </button>
              );
            })}
          </div>

          {method === "physical" ? (
            <div className="space-y-5">
              <div>
                <label htmlFor="sit-in-course" className="mb-1.5 block text-sm font-medium text-gray-700">Course</label>
                {coursesLoading ? (
                  <div className="h-[38px] animate-pulse rounded-sm bg-gray-200" />
                ) : (
                  <Select
                    id="sit-in-course"
                    placeholder="Select a course"
                    value={courseID}
                    onChange={(e) => { setCourseID(e.target.value); setSelectedSessions(new Set()); }}
                  >
                    {courses.map((course) => (
                      <option key={course.id} value={course.id}>
                        {course.subject_name || `${course.code} - ${course.name}`}
                      </option>
                    ))}
                  </Select>
                )}
              </div>

              {candidates.length > 0 ? (
                <div>
                  <p className="mb-2 text-sm font-medium text-gray-700">Sessions ({candidates.length})</p>
                  <div className="space-y-2">
                    {candidates.map((candidate) => {
                      const selected = selectedSessions.has(candidate.id);
                      return (
                        <button
                          key={candidate.id}
                          type="button"
                          onClick={() => toggleSession(candidate.id)}
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
                            <span className="text-sm font-medium text-gray-900">
                              {displayDate(candidate.start_at)} &nbsp;
                              <span className="font-normal">{displayTime(candidate.start_at)} – {displayTime(candidate.end_at)}</span>
                            </span>
                            {candidate.room_name ? (
                              <span className="ml-3 inline-flex items-center gap-1 text-xs text-gray-500">
                                <MapPin className="h-3 w-3" /> {candidate.room_name}
                              </span>
                            ) : null}
                          </div>
                          {candidate.capacity_warning ? (
                            <span className="inline-flex shrink-0 items-center gap-1 rounded-full bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-700">
                              <AlertTriangle className="h-3 w-3" /> Near capacity
                            </span>
                          ) : null}
                        </button>
                      );
                    })}
                  </div>
                </div>
              ) : null}

              {courseID && !coursesLoading && candidates.length === 0 ? (
                <p className="text-sm text-gray-400">No available sessions for this course.</p>
              ) : null}
            </div>
          ) : (
            <div className="flex items-start gap-3 rounded-lg border border-gray-100 bg-gray-50/30 px-4 py-3">
              <Info className="mt-0.5 h-4 w-4 shrink-0 text-gray-400" />
              <p className="text-sm leading-relaxed text-gray-600">
                The student will attend via Zoom. No physical sit-in sessions are needed.
                The sit-in record will be marked as <span className="font-medium text-gray-700">Zoom</span>.
              </p>
            </div>
          )}


        </div>
      )}
    </Modal>
  );
}
