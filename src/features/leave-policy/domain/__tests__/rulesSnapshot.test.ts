import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";
import { LEAVE_POLICY_COURSE_RULES } from "../rules";

// Go policy tests read this snapshot; regenerate it whenever rules.ts changes.
it("Go testdata matches LEAVE_POLICY_COURSE_RULES", () => {
  const path = resolve(__dirname, "../../../../../backend/internal/httpapi/absenceshttp/testdata/sat_verbal_policy_rules.json");
  expect(JSON.parse(readFileSync(path, "utf8"))).toEqual(JSON.parse(JSON.stringify(LEAVE_POLICY_COURSE_RULES)));
});
