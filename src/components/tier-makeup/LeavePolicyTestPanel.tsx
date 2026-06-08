import { useState } from "react";
import Button from "../ui/Button";
import { LEAVE_POLICY_COURSE_RULES, evaluateLeavePolicy, getRuleTypeBadgeColor, getRuleTypeLabel } from "./leavePolicyData";
import type { LeavePolicyTestInput, LeavePolicyTestResult } from "../../types";

export default function LeavePolicyTestPanel() {
  const [selectedRuleId, setSelectedRuleId] = useState("");
  const [missedCourseName, setMissedCourseName] = useState("");
  const [missedSection, setMissedSection] = useState("Section 1");
  const [missedOccurrence, setMissedOccurrence] = useState(1);
  const [totalSessions, setTotalSessions] = useState(10);
  const [isLastClass, setIsLastClass] = useState(false);
  const [result, setResult] = useState<LeavePolicyTestResult | null>(null);
  const [maxPriorityToShow, setMaxPriorityToShow] = useState(1);

  const selectedRule = LEAVE_POLICY_COURSE_RULES.find((r) => r.id === selectedRuleId);

  function handleTest() {
    if (!selectedRuleId) return;
    const input: LeavePolicyTestInput = {
      courseRuleId: selectedRuleId,
      missedCourseName: missedCourseName || selectedRule?.courseName || "",
      missedSection,
      missedOccurrence,
      totalSessions,
      isLastClass,
    };
    const testResult = evaluateLeavePolicy(input, undefined, maxPriorityToShow);
    setResult(testResult);
  }

  function handleReset() {
    setSelectedRuleId("");
    setMissedCourseName("");
    setMissedSection("Section 1");
    setMissedOccurrence(1);
    setTotalSessions(10);
    setIsLastClass(false);
    setResult(null);
    setMaxPriorityToShow(1);
  }

  function handleNotAvailable() {
    const nextPriority = maxPriorityToShow + 1;
    if (selectedRule && nextPriority <= selectedRule.priorityCount) {
      setMaxPriorityToShow(nextPriority);
      // Re-evaluate with new priority level
      const input: LeavePolicyTestInput = {
        courseRuleId: selectedRuleId,
        missedCourseName: missedCourseName || selectedRule?.courseName || "",
        missedSection,
        missedOccurrence,
        totalSessions,
        isLastClass,
      };
      const testResult = evaluateLeavePolicy(input, undefined, nextPriority);
      setResult(testResult);
    }
  }

  return (
    <div>
      <div className="mb-4">
        <p className="text-sm text-gray-500">
          Test the leave policy rules by simulating a student absence. Select a course rule and input the missed
          session details to see available makeup options.
        </p>
        <p className="text-xs text-gray-400 mt-1">
          Options are revealed step-by-step: only 1st Priority is shown first. Click "Not available" to reveal the next priority.
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* Input form */}
        <div className="rounded-sm border border-gray-200 bg-white p-4">
          <h3 className="text-sm font-semibold text-gray-800 mb-4">Test Input</h3>

          <div className="space-y-4">
            {/* Course Rule */}
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1">Course Rule</label>
              <select
                value={selectedRuleId}
                onChange={(e) => {
                  setSelectedRuleId(e.target.value);
                  setMaxPriorityToShow(1);
                  setResult(null);
                }}
                className="w-full text-sm border border-gray-200 rounded-sm px-3 py-2 bg-white"
              >
                <option value="">-- Select a course rule --</option>
                {LEAVE_POLICY_COURSE_RULES.map((rule) => (
                  <option key={rule.id} value={rule.id}>
                    {rule.courseName}
                  </option>
                ))}
              </select>
            </div>

            {/* Show rule info when selected */}
            {selectedRule && (
              <div className="rounded-sm bg-gray-50 p-3 text-xs space-y-1">
                <div className="flex items-center gap-2">
                  <span className={`inline-block rounded-sm px-2 py-0.5 font-medium ${getRuleTypeBadgeColor(selectedRule.ruleType).bg} ${getRuleTypeBadgeColor(selectedRule.ruleType).text}`}>
                    {getRuleTypeLabel(selectedRule.ruleType)}
                  </span>
                  <span className="text-gray-500">{selectedRule.priorityCount} priorities</span>
                </div>
                <p className="text-gray-600">{selectedRule.description}</p>
                {selectedRule.priorities && selectedRule.priorities.length > 0 && (
                  <div className="mt-2 space-y-1">
                    {selectedRule.priorities.map((p) => (
                      <div key={p.level} className="flex items-center gap-2">
                        <span className={`w-5 h-5 rounded-full flex items-center justify-center text-xs font-semibold ${
                          p.level <= maxPriorityToShow
                            ? "bg-[var(--color-wi-primary)] text-white"
                            : "bg-gray-200 text-gray-500"
                        }`}>
                          {p.level}
                        </span>
                        <span className={`${p.level <= maxPriorityToShow ? "text-gray-800" : "text-gray-400"}`}>
                          {p.label}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Student's Enrolled Course Name (for rank derivation) */}
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1">
                Student's Enrolled Course
                <span className="text-gray-400 ml-1">(used to determine rank)</span>
              </label>
              <input
                type="text"
                value={missedCourseName}
                onChange={(e) => setMissedCourseName(e.target.value)}
                placeholder={selectedRule?.courseName || "e.g., SAT Verbal Rank 3"}
                className="w-full text-sm border border-gray-200 rounded-sm px-3 py-2"
              />
              <p className="mt-1 text-xs text-gray-400">
                The rank is extracted from this name (e.g., "Rank 3" → Rank 3)
              </p>
            </div>

            {/* Missed Section */}
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1">Missed Section</label>
              <select
                value={missedSection}
                onChange={(e) => setMissedSection(e.target.value)}
                className="w-full text-sm border border-gray-200 rounded-sm px-3 py-2 bg-white"
              >
                <option value="Section 1">Section 1</option>
                <option value="Section 2">Section 2</option>
                <option value="Section 3">Section 3</option>
              </select>
            </div>

            {/* Occurrence Number */}
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1">Missed Occurrence #</label>
              <input
                type="number"
                min={1}
                max={totalSessions}
                value={missedOccurrence}
                onChange={(e) => setMissedOccurrence(Math.max(1, parseInt(e.target.value) || 1))}
                className="w-full text-sm border border-gray-200 rounded-sm px-3 py-2"
              />
            </div>

            {/* Total Sessions */}
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1">Total Sessions in Cycle</label>
              <input
                type="number"
                min={1}
                value={totalSessions}
                onChange={(e) => setTotalSessions(Math.max(1, parseInt(e.target.value) || 1))}
                className="w-full text-sm border border-gray-200 rounded-sm px-3 py-2"
              />
            </div>

            {/* Is Last Class */}
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="isLastClass"
                checked={isLastClass}
                onChange={(e) => setIsLastClass(e.target.checked)}
                className="rounded border-gray-300"
              />
              <label htmlFor="isLastClass" className="text-sm text-gray-600">
                This is the last class of the cycle (End-of-class Meal)
              </label>
            </div>

            {/* Actions */}
            <div className="flex items-center gap-2 pt-2">
              <Button
                variant="primary"
                size="sm"
                disabled={!selectedRuleId}
                onClick={handleTest}
              >
                Run Test
              </Button>
              <Button variant="secondary" size="sm" onClick={handleReset}>
                Reset
              </Button>
            </div>
          </div>
        </div>

        {/* Results */}
        <div className="rounded-sm border border-gray-200 bg-white p-4">
          <h3 className="text-sm font-semibold text-gray-800 mb-4">Test Result</h3>

          {!result ? (
            <div className="flex items-center justify-center h-48 text-sm text-gray-400">
              Select a course rule and click "Run Test" to see results
            </div>
          ) : (
            <div className="space-y-4">
              {/* Summary */}
              <div className={`rounded-sm p-3 ${result.isBlocked ? "bg-red-50 border border-red-200" : "bg-green-50 border border-green-200"}`}>
                <div className="flex items-center gap-2 mb-1">
                  {result.isBlocked ? (
                    <span className="text-red-600 font-semibold text-sm">BLOCKED</span>
                  ) : (
                    <span className="text-green-600 font-semibold text-sm">AVAILABLE</span>
                  )}
                </div>
                {result.blockReason && (
                  <p className="text-sm text-red-700">{result.blockReason}</p>
                )}
              </div>

              {/* Input summary */}
              <div className="rounded-sm bg-gray-50 p-3 text-xs space-y-1">
                <p className="font-medium text-gray-700">Input:</p>
                <p className="text-gray-600">
                  Course Rule: {LEAVE_POLICY_COURSE_RULES.find((r) => r.id === result.input.courseRuleId)?.courseName}
                </p>
                <p className="text-gray-600">
                  Enrolled Course: {result.input.missedCourseName || "(not specified)"}
                </p>
                <p className="text-gray-600">
                  Missed: {result.input.missedSection}, Occurrence #{result.input.missedOccurrence}
                </p>
                <p className="text-gray-600">
                  Last class: {result.input.isLastClass ? "Yes" : "No"}
                </p>
              </div>

              {/* Makeup options */}
              {result.options.length > 0 && (
                <div>
                  <p className="text-xs font-medium text-gray-700 mb-2">Available Makeup Options:</p>
                  <div className="space-y-2">
                    {result.options.map((opt, i) => (
                      <div
                        key={i}
                        className={`flex items-center justify-between rounded-sm px-3 py-2 text-sm ${
                          opt.available
                            ? "bg-green-50 border border-green-200"
                            : "bg-gray-50 border border-gray-200 opacity-50"
                        }`}
                      >
                        <div className="flex items-center gap-2">
                          <span className={`w-2 h-2 rounded-full ${opt.available ? "bg-green-500" : "bg-gray-400"}`} />
                          <span className={opt.available ? "text-gray-800" : "text-gray-500"}>
                            {opt.label}
                          </span>
                        </div>
                        {opt.reason && (
                          <span className={`text-xs ${opt.available ? "text-green-600" : "text-gray-400"}`}>
                            {opt.reason}
                          </span>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Not available button for stepped reveal */}
              {!result.isBlocked && selectedRule && maxPriorityToShow < selectedRule.priorityCount && (
                <div className="mt-4 pt-4 border-t border-gray-100">
                  <p className="text-xs text-gray-500 mb-2">
                    If the student cannot attend the {maxPriorityToShow === 1 ? "1st" : maxPriorityToShow === 2 ? "2nd" : "3rd"} Priority option:
                  </p>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={handleNotAvailable}
                  >
                    Not available for {maxPriorityToShow === 1 ? "1st" : maxPriorityToShow === 2 ? "2nd" : "3rd"} Priority — Show next
                  </Button>
                </div>
              )}

              {/* Priority flow visualization */}
              {!result.isBlocked && selectedRule && selectedRule.priorityCount > 1 && (
                <div className="mt-4">
                  <p className="text-xs font-medium text-gray-700 mb-2">Priority Flow:</p>
                  <div className="flex items-center gap-2 text-xs">
                    {Array.from({ length: selectedRule.priorityCount }, (_, i) => i + 1).map((p, i) => (
                      <div key={p} className="flex items-center gap-2">
                        {i > 0 && <span className="text-gray-400">→</span>}
                        <div className={`rounded-sm px-2 py-1 font-medium ${
                          p <= maxPriorityToShow ? "bg-[var(--color-wi-primary)] text-white" : "bg-gray-100 text-gray-600"
                        }`}>
                          {p}{p === 1 ? "st" : p === 2 ? "nd" : "rd"} Priority
                        </div>
                      </div>
                    ))}
                  </div>
                  <p className="mt-1 text-xs text-gray-400">
                    Student should try 1st Priority first. Only if unavailable, proceed to next.
                  </p>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
