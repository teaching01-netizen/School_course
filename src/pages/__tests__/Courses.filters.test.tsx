import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Courses from "../Courses";
import { ToastProvider } from "../../hooks/useToast";
import { queryClient } from "../../query/cache";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => { store[key] = value; }),
    removeItem: vi.fn((key: string) => { delete store[key]; }),
    clear: vi.fn(() => { store = {}; }),
    get length() { return Object.keys(store).length; },
    key: vi.fn((index: number) => Object.keys(store)[index] ?? null),
  };
})();
Object.defineProperty(globalThis, "localStorage", { value: localStorageMock, writable: true });

const TEACHERS = [
  { id: "t1", username: "Alice", role: "Teacher" as const },
  { id: "t2", username: "Bob", role: "Teacher" as const },
];

type Course = {
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
  legacy_course_id: string | null;
  has_overlap?: boolean;
  has_conflict?: boolean;
  absence_form_visible?: boolean;
  is_active_course?: boolean;
};

// 120 live courses: even indexes Private, odd General; every third course has
// teacher t1, the rest have no primary teacher.
const LIVE_COURSES: Course[] = Array.from({ length: 120 }, (_, i) => ({
  id: `live-${i}`,
  course_no: 1000 + i,
  code: `CODE-${String(i).padStart(3, "0")}`,
  name: `Course ${i}`,
  year: 2026,
  teacher_id: i % 3 === 0 ? "t1" : null,
  teacher_name: i % 3 === 0 ? "Alice" : "",
  subject_id: null,
  subject_code: "",
  subject_name: "",
  hour: null,
  student_count: null,
  course_type: i % 2 === 0 ? "Private" : "General",
  legacy_course_id: null,
  has_overlap: i === 1,
  has_conflict: i === 0,
  absence_form_visible: i !== 1,
  is_active_course: i !== 2,
}));

const ARCHIVED_COURSES: Course[] = Array.from({ length: 10 }, (_, i) => ({
  id: `arch-${i}`,
  course_no: 2000 + i,
  code: `ARCH-${String(i).padStart(3, "0")}`,
  name: `Archived Course ${i}`,
  year: 2024,
  teacher_id: null,
  teacher_name: "",
  subject_id: null,
  subject_code: "",
  subject_name: "",
  hour: null,
  student_count: null,
  course_type: "General",
  legacy_course_id: `LEG-${i}`,
  absence_form_visible: true,
  is_active_course: true,
}));

const courseRequests: string[] = [];

// Mirrors the server: /api/v1/courses?… returns the paginated envelope, and
// every request URL is recorded for assertions.
async function listMock(url: string): Promise<unknown> {
  if (url === "/api/v1/users?role=Teacher") return TEACHERS;
  const match = url.match(/^\/api\/v1\/courses\?(.*)$/);
  if (!match) throw new Error(`Unmocked: ${url}`);
  courseRequests.push(url);
  const params = new URLSearchParams(match[1]);
  let items = params.get("status") === "archived" ? ARCHIVED_COURSES : LIVE_COURSES;
  const type = params.get("type");
  if (type === "private") items = items.filter((c) => c.course_type === "Private");
  else if (type === "general") items = items.filter((c) => c.course_type === "General" || c.course_type === "Group");
  const teacherId = params.get("teacher_id");
  if (teacherId === "none") items = items.filter((c) => !c.teacher_id);
  else if (teacherId) items = items.filter((c) => c.teacher_id === teacherId);
  const absenceForm = params.get("absence_form");
  if (absenceForm === "active") items = items.filter((c) => c.is_active_course !== false && c.absence_form_visible !== false);
  else if (absenceForm === "hidden") items = items.filter((c) => c.is_active_course === false || c.absence_form_visible === false);
  const q = params.get("q");
  if (q) {
    const needle = q.toLowerCase();
    items = items.filter((c) => c.code.toLowerCase().includes(needle) || c.name.toLowerCase().includes(needle) || String(c.course_no).includes(needle));
  }
  const offset = Number(params.get("offset") ?? 0);
  const limit = Number(params.get("limit") ?? 50);
  return { items: items.slice(offset, offset + limit), total_count: items.length, offset, limit };
}

function renderCourses(initialPath = "/courses") {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <ToastProvider>
        <Courses />
      </ToastProvider>
    </MemoryRouter>
  );
}

