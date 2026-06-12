import { useCallback, useEffect, useRef, useState } from "react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import {
  crmCourseLabel,
  formatCRMConflictTechnicalDetail,
  formatCRMConflictTime,
  getCRMConflictDetails,
  type CRMStudentScheduleConflictDetails,
} from "../../utils/crmConflict";

export type CourseFilter = {
  cycle_labels: string[];
  cycle_blank_mode: "any" | "only_blank" | "only_value";
  course_name_values: string[];
  course_name_blank_mode: "any" | "only_blank" | "only_value";
  academic_level_values: string[];
  academic_level_blank_mode: "any" | "only_blank" | "only_value";
  secondary_school_values: string[];
  secondary_school_blank_mode: "any" | "only_blank" | "only_value";
  teachers_contains: string;
  teachers_blank_mode: "any" | "only_blank" | "only_value";
};

const defaultFilter: CourseFilter = {
  cycle_labels: [],
  cycle_blank_mode: "any",
  course_name_values: [],
  course_name_blank_mode: "any",
  academic_level_values: [],
  academic_level_blank_mode: "any",
  secondary_school_values: [],
  secondary_school_blank_mode: "any",
  teachers_contains: "",
  teachers_blank_mode: "any",
};

type CrmOptions = {
  cycle_labels: string[] | null;
  course_names: string[] | null;
  academic_levels: string[] | null;
  secondary_schools: string[] | null;
};

type CrmFilterResponse = {
  enabled: boolean;
  locked: boolean;
  filter: CourseFilter;
};

type CourseFilterMutationResponse = {
  ok: boolean;
  job_id?: string;
  status?: string;
};

type CourseReconcileJobStatus = {
  job_id: string;
  status: string;
  message?: string;
  details?: CRMStudentScheduleConflictDetails | Record<string, unknown> | null;
};

type Props = {
  courseId: string;
  isAdmin: boolean;
  onRosterChanged: () => void;
  embeddedInModal?: boolean;
};

function isActiveJob(status?: string): boolean {
  return status === "queued" || status === "running" || status === "retry";
}

function MultiSelect<T extends string>({
  label,
  options,
  selected,
  onChange,
}: {
  label: string;
  options: T[];
  selected: T[];
  onChange: (v: T[]) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const safeSelected = selected ?? [];
  const safeOptions = options ?? [];

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const toggle = (v: T) => {
    if (safeSelected.includes(v)) {
      onChange(safeSelected.filter((x) => x !== v));
    } else {
      onChange([...safeSelected, v]);
    }
  };

  return (
    <div ref={ref} className="relative">
      <label className="block text-[11px] text-gray-500 mb-0.5">{label}</label>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="w-full text-left px-2 py-1 text-xs border border-gray-300 rounded-sm bg-white hover:bg-gray-50"
      >
        {safeSelected.length === 0 ? "Any" : `${safeSelected.length} selected`}
      </button>
      {open && (
        <div className="absolute z-10 mt-1 w-56 max-h-48 overflow-y-auto border border-gray-200 rounded-sm bg-white shadow">
          {safeOptions.length === 0 && (
            <div className="px-2 py-1 text-xs text-gray-400 italic">No options (upload CRM data first)</div>
          )}
          {safeOptions.map((opt) => (
            <label
              key={opt}
              className="flex items-center gap-2 px-2 py-1 text-xs hover:bg-gray-50 cursor-pointer"
            >
              <input
                type="checkbox"
                checked={safeSelected.includes(opt)}
                onChange={() => toggle(opt)}
                className="accent-[var(--color-wi-green)]"
              />
              {opt}
            </label>
          ))}
        </div>
      )}
    </div>
  );
}

function BlankModeSelect({
  label,
  value,
  onChange,
}: {
  label: string;
  value: "any" | "only_blank" | "only_value";
  onChange: (v: "any" | "only_blank" | "only_value") => void;
}) {
  return (
    <div>
      <label className="block text-[11px] text-gray-500 mb-0.5">{label} blank</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value as "any" | "only_blank" | "only_value")}
        className="w-full px-2 py-1 text-xs border border-gray-300 rounded-sm bg-white"
      >
        <option value="any">Any</option>
        <option value="only_blank">Blank only</option>
        <option value="only_value">Non-blank only</option>
      </select>
    </div>
  );
}

