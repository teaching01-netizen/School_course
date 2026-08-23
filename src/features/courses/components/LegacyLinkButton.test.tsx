import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "../../../hooks/useToast";
import { LegacyLinkButton } from "./LegacyLinkButton";
import type { Course } from "../types";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function makeCourse(overrides: Partial<Course> = {}): Course {
  return {
    id: "course-1",
    version: 1,
    code: "MATH-101",
    name: "Math",
    primary_teacher_id: null,
    ...overrides,
  };
}

function renderButton(course: Course, onLinked = vi.fn()) {
  render(
    <ToastProvider>
      <LegacyLinkButton course={course} onLinked={onLinked} />
    </ToastProvider>,
  );
  return onLinked;
}

function putBodies() {
  return mockApiJson.mock.calls
    .filter(
      ([path, init]) => path === "/api/v1/courses/course-1" && (init as RequestInit | undefined)?.method === "PUT",
    )
    .map((call) => JSON.parse((call[1] as RequestInit).body as string));
}

describe("LegacyLinkButton", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("renders as a single icon with no management UI in the resting state", () => {
    renderButton(makeCourse());

    expect(screen.getByRole("button", { name: "Link to old system" })).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.queryByText("Link to Old System")).not.toBeInTheDocument();
  });

  it("links a pasted old system URL and closes the popover", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PUT") {
        return Promise.resolve(makeCourse({ legacy_course_id: "4321" }));
      }
      throw new Error(`Unexpected API call: ${path}`);
    });
    const onLinked = renderButton(makeCourse());
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Link to old system" }));
    const input = await screen.findByRole("textbox", { name: "Legacy course ID or URL" });
    await user.type(input, "https://old.example/course?id=4321");
    await user.click(await screen.findByRole("button", { name: "Link" }));

    await screen.findByText("Linked to old system ID 4321");
    expect(onLinked).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();

    const body = putBodies().at(-1);
    expect(body.legacy_course_id).toBe("4321");
    expect(body.code).toBe("MATH-101");
    expect(body.name).toBe("Math");
  });

  it("links a raw numeric ID by pressing Enter in the input", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PUT") {
        return Promise.resolve(makeCourse({ legacy_course_id: "777" }));
      }
      throw new Error(`Unexpected API call: ${path}`);
    });
    const onLinked = renderButton(makeCourse());
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Link to old system" }));
    const input = await screen.findByRole("textbox", { name: "Legacy course ID or URL" });
    await user.type(input, "777{Enter}");

    await screen.findByText("Linked to old system ID 777");
    expect(onLinked).toHaveBeenCalledTimes(1);
    expect(putBodies().at(-1).legacy_course_id).toBe("777");
  });

  it("shows an invalid URL as an error without calling the API", async () => {
    mockApiJson.mockImplementation(() => {
      throw new Error("should not be called");
    });
    renderButton(makeCourse());
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Link to old system" }));
    const input = await screen.findByRole("textbox", { name: "Legacy course ID or URL" });
    await user.type(input, "not-a-number");
    await user.click(screen.getByRole("button", { name: "Link" }));

    await screen.findByText(/Could not extract a numeric ID/);
    expect(mockApiJson).not.toHaveBeenCalled();
    // The popover stays open so the user can correct the input.
    expect(screen.getByRole("textbox", { name: "Legacy course ID or URL" })).toBeInTheDocument();
  });

  it("shows the linked state with sync metadata and unlinks the course", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PUT") {
        return Promise.resolve(makeCourse({ legacy_course_id: null }));
      }
      throw new Error(`Unexpected API call: ${path}`);
    });
    const onLinked = renderButton(
      makeCourse({ legacy_course_id: "1234", legacy_last_synced_at: "2026-08-01T00:00:00Z" }),
    );
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Legacy system link" }));
    expect(await screen.findByText("ID: 1234")).toBeInTheDocument();
    expect(screen.getByText(/Last synced: .*2026/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Remove link" }));

    await screen.findByText("Removed legacy link");
    expect(onLinked).toHaveBeenCalledTimes(1);
    expect(putBodies().at(-1).legacy_course_id).toBeNull();
    // Popover closes after the action.
    expect(screen.queryByText("ID: 1234")).not.toBeInTheDocument();
  });

  it("queues a legacy refresh from the linked popover", async () => {
    mockApiJson.mockImplementation((path: string) => {
      if (path === "/api/v1/courses/course-1/legacy-sync") return Promise.resolve(undefined);
      throw new Error(`Unexpected API call: ${path}`);
    });
    const onLinked = renderButton(makeCourse({ legacy_course_id: "1234" }));
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Legacy system link" }));
    await user.click(await screen.findByRole("button", { name: "Queue refresh" }));

    await screen.findByText("Legacy refresh queued");
    expect(onLinked).toHaveBeenCalledTimes(1);
    expect(
      mockApiJson.mock.calls.some(
        ([path, init]) => path === "/api/v1/courses/course-1/legacy-sync" && init?.method === "POST",
      ),
    ).toBe(true);
  });

  it("closes the popover with Escape", async () => {
    renderButton(makeCourse());
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Link to old system" }));
    expect(await screen.findByRole("textbox", { name: "Legacy course ID or URL" })).toBeInTheDocument();

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("textbox", { name: "Legacy course ID or URL" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Link to old system" })).toHaveAttribute("aria-expanded", "false");
  });
});
