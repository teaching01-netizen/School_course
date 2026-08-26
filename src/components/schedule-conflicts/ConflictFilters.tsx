import SearchInput from "@/components/ui/SearchInput";
import SearchableSelect from "@/components/ui/SearchableSelect";
import SessionDateFilter from "@/components/SessionDateFilter";

type LookupOption = Readonly<{ id: string; label: string }>;

type ConflictFiltersProps = Readonly<{
  query: string;
  type: string;
  subjectId: string;
  teacherId: string;
  studentId: string;
  dateFrom: string;
  dateTo: string;
  subjects: readonly LookupOption[];
  teachers: readonly LookupOption[];
  students: readonly LookupOption[];
  onQueryChange: (value: string) => void;
  onFilterChange: (key: string, value: string) => void;
  onDateChange: (from: string, to: string) => void;
}>;

export function ConflictFilters(props: ConflictFiltersProps) {
  return (
    <section className="mb-4 rounded-sm border border-wi-line bg-white p-3" aria-label="Schedule conflict filters">
      <div className="flex flex-wrap items-center gap-3">
        <div className="w-full max-w-sm">
          <SearchInput value={props.query} onChange={props.onQueryChange} placeholder="Course, subject, teacher, or student" />
        </div>
        <SearchableSelect aria-label="Conflict type filter" value={props.type} onChange={(event) => props.onFilterChange("conflict_type", event.target.value)} className="w-full max-w-[180px]">
          <option value="">All conflict types</option>
          <option value="room_overlap">Room overlaps</option>
          <option value="teacher_overlap">Teacher overlaps</option>
          <option value="student_overlap">Student overlaps</option>
        </SearchableSelect>
        <LookupSelect label="Subject filter" placeholder="All subjects" value={props.subjectId} options={props.subjects} onChange={(value) => props.onFilterChange("subject_id", value)} />
        <LookupSelect label="Teacher filter" placeholder="All teachers" value={props.teacherId} options={props.teachers} onChange={(value) => props.onFilterChange("teacher_id", value)} />
        <LookupSelect label="Student filter" placeholder="All students" value={props.studentId} options={props.students} onChange={(value) => props.onFilterChange("student_id", value)} />
      </div>
      <div className="mt-3">
        <SessionDateFilter
          value={{ from: props.dateFrom, to: props.dateTo }}
          onChange={(value) => props.onDateChange(value.from, value.to)}
          onClear={() => props.onDateChange("", "")}
          idPrefix="schedule-conflicts-date"
        />
      </div>
    </section>
  );
}

function LookupSelect({ label, placeholder, value, options, onChange }: Readonly<{ label: string; placeholder: string; value: string; options: readonly LookupOption[]; onChange: (value: string) => void }>) {
  return (
    <SearchableSelect aria-label={label} value={value} onChange={(event) => onChange(event.target.value)} className="w-full max-w-[200px]" triggerMode="input">
      <option value="">{placeholder}</option>
      {options.map((option) => <option key={option.id} value={option.id}>{option.label}</option>)}
    </SearchableSelect>
  );
}
