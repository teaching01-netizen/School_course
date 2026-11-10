import { useCallback, useEffect, useRef, useState } from "react";
import { ExternalLink } from "lucide-react";
import { Link } from "react-router-dom";
import CourseLevelManagerPanel from "../components/CourseLevelManagerPanel";
import CourseLinkSuggestionsPanel from "../components/CourseLinkSuggestionsPanel";
import { apiJson } from "../api/client";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import type { SitInRule } from "../types";
import type { CourseLevelItem, CourseMergeGroupConfig, RootCourseGroupInfo } from "../utils/levels";

type CourseLevelsPage = {
  items: CourseLevelItem[];
  total: number;
  limit: number;
  offset: number;
};

const COURSE_PAGE_SIZE = 100;
const SIT_IN_RULES_HREF = "/admin/operations?tab=sit-in-rules";

function parseCourseLevelsPage(value: CourseLevelItem[] | CourseLevelsPage): CourseLevelsPage {
  if (Array.isArray(value)) return { items: value, total: value.length, limit: value.length, offset: 0 };
  return value;
}

async function loadAllCourseLevels(): Promise<CourseLevelItem[]> {
  const firstResponse = await apiJson<CourseLevelItem[] | CourseLevelsPage>(
    `/api/v1/admin/course-levels?limit=${COURSE_PAGE_SIZE}&offset=0`,
    { method: "GET" },
  );
  const firstPage = parseCourseLevelsPage(firstResponse);
  if (firstPage.total <= firstPage.items.length) return firstPage.items;

  const remainingOffsets: number[] = [];
  for (let offset = firstPage.items.length; offset < firstPage.total; offset += COURSE_PAGE_SIZE) remainingOffsets.push(offset);
  const remainingPages = await Promise.all(remainingOffsets.map((offset) => apiJson<CourseLevelItem[] | CourseLevelsPage>(
    `/api/v1/admin/course-levels?limit=${COURSE_PAGE_SIZE}&offset=${offset}`,
    { method: "GET" },
  )));
  return [firstPage.items, ...remainingPages.map((page) => parseCourseLevelsPage(page).items)].flat();
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

export default function CourseLevels() {
  const [courses, setCourses] = useState<CourseLevelItem[]>([]);
  const [groups, setGroups] = useState<RootCourseGroupInfo[]>([]);
  const [rules, setRules] = useState<SitInRule[]>([]);
  const [mergeGroups, setMergeGroups] = useState<CourseMergeGroupConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [requiredError, setRequiredError] = useState<string | null>(null);
  const [rulesError, setRulesError] = useState<string | null>(null);
  const [mergeConfigError, setMergeConfigError] = useState<string | null>(null);
  const [retryingRules, setRetryingRules] = useState(false);
  const [retryingMergeConfig, setRetryingMergeConfig] = useState(false);
  const requestRef = useRef(0);
  const rulesRequestRef = useRef(0);
  const mergeRequestRef = useRef(0);

  const loadWorkspace = useCallback(async () => {
    const requestId = ++requestRef.current;
    setLoading(true);
    setRequiredError(null);
    try {
      const [coursesResult, groupsResult, rulesResult, mergeResult] = await Promise.allSettled([
        loadAllCourseLevels(),
        apiJson<RootCourseGroupInfo[]>("/api/v1/admin/root-course-groups", { method: "GET" }),
        apiJson<SitInRule[]>("/api/v1/admin/sit-in-rules", { method: "GET" }),
        apiJson<CourseMergeGroupConfig[]>("/api/v1/admin/course-merge-groups", { method: "GET" }),
      ]);
      if (requestId !== requestRef.current) return;

      if (coursesResult.status === "fulfilled") setCourses(coursesResult.value);
      if (groupsResult.status === "fulfilled") setGroups(groupsResult.value ?? []);

      if (rulesResult.status === "fulfilled") {
        setRules(rulesResult.value ?? []);
        setRulesError(null);
      } else {
        setRulesError(errorMessage(rulesResult.reason, "Sit-in rules are unavailable."));
      }

      if (mergeResult.status === "fulfilled") {
        setMergeGroups(mergeResult.value ?? []);
        setMergeConfigError(null);
      } else {
        setMergeConfigError(errorMessage(mergeResult.reason, "Linked course settings are unavailable."));
      }

      if (coursesResult.status === "rejected" || groupsResult.status === "rejected") {
        const reason = coursesResult.status === "rejected" ? coursesResult.reason : groupsResult.status === "rejected" ? groupsResult.reason : null;
        setRequiredError(errorMessage(reason, "The course level workspace could not be loaded."));
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadWorkspace();
    return () => {
      requestRef.current += 1;
      rulesRequestRef.current += 1;
      mergeRequestRef.current += 1;
    };
  }, [loadWorkspace]);

  const retryRules = useCallback(async () => {
    const requestId = ++rulesRequestRef.current;
    setRetryingRules(true);
    try {
      const data = await apiJson<SitInRule[]>("/api/v1/admin/sit-in-rules", { method: "GET" });
      if (requestId !== rulesRequestRef.current) return;
      setRules(data ?? []);
      setRulesError(null);
    } catch (error) {
      if (requestId === rulesRequestRef.current) setRulesError(errorMessage(error, "Sit-in rules are unavailable."));
    } finally {
      if (requestId === rulesRequestRef.current) setRetryingRules(false);
    }
  }, []);

  const retryMergeConfig = useCallback(async () => {
    const requestId = ++mergeRequestRef.current;
    setRetryingMergeConfig(true);
    try {
      const data = await apiJson<CourseMergeGroupConfig[]>("/api/v1/admin/course-merge-groups", { method: "GET" });
      if (requestId !== mergeRequestRef.current) return;
      setMergeGroups(data ?? []);
      setMergeConfigError(null);
    } catch (error) {
      if (requestId === mergeRequestRef.current) setMergeConfigError(errorMessage(error, "Linked course settings are unavailable."));
    } finally {
      if (requestId === mergeRequestRef.current) setRetryingMergeConfig(false);
    }
  }, []);

  const refreshAfterLink = useCallback(async () => {
    const requestId = ++requestRef.current;
    try {
      const [coursesData, groupsData, mergeData] = await Promise.all([
        loadAllCourseLevels(),
        apiJson<RootCourseGroupInfo[]>("/api/v1/admin/root-course-groups", { method: "GET" }),
        apiJson<CourseMergeGroupConfig[]>("/api/v1/admin/course-merge-groups", { method: "GET" }),
      ]);
      if (requestId !== requestRef.current) return;
      setCourses(coursesData);
      setGroups(groupsData ?? []);
      setMergeGroups(mergeData ?? []);
      setMergeConfigError(null);
    } catch (error) {
      if (requestId === requestRef.current) {
        setMergeConfigError(errorMessage(error, "Linked course settings could not be refreshed."));
      }
      throw error;
    }
  }, []);

  return (
    <div className="w-full space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <PageHeading>Course Levels</PageHeading>
          <p className="text-sm text-[var(--color-wi-text-light)]">Manage course groups, level assignments, and readiness status from one workspace.</p>
        </div>
        <Link
          to={SIT_IN_RULES_HREF}
          className="inline-flex min-h-[32px] items-center gap-1.5 rounded-sm border border-wi-line bg-white px-3 py-1.5 text-sm font-medium text-[var(--color-wi-text)] hover:bg-[var(--color-wi-row-alt)]"
        >
          Open Sit-in Rules
          <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
        </Link>
      </header>

      <section aria-labelledby="course-level-manager-heading" className="rounded-md border border-wi-line bg-white p-4 sm:p-5">
        <div>
          <h2 id="course-level-manager-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Course level manager</h2>
          <p className="mt-1 max-w-3xl text-sm text-[var(--color-wi-text-light)]">
            Set levels, move courses between groups, and review readiness. Levels save per course.
          </p>
        </div>
        <div className="mt-4">
          {requiredError ? (
            <div role="alert" className="rounded-sm border border-[var(--color-wi-red)]/20 bg-[var(--color-wi-danger-bg)] px-4 py-4">
              <p className="text-sm font-semibold text-[var(--color-wi-red-dark)]">Course levels could not be loaded.</p>
              <p className="mt-1 text-sm text-[var(--color-wi-text)]">{requiredError}</p>
              <Button
                variant="secondary"
                size="sm"
                className="mt-3"
                loading={loading}
                aria-label="Retry loading course levels"
                onClick={() => void loadWorkspace()}
              >
                Retry
              </Button>
            </div>
          ) : loading ? (
            <LoadingSkeleton type="table" lines={5} />
          ) : (
            <CourseLevelManagerPanel
              courses={courses}
              groups={groups}
              rules={rules}
              mergeGroups={mergeGroups}
              rulesError={rulesError}
              rulesRetrying={retryingRules}
              onRetryRules={() => void retryRules()}
              mergeConfigError={mergeConfigError}
              mergeConfigRetrying={retryingMergeConfig}
              onRetryMergeConfig={() => void retryMergeConfig()}
              onCoursesChange={setCourses}
              onGroupsChange={setGroups}
            />
          )}
        </div>
      </section>

      <CourseLinkSuggestionsPanel onLinked={refreshAfterLink} />
    </div>
  );
}
