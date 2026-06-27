import { useEffect, useMemo, useState } from "react";
import { ChevronRight, ChevronLeft, Info } from "lucide-react";
import { apiJson } from "../../api/client";
import { loadSessionsInRange } from "../../features/absences/api/absenceFormApi";
import { useToast } from "../../hooks/useToast";
import { formatDate, formatTime } from "../../utils/date";
import { groupByDay, isDayGroupSelected, mergedSessionValue, getSelectedSessionsForGroup, splitMergedSessionValue } from "../../features/absences/domain/sessionGrouping";
import {
  sitInForMissedSession, groupWithSitInForMissedSession,
  availableSessionsForMissedSessions, unavailableSessionsForMissedSession,
  firstPriorityLevel, hasServerPriorityReveal,
  nextPriorityLevel, previousPriorityLevel, prioritiesForLevel,
  rootAvailableSessionsForMissedSessions,
  getSitInSessionGroupLabel, getReviewSitInLabel,
} from "../../features/absences/domain/sitInResolution";
import { selectedSitInCourseIDForGroup } from "../../features/absences/domain/submissionPayload";
import type { SubjectSessions, StudentLookupResponse, AbsenceFormConfig, SmsPreview } from "../../types";
import Button from "../ui/Button";
import Select from "../ui/Select";
import Modal from "../Modal";
import SubjectCard from "./SubjectCard";
import SmsConfirmModal from "./SmsConfirmModal";

type ModalStep = "subjects" | "sessions" | "confirm";

type Props = {
  onClose: () => void;
  onCreated: () => void;
};

