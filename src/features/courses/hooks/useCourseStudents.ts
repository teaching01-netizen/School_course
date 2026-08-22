import { useState, useCallback, useRef, useEffect } from "react";
import { apiJson } from "@/api/client";
import type { Student } from "@/types/shared";

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
  // In-flight dedupe lives in a ref so a second call before the first state
  // commit cannot start a duplicate request.
  const inFlight = useRef<Set<string>>(new Set());
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const fetchStudents = useCallback(async (courseId: string) => {
    if (state.cache[courseId] !== undefined || inFlight.current.has(courseId)) return;
    inFlight.current.add(courseId);
    setState((prev) => ({
      ...prev,
      loading: { ...prev.loading, [courseId]: true },
      errors: { ...prev.errors, [courseId]: null },
    }));

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
    } finally {
      inFlight.current.delete(courseId);
    }
  }, [state.cache]);

  return {
    cache: state.cache,
    loading: state.loading,
    errors: state.errors,
    fetchStudents,
  };
}
