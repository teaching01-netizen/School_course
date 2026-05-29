import { useState, useEffect, useMemo } from "react";
import type { Course, Room, User } from "@/types";
import { apiJson } from "@/api/client";

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

export default function useLookups(): Lookups {
  const [courses, setCourses] = useState<Course[]>([]);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [teachers, setTeachers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);

  const load = () => {
    setLoading(true);
    Promise.all([
      apiJson<Course[]>("/api/v1/courses", { method: "GET" }),
      apiJson<Room[]>("/api/v1/rooms", { method: "GET" }),
      apiJson<User[]>("/api/v1/users?role=Teacher", { method: "GET" }),
    ])
      .then(([c, r, t]) => {
        setCourses(c);
        setRooms(r);
        setTeachers(t);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  };

  useEffect(() => { load(); }, []);

  const courseById = useMemo(() => new Map(courses.map((c) => [c.id, c])), [courses]);
  const roomById = useMemo(() => new Map(rooms.map((r) => [r.id, r])), [rooms]);
  const teacherById = useMemo(() => new Map(teachers.map((t) => [t.id, t])), [teachers]);
  const courseOptions = useMemo(
    () => courses.map((c) => ({ value: c.id, label: `${c.code} — ${c.name}`, keywords: `${c.code} ${c.name}` })),
    [courses]
  );
  const teacherOptions = useMemo(
    () => teachers.map((t) => ({ value: t.id, label: t.username, keywords: t.username })),
    [teachers]
  );

  return { courses, rooms, teachers, courseById, roomById, teacherById, courseOptions, teacherOptions, loading, reload: load };
}