const STEP_KEYS: ModalStep[] = ["subjects", "sessions", "confirm"];

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
              {isComplete ? <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}><path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" /></svg> : i + 1}
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
  const [step, setStep] = useState<ModalStep>("subjects");

  // Step 1: Student lookup + subject selection
  const [wcode, setWcode] = useState("");
  const [student, setStudent] = useState<StudentLookupResponse | null>(null);
  const [lookingUp, setLookingUp] = useState(false);
  const [selectedSubjectIds, setSelectedSubjectIds] = useState<string[]>([]);

  // Step 2: Sessions + sit-in
  const [sessions, setSessions] = useState<SubjectSessions[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [sessionsError, setSessionsError] = useState<string | null>(null);
  const [selectedSessionIds, setSelectedSessionIds] = useState<Set<string>>(new Set());
  const [sitInSelections, setSitInSelections] = useState<Record<string, string>>({});
  const [sitInPriorityLevels, setSitInPriorityLevels] = useState<Record<string, number>>({});
  const [sitInPriorityHistory, setSitInPriorityHistory] = useState<Record<string, Record<number, SubjectSessions>>>({});
  const [revealingPrioritySessionIds, setRevealingPrioritySessionIds] = useState<Set<string>>(new Set());

  // Step 3: Confirm
  const [formConfig, setFormConfig] = useState<AbsenceFormConfig | null>(null);
  const [reasonCategory, setReasonCategory] = useState("");
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [smsPreview, setSmsPreview] = useState<SmsPreview | null>(null);
  const [createdAbsenceIds, setCreatedAbsenceIds] = useState<string[]>([]);
  const [sendingSms, setSendingSms] = useState(false);

  const selectedSessionCount = useMemo(() => {
    let count = 0;
    for (const group of sessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      for (const dayGroup of groupByDay(group.sessions)) {
        if (isDayGroupSelected(dayGroup, selectedSessionIds)) count++;
      }
    }
    return count;
  }, [sessions, selectedSubjectIds, selectedSessionIds]);

  const missingSitIn = useMemo(() => {
    for (const group of sessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      for (const session of group.sessions) {
        if (!selectedSessionIds.has(session.id)) continue;
        const sitIn = sitInForMissedSession(group, session.id);
        if (sitIn?.sit_in_method === "physical" && !sitInSelections[session.id]) return true;
      }
    }
    return false;
  }, [sessions, selectedSubjectIds, selectedSessionIds, sitInSelections]);

  // Load sessions when entering step "sessions"
  useEffect(() => {
    if (step !== "sessions" || !student || selectedSubjectIds.length === 0) return;
    const controller = new AbortController();
    setSessionsLoading(true);
    setSessionsError(null);
    void loadSessionsInRange(student.wcode, "1970-01-01", "2100-01-01", { signal: controller.signal }, { bypassTiming: true })
      .then((data) => { if (!controller.signal.aborted) setSessions(data.subjects ?? []); })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setSessions([]);
        setSessionsError(error instanceof Error ? error.message : "Failed to load sessions");
      })
      .finally(() => { if (!controller.signal.aborted) setSessionsLoading(false); });
    return () => controller.abort();
  }, [step, student, selectedSubjectIds]);

  // Load form config when entering step "confirm"
  useEffect(() => {
    if (step !== "confirm" || formConfig) return;
    void apiJson<AbsenceFormConfig>("/api/v1/absence-form-config", { method: "GET" })
      .then((config) => setFormConfig(config))
      .catch(() => { addToast("error", "Failed to load form settings"); setFormConfig(null); });
  }, [step, formConfig, addToast]);

  function toggleSubject(subjectId: string) {
    setSelectedSubjectIds((current) =>
      current.includes(subjectId) ? current.filter((id) => id !== subjectId) : [...current, subjectId],
    );
  }

  function handleSessionGroupToggle(sessionIds: string[]) {
    setSelectedSessionIds((current) => {
      const selected = sessionIds.every((id) => current.has(id));
      if (selected) {
        const next = new Set(current);
        for (const id of sessionIds) next.delete(id);
        setSitInSelections((cs) => {
          const n = { ...cs };
          for (const id of sessionIds) delete n[id];
          return n;
        });
        return next;
      }
      const next = new Set(current);
      for (const id of sessionIds) next.add(id);
      return next;
    });
  }

  function handleSitInSelectForSessions(sessionIds: string[], sitInSessionId: string) {
    setSitInSelections((current) => {
      const next = { ...current };
      for (const id of sessionIds) {
        if (!sitInSessionId) delete next[id];
        else next[id] = sitInSessionId;
      }
      return next;
    });
  }

  async function handleNotAvailable(group: SubjectSessions, sessionId: string) {
    const currentLevel = sitInPriorityLevels[sessionId] || group.sit_in?.current_priority_level || firstPriorityLevel(group);
    if (student && hasServerPriorityReveal(group)) {
      setRevealingPrioritySessionIds((current) => new Set(current).add(sessionId));
      setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
      setSitInPriorityHistory((prev) => ({ ...prev, [sessionId]: { ...(prev[sessionId] ?? {}), [currentLevel]: group } }));
      try {
        const data = await loadSessionsInRange(
          student.wcode,
          undefined,
          undefined,
          undefined,
          { courseIds: [group.course_id], satVerbalAfterPriority: currentLevel },
        );
        const updatedGroup = data.subjects.find((subject) => subject.course_id === group.course_id);
        if (!updatedGroup) { addToast("error", "No more make-up times are available for this class."); return; }
        const updatedSessionGroup = groupWithSitInForMissedSession(updatedGroup, sessionId);
        const updatedLevel = updatedSessionGroup.sit_in?.current_priority_level ?? firstPriorityLevel(updatedSessionGroup);
        setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: updatedLevel }));
        setSitInPriorityHistory((prev) => ({ ...prev, [sessionId]: { ...(prev[sessionId] ?? {}), [updatedLevel]: updatedSessionGroup } }));
      } catch (error) {
        addToast("error", error instanceof Error ? error.message : "Couldn't load other make-up times");
      } finally {
        setRevealingPrioritySessionIds((current) => { const n = new Set(current); n.delete(sessionId); return n; });
      }
      return;
    }
    const nextLvl = nextPriorityLevel(group, currentLevel);
    if (nextLvl == null) return;
    setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: nextLvl }));
    setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
  }

  function handlePreviousPriority(group: SubjectSessions, sessionId: string) {
    const currentLevel = sitInPriorityLevels[sessionId] || group.sit_in?.current_priority_level || firstPriorityLevel(group);
    if (hasServerPriorityReveal(group)) {
      const history = sitInPriorityHistory[sessionId] ?? {};
      const previousLevel = Object.keys(history).map(Number).filter((lvl) => lvl < currentLevel).sort((a, b) => b - a)[0];
      const previousGroup = previousLevel !== undefined ? history[previousLevel] : undefined;
      if (!previousGroup) return;
      setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: previousLevel }));
      setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
      return;
    }
    const prevLvl = previousPriorityLevel(group, currentLevel);
    if (prevLvl == null) return;
    setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: prevLvl }));
    setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
  }

  function canAdvanceFromSubjects(): boolean {
    return !!student && selectedSubjectIds.length > 0;
  }

  function canAdvanceFromSessions(): boolean {
    return selectedSessionCount > 0;
  }

  function handleNext() {
    if (step === "subjects") {
      if (!canAdvanceFromSubjects()) {
        addToast("error", !student ? "Look up a student first" : "Select at least one subject");
        return;
      }
      setSelectedSessionIds(new Set());
      setSitInSelections({});
      setSitInPriorityLevels({});
      setSitInPriorityHistory({});
      setSessions([]);
      setStep("sessions");
    } else if (step === "sessions") {
      if (!canAdvanceFromSessions()) {
        addToast("error", "Select at least one missed class");
        return;
      }
      setStep("confirm");
    }
  }

  function handleBack() {
    if (step === "sessions") setStep("subjects");
    else if (step === "confirm") setStep("sessions");
  }

  async function lookupStudent() {
    if (!wcode.trim()) return;
    setLookingUp(true);
    setStudent(null);
    setSelectedSubjectIds([]);
    try {
      const data = await apiJson<StudentLookupResponse>(`/api/v1/absences/student-lookup?wcode=${encodeURIComponent(wcode.trim())}`, { method: "GET" });
      setStudent(data);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Student not found");
    } finally {
      setLookingUp(false);
    }
  }

  async function handleSubmit() {
    if (!student) return;
    setSubmitting(true);
    const created: string[] = [];
    let lastSms: SmsPreview | null = null;
    let lastCreatedId: string | null = null;

    for (const group of sessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      const selectedSessions = getSelectedSessionsForGroup(group, selectedSessionIds);
      if (selectedSessions.length === 0) continue;
      const dates = [...new Set(selectedSessions.map((s) => s.date))].sort();
      const dateFrom = dates[0];
      const dateTo = dates[dates.length - 1];
      if (!dateFrom || !dateTo) continue;

      const missedIds = selectedSessions.map((s) => s.id);
      const sitInSessionIds: string[] = [];
      let sitInMethod: string | undefined;

      for (const session of missedIds) {
        const selected = splitMergedSessionValue(sitInSelections[session]);
        for (const sid of selected) sitInSessionIds.push(sid);
        const sitIn = sitInForMissedSession(group, session);
        if (sitIn?.sit_in_method === "physical" || sitIn?.sit_in_method === "zoom") {
          sitInMethod = sitIn.sit_in_method;
        }
      }

      const uniqueSitInSessionIds = [...new Set(sitInSessionIds)];
      const sitInCourseId = selectedSitInCourseIDForGroup(group, missedIds, sitInSelections, sitInPriorityLevels, sitInPriorityHistory);

      try {
        const res = await apiJson<{ id: string; sms_preview?: SmsPreview }>("/api/v1/absences/staff-create", {
          method: "POST",
          body: JSON.stringify({
            wcode: student.wcode,
            subject_id: group.subject_id,
            course_id: group.course_id,
            date_from: dateFrom,
            date_to: dateTo,
            missed_session_ids: missedIds,
            sit_in_method: sitInMethod,
            sit_in_course_id: sitInCourseId,
            sit_in_session_ids: uniqueSitInSessionIds,
            reason_category: reasonCategory || undefined,
            reason: reason || undefined,
          }),
        });
        created.push(res.id);
        if (res.sms_preview && res.sms_preview.phones.length > 0 && !lastSms) {
          lastSms = res.sms_preview;
          lastCreatedId = res.id;
        }
      } catch (err) {
        addToast("error", `${group.subject_name || group.course_code}: ${err instanceof Error ? err.message : "Failed"}`);
      }
    }

    setSubmitting(false);
    if (created.length === 0) return;
    setCreatedAbsenceIds(created);

    if (lastSms && lastCreatedId) {
      setSmsPreview(lastSms);
      setCreatedAbsenceIds([lastCreatedId]);
    } else {
      addToast("success", `${created.length} absence${created.length !== 1 ? "s" : ""} created`);
      onCreated();
    }
  }

  async function handleSendSms() {
    if (createdAbsenceIds.length === 0) {
      addToast("error", "Missing absence ID, cannot send SMS");
      return;
    }
    setSendingSms(true);
    try {
      const res = await apiJson<{ sent: boolean; recipient_count: number }>(`/api/v1/absences/${createdAbsenceIds[0]}/send-success-sms`, { method: "POST" });
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
    addToast("success", "Absence(s) created successfully (SMS skipped)");
    onCreated();
  }

  if (smsPreview && createdAbsenceIds.length > 0) {
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
            {step !== "subjects" ? (
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

      {/* Step 1: Student + Subjects */}
      {step === "subjects" && (
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
                onChange={(e) => { setWcode(e.target.value); setStudent(null); setSelectedSubjectIds([]); }}
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

              {student.subjects.length > 0 ? (
                <div>
                  <label className="mb-2 block text-sm font-medium text-gray-700">Subjects</label>
                  <p className="mb-2 text-xs text-gray-500">Select one or more subjects</p>
                  <div className="divide-y divide-gray-100 rounded-lg border border-gray-200 overflow-hidden">
                    {student.subjects.map((subject) => (
                      <SubjectCard
                        key={subject.id}
                        id={subject.id}
                        name={subject.name}
                        code={subject.code}
                        selected={selectedSubjectIds.includes(subject.id)}
                        onToggle={() => toggleSubject(subject.id)}
                      />
                    ))}
                  </div>
                  {selectedSubjectIds.length > 0 ? (
                    <p className="mt-2 text-xs text-gray-500">{selectedSubjectIds.length} subject{selectedSubjectIds.length !== 1 ? "s" : ""} selected</p>
                  ) : null}
                </div>
              ) : (
                <p className="text-sm text-gray-500">No enrolled subjects found for this student.</p>
              )}
            </>
          ) : null}
        </div>
      )}

      {/* Step 2: Classes + Make-up */}
      {step === "sessions" && (
        <div className="space-y-4">
          {selectedSubjectIds.length > 0 && student ? (
            <>
              {sessionsLoading ? (
                <SkeletonRows count={3} />
              ) : sessionsError ? (
                <p role="alert" className="text-sm text-red-600">{sessionsError}</p>
              ) : sessions.filter((s) => selectedSubjectIds.includes(s.subject_id)).length === 0 ? (
                <p className="text-sm text-gray-400">No classes found for the selected subjects.</p>
              ) : (
                <div className="space-y-4">
                  {sessions.filter((s) => selectedSubjectIds.includes(s.subject_id)).map((group) => {
                    const sessionGroups = groupByDay(group.sessions.filter((s) => !s.already_absent));
                    const groupLabel = group.subject_name?.trim() || group.course_name?.trim() || group.course_code;
                    if (sessionGroups.length === 0) return null;
                    return (
                      <div key={group.course_id} className="rounded-lg border border-gray-200 bg-white overflow-hidden shadow-sm">
                        <div className="flex items-center justify-between gap-2 border-b border-gray-200 bg-gray-50/50 px-4 py-3">
                          <span className="text-sm font-semibold text-gray-900 truncate">{groupLabel} ({sessionGroups.length} class day{sessionGroups.length !== 1 ? "s" : ""})</span>
                          <span className="text-xs font-semibold text-gray-500 shrink-0">{sessionGroups.filter((g) => isDayGroupSelected(g, selectedSessionIds)).length} selected</span>
                        </div>
                        <div className="space-y-2 p-4">
                          {sessionGroups.map((dayGroup) => {
                            const sessionIds = dayGroup.items.map((item) => item.id);
                            const selected = isDayGroupSelected(dayGroup, selectedSessionIds);
                            const firstSessionId = sessionIds[0];
                            const sessionGroup = groupWithSitInForMissedSession(group, firstSessionId);
                            const sitIn = sessionGroup.sit_in;
                            const hasPriorities = Boolean(sitIn?.priorities && sitIn.priorities.length > 0);
                            const baseLevel = sitIn?.current_priority_level || firstPriorityLevel(sessionGroup);
                            const currentLevel = sitInPriorityLevels[firstSessionId] || baseLevel;
                            const priorityGroup = sitInPriorityHistory[firstSessionId]?.[currentLevel] ?? sessionGroup;
                            const currentPriorities = hasPriorities ? prioritiesForLevel(priorityGroup, currentLevel) : [];
                            const currentSitIn = sitInSelections[firstSessionId] || "";

                            return (
                              <div key={dayGroup.id} className={`rounded-lg border px-4 py-3 transition-colors ${selected ? "border-blue-300 bg-blue-50/30" : "border-gray-200 bg-white"}`}>
                                <div className="flex items-center gap-3">
                                  <input
                                    type="checkbox"
                                    id={`staff-session-${dayGroup.id}`}
                                    checked={selected}
                                    onChange={() => handleSessionGroupToggle(sessionIds)}
                                    className="h-4 w-4 shrink-0 rounded border-gray-300 text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                                  />
                                  <label htmlFor={`staff-session-${dayGroup.id}`} className="min-w-0 cursor-pointer flex-1">
                                    <span className="text-sm font-medium text-gray-900">
                                      {formatDate(dayGroup.date)} {formatTime(dayGroup.start_at)}-{formatTime(dayGroup.end_at)}
                                    </span>
                                  </label>
                                </div>

                                {selected && sitIn ? (
                                  <div className="mt-3 pl-7">
                                    {sitIn.sit_in_method === "physical" && hasPriorities ? (
                                      (() => {
                                        const serverReveal = hasServerPriorityReveal(priorityGroup);
                                        const nextLevelValue = serverReveal ? Boolean(sitIn.has_next_priority) : nextPriorityLevel(priorityGroup, currentLevel) !== null;
                                        const hasPreviousPriority = serverReveal
                                          ? Object.keys(sitInPriorityHistory[firstSessionId] ?? {}).some((l) => Number(l) < currentLevel)
                                          : previousPriorityLevel(priorityGroup, currentLevel) !== null;
                                        const revealing = revealingPrioritySessionIds.has(firstSessionId);
                                        const available = currentPriorities.flatMap((p) => availableSessionsForMissedSessions(p, sessionIds));
                                        const unavailable = currentPriorities.flatMap((p) => unavailableSessionsForMissedSession(p, firstSessionId));

                                        if (available.length === 0 && currentPriorities.length > 0 && unavailable.length === 0) {
                                          return (
                                            <div className="text-sm text-gray-500">
                                              <p className="font-medium">No more options available</p>
                                              <p className="text-xs text-gray-400 mt-0.5">Admin will contact student to arrange a make-up class.</p>
                                            </div>
                                          );
                                        }

                                        return (
                                          <div className="rounded-lg border border-gray-200 bg-gray-50/50 p-3">
                                            {(hasPreviousPriority || nextLevelValue) && (
                                              <div className="mb-3 flex items-center gap-1.5">
                                                {hasPreviousPriority && (
                                                  <button
                                                    type="button"
                                                    disabled={revealing}
                                                    onClick={() => handlePreviousPriority(priorityGroup, firstSessionId)}
                                                    className="inline-flex h-7 items-center gap-1 rounded-full px-2.5 text-xs font-medium text-gray-500 transition hover:bg-white hover:text-gray-700 hover:shadow-sm"
                                                  >
                                                    <ChevronLeft className="h-3.5 w-3.5" />
                                                    Back
                                                  </button>
                                                )}
                                                {nextLevelValue && (
                                                  <button
                                                    type="button"
                                                    disabled={revealing}
                                                    onClick={() => void handleNotAvailable(priorityGroup, firstSessionId)}
                                                    className="inline-flex h-7 items-center gap-1 rounded-full px-2.5 text-xs font-semibold text-gray-500 transition hover:bg-white hover:text-gray-700 hover:shadow-sm"
                                                  >
                                                    <span>{revealing ? "Loading..." : "See other times"}</span>
                                                    {!revealing && <ChevronRight className="h-3.5 w-3.5" />}
                                                  </button>
                                                )}
                                              </div>
                                            )}
                                            {available.length > 0 ? (
                                              <div>
                                                <label className="mb-1 block text-xs font-medium text-gray-500" htmlFor={`staff-sit-in-${firstSessionId}`}>
                                                  Make-up class
                                                </label>
                                                <select
                                                  id={`staff-sit-in-${firstSessionId}`}
                                                  value={currentSitIn}
                                                  onChange={(e) => handleSitInSelectForSessions(sessionIds, e.target.value)}
                                                  className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                                                >
                                                  <option value="">Not yet selected</option>
                                                  {currentPriorities.flatMap((p) =>
                                                    groupByDay(availableSessionsForMissedSessions(p, sessionIds)).map((optGroup) => (
                                                      <option key={`${p.sit_in_course?.id ?? "course"}:${optGroup.id}`} value={mergedSessionValue(optGroup.items)}>
                                                        {getSitInSessionGroupLabel(optGroup.items, p.sit_in_course, groupLabel, sessions)}
                                                      </option>
                                                    ))
                                                  )}
                                                </select>
                                              </div>
                                            ) : unavailable.length > 0 ? (
                                              <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700">
                                                <p className="font-semibold">Checked same-number slot:</p>
                                                <ul className="mt-1 space-y-1">
                                                  {unavailable.map((u, idx) => (
                                                    <li key={`${u.reason_code}-${idx}`}>
                                                      <span className="font-medium">{getSitInSessionGroupLabel(u.session ? [u.session] : [], currentPriorities[0]?.sit_in_course, groupLabel, sessions)}</span>
                                                      <span className="text-amber-600"> — {u.reason}</span>
                                                    </li>
                                                  ))}
                                                </ul>
                                              </div>
                                            ) : null}
                                          </div>
                                        );
                                      })()
                                    ) : sitIn.sit_in_method === "physical" && !hasPriorities ? (
                                      <div>
                                        <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-amber-600 mb-2">
                                          Pick a make-up class
                                        </div>
                                        <p className="mb-2 text-xs text-gray-500 truncate">
                                          Sit-in: {getSitInSessionGroupLabel(
                                            rootAvailableSessionsForMissedSessions(sitIn, sessionIds),
                                            sitIn.sit_in_course,
                                            groupLabel,
                                            sessions,
                                          ) || sitIn.sit_in_course?.name || "To arrange"}
                                        </p>
                                        <select
                                          value={currentSitIn}
                                          onChange={(e) => handleSitInSelectForSessions(sessionIds, e.target.value)}
                                          className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                                        >
                                          <option value="">— Not yet —</option>
                                          {groupByDay(rootAvailableSessionsForMissedSessions(sitIn, sessionIds)).map((optGroup) => (
                                            <option key={optGroup.id} value={mergedSessionValue(optGroup.items)}>
                                              {getSitInSessionGroupLabel(optGroup.items, sitIn.sit_in_course, groupLabel, sessions)}
                                            </option>
                                          ))}
                                        </select>
                                      </div>
                                    ) : sitIn.sit_in_method === "zoom" ? (
                                      <div className="flex items-start gap-2 text-sm text-gray-700">
                                        <span className="inline-flex h-5 w-5 items-center justify-center rounded-full bg-blue-100 text-[10px] font-bold text-blue-700">Z</span>
                                        <span>Online make-up (Zoom) — no session selection needed</span>
                                      </div>
                                    ) : (
                                      <div className="text-sm text-gray-500">
                                        <p className="font-medium">To arrange</p>
                                        <p className="text-xs text-gray-400 mt-0.5">Admin will contact the student.</p>
                                      </div>
                                    )}
                                  </div>
                                ) : null}
                              </div>
                            );
                          })}
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
              {selectedSessionCount > 0 ? (
                <p className="text-xs text-gray-500">{selectedSessionCount} class day{selectedSessionCount !== 1 ? "s" : ""} selected</p>
              ) : null}
            </>
          ) : null}
        </div>
      )}

      {/* Step 3: Confirm */}
      {step === "confirm" && (
        <div className="space-y-5">
          <div className="rounded-lg border border-gray-200 bg-gray-50/50 p-4 space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <span className="text-xs text-gray-500">Student</span>
                <p className="text-sm font-medium text-gray-900">{student?.full_name} ({student?.wcode})</p>
              </div>
              <span className="text-xs text-gray-500">{selectedSubjectIds.length} subject{selectedSubjectIds.length !== 1 ? "s" : ""}</span>
            </div>

            {sessions.filter((s) => selectedSubjectIds.includes(s.subject_id)).map((group) => {
              const selectedSessions = getSelectedSessionsForGroup(group, selectedSessionIds);
              if (selectedSessions.length === 0) return null;
              const groupLabel = group.subject_name?.trim() || group.course_name?.trim() || group.course_code;
              return (
                <div key={group.course_id}>
                  <p className="text-sm font-semibold text-gray-900">{groupLabel}</p>
                  <div className="mt-1 space-y-1">
                    {groupByDay(selectedSessions).map((dayGroup) => (
                      <p key={dayGroup.id} className="text-xs text-gray-600">
                        {formatDate(dayGroup.date)} {formatTime(dayGroup.start_at)}–{formatTime(dayGroup.end_at)}
                        <span className="text-gray-400"> — Make-up: </span>
                        <span className="font-medium text-gray-800">
                          {getReviewSitInLabel(dayGroup.items[0], group, sitInSelections, sitInPriorityLevels, sitInPriorityHistory, sessions)}
                        </span>
                      </p>
                    ))}
                  </div>
                </div>
              );
            })}
          </div>

          {missingSitIn ? (
            <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700 flex items-start gap-2">
              <Info className="mt-0.5 h-4 w-4 shrink-0" />
              <span>Some classes don't have a make-up class selected. You can still create the absence.</span>
            </div>
          ) : null}

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
