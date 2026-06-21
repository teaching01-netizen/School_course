import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "./cache";
import { useOperationalQuery } from "./useOperationalQuery";

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("useOperationalQuery", () => {
  beforeEach(() => {
    queryClient.clear();
    mockApiJson.mockReset();
  });

  it("deduplicates simultaneous operational requests", async () => {
    mockApiJson.mockResolvedValue([{ id: "session-1" }]);
    const first = renderHook(() => useOperationalQuery(["sessions", "list", "today"], "/api/v1/sessions?today=1"));
    const second = renderHook(() => useOperationalQuery(["sessions", "list", "today"], "/api/v1/sessions?today=1"));

    await waitFor(() => expect(first.result.current.data).toEqual([{ id: "session-1" }]));
    await waitFor(() => expect(second.result.current.data).toEqual([{ id: "session-1" }]));
    expect(mockApiJson).toHaveBeenCalledTimes(1);
  });

  it("keeps previous data visible while a changed request is loading", async () => {
    let resolveNext: (value: unknown) => void = () => undefined;
    mockApiJson
      .mockResolvedValueOnce([{ id: "old" }])
      .mockImplementationOnce(() => new Promise((resolve) => { resolveNext = resolve; }));

    const { result, rerender } = renderHook(
      ({ request }) => useOperationalQuery(["sessions", "list", request], `/api/v1/sessions?range=${request}`),
      { initialProps: { request: "old" } },
    );
    await waitFor(() => expect(result.current.data).toEqual([{ id: "old" }]));

    rerender({ request: "new" });
    expect(result.current.data).toEqual([{ id: "old" }]);
    expect(result.current.isFetching).toBe(true);

    await act(async () => resolveNext([{ id: "new" }]));
    await waitFor(() => expect(result.current.data).toEqual([{ id: "new" }]));
  });
});
