import { useEffect, useState, useRef, useCallback } from "react";
import { apiJson, apiUpload } from "../api/client";
import { useToast } from "../hooks/useToast";
import PageHeading from "../components/ui/PageHeading";
import {
  crmCourseLabel,
  formatCRMConflictTechnicalDetail,
  formatCRMConflictTime,
  getCRMConflictDetails,
  type CRMStudentScheduleConflictDetails,
} from "../utils/crmConflict";
import {
  Upload,
  FileSpreadsheet,
  CheckCircle2,
  XCircle,
  Loader2,
  AlertTriangle,
  Database,
  ChevronRight,
} from "lucide-react";

type UploadJobStatusResponse = {
  job_id: string;
  status: string;
  message?: string;
  details?: CRMStudentScheduleConflictDetails | Record<string, unknown> | null;
};

type BusyRangeConflict = {
  studentWCode: string | null;
  studentName?: string | null;
  targetCourse?: string | null;
  conflictingCourse?: string | null;
  conflictTime?: string | null;
  detail: string;
};

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function statusLabel(status: string): string {
  switch (status) {
    case "importing":
    case "queued":
      return "Waiting…";
    case "running":
      return "Processing…";
    case "succeeded":
      return "Complete";
    case "failed":
      return "Failed";
    default:
      return status;
  }
}

function isActive(status: string): boolean {
  return ["importing", "queued", "running"].includes(status);
}

export function parseBusyRangeConflict(message?: string, details?: UploadJobStatusResponse["details"]): BusyRangeConflict | null {
  const conflictDetails = getCRMConflictDetails(details);
  if (conflictDetails) {
    const firstConflict = conflictDetails.conflicts?.[0];
    const studentWCode = conflictDetails.student?.wcode ?? null;
    const studentName = conflictDetails.student?.full_name ?? null;
    const targetCourse = crmCourseLabel(conflictDetails.target_course);
    const conflictingCourse = crmCourseLabel(firstConflict?.course);
    const conflictTime = formatCRMConflictTime(firstConflict?.start_at, firstConflict?.end_at);
    return {
      studentWCode,
      studentName,
      targetCourse,
      conflictingCourse,
      conflictTime,
      detail: formatCRMConflictTechnicalDetail(message, studentName, studentWCode, targetCourse, conflictingCourse, conflictTime),
    };
  }
  if (!message) return null;
  const normalized = message.toLowerCase();
  const hasBusyRangeSignal =
    normalized.includes("student_busy_ranges_no_overlap") ||
    normalized.includes("sqlstate 23p01");
  if (!hasBusyRangeSignal) return null;

  const match = message.match(/add student ([A-Za-z0-9_-]+):/i);
  return {
    studentWCode: match?.[1] ?? null,
    detail: message,
  };
}

