import { useState, useCallback, useRef } from "react";
import { apiJson } from "@/api/client";
import type { Student } from "@/types";

type CourseStudentsState = {
  cache: Record<string, Student[]>;
  loading: Record<string, boolean>;
  errors: Record<string, string | null>;
};

export function useCourseStudents() {
  const [state, setState] = useState<CourseStudentsState>({
    cache: {},
    loading: {},
    errors: {},
  });
  const mountedRef = useRef(true);
  mountedRef.current = true;

  const fetchStudents = useCallback(async (courseId: string) => {
    setState((prev) => {
      if (prev.cache[courseId] !== undefined || prev.loading[courseId]) {
        return prev;
      }
      return {
        ...prev,
        loading: { ...prev.loading, [courseId]: true },
        errors: { ...prev.errors, [courseId]: null },
      };
    });

    const shouldFetch = state.cache[courseId] === undefined && !state.loading[courseId];
    if (!shouldFetch) return;

    try {
      const students = await apiJson<Student[]>(
        `/api/v1/courses/${courseId}/students`,
        { method: "GET" }
      );
      if (mountedRef.current) {
        setState((prev) => ({
          cache: { ...prev.cache, [courseId]: students },
          loading: { ...prev.loading, [courseId]: false },
          errors: { ...prev.errors, [courseId]: null },
        }));
      }
    } catch (err) {
      if (mountedRef.current) {
        setState((prev) => ({
          ...prev,
          loading: { ...prev.loading, [courseId]: false },
          errors: {
            ...prev.errors,
            [courseId]: err instanceof Error ? err.message : "Failed to load students",
          },
        }));
      }
    }
  }, [state.cache, state.loading]);

  return {
    cache: state.cache,
    loading: state.loading,
    errors: state.errors,
    fetchStudents,
  };
}
