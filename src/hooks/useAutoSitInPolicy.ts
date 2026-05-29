import { useState, useCallback } from "react";
import { apiJson } from "../api/client";
import type { PolicyResponse } from "../utils/levels";

export function useAutoSitInPolicy() {
  const [autoSitInToggles, setAutoSitInToggles] = useState<Record<string, boolean>>({});
  const [initialAutoSitIn, setInitialAutoSitIn] = useState<Record<string, boolean>>({});
  const [savingPolicy, setSavingPolicy] = useState<Record<string, boolean>>({});

  const loadPolicies = useCallback(async (): Promise<void> => {
    const resp = await apiJson<PolicyResponse>("/api/v1/admin/absence-policies", { method: "GET" });
    const rootGroupPolicies = resp.absence_policies?.root_course_groups ?? {};
    const toggles: Record<string, boolean> = {};
    const initial: Record<string, boolean> = {};
    for (const [rootCourseId, policy] of Object.entries(rootGroupPolicies)) {
      toggles[rootCourseId] = policy.auto_sit_in_enabled;
      initial[rootCourseId] = policy.auto_sit_in_enabled;
    }
    setAutoSitInToggles(toggles);
    setInitialAutoSitIn(initial);
  }, []);

  const savePolicy = useCallback(async (rootCourseId: string, enabled: boolean): Promise<void> => {
    setSavingPolicy((p) => ({ ...p, [rootCourseId]: true }));
    try {
      await apiJson("/api/v1/admin/absence-policies", {
        method: "PUT",
        body: JSON.stringify({
          absence_policies: {
            root_course_groups: {
              [rootCourseId]: { auto_sit_in_enabled: enabled },
            },
          },
        }),
      });
      setInitialAutoSitIn((prev) => ({ ...prev, [rootCourseId]: enabled }));
    } finally {
      setSavingPolicy((p) => ({ ...p, [rootCourseId]: false }));
    }
  }, []);

  const setPolicyToggle = useCallback((rootCourseId: string, enabled: boolean) => {
    setAutoSitInToggles((prev) => ({ ...prev, [rootCourseId]: enabled }));
  }, []);

  const setPolicyInitialState = useCallback((toggles: Record<string, boolean>) => {
    setAutoSitInToggles(toggles);
    setInitialAutoSitIn({ ...toggles });
  }, []);

  const isDirty = useCallback(
    (rootCourseId: string): boolean => {
      return autoSitInToggles[rootCourseId] !== initialAutoSitIn[rootCourseId];
    },
    [autoSitInToggles, initialAutoSitIn],
  );

  return {
    autoSitInToggles,
    initialAutoSitIn,
    savingPolicy,
    loadPolicies,
    savePolicy,
    setPolicyToggle,
    setPolicyInitialState,
    isDirty,
  };
}
