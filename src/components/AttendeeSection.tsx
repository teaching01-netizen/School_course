import { Link } from "react-router-dom";
import { useState } from "react";
import Modal from "./Modal";
import CrmFilterPanel from "./crm/CrmFilterPanel";
import { ApiRequestError, apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { formatConflictToastMessage } from "../utils/conflictErrors";


export type Student = {
  id: string;
  wcode: string;
  full_name: string;
  notes: string;
  nickname?: string;
  school?: string;
  level?: string;
  year?: string;
  student_phone?: string;
  email?: string;
  status?: string;
};
export type AddStudentResult = { ok: true } | { ok: false; error: string };

type Props = {
  courseId: string;
  isAdmin: boolean;
  crmEnabled: boolean;
  crmLocked: boolean;
  roster: Student[];
  rosterLoading: boolean;
  addingWcode: string;
  adding: boolean;
  onRosterChanged: () => void;
  onSetAddingWcode: (v: string) => void;
  onAddStudentByWcode: () => Promise<AddStudentResult>;
  onRemoveStudent: (studentId: string) => Promise<void>;
};

export function AttendeeSection({
  courseId,
  isAdmin,
  crmEnabled,
  crmLocked,
  roster,
  rosterLoading,
  addingWcode,
  adding,
  onRosterChanged,
  onSetAddingWcode,
  onAddStudentByWcode,
  onRemoveStudent,
}: Props) {
  const [manualModalOpen, setManualModalOpen] = useState(false);
  const [manualError, setManualError] = useState<string | null>(null);
  const [draftModalOpen, setDraftModalOpen] = useState(false);
  const [draftWcode, setDraftWcode] = useState("");
  const [draftAdding, setDraftAdding] = useState(false);
  const [draftError, setDraftError] = useState<string | null>(null);
  const [sageModalOpen, setSageModalOpen] = useState(false);
  const { addToast } = useToast();

  const openSageModal = () => setSageModalOpen(true);
  const closeSageModal = () => setSageModalOpen(false);

  const openManualModal = () => {
    setManualModalOpen(true);
    setManualError(null);
  };
  const closeManualModal = () => {
    setManualModalOpen(false);
    setManualError(null);
  };

  const handleManualAdd = async () => {
    setManualError(null);
    const result = await onAddStudentByWcode();
    if (result.ok) {
      closeManualModal();
    } else {
      setManualError(result.error);
    }
  };

  const handleSageSaved = () => {
    closeSageModal();
    onRosterChanged();
  };

  const openDraftModal = () => {
    setDraftModalOpen(true);
    setDraftWcode("");
    setDraftError(null);
  };
  const closeDraftModal = () => setDraftModalOpen(false);

  const addDraftStudent = async () => {
    const w = draftWcode.trim();
    if (!w) return;
    try {
      setDraftAdding(true);
      setDraftError(null);
      // Find student by wcode.
      const st = await apiJson<Student>(`/api/v1/students/${encodeURIComponent(w)}`, { method: "GET" });
      await apiJson(`/api/v1/courses/${courseId}/students/draft`, {
        method: "POST",
        body: JSON.stringify({ student_id: st.id }),
      });
      addToast("success", `Added ${st.wcode} as draft`);
      setDraftWcode("");
      closeDraftModal();
      onRosterChanged();
    } catch (err) {
      setDraftError(formatConflictToastMessage(err, "Failed to add draft"));
    } finally {
      setDraftAdding(false);
    }
  };

  const convertDraftStudent = async (studentId: string) => {
    try {
      await apiJson(`/api/v1/courses/${courseId}/students/${studentId}/convert`, {
        method: "POST",
      });
      addToast("success", "Student enrolled");
      onRosterChanged();
    } catch (err) {
      if (err instanceof ApiRequestError) {
        addToast("error", `${err.code}: ${err.message}`);
      } else {
        addToast("error", err instanceof Error ? err.message : "Conversion failed");
      }
    }
  };

  // Count drafts for display.
  const draftCount = roster.filter((st) => st.status === "draft").length;

  // Separate draft and enrolled students.
  const sortedRoster = [...roster].sort((a, b) => {
    // Drafts at the top.
    if ((a.status === "draft") !== (b.status === "draft")) {
      return a.status === "draft" ? -1 : 1;
    }
    return a.wcode.localeCompare(b.wcode);
  });

  return (
    <div className="mb-8">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-xl font-semibold text-[var(--color-wi-text)]">
          Attendee
          {draftCount > 0 && (
            <span className="ml-2 text-sm font-normal text-amber-600 align-middle">
              ({draftCount} draft)
            </span>
          )}
        </h2>

        {isAdmin && (
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={openManualModal}
              disabled={crmEnabled}
              className={`px-3 py-1.5 text-sm rounded-sm border transition-[background-color,color,transform,border-color] duration-150 active:scale-[0.96] motion-reduce:transition-none motion-reduce:transform-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]/30 ${
                crmEnabled
                  ? "var(--color-wi-line) text-[var(--color-wi-text-light)] cursor-not-allowed"
                  : "var(--color-wi-line) text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] cursor-pointer"
              }`}
              title={crmEnabled ? "Disable CRM filter to add manual students" : "Add a student manually by W-code"}
            >
              Add Manual
            </button>
            <button
              type="button"
              onClick={openDraftModal}
              disabled={crmEnabled}
              className={`px-3 py-1.5 text-sm rounded-sm border transition-[background-color,color,transform,border-color] duration-150 active:scale-[0.96] motion-reduce:transition-none motion-reduce:transform-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]/30 ${
                crmEnabled
                  ? "var(--color-wi-line) text-[var(--color-wi-text-light)] cursor-not-allowed"
                  : "border-amber-300 text-amber-700 hover:bg-amber-50 cursor-pointer"
              }`}
              title={crmEnabled ? "Disable CRM filter to add draft students" : "Add a student as draft (tentative)"}
            >
              + Draft
            </button>
            <button
              type="button"
              onClick={openSageModal}
              className="px-3 py-1.5 text-sm rounded-sm bg-[var(--color-wi-green)] hover:bg-[var(--color-wi-green-dark)] text-white cursor-pointer transition-[background-color,transform] duration-150 active:scale-[0.96] motion-reduce:transition-none motion-reduce:transform-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-green)]/40"
            >
              {crmEnabled ? "Edit Sage filter" : "Add from Sage"}
            </button>
          </div>
        )}
      </div>

      {crmLocked && (
        <div className="bg-amber-50 border border-amber-200 rounded-sm px-3 py-2 text-xs text-amber-800 mb-3">
          Roster is locked — won't auto-update on future uploads
        </div>
      )}

      {crmEnabled && !crmLocked && (
        <div className="text-xs text-[var(--color-wi-text-light)] mb-3">
          Roster is managed by CRM filter. Disable CRM filter in the Sage settings to edit manually.
        </div>
      )}

      <div className="border var(--color-wi-line) rounded-sm overflow-x-auto">
        <table className="w-full text-[13px]">
          <thead className="bg-[var(--color-wi-row-alt)]">
            <tr className="border-b var(--color-wi-line)">
              <th className="text-left py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">W-code</th>
              <th className="text-left py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Name</th>
              <th className="text-left py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Nickname</th>
              <th className="text-left py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">School</th>
              <th className="text-left py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Level</th>
              <th className="text-left py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Year</th>
              <th className="text-left py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Phone</th>
              <th className="text-left py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Status</th>
              <th className="text-left py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Notes</th>
              <th className="text-right py-2 px-3 font-semibold text-[var(--color-wi-text-light)]"></th>
            </tr>
          </thead>
          <tbody>
            {rosterLoading ? (
              <tr>
                <td className="py-6 px-3 text-sm text-[var(--color-wi-text-light)]" colSpan={10}>
                  Loading…
                </td>
              </tr>
            ) : roster.length === 0 ? (
              <tr>
                <td className="py-6 px-3 text-sm text-[var(--color-wi-text-light)]" colSpan={10}>
                  No students
                </td>
              </tr>
            ) : (
              sortedRoster.map((st) => {
                const isDraft = st.status === "draft";
                return (
                  <tr
                    key={st.id}
                    className={`border-b var(--color-wi-line) hover:bg-[var(--color-wi-row-alt)] ${
                      isDraft ? "bg-amber-50/30" : ""
                    }`}
                  >
                    <td className="py-2 px-3 font-mono text-xs text-[var(--color-wi-text-light)]">{st.wcode}</td>
                    <td className="py-2 px-3">{st.full_name}</td>
                    <td className="py-2 px-3 text-[var(--color-wi-text-light)]">{st.nickname || "—"}</td>
                    <td className="py-2 px-3 text-[var(--color-wi-text-light)]">{st.school || "—"}</td>
                    <td className="py-2 px-3 text-[var(--color-wi-text-light)]">{st.level || "—"}</td>
                    <td className="py-2 px-3 text-[var(--color-wi-text-light)]">{st.year || "—"}</td>
                    <td className="py-2 px-3 text-[var(--color-wi-text-light)]">{st.student_phone || "—"}</td>
                    <td className="py-2 px-3">
                      {isDraft ? (
                        <span className="inline-flex items-center px-1.5 py-0.5 rounded-sm text-[10px] font-medium bg-amber-100 text-amber-800 border border-amber-200">
                          DRAFT
                        </span>
                      ) : (
                        <span className="inline-flex items-center px-1.5 py-0.5 rounded-sm text-[10px] font-medium bg-green-100 text-green-800 border border-green-200">
                          ENROLLED
                        </span>
                      )}
                    </td>
                    <td className="py-2 px-3 text-[var(--color-wi-text-light)]">{st.notes}</td>
                    <td className="py-2 px-3 text-right">
                      <div className="flex justify-end gap-2">
                        {isDraft && !crmEnabled && (
                          <button
                            onClick={() => void convertDraftStudent(st.id)}
                            className="px-2 py-1 text-xs border border-green-300 text-green-700 rounded-sm hover:bg-green-50"
                          >
                            Enroll
                          </button>
                        )}
                        <Link
                          to={`/students/${encodeURIComponent(st.wcode)}`}
                          className="px-2 py-1 text-xs border var(--color-wi-line) rounded-sm hover:bg-[var(--color-wi-row-alt)]"
                        >
                          edit
                        </Link>
                        {!crmEnabled && (
                          <button
                            onClick={() => void onRemoveStudent(st.id)}
                            className="px-2 py-1 text-xs bg-[var(--color-wi-red)] hover:bg-[var(--color-wi-red-dark)] text-white rounded-sm"
                          >
                            remove
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* Add Manual modal */}
      {manualModalOpen && isAdmin && (
        <Modal
          title="Add Student Manually"
          onClose={closeManualModal}
          footer={
            <>
              <button
                onClick={closeManualModal}
                className="px-3 py-1 text-sm border var(--color-wi-line) rounded-sm hover:bg-[var(--color-wi-row-alt)]"
              >
                Cancel
              </button>
              {manualError && (
                <span className="text-xs text-red-600 mr-2">{manualError}</span>
              )}
              <button
                onClick={() => void handleManualAdd()}
                disabled={adding || !addingWcode.trim()}
                className="px-3 py-1 text-sm bg-[var(--color-wi-green)] hover:bg-[var(--color-wi-green-dark)] text-white rounded-sm disabled:opacity-60"
              >
                {adding ? "Adding…" : "Add"}
              </button>
            </>
          }
        >
          <div>
            <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Add by W-code</label>
            <input
              value={addingWcode}
              onChange={(e) => {
                setManualError(null);
                onSetAddingWcode(e.target.value);
              }}
              placeholder="e.g. W250389"
              autoFocus
              className="w-full px-2 py-1.5 text-sm border var(--color-wi-line) rounded-sm"
            />
          </div>
        </Modal>
      )}

      {/* Add Draft modal */}
      {draftModalOpen && isAdmin && (
        <Modal
          title="Add Student as Draft"
          onClose={closeDraftModal}
          footer={
            <>
              <button
                onClick={closeDraftModal}
                className="px-3 py-1 text-sm border var(--color-wi-line) rounded-sm hover:bg-[var(--color-wi-row-alt)]"
              >
                Cancel
              </button>
              {draftError && (
                <span className="text-xs text-red-600 mr-2">{draftError}</span>
              )}
              <button
                onClick={() => void addDraftStudent()}
                disabled={draftAdding || !draftWcode.trim()}
                className="px-3 py-1 text-sm bg-amber-500 hover:bg-amber-600 text-white rounded-sm disabled:opacity-60"
              >
                {draftAdding ? "Adding…" : "Add Draft"}
              </button>
            </>
          }
        >
          <div className="space-y-3">
            <div className="text-xs text-[var(--color-wi-text-light)] bg-amber-50 border border-amber-200 rounded-sm px-3 py-2">
              Draft students are tentative — they will block scheduling conflicts but won't be counted as enrolled.
              Use "Enroll" to confirm them after any conflicts are resolved.
            </div>
            <div>
              <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Add by W-code</label>
              <input
                value={draftWcode}
                onChange={(e) => setDraftWcode(e.target.value)}
                placeholder="e.g. W250389"
                autoFocus
                className="w-full px-2 py-1.5 text-sm border var(--color-wi-line) rounded-sm"
              />
            </div>
          </div>
        </Modal>
      )}

      {/* Add from Sage / Edit Sage filter modal */}
      {sageModalOpen && isAdmin && (
        <Modal
          title="CRM Filter"
          onClose={closeSageModal}
          maxWidth="max-w-3xl"
        >
          <CrmFilterPanel
            courseId={courseId}
            isAdmin={isAdmin}
            onRosterChanged={handleSageSaved}
            embeddedInModal
          />
        </Modal>
      )}
    </div>
  );
}
