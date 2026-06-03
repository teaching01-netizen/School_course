import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useToast } from "../hooks/useToast";
import { useApiQuery } from "@/hooks/useApiQuery";
import PageHeading from "../components/ui/PageHeading";
import SearchInput from "../components/ui/SearchInput";
import Button from "../components/ui/Button";
import EmptyState from "../components/ui/EmptyState";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";

export default function Courses() {
  type CourseRow = {
    id: string;
    course_no: number;
    code: string;
    name: string;
    year: number | null;
    teacher_id: string | null;
    teacher_name: string;
    subject_id: string | null;
    subject_code: string;
    subject_name: string;
    hour: number | null;
    student_count: number | null;
    course_type: string | null;
  };

  const [search, setSearch] = useState("");
  const { addToast } = useToast();

  const { data: courses, loading, error } = useApiQuery<CourseRow[]>("/api/v1/courses");

  useEffect(() => {
    if (error) addToast("error", error.message);
  }, [error]);

  const filtered = useMemo(() => {
    let data = [...(courses ?? [])];
    if (search) {
      const q = search.toLowerCase();
      data = data.filter((c) => {
        const subjectLabel = `${c.subject_code} ${c.subject_name}`.toLowerCase();
        const teacherLabel = (c.teacher_name ?? "").toLowerCase();
        return (
          String(c.course_no).includes(q) ||
          c.id.toLowerCase().includes(q) ||
          c.code.toLowerCase().includes(q) ||
          (c.name ?? "").toLowerCase().includes(q) ||
          subjectLabel.includes(q) ||
          teacherLabel.includes(q)
        );
      });
    }
    return data;
  }, [search, courses]);

  return (
    <div>
      <PageHeading>Course</PageHeading>

        <div className="flex flex-wrap items-center gap-4 mb-4">
        <div className="w-full max-w-sm"><SearchInput value={search} onChange={setSearch} placeholder="C-ID, C-Code, P-Code, W-Code" /></div>
        <Button variant="secondary" size="md" onClick={() => {}}>Search</Button>
        <Link to="/courses/create" className="px-4 py-2 text-sm rounded-md bg-[var(--color-wi-green)] hover:bg-[var(--color-wi-green-dark)] text-white inline-block">
          Create
        </Link>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-[13px]">
          <thead>
            <tr className="border-b-2 border-gray-200">
              <th className="text-left py-2 px-2 font-semibold text-gray-700">C-ID</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">C-Code</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Year</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Teacher</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Subject</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Hour</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Student</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Type</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700"></th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((course) => (
              <tr key={course.id} className="border-b border-gray-200 hover:bg-gray-50">
                <td className="py-3 px-2 font-mono text-xs text-gray-700">{course.course_no}</td>
                <td className="py-3 px-2 font-mono text-xs text-gray-700">{course.code}</td>
                <td className="py-3 px-2">{course.year ?? "—"}</td>
                <td className="py-3 px-2">{course.teacher_name || "—"}</td>
                <td className="py-3 px-2">
                  {course.subject_code ? `[${course.subject_code}] ` : ""}
                  {course.subject_name || "—"}
                </td>
                <td className="py-3 px-2">{course.hour ?? "—"}</td>
                <td className="py-3 px-2">{course.student_count ?? "—"}</td>
                <td className="py-3 px-2">{course.course_type ?? "—"}</td>
                <td className="py-3 px-2">
                  <Link to={`/courses/${course.id}`} className="px-3 py-1 text-xs bg-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary-dark)] text-white rounded-sm inline-block">
                    detail
                  </Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {loading && <LoadingSkeleton type="table" lines={3} />}

      {!loading && filtered.length === 0 && (
        <EmptyState message="No courses found" />
      )}
    </div>
  );
}
