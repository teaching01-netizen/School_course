import path from "node:path";
import { defineConfig } from "vitest/config";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(process.cwd(), "src") } },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: "./src/test/setup.ts",
    css: false,
    testTimeout: 15_000,
    exclude: ["e2e/**"],
      include: [
        "src/features/scheduling/hooks/useDebouncedPreflight.test.ts",
        "src/features/scheduling/hooks/useCreateSession.test.ts",
        "src/features/scheduling/hooks/useEditSession.test.ts",
        "src/pages/__tests__/Schedule.createSession.test.tsx",
        "src/pages/__tests__/Schedule.failureRecovery.test.tsx",
        "src/pages/__tests__/Schedule.editJourneys.test.tsx",
        "src/pages/__tests__/Schedule.accessibility.test.tsx",
        "src/pages/__tests__/Schedule.seriesJourneys.test.tsx",
        "src/test/usePreflight.test.ts",
        "src/test/usePreflightGate.test.ts",
        "src/test/schedulePreflight.test.ts",
        "src/test/PreflightIndicator.test.tsx",
        "src/components/__tests__/PreflightIndicator.test.tsx",
        "src/components/__tests__/SeriesFormFields.test.tsx",
        "src/components/__tests__/SessionActions.test.tsx",
        "src/pages/__tests__/Schedule.test.tsx",
        "src/pages/__tests__/Schedule.seriesStabilization.test.tsx",
        "src/pages/__tests__/CourseDetail.create.test.tsx",
        "src/pages/__tests__/CourseDetail.schedulePaste.test.tsx",
        "src/pages/__tests__/SlotFinder.test.tsx",
      ],
      coverage: {
        reportsDirectory: "coverage/schedule",
        include: [
          "src/features/scheduling/hooks/usePreflight.ts",
          "src/features/scheduling/hooks/usePreflightGate.ts",
          "src/features/scheduling/hooks/useDebouncedPreflight.ts",
          "src/features/scheduling/hooks/useCreateSession.ts",
          "src/features/scheduling/hooks/useEditSession.ts",
          "src/features/scheduling/recurrenceLimits.ts",
          "src/components/PreflightIndicator.tsx",
          "src/components/SeriesFormFields.tsx",
          "src/pages/Schedule.tsx",
          "src/utils/preflight.ts",
          "src/api/client.ts",
        ],
        reporter: ["text", "json-summary"],
        thresholds: {
          statements: 100,
          functions: 100,
          branches: 100,
          lines: 100,
        },
      },
  },
});
