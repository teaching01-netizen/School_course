import { useEffect, useState } from "react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import LoadingSkeleton from "../../components/ui/LoadingSkeleton";
import Button from "../../components/ui/Button";
import type { StaffAbsencePolicies } from "../../types";

const DURATION_OPTIONS = [30, 45, 60, 90, 120];

export function StaffAbsenceRulesSection() {
  const { addToast } = useToast();
  const [policies, setPolicies] = useState<StaffAbsencePolicies | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const data = await apiJson<{ staff_absence_policies: StaffAbsencePolicies }>(
          "/api/v1/admin/staff-absence-policies",
          { method: "GET" },
        );
        if (active) setPolicies(data.staff_absence_policies);
      } catch (err) {
        if (active) addToast("error", err instanceof Error ? err.message : "Failed to load staff absence rules");
      }
    })();
    return () => { active = false; };
  }, [addToast]);

  async function save() {
    if (!policies) return;
    setSaving(true);
    try {
      const res = await apiJson<{ staff_absence_policies: StaffAbsencePolicies }>(
        "/api/v1/admin/staff-absence-policies",
        { method: "PUT", body: JSON.stringify({ staff_absence_policies: policies }) },
      );
      setPolicies(res.staff_absence_policies);
      addToast("success", "Staff absence rules saved");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  if (!policies) return <LoadingSkeleton type="card" lines={5} />;

  return (
    <div>
      <div className="mb-6">
        <p className="text-sm text-gray-500">Manage teacher absence notifications and cover lesson policies.</p>
      </div>

      <div className="space-y-6">
        <div className="rounded-sm border border-gray-200 bg-white p-5">
          <h3 className="text-base font-semibold mb-4">Notifications</h3>
          <div className="space-y-3">
            <label className="flex gap-2 text-sm items-center">
              <input
                type="checkbox"
                checked={policies.notify_admin_on_teacher_absence}
                onChange={(e) => setPolicies({ ...policies, notify_admin_on_teacher_absence: e.target.checked })}
              />
              Notify admin when teacher is absent
            </label>
            <label className="flex gap-2 text-sm items-center">
              <input
                type="checkbox"
                checked={policies.notify_substitute_teachers}
                onChange={(e) => setPolicies({ ...policies, notify_substitute_teachers: e.target.checked })}
              />
              Notify substitute teachers
            </label>
          </div>
        </div>

        <div className="rounded-sm border border-gray-200 bg-white p-5">
          <h3 className="text-base font-semibold mb-4">Cover Lesson Policy</h3>
          <div className="space-y-4">
            <div className="flex gap-2 text-sm items-start">
              <input
                type="checkbox"
                checked={policies.auto_assign_cover_enabled}
                onChange={(e) => setPolicies({ ...policies, auto_assign_cover_enabled: e.target.checked })}
                className="mt-0.5"
              />
              <div>
                <label>Auto-assign cover for absences &gt;</label>
                {policies.auto_assign_cover_enabled ? (
                  <span className="ml-1">
                    <input
                      type="number"
                      min={1}
                      max={30}
                      value={policies.cover_threshold_days}
                      onChange={(e) => setPolicies({ ...policies, cover_threshold_days: Math.max(1, Math.min(30, parseInt(e.target.value) || 1)) })}
                      className="w-20 rounded-sm border border-gray-300 p-1 text-sm"
                    />
                    <span className="ml-1">days</span>
                  </span>
                ) : null}
              </div>
            </div>

            <div className="flex gap-2 text-sm items-center">
              <span className="text-gray-700">Default cover slot duration</span>
              <select
                value={policies.default_cover_duration_minutes}
                onChange={(e) => setPolicies({ ...policies, default_cover_duration_minutes: parseInt(e.target.value) })}
                className="rounded-sm border border-gray-300 p-1.5 text-sm"
              >
                {DURATION_OPTIONS.map((d) => (
                  <option key={d} value={d}>{d} minutes</option>
                ))}
              </select>
            </div>
          </div>
        </div>
      </div>

      <div className="mt-6">
        <Button variant="primary" onClick={save} loading={saving}>Save</Button>
      </div>
    </div>
  );
}
