import { render, type RenderOptions } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { ToastProvider } from "../../../hooks/useToast";

/**
 * Create a fresh QueryClient for each test to avoid shared state.
 * Uses retry: false and gcTime: Infinity to keep data around for assertions.
 */
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: Infinity,
      },
      mutations: {
        retry: false,
      },
    },
  });
}

/**
 * Render a React element with QueryClient, MemoryRouter, and Toast providers.
 * Do not use the production global query client in unit tests.
 */
export function renderScheduleImpact(
  ui: React.ReactElement,
  options: RenderOptions & { routes?: string[] } = {},
) {
  const queryClient = createTestQueryClient();
  const { routes, ...renderOptions } = options;

  const result = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={routes ?? ["/operations/schedule-impact"]}>
        <ToastProvider>{ui}</ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
    renderOptions,
  );

  return { ...result, queryClient };
}
