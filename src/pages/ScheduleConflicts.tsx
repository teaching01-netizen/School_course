import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import PageHeading from "@/components/ui/PageHeading";
import { ConflictFilters } from "@/components/schedule-conflicts/ConflictFilters";
import { ConflictSummary } from "@/components/schedule-conflicts/ConflictSummary";
import { ConflictTable } from "@/components/schedule-conflicts/ConflictTable";
import Button from "@/components/ui/Button";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import { parseConflictOverviewResponse, parseConflictSummaryResponse, scheduleConflictsSummaryURL, scheduleConflictsURL } from "@/features/scheduling/api/conflictOverviewApi";
import { useApiQuery } from "@/hooks/useApiQuery";

type Lookup = Readonly<{ id: string; label: string }>;

export default function ScheduleConflicts() {
  const [searchParams, setSearchParams] = useSearchParams();
  const urlQuery = searchParams.get("q") ?? "";
  const [searchInput, setSearchInput] = useState(urlQuery);
  const requestURL = useMemo(() => scheduleConflictsURL(searchParams), [searchParams]);
  const summaryURL = useMemo(() => scheduleConflictsSummaryURL(searchParams), [searchParams]);
  const query = useApiQuery<unknown>(requestURL, [], { keepPreviousData: true });
  const summaryQuery = useApiQuery<unknown>(summaryURL, [], { keepPreviousData: true });
  const subjectsQuery = useApiQuery<unknown>("/api/v1/subjects");
  const teachersQuery = useApiQuery<unknown>("/api/v1/users?role=Teacher");
  const studentsQuery = useApiQuery<unknown>("/api/v1/students?limit=200");
  const parsed = query.data === null ? null : parseConflictOverviewResponse(query.data);
  const summary = summaryQuery.data === null ? null : parseConflictSummaryResponse(summaryQuery.data);

  useEffect(() => setSearchInput(urlQuery), [urlQuery]);
  useEffect(() => {
    const value = searchInput.trim();
    if (value === urlQuery) return;
    const timer = window.setTimeout(() => {
      const next = new URLSearchParams(searchParams);
      if (value) next.set("q", value);
      else next.delete("q");
      next.delete("cursor");
      setSearchParams(next);
    }, 300);
    return () => window.clearTimeout(timer);
  }, [searchInput, urlQuery, searchParams, setSearchParams]);

  function updateFilter(key: string, value: string) {
    const next = new URLSearchParams(searchParams);
    if (value) next.set(key, value);
    else next.delete(key);
    if (key !== "cursor") next.delete("cursor");
    setSearchParams(next);
  }

  function updateDateFilter(from: string, to: string) {
    const next = new URLSearchParams(searchParams);
    if (from) next.set("date_from", from);
    else next.delete("date_from");
    if (to) next.set("date_to", to);
    else next.delete("date_to");
    next.delete("cursor");
    setSearchParams(next);
  }

  const subjects = lookupOptions(subjectsQuery.data, "name");
  const teachers = lookupOptions(teachersQuery.data, "full_name", "username");
  const students = lookupOptions(studentsQuery.data, "full_name", "wcode");
  return <div>
    <PageHeading>Schedule conflicts</PageHeading>
    {summary ? <ConflictSummary summary={summary} /> : <LoadingSkeleton type="card" lines={4} />}
    <ConflictFilters
      query={searchInput}
      type={searchParams.get("conflict_type") ?? ""}
      subjectId={searchParams.get("subject_id") ?? ""}
      teacherId={searchParams.get("teacher_id") ?? ""}
      studentId={searchParams.get("student_id") ?? ""}
      dateFrom={searchParams.get("date_from") ?? ""}
      dateTo={searchParams.get("date_to") ?? ""}
      subjects={subjects}
      teachers={teachers}
      students={students}
      onQueryChange={setSearchInput}
      onFilterChange={updateFilter}
      onDateChange={updateDateFilter}
    />
    {query.loading && parsed === null ? <LoadingSkeleton type="table" lines={6} /> : query.error && parsed === null ? (
      <div className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-700" role="alert"><p className="font-medium">Couldn&apos;t load schedule conflicts</p><p className="mt-1">{query.error.message}</p><Button variant="secondary" size="sm" className="mt-3" onClick={() => void query.refetch()}>Retry</Button></div>
    ) : parsed ? <>
      <ConflictTable items={parsed.items} refreshing={query.refreshing} />
      <div className="mt-3 flex items-center justify-between gap-3 text-sm text-[var(--color-wi-text-light)]"><span>{parsed.items.length} {parsed.items.length === 1 ? "conflict" : "conflicts"} on this page</span><div className="flex gap-2"><Button variant="secondary" size="sm" disabled={!parsed.has_prev || parsed.prev_cursor === null} onClick={() => updateFilter("cursor", parsed.prev_cursor ?? "")}>Previous</Button><Button variant="secondary" size="sm" disabled={!parsed.has_next || parsed.next_cursor === null} onClick={() => updateFilter("cursor", parsed.next_cursor ?? "")}>Next</Button></div></div>
    </> : null}
  </div>;
}

function lookupOptions(value: unknown, primary: string, secondary?: string): readonly Lookup[] {
  const source = Array.isArray(value) ? value : isRecord(value) && Array.isArray(value.items) ? value.items : [];
  return source.flatMap((item) => {
    if (!isRecord(item) || typeof item.id !== "string") return [];
    const primaryValue = item[primary];
    const secondaryValue = secondary ? item[secondary] : undefined;
    const label = typeof primaryValue === "string" && primaryValue ? primaryValue : typeof secondaryValue === "string" ? secondaryValue : "";
    return label ? [{ id: item.id, label }] : [];
  });
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
