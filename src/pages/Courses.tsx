import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { ChevronRight, Trash2 } from "lucide-react";
import { apiJson } from "@/api/client";
import { useToast } from "@/hooks/useToast";
import { useApiQuery } from "@/hooks/useApiQuery";
import { useCourseStudents } from "@/hooks/useCourseStudents";
import type { User } from "@/types";
import PageHeading from "@/components/ui/PageHeading";
import SearchInput from "@/components/ui/SearchInput";
import Button from "@/components/ui/Button";
import EmptyState from "@/components/ui/EmptyState";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import Modal from "@/components/Modal";
import StudentStatusBadge from "@/components/StudentStatusBadge";
import CourseAttendeeRow from "@/components/CourseAttendeeRow";

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
    legacy_course_id?: string | null;
    teachers?: { id: string; username: string }[];
  };

  const [searchParams, setSearchParams] = useSearchParams();
  const [search, setSearch] = useState(searchParams.get("q") ?? "");
  const teacherFilter = searchParams.get("teacher_id") ?? "";
  const { addToast } = useToast();

  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [teachers, setTeachers] = useState<User[]>([]);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const selectedRef = useRef(selected);
  selectedRef.current = selected;

  const [expandedIds, setExpandedIds] = useState<Set<string>>(() => new Set());
  const { cache, loading: studentsLoading, errors: studentsErrors, fetchStudents } = useCourseStudents();

  const { data: courses, loading, error, refetch } = useApiQuery<CourseRow[]>("/api/v1/courses");

  useEffect(() => {
    if (error) addToast("error", error.message);
  }, [error]);

  useEffect(() => {
    apiJson<User[]>("/api/v1/users?role=Teacher", { method: "GET" })
      .then(setTeachers)
      .catch(() => {});
  }, []);

  function updateFilter(key: string, value: string) {
    const params = new URLSearchParams(searchParams);
    if (value) params.set(key, value);
    else params.delete(key);
    setSearchParams(params);
  }

  const filtered = useMemo(() => {
    let data = [...(courses ?? [])];
    if (teacherFilter === "__none__") {
      data = data.filter((c) => !c.teacher_id);
    } else if (teacherFilter) {
      data = data.filter((c) => c.teacher_id === teacherFilter);
    }
    if (search) {
      const q = search.toLowerCase();
      data = data.filter((c) => {
        const subjectLabel = `${c.subject_code} ${c.subject_name}`.toLowerCase();
        const teacherLabel = (c.teacher_name ?? "").toLowerCase();
        const allTeacherLabels = (c.teachers ?? []).map((t) => t.username.toLowerCase()).join(" ");
        return (
          String(c.course_no).includes(q) ||
          c.id.toLowerCase().includes(q) ||
          c.code.toLowerCase().includes(q) ||
          (c.name ?? "").toLowerCase().includes(q) ||
          subjectLabel.includes(q) ||
          teacherLabel.includes(q) ||
          allTeacherLabels.includes(q)
        );
      });
    }
    return data;
  }, [search, teacherFilter, courses]);

  const allSelected = filtered.length > 0 && filtered.every((c) => selected.has(c.id));
  const selectedCount = selected.size;

  function handleSelectAll(checked: boolean) {
    setSelected(checked ? new Set(filtered.map((c) => c.id)) : new Set());
  }

  function handleSelectRow(id: string, checked: boolean) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  function handleToggleExpand(courseId: string) {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(courseId)) {
        next.delete(courseId);
      } else {
        next.add(courseId);
        void fetchStudents(courseId);
      }
      return next;
    });
  }

  async function handleBatchDelete() {
    setDeleting(true);
    const ids = [...selectedRef.current];
    try {
      const result = await apiJson<{
        succeeded: string[];
        failed: Array<{ id: string; error: string }>;
        total_processed: number;
      }>("/api/v1/courses/batch-delete", {
        method: "POST",
        body: JSON.stringify({ ids }),
      });
      if (result.failed.length === 0) {
        addToast("success", `${result.succeeded.length} courses deleted`);
      } else {
        addToast(
          "error",
          `${result.succeeded.length} succeeded, ${result.failed.length} failed`
        );
      }
      setConfirmDelete(false);
      setSelected(new Set());
      await refetch();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Batch delete failed");
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div>
      <PageHeading>Course</PageHeading>

      <section className="mb-4 rounded-sm border border-gray-200 bg-white p-3" aria-label="Course filters">
        <div className="flex flex-wrap items-center gap-3">
          <div className="w-full max-w-sm">
            <SearchInput
              value={search}
              onChange={(value) => {
                setSearch(value);
                updateFilter("q", value);
              }}
              placeholder="C-ID, C-Code, P-Code, W-Code"
            />
          </div>
          <select
            aria-label="Teacher filter"
            value={teacherFilter}
            onChange={(event) => updateFilter("teacher_id", event.target.value)}
            className="w-full max-w-[200px] rounded-sm border border-gray-300 px-2 py-1 text-sm"
          >
            <option value="">All teachers</option>
            <option value="__none__">No teacher</option>
            {teachers.map((t) => (
              <option key={t.id} value={t.id}>
                {t.username}
              </option>
            ))}
          </select>
          <Button variant="secondary" size="md" onClick={() => {}}>
            Search
          </Button>
          <Link
            to="/courses/create"
            className="px-4 py-2 text-sm rounded-md bg-[var(--color-wi-green)] hover:bg-[var(--color-wi-green-dark)] text-white inline-block"
          >
            Create
          </Link>
        </div>
      </section>

      {selectedCount > 0 ? (
        <div className="mb-3 flex items-center gap-3 rounded-sm border border-blue-100 bg-blue-50 px-3 py-2 text-sm">
          <span className="font-medium text-blue-800">{selectedCount} selected</span>
          <Button
            variant="danger"
            size="sm"
            onClick={() => setConfirmDelete(true)}
          >
            <Trash2 className="mr-1.5 h-4 w-4" />
            Delete Selected
          </Button>
        </div>
      ) : null}

      <div className="overflow-x-auto">
        <table className="w-full text-[13px]">
          <thead>
            <tr className="border-b-2 border-gray-200">
              <th className="w-8 px-2">
                <input
                  aria-label="Select all courses"
                  type="checkbox"
                  checked={allSelected}
                  ref={(el) => {
                    if (el) {
                      el.indeterminate = selectedCount > 0 && !allSelected;
                    }
                  }}
                  onChange={(event) => handleSelectAll(event.target.checked)}
                />
              </th>
              <th className="w-8 px-2"></th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">C-ID</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">C-Code</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Year</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Teacher</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Subject</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Hour</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Student</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Type</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700">Legacy</th>
              <th className="text-left py-2 px-2 font-semibold text-gray-700"></th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((course) => (
              <Fragment key={course.id}>
                <tr className="border-b border-gray-200 hover:bg-gray-50">
                  <td className="py-3 px-2">
                    <input
                      aria-label={`Select ${course.code}`}
                      type="checkbox"
                      checked={selected.has(course.id)}
                      onChange={(event) => handleSelectRow(course.id, event.target.checked)}
                    />
                  </td>
                  <td className="w-8 py-3 px-1">
                    <button
                      type="button"
                      onClick={() => handleToggleExpand(course.id)}
                      className="flex items-center justify-center h-6 w-6 rounded-sm text-gray-400 hover:text-gray-700 hover:bg-gray-200 cursor-pointer"
                      aria-label={expandedIds.has(course.id) ? "Collapse attendees" : "Expand attendees"}
                      aria-expanded={expandedIds.has(course.id)}
                    >
                      <ChevronRight
                        className={`h-4 w-4 transition-transform duration-150 ${
                          expandedIds.has(course.id) ? "rotate-90" : ""
                        }`}
                      />
                    </button>
                  </td>
                  <td className="py-3 px-2 font-mono text-xs text-gray-700">{course.course_no}</td>
                  <td className="py-3 px-2 font-mono text-xs text-gray-700">{course.code}</td>
                  <td className="py-3 px-2">{course.year ?? "—"}</td>
                  <td className="py-3 px-2">
                    {(course.teachers ?? []).length > 0
                      ? (course.teachers ?? []).map((t) => (
                          <span key={t.id} className="inline-block mr-1 mb-0.5 px-1.5 py-0.5 text-xs bg-blue-50 text-blue-700 border border-blue-200 rounded-sm">
                            {t.username}
                          </span>
                        ))
                      : course.teacher_name || "—"}
                  </td>
                  <td className="py-3 px-2">
                    {course.subject_code ? `[${course.subject_code}] ` : ""}
                    {course.subject_name || "—"}
                  </td>
                  <td className="py-3 px-2">{course.hour ?? "—"}</td>
                  <td className="py-3 px-2">
                    <StudentStatusBadge count={course.student_count} />
                  </td>
                  <td className="py-3 px-2">{course.course_type ?? "—"}</td>
                  <td className="py-3 px-2">
                    {course.legacy_course_id ? (
                      <a
                        href={`https://warwick.azurewebsites.net/Admin/Courses/Detail?id=${course.legacy_course_id}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-blue-600 underline text-xs"
                        title="Linked to old system"
                      >
                        ⚡
                      </a>
                    ) : (
                      <span className="text-gray-300">—</span>
                    )}
                  </td>
                  <td className="py-3 px-2">
                    <Link
                      to={`/courses/${course.id}`}
                      className="px-3 py-1 text-xs bg-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary-dark)] text-white rounded-sm inline-block"
                    >
                      detail
                    </Link>
                  </td>
                </tr>
                {expandedIds.has(course.id) && (
                  <tr className="border-b border-gray-200">
                    <td colSpan={12} className="p-0">
                      <CourseAttendeeRow
                        students={cache[course.id] ?? []}
                        loading={!!studentsLoading[course.id]}
                        error={studentsErrors[course.id] ?? null}
                      />
                    </td>
                  </tr>
                )}
              </Fragment>
            ))}
          </tbody>
        </table>
      </div>

      {loading && <LoadingSkeleton type="table" lines={3} />}

      {!loading && filtered.length === 0 && (
        <EmptyState message="No courses found" />
      )}

      {confirmDelete ? (
        <Modal
          title={`Delete ${selectedCount} course${selectedCount === 1 ? "" : "s"}?`}
          onClose={() => setConfirmDelete(false)}
          footer={
            <>
              <Button variant="secondary" onClick={() => setConfirmDelete(false)}>
                Cancel
              </Button>
              <Button variant="danger" loading={deleting} onClick={() => void handleBatchDelete()}>
                Delete
              </Button>
            </>
          }
        >
          <p className="text-sm text-gray-600">
            This permanently deletes the selected courses and all associated data
            (sessions, enrollments, attendance records). This action cannot be undone.
          </p>
        </Modal>
      ) : null}
    </div>
  );
}