export default function CrmAdmin() {
  const { addToast } = useToast();
  const [job, setJob] = useState<UploadJobStatusResponse | null>(null);
  const [jobID, setJobID] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const poll = useCallback(async (jid: string) => {
    try {
      const res = await apiJson<UploadJobStatusResponse>(`/api/v1/crm/upload/${jid}`, { method: "GET" });
      setJob(res);
      if (!isActive(res.status)) {
        setJobID(null);
      }
    } catch {
      setJobID(null);
    }
  }, []);

  // Poll while job is active.
  useEffect(() => {
    if (!jobID) return;
    const t = setInterval(() => void poll(jobID), 1500);
    return () => clearInterval(t);
  }, [jobID, poll]);

  const handleFileSelect = (file: File | null) => {
    if (!file) {
      setSelectedFile(null);
      return;
    }
    if (!file.name.endsWith(".xlsx")) {
      addToast("error", "Please select an XLSX file");
      return;
    }
    setSelectedFile(file);
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    handleFileSelect(e.target.files?.[0] ?? null);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    handleFileSelect(e.dataTransfer.files?.[0] ?? null);
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
  };

  const confirmUpload = () => {
    if (!selectedFile) return;
    setShowConfirm(true);
  };

  const doUpload = async () => {
    const file = selectedFile;
    if (!file) {
      addToast("error", "Please select a file");
      return;
    }
    setShowConfirm(false);
    try {
      setUploading(true);
      const res = await apiUpload<UploadJobStatusResponse>("/api/v1/crm/upload", file);
      // res = { job_id, status: "importing", message }
      setJob(res);
      if (res.job_id) {
        setJobID(res.job_id);
      }
      addToast("success", "Upload accepted — processing asynchronously…");
      setSelectedFile(null);
    } catch (err: any) {
      addToast("error", err?.message ?? "Upload failed");
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  };

  const clearSelection = () => {
    setSelectedFile(null);
    setShowConfirm(false);
    if (fileRef.current) fileRef.current.value = "";
  };

  const resetStatus = () => {
    setJob(null);
    setJobID(null);
  };

  const running = isActive(job?.status ?? "");
  const isFailed = job?.status === "failed";
  const isSucceeded = job?.status === "succeeded";
  const showSpinner = uploading || running;
  const busyRangeConflict = isFailed ? parseBusyRangeConflict(job?.message, job?.details) : null;

  return (
    <div className="max-w-2xl">
      {/* Header */}
      <div className="flex items-center gap-3 mb-6">
        <div className="w-10 h-10 rounded-lg bg-emerald-100 flex items-center justify-center">
          <Database className="w-5 h-5 text-emerald-700" />
        </div>
        <div>
          <PageHeading>CRM Import</PageHeading>
          <p className="text-sm text-gray-500">
            Upload your CRM export to manage course rosters
          </p>
        </div>
      </div>

      {/* Step indicator */}
      <div className="flex items-center gap-2 mb-6 text-sm">
        {[
          { label: "Select file", done: !!selectedFile || isSucceeded || running },
          { label: "Upload", done: isSucceeded || running },
          { label: "Processing", done: isSucceeded },
        ].map((step, i) => (
          <div key={step.label} className="flex items-center gap-2">
            {i > 0 && <ChevronRight className="w-3.5 h-3.5 text-gray-300" />}
            <span
              className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium ${
                step.done
                  ? "bg-emerald-100 text-emerald-800"
                  : "bg-gray-100 text-gray-500"
              }`}
            >
              {step.done && <CheckCircle2 className="w-3 h-3" />}
              {step.label}
            </span>
          </div>
        ))}
      </div>

      {/* Upload Zone */}
      <div
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onClick={() => !showSpinner && fileRef.current?.click()}
        className={`
          relative border-2 border-dashed rounded-xl p-8 mb-4 text-center cursor-pointer
          transition-all duration-200
          ${
            dragOver
              ? "border-emerald-400 bg-emerald-50/70 shadow-lg shadow-emerald-100/50"
              : selectedFile
                ? "border-emerald-300 bg-emerald-50/40"
                : "border-gray-300 bg-white hover:border-gray-400 hover:bg-gray-50/50"
          }
          ${showSpinner ? "pointer-events-none opacity-60" : ""}
        `}
      >
        <input
          ref={fileRef}
          type="file"
          accept=".xlsx"
          className="hidden"
          onChange={handleFileChange}
          disabled={showSpinner}
        />

        {showSpinner ? (
          <div className="py-4">
            <Loader2 className="w-10 h-10 text-emerald-600 animate-spin mx-auto mb-3" />
            <p className="text-sm font-semibold text-gray-800">
              {uploading ? "Uploading file…" : "Processing CRM data…"}
            </p>
            <p className="text-xs text-gray-500 mt-1">
              {uploading
                ? "Please wait while your file is being uploaded"
                : "Parsing rows, syncing students, and reconciling rosters"}
            </p>
          </div>
        ) : selectedFile ? (
          <div className="py-2">
            <div className="w-12 h-12 rounded-xl bg-emerald-100 flex items-center justify-center mx-auto mb-3">
              <FileSpreadsheet className="w-6 h-6 text-emerald-700" />
            </div>
            <p className="text-sm font-semibold text-gray-800">{selectedFile.name}</p>
            <p className="text-xs text-gray-500 mt-0.5">{formatBytes(selectedFile.size)}</p>
            <div className="flex items-center justify-center gap-2 mt-4">
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  confirmUpload();
                }}
                className="px-4 py-2 text-sm font-medium rounded-lg bg-emerald-600 hover:bg-emerald-700 text-white shadow-sm transition-colors"
              >
                <Upload className="w-4 h-4 inline-block mr-1.5 -mt-0.5" />
                Upload to CRM
              </button>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  clearSelection();
                }}
                className="px-4 py-2 text-sm font-medium rounded-lg border border-gray-300 text-gray-600 hover:bg-gray-50 transition-colors"
              >
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <div className="py-4">
            <div className="w-14 h-14 rounded-2xl bg-gray-100 flex items-center justify-center mx-auto mb-4">
              <Upload className="w-7 h-7 text-gray-400" />
            </div>
            <p className="text-sm font-semibold text-gray-800">
              Drop your CRM export here
            </p>
            <p className="text-xs text-gray-500 mt-1">
              or click to browse &mdash; accepts <strong>.xlsx</strong> files only
            </p>
          </div>
        )}

        {/* Drag overlay hint */}
        {dragOver && (
          <div className="absolute inset-0 rounded-xl border-2 border-emerald-400 bg-emerald-50/60 flex items-center justify-center">
            <p className="text-sm font-semibold text-emerald-700">
              <Upload className="w-5 h-5 inline-block mr-2" />
              Drop file to upload
            </p>
          </div>
        )}
      </div>

      {/* Data replacement warning */}
      <div className="flex items-start gap-2.5 px-4 py-3 mb-5 rounded-lg bg-amber-50 border border-amber-200 text-[13px] text-amber-800">
        <AlertTriangle className="w-4 h-4 text-amber-500 mt-0.5 shrink-0" />
        <div>
          <span className="font-semibold">Full data replacement.</span>{" "}
          Uploading will snapshot all existing CRM data. Courses with CRM
          filter enabled and not locked will be auto-reconciled.
        </div>
      </div>

      {/* Upload confirmation modal */}
      {showConfirm && selectedFile && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/30"
          onClick={clearSelection}
        >
          <div
            className="bg-white rounded-xl shadow-2xl max-w-md w-full mx-4 p-6"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center gap-3 mb-4">
              <div className="w-10 h-10 rounded-full bg-amber-100 flex items-center justify-center">
                <AlertTriangle className="w-5 h-5 text-amber-600" />
              </div>
              <div>
                <h3 className="text-base font-semibold text-gray-900">
                  Replace CRM data?
                </h3>
                <p className="text-sm text-gray-500">
                  This action will snapshot and replace all existing data
                </p>
              </div>
            </div>

            <div className="bg-gray-50 rounded-lg px-4 py-3 mb-4 text-sm">
              <div className="flex items-center gap-2 text-gray-700">
                <FileSpreadsheet className="w-4 h-4 text-emerald-600" />
                <span className="font-medium">{selectedFile.name}</span>
                <span className="text-gray-400">({formatBytes(selectedFile.size)})</span>
              </div>
            </div>

            <div className="bg-amber-50 border border-amber-200 rounded-lg px-4 py-3 text-[13px] text-amber-800 mb-5">
              <span className="font-semibold">What happens next:</span>
              <ul className="list-disc pl-5 mt-1 space-y-0.5 text-amber-700">
                <li>Upload is processed asynchronously</li>
                <li>Imported rows are parsed and stored in a new snapshot</li>
                <li>Student identities are synced</li>
                <li>Courses with active CRM filters will auto-reconcile</li>
                <li>Locked courses will not be affected</li>
              </ul>
            </div>

            <div className="flex justify-end gap-2">
              <button
                onClick={clearSelection}
                className="px-4 py-2 text-sm font-medium rounded-lg border border-gray-300 text-gray-600 hover:bg-gray-50 transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={doUpload}
                className="px-4 py-2 text-sm font-medium rounded-lg bg-emerald-600 hover:bg-emerald-700 text-white shadow-sm transition-colors"
              >
                <Upload className="w-4 h-4 inline-block mr-1.5 -mt-0.5" />
                Start import
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Status / Result Card */}
      {job && (
        <div className="rounded-xl border border-gray-200 bg-white overflow-hidden mb-4">
          {/* Status bar */}
          <div
            className={`flex items-center gap-3 px-5 py-3.5 border-b border-gray-100 ${
              running
                ? "bg-blue-50"
                : isSucceeded
                  ? "bg-emerald-50"
                  : isFailed
                    ? "bg-red-50"
                    : ""
            }`}
          >
            {running ? (
              <Loader2 className="w-5 h-5 text-blue-600 animate-spin shrink-0" />
            ) : isSucceeded ? (
              <CheckCircle2 className="w-5 h-5 text-emerald-600 shrink-0" />
            ) : isFailed ? (
              <XCircle className="w-5 h-5 text-red-600 shrink-0" />
            ) : (
              <Loader2 className="w-5 h-5 text-blue-600 animate-spin shrink-0" />
            )}
            <div className="flex-1 min-w-0">
              <p className="text-sm font-semibold text-gray-800">
                {running
                  ? "Processing import…"
                  : isSucceeded
                    ? "Import complete"
                    : isFailed
                      ? "Import failed"
                      : statusLabel(job.status)}
              </p>
              {job.message && (
                <p className="text-xs text-gray-500 truncate">{job.message}</p>
              )}
            </div>
            <button
              onClick={resetStatus}
              className="shrink-0 p-1.5 rounded-lg hover:bg-white/50 text-gray-400 hover:text-gray-600 transition-colors"
              title="Dismiss"
            >
              <XCircle className="w-4 h-4" />
            </button>
          </div>

          {running && (
            <div className="px-5 py-4 text-center">
              <div className="flex items-center justify-center gap-2 text-sm text-gray-500">
                <Loader2 className="w-4 h-4 text-blue-500 animate-spin" />
                {job.status === "importing" && "Parsing file and importing rows…"}
                {job.status === "queued" && "Waiting for worker…"}
                {job.status === "running" && "Processing — syncing students and reconciling rosters…"}
              </div>
            </div>
          )}

          {/* Error detail */}
          {isFailed && (
            <div className="px-5 py-3 bg-red-50 border-b border-red-100">
              {busyRangeConflict ? (
                <div className="space-y-2">
                  <div className="rounded-lg border border-red-200 bg-white px-3 py-2">
                    <p className="text-xs font-semibold text-red-800">
                      Roster update conflict detected
                    </p>
                    <p className="mt-1 text-xs text-red-700">
                      {busyRangeConflict.studentName || busyRangeConflict.studentWCode
                        ? `${busyRangeConflict.studentName ?? "Student"}${busyRangeConflict.studentWCode ? ` (${busyRangeConflict.studentWCode})` : ""} cannot be added${busyRangeConflict.targetCourse ? ` to ${busyRangeConflict.targetCourse}` : ""}.`
                        : "A student cannot be added because they already have an overlapping scheduled time."}
                    </p>
                    {(busyRangeConflict.conflictingCourse || busyRangeConflict.conflictTime) && (
                      <p className="mt-1 text-xs text-red-700">
                        Conflicts with {busyRangeConflict.conflictingCourse ?? "another course"}
                        {busyRangeConflict.conflictTime ? ` at ${busyRangeConflict.conflictTime}` : ""}.
                      </p>
                    )}
                    <p className="mt-1 text-xs text-red-700">
                      Check the student schedule or course roster before retrying the CRM import.
                    </p>
                  </div>
                  <details className="text-xs">
                    <summary className="cursor-pointer text-red-700 hover:text-red-800">
                      Technical details
                    </summary>
                    <p className="mt-2 text-red-700 font-mono break-all">
                      {busyRangeConflict.detail}
                    </p>
                  </details>
                </div>
              ) : (
                <p className="text-xs text-red-700 font-mono break-all">
                  {job.message || "Import failed — check server logs"}
                </p>
              )}
            </div>
          )}

          {/* Success completion */}
          {isSucceeded && (
            <div className="px-5 py-4">
              <div className="flex items-center gap-2 text-sm text-gray-600">
                <CheckCircle2 className="w-4 h-4 text-emerald-500" />
                Upload processed successfully. Downstream roster reconcile jobs completed.
              </div>
              <div className="mt-3 text-xs text-gray-400">
                Visit individual course pages to review roster changes.
              </div>
            </div>
          )}
        </div>
      )}

      {/* Help section — collapsed by default */}
      <details className="group rounded-xl border border-gray-200 bg-white overflow-hidden">
        <summary className="flex items-center gap-2 px-5 py-3 cursor-pointer hover:bg-gray-50 transition-colors text-sm font-medium text-gray-700 list-none [&::-webkit-details-marker]:hidden">
          <ChevronRight className="w-4 h-4 text-gray-400 transition-transform group-open:rotate-90" />
          What format does my file need to be in?
        </summary>
        <div className="px-5 pb-4 pt-1 border-t border-gray-100">
          <div className="text-xs text-gray-600 space-y-2">
            <p>
              Your XLSX file must contain at least these columns (by header name):
            </p>
            <div className="bg-gray-50 rounded-lg px-3 py-2 font-mono text-[11px] text-gray-700 space-y-0.5">
              <div><span className="text-emerald-700 font-semibold">Student Id</span> — Required</div>
              <div><span className="text-emerald-700 font-semibold">Course Name</span> — Required</div>
              <div><span className="text-emerald-700 font-semibold">Cycle</span> — Required</div>
              <div className="text-gray-400 pt-0.5">First Name, Last Name, Teacher(s), and more (optional)</div>
            </div>
            <p>
              Column order doesn't matter — headers are matched by name. Only .xlsx
              files are accepted.
            </p>
          </div>
        </div>
      </details>

      <div className="mt-6 text-xs text-center text-gray-400">
        Need help?{" "}
        <span className="text-gray-500 font-medium">
          Contact support for the expected XLSX format
        </span>
      </div>
    </div>
  );
}
