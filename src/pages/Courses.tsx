import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { ChevronRight, Trash2 } from "lucide-react";
import { apiJson } from "@/api/client";
import { useToast } from "@/hooks/useToast";
import { useApiQuery } from "@/hooks/useApiQuery";
import { useCourseStudents } from "@/features/courses/hooks/useCourseStudents";
import type { User } from "@/types";
import PageHeading from "@/components/ui/PageHeading";
import SearchInput from "@/components/ui/SearchInput";
import Button from "@/components/ui/Button";
import EmptyState from "@/components/ui/EmptyState";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import Modal from "@/components/Modal";
import StudentStatusBadge from "@/components/StudentStatusBadge";
import CourseAttendeeRow from "@/components/CourseAttendeeRow";

const PAGE_SIZE = 50;

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
  teachers?: { id: string; username: string; full_name?: string | null }[];
};

type CourseListPage = {
  items: CourseRow[];
  total_count: number;
  offset: number;
  limit: number;
};

type CourseBucket = "live" | "archived";

export default function Courses() {
  const [searchParams, setSearchParams] = useSearchParams();
  const { addToast } = useToast();

  const bucket: CourseBucket = searchParams.get("status") === "archived" ? "archived" : "live";
  const typeFilter = searchParams.get("type") ?? "";
  const teacherFilter = searchParams.get("teacher_id") ?? "";
  const urlQuery = searchParams.get("q") ?? "";
  const offset = Math.max(0, Number(searchParams.get("offset") ?? 0) || 0);
  const [searchInput, setSearchInput] = useState(urlQuery);

  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [teachers, setTeachers] = useState<User[]>([]);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const selectedRef = useRef(selected);
  selectedRef.current = selected;

  const [expandedIds, setExpandedIds] = useState<Set<string>>(() => new Set());
  const { cache, loading: studentsLoading, errors: studentsErrors, fetchStudents } = useCourseStudents();

  // The list is fully server-driven: every filter and the page offset live in
  // the URL, and the query key is URL-derived so each combination gets its own
  // cache entry. keepPreviousData keeps the current page visible while the
  // next one loads.
  const requestQuery = useMemo(() => {
    const params = new URLSearchParams();
    params.set("limit", String(PAGE_SIZE));
    params.set("offset", String(offset));
    if (bucket === "archived") params.set("status", "archived");
    if (typeFilter) params.set("type", typeFilter);
    if (teacherFilter) params.set("teacher_id", teacherFilter);
    if (urlQuery) params.set("q", urlQuery);
    return params.toString();
  }, [bucket, typeFilter, teacherFilter, urlQuery, offset]);

  const requestUrl = `/api/v1/courses?${requestQuery}`;
  const { data: page, loading, error, refetch } = useApiQuery<CourseListPage>(requestUrl, [], { keepPreviousData: true });

  // Keep the search box in sync with the URL (deep links, back/forward).
  useEffect(() => {
    setSearchInput(urlQuery);
  }, [urlQuery]);

  // Debounce typing into the q URL param; any q change resets the offset.
  useEffect(() => {
    const value = searchInput.trim();
    if (value === urlQuery) return;
    const timer = setTimeout(() => {
      const params = new URLSearchParams(searchParams);
      if (value) params.set("q", value);
      else params.delete("q");
      params.delete("offset");
      setSearchParams(params);
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput, urlQuery, searchParams, setSearchParams]);

  useEffect(() => {
    if (error) addToast("error", error.message);
  }, [error, addToast]);

  useEffect(() => {
    apiJson<User[]>("/api/v1/users?role=Teacher", { method: "GET" })
      .then(setTeachers)
      .catch(() => {});
  }, []);

  useEffect(() => {
    setSelected(new Set());
  }, [requestQuery]);

  function updateFilter(key: string, value: string) {
    const params = new URLSearchParams(searchParams);
    if (value) params.set(key, value);
    else params.delete(key);
    if (key !== "offset") params.delete("offset");
    setSearchParams(params);
  }

  function setBucket(next: CourseBucket) {
    const params = new URLSearchParams(searchParams);
    if (next === "archived") params.set("status", "archived");
    else params.delete("status");
    params.delete("offset");
    setSearchParams(params);
  }

  const items = page?.items ?? [];
  const allSelected = items.length > 0 && items.every((c) => selected.has(c.id));
  const selectedCount = selected.size;
  const hasPrevious = offset > 0;
  const hasNext = offset + PAGE_SIZE < (page?.total_count ?? 0);
  const totalPages = Math.ceil((page?.total_count ?? 0) / PAGE_SIZE);
  const currentPage = Math.floor(offset / PAGE_SIZE) + 1;

  function handleSelectAll(checked: boolean) {
    setSelected(checked ? new Set(items.map((c) => c.id)) : new Set());
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

  function jumpToPage(event: React.ChangeEvent<HTMLInputElement>) {
    const next = Math.max(1, Math.min(totalPages, Number(event.target.value) || 1));
    updateFilter("offset", String((next - 1) * PAGE_SIZE));
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

      <section className="mb-4 rounded-sm border border-[var(--color-wi-line)] bg-white p-3" aria-label="Course filters">
        <div className="flex flex-wrap items-center gap-3">
          <div className="w-full max-w-sm">
            <SearchInput
              value={searchInput}
              onChange={setSearchInput}
              placeholder="C-ID, C-Code, P-Code, W-Code"
            />
          </div>
          <select
            aria-label="Course type filter"
            value={typeFilter}
            onChange={(event) => updateFilter("type", event.target.value)}
            className="w-full max-w-[200px] rounded-sm border border-[var(--color-wi-line)] px-2 py-1 text-sm"
          >
            <option value="">All types</option>
            <option value="private">Private</option>
            <option value="general">General</option>
          </select>
          <select
            aria-label="Teacher filter"
            value={teacherFilter}
            onChange={(event) => updateFilter("teacher_id", event.target.value)}
            className="w-full max-w-[200px] rounded-sm border border-[var(--color-wi-line)] px-2 py-1 text-sm"
          >
            <option value="">All teachers</option>
            <option value="none">No teacher</option>
            {teachers.map((t) => (
              <option key={t.id} value={t.id}>
                {t.full_name || t.username}
              </option>
            ))}
          </select>
          <Link
            to="/courses/create"
            className="px-4 py-2 text-sm rounded-md bg-[var(--color-wi-green)] hover:bg-[var(--color-wi-green-dark)] text-white inline-block"
          >
            Create
          </Link>
        </div>
      </section>

      <div className="mb-4 flex gap-4 border-b border-b-[var(--color-wi-line)] text-sm" aria-label="Course sections">
        <button
          type="button"
          onClick={() => setBucket("live")}
          className={`border-b px-1 pb-2 font-medium transition-colors ${
            bucket === "live"
              ? "border-[var(--color-wi-primary)] text-[var(--color-wi-primary)]"
              : "border-transparent text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text)]"
          }`}
          aria-current={bucket === "live" ? "page" : undefined}
        >
          Active courses
        </button>
        <button
          type="button"
          onClick={() => setBucket("archived")}
          className={`border-b px-1 pb-2 font-medium transition-colors ${
            bucket === "archived"
              ? "border-[var(--color-wi-primary)] text-[var(--color-wi-primary)]"
              : "border-transparent text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text)]"
          }`}
          aria-current={bucket === "archived" ? "page" : undefined}
        >
          Archived courses
        </button>
      </div>

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

      {loading && page === null ? (
        <LoadingSkeleton type="table" lines={5} />
      ) : (
        <>
          <div className="overflow-x-auto data-table-wrapper">
            <table className="w-full table-fixed text-[13px]">
              <caption className="sr-only">List of courses</caption>
              <colgroup>
                <col className="w-[5%]" />
                <col className="w-[4%]" />
                <col className="w-[5%]" />
                <col className="w-[10%]" />
                <col className="w-[5%]" />
                <col className="w-[10%]" />
                <col className="w-[28%]" />
                <col className="w-[5%]" />
                <col className="w-[10%]" />
                <col className="w-[7%]" />
                <col className="w-[5%]" />
                <col className="w-[6%]" />
              </colgroup>
              <thead>
                <tr className="border-b border-b-[var(--color-wi-line)]">
                  <th scope="col" className="w-8 px-2">
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
                  <th scope="col" className="w-8 px-2"></th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">C-ID</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">C-Code</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Year</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Teacher</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Subject</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Hour</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Student</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Type</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Legacy</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]"></th>
                </tr>
              </thead>
              <tbody>
                {items.map((course) => (
                  <Fragment key={course.id}>
                    <tr className="border-b border-b-[var(--color-wi-line)] hover:bg-[var(--color-wi-row-alt)]">
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
                          className="flex items-center justify-center h-6 w-6 rounded-sm text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] cursor-pointer"
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
                      <td className="py-3 px-2 font-mono text-xs text-[var(--color-wi-text-light)]">{course.course_no}</td>
                      <td className="py-3 px-2 break-words font-mono text-xs text-[var(--color-wi-text-light)]">{course.code}</td>
                      <td className="py-3 px-2">{course.year ?? "—"}</td>
                      <td className="py-3 px-2 break-words">
                        {(course.teachers ?? []).length > 0
                          ? (course.teachers ?? []).map((t) => (
                              <span key={t.id} className="inline-block mr-1 mb-0.5 px-1.5 py-0.5 text-xs bg-blue-50 text-blue-700 border border-blue-200 rounded-sm">
                                {t.username}
                              </span>
                            ))
                          : course.teacher_name || "—"}
                      </td>
                      <td className="py-3 px-2 break-words">
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
                          <span className="text-xs font-medium text-[var(--color-wi-primary)]" title={`Managed by legacy sync, ID ${course.legacy_course_id}`}>Legacy</span>
                        ) : (
                          <span className="text-[var(--color-wi-text-light)]">—</span>
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
                      <tr className="border-b border-b-[var(--color-wi-line)]">
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

          {items.length === 0 && (
            <EmptyState message={bucket === "archived" ? "No archived courses found" : "No courses found"} />
          )}

          <div className="mt-3 flex items-center justify-between text-sm text-[var(--color-wi-text-light)]">
            <span>{page?.total_count ?? 0} records</span>
            <div className="flex items-center gap-2">
              <Button variant="secondary" size="sm" disabled={!hasPrevious} onClick={() => updateFilter("offset", String(Math.max(0, offset - PAGE_SIZE)))}>Previous</Button>
              <div className="flex items-center gap-1">
                <input aria-label="Go to page" type="number" min={1} max={totalPages} value={currentPage} onChange={jumpToPage} className="w-14 rounded-sm border border-[var(--color-wi-line)] px-2 py-1 text-sm text-center" />
                <span>of {totalPages}</span>
              </div>
              <Button variant="secondary" size="sm" disabled={!hasNext} onClick={() => updateFilter("offset", String(offset + PAGE_SIZE))}>Next</Button>
            </div>
          </div>
        </>
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
          <p className="text-sm text-[var(--color-wi-text-light)]">
            This permanently deletes the selected courses and all associated data
            (sessions, enrollments, attendance records). This action cannot be undone.
          </p>
        </Modal>
      ) : null}
    </div>
  );
}
