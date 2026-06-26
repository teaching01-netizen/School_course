import { describe, expect, it } from "vitest";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

function filesUnder(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) return filesUnder(path);
    return path.endsWith(".ts") || path.endsWith(".tsx") ? [path] : [];
  });
}

describe("feature architecture boundaries", () => {
  it("keeps absence domain modules free of UI, API, and browser storage imports", () => {
    const domainFiles = filesUnder("src/features/absences/domain").filter((path) => !path.includes("__tests__"));
    const forbidden = [
      /from\s+["']react["']/,
      /from\s+["']@\/api\/client["']/,
      /window\.sessionStorage/,
      /window\.localStorage/,
    ];

    const violations = domainFiles.flatMap((path) => {
      const source = readFileSync(path, "utf8");
      return forbidden.some((pattern) => pattern.test(source)) ? [path] : [];
    });

    expect(violations).toEqual([]);
  });

  it("keeps feature modules independent from the central type barrel", () => {
    const featureFiles = filesUnder("src/features")
      .filter((path) => !path.endsWith("architecture-boundaries.test.ts"))
      .filter((path) => !path.includes("__tests__"));
    const forbidden = /from\s+["']@\/types["']/;

    const violations = featureFiles.flatMap((path) => {
      const source = readFileSync(path, "utf8");
      return forbidden.test(source) ? [path] : [];
    });

    expect(violations).toEqual([]);
  });

  it("keeps feature-specific hooks out of the shared hooks directory", () => {
    const sharedHookFiles = [
      "src/hooks/useAttendanceModal.ts",
      "src/hooks/useCourseStudents.ts",
      "src/hooks/useSitInRules.ts",
      "src/hooks/useCreateSession.ts",
      "src/hooks/useEditSession.ts",
      "src/hooks/usePreflight.ts",
      "src/hooks/usePreflightGate.ts",
      "src/hooks/useLookups.ts",
    ];

    expect(sharedHookFiles.filter((path) => existsSync(path))).toEqual([]);
  });
});