export default function CrmFilterPanel({ courseId, isAdmin, onRosterChanged, embeddedInModal = false }: Props) {
  const { addToast } = useToast();
  const [enabled, setEnabled] = useState(false);
  const [locked, setLocked] = useState(false);
  const [filter, setFilter] = useState<CourseFilter>(defaultFilter);
  const [loaded, setLoaded] = useState(false);
  const [options, setOptions] = useState<CrmOptions | null>(null);
  const [previewCount, setPreviewCount] = useState<number | null>(null);
  const [saving, setSaving] = useState(false);
  const [reconcileJobID, setReconcileJobID] = useState<string | null>(null);
  const [reconcileJob, setReconcileJob] = useState<CourseReconcileJobStatus | null>(null);
  const reconcileActive = isActiveJob(reconcileJob?.status);

  const loadCrmFilter = useCallback(async () => {
    try {
      const res = await apiJson<CrmFilterResponse>(`/api/v1/courses/${courseId}/crm-filter`, {
        method: "GET",
      });
      setEnabled(res.enabled);
      setLocked(res.locked);
      setFilter({ ...defaultFilter, ...res.filter });
      setLoaded(true);
    } catch {
      // Not available for non-admin or if not configured.
      setLoaded(true);
    }
  }, [courseId]);

  const loadOptions = useCallback(async () => {
    try {
      const opts = await apiJson<CrmOptions>("/api/v1/crm/options", { method: "GET" });
      setOptions(opts);
    } catch {
      // Ignore.
    }
  }, []);

  useEffect(() => {
    void loadCrmFilter();
    void loadOptions();
  }, [loadCrmFilter, loadOptions]);

  const computePreview = useCallback(
    async (f: CourseFilter) => {
      try {
        const res = await apiJson<{ distinct_students: number }>(
          `/api/v1/courses/${courseId}/crm-filter/preview`,
          { method: "POST", body: JSON.stringify({ filter: f }) },
        );
        setPreviewCount(res.distinct_students);
      } catch {
        setPreviewCount(null);
      }
    },
    [courseId],
  );

  useEffect(() => {
    const t = setTimeout(() => void computePreview(filter), 300);
    return () => clearTimeout(t);
  }, [filter, computePreview]);

  const pollReconcileJob = useCallback(async (jobId: string) => {
    try {
      const res = await apiJson<CourseReconcileJobStatus>(
        `/api/v1/courses/${courseId}/crm-filter/jobs/${jobId}`,
        { method: "GET" },
      );
      setReconcileJob(res);
      if (!isActiveJob(res.status)) {
        setReconcileJobID(null);
        if (res.status === "succeeded") {
          addToast("success", "CRM reconcile completed");
          onRosterChanged();
        } else if (res.status === "failed") {
          addToast("error", res.message ?? "CRM reconcile failed");
        }
      }
    } catch (err: any) {
      setReconcileJobID(null);
      setReconcileJob({
        job_id: jobId,
        status: "failed",
        message: err?.message ?? "Failed to load CRM reconcile status",
      });
    }
  }, [addToast, courseId, onRosterChanged]);

  useEffect(() => {
    if (!reconcileJobID) return;
    void pollReconcileJob(reconcileJobID);
    const t = setInterval(() => void pollReconcileJob(reconcileJobID), 1500);
    return () => clearInterval(t);
  }, [pollReconcileJob, reconcileJobID]);

  const saveFilter = async () => {
    if (reconcileActive) return;
    try {
      setSaving(true);
      const res = await apiJson<CourseFilterMutationResponse>(`/api/v1/courses/${courseId}/crm-filter`, {
        method: "PUT",
        body: JSON.stringify({ enabled, filter }),
      });
      if (res.job_id) {
        setReconcileJob({ job_id: res.job_id, status: res.status ?? "queued", message: "CRM reconcile queued" });
        setReconcileJobID(res.job_id);
        addToast("info", "CRM filter saved — reconcile queued");
      } else {
        setReconcileJob(null);
        setReconcileJobID(null);
        addToast("success", enabled ? "CRM filter saved" : "CRM filter disabled");
        onRosterChanged();
      }
    } catch (err: any) {
      addToast("error", err?.message ?? "Failed to save filter");
    } finally {
      setSaving(false);
    }
  };

  const toggleLock = async () => {
    if (reconcileActive) return;
    try {
      const newLocked = !locked;
      const res = await apiJson<CourseFilterMutationResponse>(`/api/v1/courses/${courseId}/crm-filter/lock`, {
        method: "POST",
        body: JSON.stringify({ locked: newLocked }),
      });
      setLocked(newLocked);
      if (res.job_id) {
        setReconcileJob({ job_id: res.job_id, status: res.status ?? "queued", message: "CRM reconcile queued" });
        setReconcileJobID(res.job_id);
        addToast("info", "Roster unlocked — reconcile queued");
      } else {
        setReconcileJob(null);
        setReconcileJobID(null);
        addToast(
          "success",
          newLocked ? "Roster locked — won't auto-update on future uploads" : "Roster unlocked",
        );
        onRosterChanged();
      }
    } catch (err: any) {
      addToast("error", err?.message ?? "Failed to toggle lock");
    }
  };

  if (!loaded) return null;
  if (!isAdmin) return null;

  const conflictDetails = reconcileJob?.status === "failed" ? getCRMConflictDetails(reconcileJob.details) : null;
  const firstConflict = conflictDetails?.conflicts?.[0];
  const conflictStudent = conflictDetails?.student?.full_name || conflictDetails?.student?.wcode;
  const conflictTarget = crmCourseLabel(conflictDetails?.target_course);
  const conflictingCourse = crmCourseLabel(firstConflict?.course);
  const conflictTime = formatCRMConflictTime(firstConflict?.start_at, firstConflict?.end_at);
  const conflictTechnicalDetail = formatCRMConflictTechnicalDetail(
    reconcileJob?.message,
    conflictStudent,
    conflictDetails?.student?.wcode,
    conflictTarget,
    conflictingCourse,
    conflictTime,
  );

  return (
    <div className={embeddedInModal ? "" : "border border-gray-200 rounded-sm p-4 mb-6"}>
      <div className="flex items-center justify-between mb-3">
        {!embeddedInModal && <h3 className="text-sm font-semibold text-gray-800">CRM Filter</h3>}
        <label className="inline-flex items-center gap-2 text-xs text-gray-700 cursor-pointer">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            className="accent-[var(--color-wi-green)]"
          />
          Enable CRM management
        </label>
      </div>

      {locked && (
        <div className="bg-amber-50 border border-amber-200 rounded-sm px-3 py-2 text-xs text-amber-800 mb-3">
          Roster is locked — won't auto-update on future uploads
        </div>
      )}

      {reconcileJob && (
        <div
          className={`mb-3 rounded-sm border px-3 py-2 text-xs ${
            reconcileJob.status === "failed"
              ? "border-red-200 bg-red-50 text-red-800"
              : reconcileJob.status === "succeeded"
                ? "border-emerald-200 bg-emerald-50 text-emerald-800"
                : "border-blue-200 bg-blue-50 text-blue-800"
          }`}
        >
          <div className="font-semibold">
            {isActiveJob(reconcileJob.status)
              ? "CRM reconcile running"
              : reconcileJob.status === "succeeded"
                ? "CRM reconcile complete"
                : "CRM reconcile failed"}
          </div>
          {conflictDetails ? (
            <div className="mt-1 space-y-1">
              <p>
                {conflictStudent ?? "Student"} cannot be added{conflictTarget ? ` to ${conflictTarget}` : ""}.
              </p>
              {(conflictingCourse || conflictTime) && (
                <p>
                  Conflicts with {conflictingCourse ?? "another course"}
                  {conflictTime ? ` at ${conflictTime}` : ""}.
                </p>
              )}
              <details>
                <summary className="cursor-pointer">Technical details</summary>
                <p className="mt-1 font-mono break-all">{conflictTechnicalDetail}</p>
              </details>
            </div>
          ) : reconcileJob.message ? (
            <p className="mt-1 break-words">{reconcileJob.message}</p>
          ) : null}
        </div>
      )}

      <div className="grid grid-cols-2 md:grid-cols-5 gap-2 mb-3">
        <MultiSelect
          label="Cycle"
          options={options?.cycle_labels ?? []}
          selected={filter.cycle_labels}
          onChange={(v) => setFilter((f) => ({ ...f, cycle_labels: v }))}
        />
        <MultiSelect
          label="Course Name"
          options={options?.course_names ?? []}
          selected={filter.course_name_values}
          onChange={(v) => setFilter((f) => ({ ...f, course_name_values: v }))}
        />
        <MultiSelect
          label="Academic Level"
          options={options?.academic_levels ?? []}
          selected={filter.academic_level_values}
          onChange={(v) => setFilter((f) => ({ ...f, academic_level_values: v }))}
        />
        <MultiSelect
          label="Secondary School"
          options={options?.secondary_schools ?? []}
          selected={filter.secondary_school_values}
          onChange={(v) => setFilter((f) => ({ ...f, secondary_school_values: v }))}
        />
        <div>
          <label className="block text-[11px] text-gray-500 mb-0.5">Teacher(s) contains</label>
          <input
            type="text"
            value={filter.teachers_contains}
            onChange={(e) => setFilter((f) => ({ ...f, teachers_contains: e.target.value }))}
            placeholder="Substring…"
            className="w-full px-2 py-1 text-xs border border-gray-300 rounded-sm"
          />
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-5 gap-2 mb-3">
        <BlankModeSelect
          label="Cycle"
          value={filter.cycle_blank_mode}
          onChange={(v) => setFilter((f) => ({ ...f, cycle_blank_mode: v }))}
        />
        <BlankModeSelect
          label="Course Name"
          value={filter.course_name_blank_mode}
          onChange={(v) => setFilter((f) => ({ ...f, course_name_blank_mode: v }))}
        />
        <BlankModeSelect
          label="Academic Level"
          value={filter.academic_level_blank_mode}
          onChange={(v) => setFilter((f) => ({ ...f, academic_level_blank_mode: v }))}
        />
        <BlankModeSelect
          label="Secondary School"
          value={filter.secondary_school_blank_mode}
          onChange={(v) => setFilter((f) => ({ ...f, secondary_school_blank_mode: v }))}
        />
        <BlankModeSelect
          label="Teacher(s)"
          value={filter.teachers_blank_mode}
          onChange={(v) => setFilter((f) => ({ ...f, teachers_blank_mode: v }))}
        />
      </div>

      <div className="flex items-center justify-between">
        <div className="text-xs text-gray-500">
          Preview:{" "}
          {previewCount != null ? (
            <span className="font-semibold text-gray-800">{previewCount} distinct students</span>
          ) : (
            <span className="text-gray-400">—</span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {enabled && (
            <button
              type="button"
              onClick={toggleLock}
              disabled={reconcileActive}
              className={`px-3 py-1 text-xs rounded-sm border ${
                locked
                  ? "border-green-600 text-green-700 hover:bg-green-50"
                  : "border-gray-300 text-gray-700 hover:bg-gray-50"
              }`}
            >
              {locked ? "Unlock roster" : "Lock roster"}
            </button>
          )}
          <button
            onClick={saveFilter}
            disabled={saving || reconcileActive}
            className="px-3 py-1 text-xs rounded-sm bg-[var(--color-wi-green)] hover:bg-[var(--color-wi-green-dark)] text-white disabled:opacity-60"
          >
            {saving ? "Saving…" : "Save filter"}
          </button>
        </div>
      </div>
    </div>
  );
}
