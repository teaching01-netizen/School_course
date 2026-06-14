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
