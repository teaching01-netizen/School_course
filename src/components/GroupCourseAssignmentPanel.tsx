import { useMemo } from "react";
import type { CourseLevelItem, GroupWithCount } from "../utils/levels";
import TypeaheadSelect from "./TypeaheadSelect";
import type { TypeaheadOption } from "./TypeaheadSelect";

interface GroupCourseAssignmentPanelProps {
  group: GroupWithCount;
  courses: CourseLevelItem[];
  savingCourseId: string | null;
  onAssignCourse: (courseId: string) => void;
}

export default function GroupCourseAssignmentPanel({
  group,
  courses,
  savingCourseId,
  onAssignCourse,
}: GroupCourseAssignmentPanelProps) {
  const assignedCourses = useMemo(
    () => courses
      .filter((course) => course.root_course_group_id === group.id)
      .sort((a, b) => a.code.localeCompare(b.code)),
    [courses, group.id],
  );

  const courseOptions = useMemo<TypeaheadOption[]>(
    () => courses.map((course) => ({
      value: course.id,
      label: `${course.code} — ${course.subject_name}`,
      description: `${course.name} · ${course.cycle_label}`,
      keywords: [course.code, course.name, course.subject_code, course.subject_name].join(" "),
      disabled: course.root_course_group_id === group.id,
    })),
    [courses, group.id],
  );

  return (
    <section className="mt-5 border-t border-wi-line pt-4" aria-labelledby="group-course-assignment-heading">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 id="group-course-assignment-heading" className="text-sm font-semibold text-[var(--color-wi-text)]">
            Courses in {group.name}
          </h3>
          <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">
            Search all courses by code, subject name, subject code, or course name.
          </p>
        </div>
        <span className="text-xs text-[var(--color-wi-text-light)]">
          {assignedCourses.length} course{assignedCourses.length === 1 ? "" : "s"}
        </span>
      </div>

      <div className="mt-3">
        <label htmlFor="manage-course-picker" className="mb-1.5 block text-xs font-medium text-[var(--color-wi-text-light)]">
          Add course to {group.name}
        </label>
        <TypeaheadSelect
          id="manage-course-picker"
          value=""
          onChange={onAssignCourse}
          options={courseOptions}
          placeholder="Search by course code or subject name"
          disabled={savingCourseId !== null}
          className="w-full"
        />
      </div>

      {assignedCourses.length === 0 ? (
        <p className="mt-4 rounded-sm bg-[var(--color-wi-callout)] px-3 py-3 text-sm text-[var(--color-wi-text-light)]">
          No courses assigned to this group yet.
        </p>
      ) : (
        <ul className="mt-4 divide-y divide-wi-line rounded-sm border border-wi-line" aria-label={`Courses in ${group.name}`}>
          {assignedCourses.map((course) => (
            <li key={course.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2.5 text-sm">
              <span className="font-mono text-xs text-[var(--color-wi-text)]">{course.code}</span>
              <span className="text-[var(--color-wi-text-light)]">— {course.subject_name}</span>
              <span className="text-xs text-[var(--color-wi-faint)]">{course.name}</span>
              {savingCourseId === course.id && (
                <span className="ml-auto text-xs text-[var(--color-wi-text-light)]" role="status">Saving…</span>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
