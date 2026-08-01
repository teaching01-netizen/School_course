import { useMemo } from "react";
import MultiTeacherSelect from "@/components/MultiTeacherSelect";
import type { TypeaheadOption } from "@/components/TypeaheadSelect";
import type { EditableTeacher } from "../types";

export function CourseTeacherEditor(props: {
  teachers: EditableTeacher[];
  onChange: (teachers: EditableTeacher[]) => void;
  options: TypeaheadOption[];
  disabled?: boolean;
}) {
  const { teachers, onChange, options, disabled } = props;

  const labelById = useMemo(() => {
    const m = new Map(options.map((o) => [o.value, o.label]));
    return (id: string) => m.get(id) ?? id;
  }, [options]);

  const handleIdsChange = (ids: string[]) => {
    const next: EditableTeacher[] = ids.map((id) => {
      const existing = teachers.find((t) => t.teacher_id === id);
      return existing ?? { teacher_id: id, is_primary: false };
    });
    onChange(next);
  };

  const setPrimary = (teacherId: string | null) => {
    onChange(teachers.map((t) => ({ ...t, is_primary: t.teacher_id === teacherId })));
  };

  const hasPrimary = teachers.some((t) => t.is_primary);

  return (
    <div className="space-y-2">
      <MultiTeacherSelect
        value={teachers.map((t) => t.teacher_id)}
        onChange={handleIdsChange}
        options={options}
        placeholder="Select teachers…"
        disabled={disabled}
      />
      {teachers.length > 0 && (
        <div role="radiogroup" aria-label="Primary teacher">
          <div className="text-[11px] font-semibold text-gray-500 uppercase tracking-wide mb-1">Primary teacher</div>
          {teachers.map((t) => (
            <label key={t.teacher_id} className="flex items-center gap-2 py-1 text-sm">
              <input
                type="radio"
                name="primary-teacher"
                checked={t.is_primary}
                onChange={() => setPrimary(t.teacher_id)}
                disabled={disabled}
                className="accent-blue-600"
              />
              <span className={t.is_primary ? "font-medium text-gray-800" : "text-gray-600"}>
                {labelById(t.teacher_id)}
              </span>
              {t.is_primary && (
                <span className="px-1.5 py-0.5 text-[10px] font-semibold uppercase rounded-sm bg-blue-700 text-white">
                  Primary
                </span>
              )}
            </label>
          ))}
          <label className="flex items-center gap-2 py-1 text-sm">
            <input
              type="radio"
              name="primary-teacher"
              checked={!hasPrimary}
              onChange={() => setPrimary(null)}
              disabled={disabled}
              className="accent-blue-600"
            />
            <span className="text-gray-600">No primary teacher</span>
          </label>
        </div>
      )}
    </div>
  );
}
