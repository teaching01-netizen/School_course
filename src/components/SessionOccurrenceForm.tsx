import { type Dispatch, type SetStateAction } from "react";
import Input from "./ui/Input";
import Select from "./ui/Select";
import FormField from "./ui/FormField";
import TypeaheadSelect from "./TypeaheadSelect";
import type { Room } from "../types";
import type { TypeaheadOption } from "./TypeaheadSelect";

interface FormValidationLike {
  errors: Record<string, string>;
  touched: Record<string, boolean>;
  touch: (field: string) => void;
  validate: (field: string) => boolean;
}

export interface SessionOccurrenceFormData {
  course_id: string;
  room_id: string;
  teacher_id: string;
  start_local: string;
  end_local: string;
}

interface SessionOccurrenceFormProps {
  form: SessionOccurrenceFormData;
  setForm: Dispatch<SetStateAction<SessionOccurrenceFormData>>;
  courseOptions: TypeaheadOption[];
  teacherOptions: TypeaheadOption[];
  rooms: Room[];
  validation?: FormValidationLike;
  prefix?: string;
}

export default function SessionOccurrenceForm({
  form,
  setForm,
  courseOptions,
  teacherOptions,
  rooms,
  validation,
  prefix = "",
}: SessionOccurrenceFormProps) {
  const err = validation?.errors ?? {};
  const tch = validation?.touched ?? {};

  return (
    <>
      <div className="bg-gray-50 rounded-sm p-3 space-y-3">
        <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Course & Teacher</div>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
          <FormField name={`${prefix}course_id`} label="Course" error={err.course_id} touched={tch.course_id} required>
            <TypeaheadSelect
              value={form.course_id}
              onChange={(v) => setForm(prev => ({ ...prev, course_id: v }))}
              options={courseOptions}
              placeholder="Search course…"
            />
          </FormField>
          <FormField name={`${prefix}room_id`} label="Room">
            <Select size="sm" value={form.room_id} onChange={(e) => setForm(prev => ({ ...prev, room_id: e.target.value }))}>
              <option value="">[NOT SET] (Provisional)</option>
              {rooms.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </Select>
          </FormField>
          <FormField name={`${prefix}teacher_id`} label="Teacher" error={err.teacher_id} touched={tch.teacher_id} required>
            <TypeaheadSelect
              value={form.teacher_id}
              onChange={(v) => setForm(prev => ({ ...prev, teacher_id: v }))}
              options={teacherOptions}
              placeholder="Search teacher…"
            />
          </FormField>
        </div>
      </div>

      <div className="bg-gray-50 rounded-sm p-3 space-y-3">
        <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Time</div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <FormField name={`${prefix}start_local`} label="Start (local time)" error={err.start_local} touched={tch.start_local} required>
            <Input
              type="datetime-local"
              size="sm"
              step={300}
              value={form.start_local}
              onChange={(e) => setForm(prev => ({ ...prev, start_local: e.target.value }))}
              onBlur={() => { validation?.touch?.("start_local"); validation?.validate?.("start_local"); }}
            />
          </FormField>
          <FormField name={`${prefix}end_local`} label="End (local time)" error={err.end_local} touched={tch.end_local} required>
            <Input
              type="datetime-local"
              size="sm"
              step={300}
              value={form.end_local}
              onChange={(e) => setForm(prev => ({ ...prev, end_local: e.target.value }))}
              onBlur={() => { validation?.touch?.("end_local"); validation?.validate?.("end_local"); }}
            />
          </FormField>
        </div>
      </div>
    </>
  );
}
