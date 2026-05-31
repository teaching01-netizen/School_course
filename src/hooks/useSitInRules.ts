import { useState, useCallback, useEffect } from "react";
import { apiJson } from "../api/client";
import { useToast } from "./useToast";
import type { SitInRule, SitInRuleCreateInput } from "../types";

export function useSitInRules() {
  const { addToast } = useToast();
  const [rules, setRules] = useState<SitInRule[]>([]);
  const [loading, setLoading] = useState(true);

  const loadRules = useCallback(async () => {
    try {
      setLoading(true);
      const resp = await apiJson<SitInRule[]>(
        "/api/v1/admin/sit-in-rules",
        { method: "GET" }
      );
      setRules(resp ?? []);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load rules");
    } finally {
      setLoading(false);
    }
  }, [addToast]);

  useEffect(() => {
    loadRules();
  }, [loadRules]);

  const createRule = useCallback(async (input: SitInRuleCreateInput) => {
    const rule = await apiJson<SitInRule>("/api/v1/admin/sit-in-rules", {
      method: "POST",
      body: JSON.stringify(input),
    });
    setRules((prev) => [...prev, rule]);
    return rule;
  }, []);

  const updateRule = useCallback(async (id: string, input: SitInRuleCreateInput) => {
    const rule = await apiJson<SitInRule>(`/api/v1/admin/sit-in-rules/${id}`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
    setRules((prev) => prev.map((r) => (r.id === id ? rule : r)));
    return rule;
  }, []);

  const deleteRule = useCallback(async (id: string) => {
    await apiJson(`/api/v1/admin/sit-in-rules/${id}`, { method: "DELETE" });
    setRules((prev) => prev.filter((r) => r.id !== id));
  }, []);

  return { rules, loading, loadRules, createRule, updateRule, deleteRule };
}
