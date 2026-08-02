import { useState, useCallback, useRef, useMemo } from "react";
import { ApiRequestError, apiJson } from "@/api/client";
import { usePreflight } from "./usePreflight";
import { useDebouncedPreflight } from "./useDebouncedPreflight";
import usePreflightGate from "./usePreflightGate";
import { localDateTimeToUTCISO } from "../domain/time";
import type { PreflightParams } from "./usePreflight";

export interface CreateSessionForm {
  course_id: string;
  room_id: string;
  teacher_id: string;
  start_local: string;
  end_local: string;
}

const emptyForm: CreateSessionForm = {
  course_id: "", room_id: "", teacher_id: "", start_local: "", end_local: "",
};

export interface UseCreateSessionReturn {
  open: boolean;
  form: CreateSessionForm;
  setForm: React.Dispatch<React.SetStateAction<CreateSessionForm>>;
  preflight: ReturnType<typeof usePreflight>;
  gate: ReturnType<typeof usePreflightGate>;
  creating: boolean;
  openModal: (defaults?: { course_id?: string; teacher_id?: string }) => void;
  closeModal: () => void;
  submit: () => Promise<void>;
}

export function useCreateSession(
  onSuccess: () => void,
  addToast: (type: "success" | "error" | "warning" | "info", msg: string) => void,
  instituteTZ: string,
): UseCreateSessionReturn {
  const [open, setOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState<CreateSessionForm>(emptyForm);
  const submittingRef = useRef(false);
  const onSuccessRef = useRef(onSuccess);
  onSuccessRef.current = onSuccess;

  const preflight = usePreflight();
  const gate = usePreflightGate(preflight, {
    requiredFields: [form.course_id, form.teacher_id, form.start_local, form.end_local],
  });

  const openModal = useCallback((defaults?: { course_id?: string; teacher_id?: string }) => {
    setForm({
      course_id: defaults?.course_id ?? "",
      room_id: "",
      teacher_id: defaults?.teacher_id ?? "",
      start_local: "",
      end_local: "",
    });
    preflight.reset();
    setOpen(true);
  }, []);

  const closeModal = useCallback(() => {
    setOpen(false);
    setForm(emptyForm);
  }, []);

  const debouncedParams = useMemo<PreflightParams | null>(() => {
    if (!open) return null;
    if (!form.course_id || !form.teacher_id || !form.start_local || !form.end_local) return null;
    const startISO = localDateTimeToUTCISO(form.start_local, instituteTZ);
    const endISO = localDateTimeToUTCISO(form.end_local, instituteTZ);
    if (!startISO || !endISO || endISO <= startISO) return null;
    return {
      course_id: form.course_id,
      teacher_id: form.teacher_id,
      room_id: form.room_id || null,
      start_at: startISO,
      end_at: endISO,
      session_id: null,
    };
  }, [open, form.course_id, form.room_id, form.teacher_id, form.start_local, form.end_local, instituteTZ]);

  useDebouncedPreflight(preflight, debouncedParams, { enabled: open });

  const submit = useCallback(async () => {
    if (submittingRef.current || !gate.canSave) return;
    const startISO = localDateTimeToUTCISO(form.start_local, instituteTZ);
    const endISO = localDateTimeToUTCISO(form.end_local, instituteTZ);
    if (!startISO || !endISO || endISO <= startISO) {
      addToast("error", "Invalid start/end time");
      return;
    }
    submittingRef.current = true;
    setCreating(true);
    try {
      await apiJson("/api/v1/sessions", {
        method: "POST",
        body: JSON.stringify({
          course_id: form.course_id,
          room_id: form.room_id || null,
          teacher_id: form.teacher_id,
          start_at: startISO,
          end_at: endISO,
        }),
      });
      addToast("success", "Session created");
      closeModal();
      onSuccessRef.current();
    } catch (err) {
      if (err instanceof ApiRequestError && err.code) {
        addToast("error", `${err.code}: ${err.message}`);
      } else {
        addToast("error", err instanceof Error ? err.message : "Failed to create session");
      }
    } finally {
      submittingRef.current = false;
      setCreating(false);
    }
  }, [gate.canSave, form, instituteTZ, addToast, closeModal]);

  return { open, form, setForm, preflight, gate, creating, openModal, closeModal, submit };
}
