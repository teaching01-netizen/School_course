import Button from "./ui/Button";
import Select from "./ui/Select";
import type { Student, AttendanceOverride } from "../types";

interface AttendancePanelProps {
  roster: Student[];
  overrides: AttendanceOverride[];
  loading: boolean;
  includeWcode: string;
  onIncludeWcodeChange: (v: string) => void;
  includeAdding: boolean;
  onAddIncluded: () => void;
  onUpsert: (studentId: string, status: "included" | "excluded") => Promise<void>;
  onDelete: (studentId: string) => Promise<void>;
  addToast: (type: "success" | "error" | "warning" | "info", msg: string) => void;
}

export default function AttendancePanel({
  roster,
  overrides,
  loading,
  includeWcode,
  onIncludeWcodeChange,
  includeAdding,
  onAddIncluded,
  onUpsert,
  onDelete,
  addToast,
}: AttendancePanelProps) {
  if (loading) {
    return <div className="text-sm text-gray-400">Loading…</div>;
  }

  const overrideByStudent = new Map(overrides.map((o) => [o.student_id, o]));

  return (
    <div className="space-y-3">
      <div className="text-xs text-gray-500">
        Changes here update session attendance overrides and drive student busy-range enforcement.
      </div>

      <div className="flex items-end gap-2">
        <div className="flex-1">
          <label className="block text-xs text-gray-500 mb-1">Include student by WCode</label>
          <input
            value={includeWcode}
            onChange={(e) => onIncludeWcodeChange(e.target.value)}
            placeholder="e.g. W0001"
            className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
          />
        </div>
        <Button variant="primary" size="md" onClick={onAddIncluded} disabled={includeAdding} loading={includeAdding}>
          {includeAdding ? "Adding…" : "Add Included"}
        </Button>
      </div>

      <div className="border border-gray-200 rounded-sm overflow-x-auto">
        <table className="w-full text-[13px]">
          <thead>
            <tr className="border-b border-gray-200 bg-gray-50">
              <th className="text-left py-2 px-2 font-semibold">WCode</th>
              <th className="text-left py-2 px-2 font-semibold">Name</th>
              <th className="text-left py-2 px-2 font-semibold">Status</th>
            </tr>
          </thead>
          <tbody>
            {roster.map((st) => {
              const ov = overrideByStudent.get(st.id);
              const value = ov ? ov.status : "default";
              return (
                <tr key={st.id} className="border-b border-gray-100 last:border-b-0">
                  <td className="py-2 px-2 font-mono text-xs text-gray-700">{st.wcode}</td>
                  <td className="py-2 px-2 text-gray-800">{st.full_name}</td>
                  <td className="py-2 px-2">
                    <Select
                      size="sm"
                      value={value}
                      onChange={(e) => {
                        const v = e.target.value as "default" | "included" | "excluded";
                        (async () => {
                          try {
                            if (v === "default") {
                              await onDelete(st.id);
                            } else {
                              await onUpsert(st.id, v);
                            }
                            addToast("success", "Saved");
                          } catch (err) {
                            addToast("error", err instanceof Error ? err.message : "Save failed");
                          }
                        })();
                      }}
                    >
                      <option value="default">Default</option>
                      <option value="included">Included</option>
                      <option value="excluded">Excluded</option>
                    </Select>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
