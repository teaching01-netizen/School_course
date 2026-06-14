import { useState, useEffect, useCallback } from "react";
import { apiJson } from "../api/client";
import CrossStudyStudentSearch from "../components/crm/CrossStudyStudentSearch";
import CrossStudyCrmRowCard from "../components/crm/CrossStudyCrmRowCard";
import CrossStudyAssignmentForm from "../components/crm/CrossStudyAssignmentForm";
import CrossStudyAssignmentList from "../components/crm/CrossStudyAssignmentList";
import PageHeading from "../components/ui/PageHeading";
import type { StudentLookupResponse } from "../types/crossStudy";

export default function CrossStudyPage() {
  const [lookupResult, setLookupResult] = useState<StudentLookupResponse | null>(null);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);
  const [courses, setCourses] = useState<{ id: string; code: string; name: string; subject_name: string }[]>([]);
  const [lastSearchedWcode, setLastSearchedWcode] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [reviewCount, setReviewCount] = useState(0);

  const loadCourses = useCallback(async () => {
    try {
      const items = await apiJson<any[]>("/api/v1/courses");
      setCourses(items.map((c) => ({ id: c.id, code: c.code, name: c.name, subject_name: c.subject_name ?? "" })));
    } catch {
      // Best-effort
    }
  }, []);

  useEffect(() => {
    loadCourses();
  }, [loadCourses]);

  const handleSearch = async (wcode: string) => {
    setSearching(true);
    setSearchError(null);
    setLookupResult(null);
    setLastSearchedWcode(wcode);
    try {
      const res = await apiJson<StudentLookupResponse>(`/api/v1/cross-study/students/${encodeURIComponent(wcode)}`);
      setLookupResult(res);
    } catch (err) {
      setSearchError(err instanceof Error ? err.message : "Lookup failed");
    } finally {
      setSearching(false);
    }
  };

  const handleSaved = async () => {
    if (lastSearchedWcode) {
      await handleSearch(lastSearchedWcode);
    }
    setRefreshKey((k) => k + 1);
  };

  return (
    <div className="max-w-4xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <PageHeading>Cross-Study (เรียนไขว้) Assignment</PageHeading>
        {reviewCount > 0 && (
          <span
            aria-label={`${reviewCount} cross-study assignments need review`}
            className="inline-flex min-w-6 items-center justify-center rounded-full bg-amber-100 px-2 py-1 text-xs font-semibold text-amber-800"
          >
            {reviewCount}
          </span>
        )}
      </div>

      {/* Master list */}
      <section className="border border-gray-200 rounded-sm p-4">
        <h2 className="text-sm font-semibold text-gray-700 mb-3">All Cross-Study Assignments</h2>
        <CrossStudyAssignmentList
          refreshKey={refreshKey}
          onSelectWCode={handleSearch}
          onReviewCountChange={setReviewCount}
        />
      </section>

      {/* Separator */}
      <div className="flex items-center gap-2 text-xs text-gray-300">
        <span className="flex-1 border-t border-gray-200" />
        <span>or search a student below</span>
        <span className="flex-1 border-t border-gray-200" />
      </div>

      {/* Student lookup */}
      <section className="border border-gray-200 rounded-sm p-4">
        <h2 className="text-sm font-semibold text-gray-700 mb-3">Student Lookup</h2>
        <CrossStudyStudentSearch onSearch={handleSearch} loading={searching} />

        <div className="mt-4">
          {/* Loading state */}
          {searching && (
            <div className="text-sm text-gray-400 py-4">Loading student data...</div>
          )}

          {/* Error state */}
          {!searching && searchError && (
            <div className="rounded-sm border border-red-200 bg-red-50 p-3 text-sm text-red-700">
              {searchError}
            </div>
          )}

          {/* Student not found in CRM */}
          {!searching && lookupResult && lookupResult.crm_row === null && (
            <div className="rounded-sm border border-amber-200 bg-amber-50 p-3 text-sm text-amber-700">
              No CRM data found for this student in the active snapshot.
              Cross-study assignment requires a CRM row with "Extra note" and a source course.
            </div>
          )}

          {/* Student found with CRM row */}
          {!searching && lookupResult && lookupResult.crm_row && (
            <div className="space-y-4">
              {/* Student info header */}
              <div className="flex items-center gap-3 p-3 bg-gray-50 rounded-sm">
                <div className="w-8 h-8 rounded-full bg-gray-900 text-white flex items-center justify-center text-sm font-semibold">
                  {lookupResult.student.full_name.charAt(0)}
                </div>
                <div>
                  <div className="font-semibold text-sm">{lookupResult.student.full_name}</div>
                  <div className="text-xs text-gray-500 font-mono">{lookupResult.student.wcode}</div>
                </div>
              </div>

              {/* CRM row card */}
              <CrossStudyCrmRowCard crmRow={lookupResult.crm_row} />

              {/* Empty extra note warning */}
              {!lookupResult.crm_row.extra_note && (
                <div className="rounded-sm border border-amber-200 bg-amber-50 p-3 text-xs text-amber-700">
                  "Extra note" column is empty for this row. The note helps determine which section
                  the student belongs to. You can still assign manually below.
                </div>
              )}

              {/* Assignment form */}
              <CrossStudyAssignmentForm
                student={lookupResult.student}
                crmRow={lookupResult.crm_row}
                currentAssignment={lookupResult.current_assignment}
                courses={courses}
                onSaved={handleSaved}
              />
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
