import { useMemo } from "react";
import { useApiQuery } from "@/hooks/useApiQuery";
import type { Course } from "@/features/courses/types";
import type { User } from "@/types/shared";
import type { Room } from "../types";

interface Lookups {
  courses: Course[];
  rooms: Room[];
  teachers: User[];
  courseById: Map<string, Course>;
  roomById: Map<string, Room>;
  teacherById: Map<string, User>;
  courseOptions: { value: string; label: string; keywords: string }[];
  teacherOptions: { value: string; label: string; keywords: string }[];
  loading: boolean;
  reload: () => void;
}

// Reference data flows through the shared query cache, so a Schedule visit
// reuses whatever the Teachers page / nav prefetch already warmed instead of
// refetching all three lists on every mount.
// Stable empties so the derived Maps keep referential identity while loading.
const NO_COURSES: Course[] = [];
const NO_ROOMS: Room[] = [];
const NO_TEACHERS: User[] = [];

export default function useLookups(): Lookups {
  const coursesQuery = useApiQuery<Course[]>("/api/v1/courses");
  const roomsQuery = useApiQuery<Room[]>("/api/v1/rooms");
  const teachersQuery = useApiQuery<User[]>("/api/v1/users?role=Teacher");

  const courses = coursesQuery.data ?? NO_COURSES;
  const rooms = roomsQuery.data ?? NO_ROOMS;
  const teachers = teachersQuery.data ?? NO_TEACHERS;
  const loading = coursesQuery.loading || roomsQuery.loading || teachersQuery.loading;

  const courseById = useMemo(() => new Map(courses.map((c) => [c.id, c])), [courses]);
  const roomById = useMemo(() => new Map(rooms.map((r) => [r.id, r])), [rooms]);
  const teacherById = useMemo(() => new Map(teachers.map((t) => [t.id, t])), [teachers]);
  const courseOptions = useMemo(
    () => courses.map((c) => ({ value: c.id, label: `${c.code} — ${c.name}`, keywords: `${c.code} ${c.name}` })),
    [courses]
  );
  const teacherOptions = useMemo(
    () => teachers.map((t) => ({ value: t.id, label: t.full_name || t.username, keywords: `${t.full_name || ""} ${t.username}` })),
    [teachers]
  );

  return {
    courses,
    rooms,
    teachers,
    courseById,
    roomById,
    teacherById,
    courseOptions,
    teacherOptions,
    loading,
    reload: () => {
      void coursesQuery.refetch();
      void roomsQuery.refetch();
      void teachersQuery.refetch();
    },
  };
}
