import Input from "@/components/ui/Input";
import FormField from "@/components/ui/FormField";
import TypeaheadSelect from "@/components/TypeaheadSelect";
import type { CourseMergeCandidate } from "../types";

type MergeCourseFormProps = {
  name: string;
  onNameChange: (name: string) => void;
  courseIDs: string[];
  onCourseIDsChange: (courseIDs: string[]) => void;
  courses: CourseMergeCandidate[];
  loading: boolean;
};

function courseLabel(course: CourseMergeCandidate): string {
  const title = course.name.trim() || course.subject_name || "Unnamed course";
  return `${course.code} — ${title}`;
}

export default function MergeCourseForm({ name, onNameChange, courseIDs, onCourseIDsChange, courses, loading }: MergeCourseFormProps) {
  const options = courses.map((course) => ({
    value: course.id,
    label: courseLabel(course),
    keywords: `${course.subject_code} ${course.subject_name} ${course.teacher_name}`,
  }));
  const selectedCourses = courseIDs.map((id) => courses.find((course) => course.id === id)).filter((course): course is CourseMergeCandidate => course !== undefined);
  const setCourse = (index: number, id: string) => {
    const next = [...courseIDs];
    next[index] = id;
    onCourseIDsChange(next);
  };
  const clearCourse = (index: number) => onCourseIDsChange(courseIDs.filter((_, current) => current !== index));

  return (
    <section className="space-y-5 rounded-lg border border-[var(--color-wi-line)] bg-[var(--color-wi-callout)] p-5" aria-labelledby="merge-course-heading">
      <div>
        <p id="merge-course-heading" className="text-base font-semibold text-[var(--color-wi-text)]">Merge two existing courses</p>
        <p className="mt-1 text-sm leading-6 text-[var(--color-wi-text-light)]">
          Combine their teachers and schedules into one operational view. The original course records stay intact for legacy sync, attendance, and absence rules.
        </p>
      </div>

      <FormField name="mergeName" label="Merged course name" required hint="This name is shown on the combined schedule.">
        <Input size="md" value={name} onChange={(event) => onNameChange(event.target.value)} placeholder="e.g. SAT Verbal Reading + Writing" />
      </FormField>

      <div className="grid gap-4 md:grid-cols-2">
        {([0, 1] as const).map((index) => {
          const selected = courseIDs[index] ?? "";
          const filteredOptions = options.filter((option) => option.value === selected || !courseIDs.includes(option.value));
          return (
            <FormField key={index} name={`mergeCourse${index + 1}`} label={`Course ${index + 1}`} required hint={index === 0 ? "Choose the first source course." : "Choose the second source course."}>
              <TypeaheadSelect
                id={`merge-course-${index + 1}`}
                value={selected}
                onChange={(value) => setCourse(index, value)}
                options={filteredOptions}
                placeholder={loading ? "Loading courses…" : "Search by code, name, or teacher"}
                disabled={loading}
              />
              {selected ? (
                <button type="button" className="mt-1 text-xs text-[var(--color-wi-text-light)] underline decoration-[var(--color-wi-line)] underline-offset-2 hover:text-[var(--color-wi-text)]" onClick={() => clearCourse(index)}>
                  Clear selection
                </button>
              ) : null}
            </FormField>
          );
        })}
      </div>

      {selectedCourses.length === 2 ? (
        <div className="rounded-md border border-[var(--color-wi-line)] bg-white p-3" aria-label="Merge preview">
          <p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-faint)]">Preview</p>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-[var(--color-wi-text)]">
            <span className="rounded-sm bg-[var(--color-wi-row-alt)] px-2 py-1">{courseLabel(selectedCourses[0])}</span>
            <span className="text-[var(--color-wi-faint)]" aria-hidden="true">+</span>
            <span className="rounded-sm bg-[var(--color-wi-row-alt)] px-2 py-1">{courseLabel(selectedCourses[1])}</span>
          </div>
          <p className="mt-2 text-xs text-[var(--color-wi-text-light)]">Teachers and sessions from both source courses will appear together under the new name.</p>
        </div>
      ) : null}
    </section>
  );
}
