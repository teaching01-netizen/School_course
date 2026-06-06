import { useState, useCallback, useEffect, useRef } from "react";
import { ApiRequestError, apiJson } from "../api/client";
import { usePreflight } from "./usePreflight";
import usePreflightGate from "./usePreflightGate";
import { localDateTimeToUTCISO } from "../types";
import { utcISOToZoneLocalInput } from "../utils/timezone";
import type { Session, AttendanceOverride } from "../types";

export interface EditSessionForm {
  course_id: string;
  room_id: string;
  teacher_id: string;
  start_local: string;
  end_local: string;
}

const emptyForm: EditSessionForm = {
  course_id: "", room_id: "", teacher_id: "", start_local: "", end_local: "",
};

export interface UseEditSessionReturn {
  open: boolean;
  session: Session | null;
  form: EditSessionForm;
  setForm: React.Dispatch<React.SetStateAction<EditSessionForm>>;
  preflight: ReturnType<typeof usePreflight>;
  gate: ReturnType<typeof usePreflightGate>;
  saving: boolean;
  attendanceOverrides: AttendanceOverride[];
  attendanceOverridesLoaded: boolean;
  openModal: (sess: Session) => void;
  closeModal: () => void;
  submit: () => Promise<void>;
}

export function useEditSession(
  onSuccess: () => void,
  addToast: (type: "success" | "error" | "warning" | "info", msg: string) => void,
  instituteTZ: string,
): UseEditSessionReturn {
  const [open, setOpen] = useState(false);
  const [session, setSession] = useState<Session | null>(null);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<EditSessionForm>(emptyForm);
  const [attendanceOverrides, setAttendanceOverrides] = useState<AttendanceOverride[]>([]);
  const [attendanceOverridesLoaded, setAttendanceOverridesLoaded] = useState(false);

  const preflight = usePreflight();
  const gate = usePreflightGate(preflight, {
    requiredFields: [form.course_id, form.teacher_id, form.start_local, form.end_local],
  });

  const onSuccessRef = useRef(onSuccess);
  onSuccessRef.current = onSuccess;
  const attendanceOverridesRef = useRef(attendanceOverrides);
  attendanceOverridesRef.current = attendanceOverrides;
  const attendanceOverridesLoadedRef = useRef(attendanceOverridesLoaded);
  attendanceOverridesLoadedRef.current = attendanceOverridesLoaded;
  const preflightCheckIdRef = useRef(0);

  const openModal = useCallback((sess: Session) => {
    const zone = instituteTZ;
    setSession(sess);
    setAttendanceOverrides([]);
    setAttendanceOverridesLoaded(false);
    setForm({
      course_id: sess.course_id,
      room_id: sess.room_id ?? "",
      teacher_id: sess.teacher_id,
      start_local: utcISOToZoneLocalInput(sess.start_at, zone) ?? "",
      end_local: utcISOToZoneLocalInput(sess.end_at, zone) ?? "",
    });
    preflight.reset();
    setOpen(true);
  }, [instituteTZ]);

  const closeModal = useCallback(() => {
    setOpen(false);
    setSession(null);
    setForm(emptyForm);
    setAttendanceOverrides([]);
    setAttendanceOverridesLoaded(false);
  }, []);

  // Load attendance overrides when modal opens
  const loadAttendanceOverrides = useCallback(async (sessionId: string) => {
    try {
      const overrides = await apiJson<AttendanceOverride[]>(
        `/api/v1/sessions/${sessionId}/attendance`,
        { method: "GET" },
      );
      setAttendanceOverrides(overrides);
      setAttendanceOverridesLoaded(true);
    } catch {
      setAttendanceOverrides([]);
      setAttendanceOverridesLoaded(false);
    }
  }, []);

  useEffect(() => {
    if (!open || !session) return;
    void loadAttendanceOverrides(session.id);
  }, [open, session?.id]);

  // Run preflight when form changes — reads latest refs to avoid closure traps
  useEffect(() => {
    if (!open || !session) {
      preflight.reset();
      return;
    }
    const startISO = localDateTimeToUTCISO(form.start_local, instituteTZ);
    const endISO = localDateTimeToUTCISO(form.end_local, instituteTZ);
    if (!startISO || !endISO || endISO <= startISO) {
      preflight.reset();
      return;
    }
    const payload: {
      session_id: string;
      course_id: string;
      room_id: string | null;
      teacher_id: string;
      start_at: string;
      end_at: string;
      included_student_ids?: string[];
      excluded_student_ids?: string[];
    } = {
      session_id: session.id,
      course_id: form.course_id,
      room_id: form.room_id || null,
      teacher_id: form.teacher_id,
      start_at: startISO,
      end_at: endISO,
    };
    if (attendanceOverridesLoadedRef.current) {
      payload.included_student_ids = attendanceOverridesRef.current
        .filter((o) => o.status === "included")
        .map((o) => o.student_id);
      payload.excluded_student_ids = attendanceOverridesRef.current
        .filter((o) => o.status === "excluded")
        .map((o) => o.student_id);
    }
    const thisCallId = ++preflightCheckIdRef.current;
    preflight.check(payload).then(() => {
      if (thisCallId !== preflightCheckIdRef.current) return;
    });
  }, [open, session?.id, form.course_id, form.room_id, form.teacher_id, form.start_local, form.end_local, instituteTZ]);

  const submit = useCallback(async () => {
    if (!session) return;
    if (!gate.canSave) return;
    const startISO = localDateTimeToUTCISO(form.start_local, instituteTZ);
    const endISO = localDateTimeToUTCISO(form.end_local, instituteTZ);
    if (!startISO || !endISO) {
      addToast("error", "Start and End are required");
      return;
    }
    setSaving(true);
    try {
      await apiJson(`/api/v1/sessions/${session.id}`, {
        method: "PATCH",
        body: JSON.stringify({
          expected_version: session.version,
          course_id: form.course_id,
          room_id: form.room_id || null,
          teacher_id: form.teacher_id,
          start_at: startISO,
          end_at: endISO,
        }),
      });
      addToast("success", "Updated session");
      closeModal();
      onSuccessRef.current();
    } catch (err) {
      if (err instanceof ApiRequestError) {
        if (err.code === "stale_edit") {
          addToast("error", "Stale edit: reloaded latest session. Please review and save again.");
          const reloaded = await apiJson<Session[]>(`/api/v1/sessions?ids=${session.id}`, { method: "GET" });
          const updated = reloaded[0];
          if (updated) {
            setSession(updated);
            setForm({
              course_id: updated.course_id,
              room_id: updated.room_id ?? "",
              teacher_id: updated.teacher_id,
              start_local: utcISOToZoneLocalInput(updated.start_at, instituteTZ) ?? "",
              end_local: utcISOToZoneLocalInput(updated.end_at, instituteTZ) ?? "",
            });
          }
          onSuccessRef.current();
          return;
        }
        addToast("error", `${err.code}: ${err.message}`);
        return;
      }
      addToast("error", err instanceof Error ? err.message : "Update failed");
    } finally {
      setSaving(false);
    }
  }, [session?.id, gate.canSave, form, instituteTZ, addToast, closeModal]);

  return {
    open,
    session,
    form,
    setForm,
    preflight,
    gate,
    saving,
    attendanceOverrides,
    attendanceOverridesLoaded,
    openModal,
    closeModal,
    submit,
  };
}