async function waitForFirstPage() {
  await waitFor(() => {
    expect(screen.getByText("CODE-000")).toBeTruthy();
  });
}

describe("Courses filters and pagination", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    courseRequests.length = 0;
    queryClient.clear();
    mockApiJson.mockImplementation(listMock);
  });

  it("loads only live courses on mount (no archived request)", async () => {
    renderCourses();
    await waitForFirstPage();
    const fetches = courseRequests.filter((u) => u.startsWith("/api/v1/courses?"));
    expect(fetches).toHaveLength(1);
    expect(fetches[0]).toBe("/api/v1/courses?limit=50&offset=0");
    expect(fetches[0]).not.toContain("status=");
  });

  it("defaults the absence-form filter to All and supports active, hidden, and All", async () => {
    renderCourses();
    await waitForFirstPage();
    const select = screen.getByLabelText("Absence form filter") as HTMLSelectElement;

    expect(select.value).toBe("all");
    expect(Array.from(select.options).map((option) => option.value)).toEqual(["all", "active", "hidden"]);

    await userEvent.selectOptions(select, "active");
    await waitFor(() => expect(courseRequests.at(-1)).toContain("absence_form=active"));
    expect(screen.getByText("CODE-000")).toBeTruthy();
    expect(screen.queryByText("CODE-001")).toBeNull();
    expect(screen.queryByText("CODE-002")).toBeNull();

    await userEvent.selectOptions(select, "hidden");
    await waitFor(() => expect(courseRequests.at(-1)).toContain("absence_form=hidden"));
    expect(screen.getByText("CODE-001")).toBeTruthy();
    expect(screen.getByText("CODE-002")).toBeTruthy();
    expect(screen.queryByText("CODE-000")).toBeNull();

    await userEvent.selectOptions(select, "all");
    await waitFor(() => {
      expect(screen.getByText("CODE-000")).toBeTruthy();
      expect(screen.getByText("CODE-001")).toBeTruthy();
    });
    expect(select.value).toBe("all");
    expect(courseRequests.some((url) => url.includes("absence_form=all"))).toBe(false);
  });

  it("shows red conflict statuses and green clear statuses in the course table", async () => {
    renderCourses();
    await waitForFirstPage();
    expect(screen.getByLabelText("Conflict detected")).toBeTruthy();
    expect(screen.getByLabelText("Overlap detected")).toBeTruthy();
    expect(screen.getAllByLabelText("No conflict").length).toBeGreaterThan(0);
    expect(screen.getAllByLabelText("No overlap").length).toBeGreaterThan(0);
  });

  it("loads the archived bucket lazily when the tab is opened", async () => {
    renderCourses();
    await waitForFirstPage();
    expect(courseRequests.some((u) => u.includes("status=archived"))).toBe(false);

    await userEvent.click(screen.getByText("Archived courses"));
    await waitFor(() => {
      expect(screen.getByText("ARCH-000")).toBeTruthy();
    });
    const archivedFetch = courseRequests.find((u) => u.includes("status=archived"));
    expect(archivedFetch).toBeTruthy();
    expect(archivedFetch).toContain("offset=0");
    expect(screen.queryByText("CODE-000")).toBeNull();
  });

  it("deep link to the archived bucket renders archived courses on load", async () => {
    renderCourses("/courses?status=archived");
    await waitFor(() => {
      expect(screen.getByText("ARCH-000")).toBeTruthy();
    });
    expect(courseRequests[0]).toContain("status=archived");
    expect(screen.queryByText("CODE-000")).toBeNull();
  });

  it("sends the type param and resets the offset when the type filter changes", async () => {
    renderCourses();
    await waitForFirstPage();
    // Navigate to page 2 first, then change the type filter.
    await userEvent.click(screen.getByText("Next"));
    await waitFor(() => {
      expect(screen.getByText("CODE-050")).toBeTruthy();
    });

    await userEvent.selectOptions(screen.getByLabelText("Course type filter"), "general");
    await waitFor(() => {
      const last = courseRequests[courseRequests.length - 1];
      expect(last).toContain("type=general");
      expect(last).toContain("offset=0");
    });
  });

  it("debounces search input into a single q request", async () => {
    renderCourses();
    await waitForFirstPage();
    const input = screen.getByLabelText("Search");
    await userEvent.type(input, "abc", { delay: 100 });
    await waitFor(() => {
      expect(courseRequests.some((u) => u.includes("q=abc"))).toBe(true);
    });
    const qRequests = courseRequests.filter((u) => u.includes("q=abc"));
    expect(qRequests).toHaveLength(1);
    expect(qRequests[0]).toContain("offset=0");
  });

  it("paginates with Previous/Next and keepPreviousData", async () => {
    renderCourses();
    await waitForFirstPage();
    expect(screen.getByText("120 records")).toBeTruthy();
    expect(screen.getByText("of 3")).toBeTruthy();

    const previousButton = screen.getByText("Previous").closest("button") as HTMLButtonElement;
    expect(previousButton.disabled).toBe(true);
    const nextButton = screen.getByText("Next").closest("button") as HTMLButtonElement;
    expect(nextButton.disabled).toBe(false);

    await userEvent.click(screen.getByText("Next"));
    await waitFor(() => {
      expect(screen.getByText("CODE-050")).toBeTruthy();
    });
    expect(courseRequests[courseRequests.length - 1]).toContain("offset=50");
    expect(screen.queryByText("CODE-000")).toBeNull();

    const previousAfterNext = screen.getByText("Previous").closest("button") as HTMLButtonElement;
    expect(previousAfterNext.disabled).toBe(false);
    await userEvent.click(screen.getByText("Previous"));
    // offset=0 was already fetched at test start, so React Query serves it from
    // the cache; the visible page must be page 1 again.
    await waitFor(() => {
      expect((screen.getByLabelText("Go to page") as HTMLInputElement).value).toBe("1");
    });
    expect(screen.getByText("CODE-000")).toBeTruthy();
    expect(screen.queryByText("CODE-050")).toBeNull();
  });

  it("keeps the table columns stable when page 1 has long cell values", async () => {
    renderCourses();
    await waitForFirstPage();

    const table = screen.getByRole("table");
    expect(table).toHaveClass("table-fixed");
    expect(table).toHaveClass("min-w-[66rem]");
    expect(table.querySelectorAll("col")).toHaveLength(table.querySelectorAll("thead th").length);

    await userEvent.click(screen.getByText("Next"));
    await waitFor(() => {
      expect(screen.getByText("CODE-050")).toBeTruthy();
    });
    expect(screen.getByRole("table")).toHaveClass("table-fixed");
  });

  it("disables Next on the last page", async () => {
    renderCourses("/courses?offset=100");
    await waitFor(() => {
      expect(screen.getByText("CODE-100")).toBeTruthy();
    });
    const nextButton = screen.getByText("Next").closest("button") as HTMLButtonElement;
    expect(nextButton.disabled).toBe(true);
  });

  it("sends teacher_id for a selected teacher and the none sentinel", async () => {
    renderCourses();
    await waitForFirstPage();
    const select = screen.getByLabelText("Teacher filter");
    await userEvent.selectOptions(select, "t1");
    await waitFor(() => {
      expect(courseRequests.some((u) => u.includes("teacher_id=t1"))).toBe(true);
    });
    await userEvent.selectOptions(select, "none");
    await waitFor(() => {
      expect(courseRequests.some((u) => u.includes("teacher_id=none"))).toBe(true);
    });
  });

  it("clears the selection when the filter changes", async () => {
    renderCourses();
    await waitForFirstPage();
    const rowCheckbox = screen.getAllByRole("checkbox").find(
      (cb) => cb.closest("tr")?.textContent?.includes("CODE-000")
    );
    if (!rowCheckbox) throw new Error("Row checkbox not found");
    await userEvent.click(rowCheckbox);
    await waitFor(() => {
      expect(screen.getByText(/1 selected/)).toBeTruthy();
    });
    await userEvent.selectOptions(screen.getByLabelText("Course type filter"), "private");
    await waitFor(() => {
      expect(screen.queryByText(/1 selected/)).toBeNull();
    });
  });

  it("shows an empty state when no courses match", async () => {
    renderCourses("/courses?type=private&q=nonexistent");
    await waitFor(() => {
      expect(screen.getByText("No courses found")).toBeTruthy();
    });
  });
});
