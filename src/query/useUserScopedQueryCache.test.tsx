import { render, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { createAppQueryClient } from "./cache";
import { useUserScopedQueryCache } from "./useUserScopedQueryCache";

function CacheScopeProbe({ userID }: { userID: string | null }) {
  useUserScopedQueryCache(userID);
  return null;
}

describe("useUserScopedQueryCache", () => {
  it("clears cached queries when the authenticated user changes", async () => {
    const client = createAppQueryClient();
    client.setQueryData(["private"], { secret: true });

    const { rerender } = render(
      <QueryClientProvider client={client}>
        <CacheScopeProbe userID="user-a" />
      </QueryClientProvider>,
    );

    expect(client.getQueryData(["private"])).toEqual({ secret: true });

    rerender(
      <QueryClientProvider client={client}>
        <CacheScopeProbe userID="user-b" />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(client.getQueryData(["private"])).toBeUndefined();
    });
  });
});
