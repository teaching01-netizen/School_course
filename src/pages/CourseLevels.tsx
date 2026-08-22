import { useEffect, useState } from "react";
import Modal from "../components/Modal";
import CourseLevelManagerPanel from "../components/CourseLevelManagerPanel";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import type { SitInRule } from "../types";
import type { CourseLevelItem, RootCourseGroupInfo } from "../utils/levels";

type CourseLevelsPage = {
  items: CourseLevelItem[];
  total: number;
  limit: number;
  offset: number;
};

const COURSE_PAGE_SIZE = 100;

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

export default function CourseLevels() {
  const { addToast } = useToast();
  const [courses, setCourses] = useState<CourseLevelItem[]>([]);
  const [groups, setGroups] = useState<RootCourseGroupInfo[]>([]);
  const [rules, setRules] = useState<SitInRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [managerOpen, setManagerOpen] = useState(true);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [coursesData, groupsData, rulesData] = await Promise.all([
          loadAllCourseLevels(),
          apiJson<RootCourseGroupInfo[]>("/api/v1/admin/root-course-groups", { method: "GET" }),
          apiJson<SitInRule[]>("/api/v1/admin/sit-in-rules", { method: "GET" }),
        ]);
        if (cancelled) return;
        setCourses(coursesData);
        setGroups(groupsData);
        setRules(rulesData ?? []);
      } catch (error) {
        if (!cancelled) addToast("error", error instanceof Error ? error.message : "Failed to load course levels");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [addToast]);

  if (loading) return <LoadingSkeleton type="table" lines={5} />;

  return (
    <div className="w-full">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <PageHeading>Course Levels</PageHeading>
          <p className="text-sm text-[var(--color-wi-text-light)]">Manage course groups, level assignments, and readiness status from one workspace.</p>
        </div>
        {!managerOpen ? <Button onClick={() => setManagerOpen(true)}>Manage levels</Button> : null}
      </div>

      <div className="mt-8 rounded-md border border-dashed border-wi-line bg-[var(--color-wi-callout)] px-5 py-10 text-center">
        <p className="text-sm font-medium text-[var(--color-wi-text)]">Course level management is open in the manager.</p>
        <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">Select a course group to add, edit, or review levels.</p>
      </div>

      {managerOpen ? (
        <Modal title="Manage Course Levels" size="full" maxWidth="modal-course-levels" onClose={() => setManagerOpen(false)}>
          <CourseLevelManagerPanel
            courses={courses}
            groups={groups}
            rules={rules}
            onCoursesChange={(update) => setCourses(update)}
            onGroupsChange={(update) => setGroups(update)}
          />
        </Modal>
      ) : null}
    </div>
  );
}
