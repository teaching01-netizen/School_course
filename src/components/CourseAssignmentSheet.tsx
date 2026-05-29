import type { CourseLevelItem } from "../utils/levels";
import Button from "./ui/Button";

interface CourseAssignmentSheetProps {
  courses: CourseLevelItem[];
  onEditLevel?: (course: CourseLevelItem) => void;
  onAssignGroup?: (course: CourseLevelItem) => void;
}

export default function CourseAssignmentSheet({
  courses,
  onEditLevel,
  onAssignGroup,
}: CourseAssignmentSheetProps) {
  if (courses.length === 0) {
    return (
      <div className="border-t border-gray-200 pt-3 mt-4">
        <div className="text-sm text-gray-400 py-4 text-center">
          Select a ladder cell to see course details.
        </div>
      </div>
    );
  }

  return (
    <div className="border-t border-gray-200 pt-3 mt-4">
      <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">
        Course Assignments ({courses.length})
      </h4>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-gray-200 text-left text-gray-500">
              <th className="py-1.5 pr-3 font-medium">Code</th>
              <th className="py-1.5 pr-3 font-medium">Name</th>
              <th className="py-1.5 pr-3 font-medium">Cycle</th>
              <th className="py-1.5 pr-3 font-medium">Level</th>
              <th className="py-1.5 font-medium" />
            </tr>
          </thead>
          <tbody>
            {courses.map((course) => (
              <tr key={course.id} className="border-b border-gray-100 hover:bg-gray-50">
                <td className="py-1.5 pr-3 font-mono text-xs">{course.code}</td>
                <td className="py-1.5 pr-3 text-xs text-gray-600">{course.name}</td>
                <td className="py-1.5 pr-3 text-xs text-gray-500">{course.cycle_label}</td>
                <td className="py-1.5 pr-3">
                  <span className={`text-xs font-medium px-1.5 py-0.5 rounded-full ${
                    course.level !== null
                      ? "bg-blue-100 text-blue-700"
                      : "bg-gray-100 text-gray-500"
                  }`}>
                    {course.level !== null ? `L${course.level}` : "Not set"}
                  </span>
                </td>
                <td className="py-1.5">
                  <div className="flex items-center gap-1">
                    {onEditLevel && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => onEditLevel(course)}
                      >
                        Edit Level
                      </Button>
                    )}
                    {onAssignGroup && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => onAssignGroup(course)}
                      >
                        Group
                      </Button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
