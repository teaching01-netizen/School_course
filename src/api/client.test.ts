import { afterEach, describe, expect, it, vi } from "vitest";
import { apiJson } from "./client";

describe("apiJson", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("does not parse JSON for 204 No Content responses", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(null, { status: 204 }));

    await expect(apiJson<void>("/api/v1/admin/email-workflows/test-id", { method: "DELETE" })).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/admin/email-workflows/test-id",
      expect.objectContaining({ method: "DELETE" }),
    );
  });
});

it("bypasses the browser HTTP cache for authenticated GET requests", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 })));

  await apiJson("/api/v1/subjects", { method: "GET" });

  expect(fetch).toHaveBeenCalledWith(
    "/api/v1/subjects",
    expect.objectContaining({ cache: "no-store", credentials: "include", method: "GET" }),
  );
});
