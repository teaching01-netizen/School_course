import { useEffect, useMemo, useState } from "react";
import { format } from "date-fns";
import { apiJson, ApiRequestError } from "../api/client";
import { useToast } from "../hooks/useToast";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import EmptyState from "../components/ui/EmptyState";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";

type UserRow = { id: string; username: string; role: "Admin" | "Teacher" };
type RoomRow = { id: string; name: string; capacity: number | null };
type AvailabilityRow = { id: string; start_at: string; end_at: string };

function toDatetimeLocalValue(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export default function Availability() {
  const { addToast } = useToast();
  const [mode, setMode] = useState<"teacher" | "room">("teacher");

  const [users, setUsers] = useState<UserRow[]>([]);
  const [rooms, setRooms] = useState<RoomRow[]>([]);
  const [selectedId, setSelectedId] = useState<string>("");

  const [rows, setRows] = useState<AvailabilityRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const [startLocal, setStartLocal] = useState<string>(() => toDatetimeLocalValue(new Date()));
  const [endLocal, setEndLocal] = useState<string>(() => toDatetimeLocalValue(new Date(Date.now() + 60 * 60 * 1000)));

  const teachers = useMemo(() => users.filter((u) => u.role === "Teacher"), [users]);

  const selectedLabel = useMemo(() => {
    if (!selectedId) return "";
    if (mode === "teacher") return teachers.find((t) => t.id === selectedId)?.username ?? selectedId;
    return rooms.find((r) => r.id === selectedId)?.name ?? selectedId;
  }, [mode, selectedId, teachers, rooms]);

  const loadBase = async () => {
    const [u, r] = await Promise.all([
      apiJson<UserRow[]>("/api/v1/users?role=Teacher"),
      apiJson<RoomRow[]>("/api/v1/rooms"),
    ]);
    setUsers(u);
    setRooms(r);
    const defaultTeacher = u.find((x) => x.role === "Teacher")?.id ?? "";
    const defaultRoom = r[0]?.id ?? "";
    setSelectedId((prev) => prev || (mode === "teacher" ? defaultTeacher : defaultRoom));
  };

  const loadWindows = async (id: string) => {
    if (!id) {
      setRows([]);
      return;
    }
    setLoading(true);
    try {
      if (mode === "teacher") {
        setRows(await apiJson<AvailabilityRow[]>(`/api/v1/availability/teachers/${id}`));
      } else {
        setRows(await apiJson<AvailabilityRow[]>(`/api/v1/availability/rooms/${id}`));
      }
    } catch (err) {
      if (err instanceof ApiRequestError && err.code) addToast("error", `${err.code}: ${err.message}`);
      else addToast("error", err instanceof Error ? err.message : "Load failed");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadBase();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void loadWindows(selectedId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, selectedId]);

  const submitCreate = async () => {
    if (!selectedId) {
      addToast("error", `Select a ${mode === "teacher" ? "teacher" : "room"}`);
      return;
    }
    const start = new Date(startLocal);
    const end = new Date(endLocal);
    if (Number.isNaN(start.getTime())) {
      addToast("error", "Invalid start time");
      return;
    }
    if (Number.isNaN(end.getTime()) || end <= start) {
      addToast("error", "Invalid end time");
      return;
    }
    setSaving(true);
    try {
      if (mode === "teacher") {
        await apiJson(`/api/v1/availability/teachers/${selectedId}`, {
          method: "POST",
          body: JSON.stringify({ start_at: start.toISOString(), end_at: end.toISOString() }),
        });
      } else {
        await apiJson(`/api/v1/availability/rooms/${selectedId}`, {
          method: "POST",
          body: JSON.stringify({ start_at: start.toISOString(), end_at: end.toISOString() }),
        });
      }
      addToast("success", "Availability window created");
      await loadWindows(selectedId);
    } catch (err) {
      if (err instanceof ApiRequestError && err.code) addToast("error", `${err.code}: ${err.message}`);
      else addToast("error", err instanceof Error ? err.message : "Create failed");
    } finally {
      setSaving(false);
    }
  };

  const submitDelete = async (id: string) => {
    if (!selectedId) return;
    setSaving(true);
    try {
      if (mode === "teacher") {
        await apiJson(`/api/v1/availability/teachers/${selectedId}/${id}`, { method: "DELETE" });
      } else {
        await apiJson(`/api/v1/availability/rooms/${selectedId}/${id}`, { method: "DELETE" });
      }
      addToast("success", "Availability window removed");
      await loadWindows(selectedId);
    } catch (err) {
      if (err instanceof ApiRequestError && err.code) addToast("error", `${err.code}: ${err.message}`);
      else addToast("error", err instanceof Error ? err.message : "Delete failed");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <PageHeading>Availability</PageHeading>
      <p className="text-sm text-gray-500 mb-4">
        Hard availability windows for teachers and rooms. If any windows exist for a teacher/room, sessions must fit inside a window.
      </p>

      <div className="flex flex-wrap items-end gap-2 mb-4">
        <div className="flex bg-gray-100 rounded-sm p-0.5">
          <button
            onClick={() => setMode("teacher")}
            className={`px-3 py-1.5 text-xs font-medium rounded-sm ${mode === "teacher" ? "bg-white text-gray-800 shadow-sm" : "text-gray-500"}`}
          >
            Teachers
          </button>
          <button
            onClick={() => setMode("room")}
            className={`px-3 py-1.5 text-xs font-medium rounded-sm ${mode === "room" ? "bg-white text-gray-800 shadow-sm" : "text-gray-500"}`}
          >
            Rooms
          </button>
        </div>

        <div>
          <label className="block text-xs text-gray-500 mb-1">{mode === "teacher" ? "Teacher" : "Room"}</label>
          <select
            value={selectedId}
            onChange={(e) => setSelectedId(e.target.value)}
            className="px-2 py-1 text-sm border border-gray-300 rounded-sm min-w-[240px]"
          >
            <option value="">Select…</option>
            {mode === "teacher" &&
              teachers.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.username} ({t.id.slice(0, 8)}…)
                </option>
              ))}
            {mode === "room" &&
              rooms.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name} ({r.capacity ?? "—"})
                </option>
              ))}
          </select>
        </div>

        <div className="ml-auto flex items-end gap-2">
          <div>
            <label className="block text-xs text-gray-500 mb-1">Start</label>
            <input
              type="datetime-local"
              value={startLocal}
              onChange={(e) => setStartLocal(e.target.value)}
              className="px-2 py-1 text-sm border border-gray-300 rounded-sm"
            />
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">End</label>
            <input
              type="datetime-local"
              value={endLocal}
              onChange={(e) => setEndLocal(e.target.value)}
              className="px-2 py-1 text-sm border border-gray-300 rounded-sm"
            />
          </div>
          <Button variant="primary" size="md" disabled={saving} onClick={submitCreate}>
            Add Window
          </Button>
        </div>
      </div>

      <div className="mb-2 text-xs text-gray-500">
        {selectedId ? (
          <>
            Managing windows for <span className="font-mono text-gray-700">{selectedLabel}</span>
          </>
        ) : (
          <>Select a {mode === "teacher" ? "teacher" : "room"} to manage windows.</>
        )}
      </div>

      <div className="overflow-x-auto"><table className="w-full text-[13px]">
        <thead>
          <tr className="border-b-2 border-gray-300">
            <th className="text-left py-2 px-2 font-semibold">Start (UTC)</th>
            <th className="text-left py-2 px-2 font-semibold">End (UTC)</th>
            <th className="text-left py-2 px-2 font-semibold">Start (Local)</th>
            <th className="text-left py-2 px-2 font-semibold">End (Local)</th>
            <th className="text-left py-2 px-2 font-semibold"></th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.id} className="border-b border-gray-200 hover:bg-gray-50">
              <td className="py-2 px-2 font-mono text-xs text-gray-600">{r.start_at}</td>
              <td className="py-2 px-2 font-mono text-xs text-gray-600">{r.end_at}</td>
              <td className="py-2 px-2 text-xs text-gray-700">{format(new Date(r.start_at), "yyyy-MM-dd HH:mm")}</td>
              <td className="py-2 px-2 text-xs text-gray-700">{format(new Date(r.end_at), "yyyy-MM-dd HH:mm")}</td>
              <td className="py-2 px-2 text-right">
                <Button variant="danger" size="sm" disabled={saving} onClick={() => submitDelete(r.id)}>
                  remove
                </Button>
              </td>
            </tr>
          ))}
        </tbody>
      </table></div>

      {loading && <LoadingSkeleton type="table" lines={3} />}
      {!loading && selectedId && rows.length === 0 && <EmptyState message="No availability windows configured for this entity. Use the form above to add one." />}
    </div>
  );
}
