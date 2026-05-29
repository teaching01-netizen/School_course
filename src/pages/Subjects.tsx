import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useToast } from "../hooks/useToast";
import { useApiQuery } from "@/hooks/useApiQuery";
import { useApiMutation } from "@/hooks/useApiMutation";
import PageHeading from "../components/ui/PageHeading";
import SearchInput from "../components/ui/SearchInput";
import Button from "../components/ui/Button";
import ConfirmModal from "../components/ConfirmModal";
import EmptyState from "../components/ui/EmptyState";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";

type Subject = { id: string; code: string; name: string };

export default function Subjects() {
  const [search, setSearch] = useState("");
  const { addToast } = useToast();
  const [deletingID, setDeletingID] = useState<string | null>(null);
  const [confirmSubject, setConfirmSubject] = useState<Subject | null>(null);

  const { data: subjects, loading, error, refetch } = useApiQuery<Subject[]>("/api/v1/subjects");
  const { mutate: deleteSubject, loading: deleting } = useApiMutation<unknown, unknown>("DELETE");

  useEffect(() => {
    if (!error) return;
    if (error.status === 401) {
      addToast("error", "Please sign in to view subjects");
    } else {
      addToast("error", error.message);
    }
  }, [error]);

  const filteredSubjects = useMemo(() => {
    if (!search) return subjects ?? [];
    const q = search.toLowerCase();
    return (subjects ?? []).filter((s) => s.code.toLowerCase().includes(q) || s.name.toLowerCase().includes(q) || s.id.toLowerCase().includes(q));
  }, [search, subjects]);

  const onDelete = async (subject: Subject) => {
    if (deleting || deletingID) return;
    setDeletingID(subject.id);
    try {
      await deleteSubject({}, `/api/v1/subjects/${subject.id}`);
      addToast("success", "Subject deleted");
      refetch();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Delete failed");
    } finally {
      setDeletingID(null);
      setConfirmSubject(null);
    }
  };

  return (
    <div>
      <PageHeading>Subject</PageHeading>
      <div className="flex flex-wrap items-center gap-2 mb-4">
        <SearchInput value={search} onChange={setSearch} placeholder="Search ID, Name" />
        <Link to="/subjects/create" className="px-4 py-2 text-sm rounded-md bg-[var(--color-wi-green)] hover:bg-[var(--color-wi-green-dark)] text-white inline-block">
          Create
        </Link>
      </div>

      <div className="overflow-x-auto"><table className="w-full text-[13px]">
        <thead>
          <tr className="border-b-2 border-gray-200">
            <th className="text-left py-2 px-2 font-semibold text-gray-700">Id</th>
            <th className="text-left py-2 px-2 font-semibold text-gray-700">Name</th>
            <th className="text-left py-2 px-2 font-semibold text-gray-700"></th>
          </tr>
        </thead>
        <tbody>
          {loading ? (
            <tr>
              <td colSpan={3}>
                <LoadingSkeleton type="table" lines={3} />
              </td>
            </tr>
          ) : filteredSubjects.length === 0 ? (
            <tr>
              <td colSpan={3}>
                <EmptyState message="No subjects found." />
              </td>
            </tr>
          ) : (
            filteredSubjects.map((s, idx) => (
              <tr key={s.id} className={`border-b border-gray-100 hover:bg-gray-50 ${idx % 2 === 1 ? "bg-gray-50/40" : ""}`}>
                <td className="py-2 px-2 font-mono text-xs text-gray-700">{s.code}</td>
                <td className="py-2 px-2">{s.name}</td>
                <td className="py-2 px-2">
                  <div className="flex items-center gap-2">
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => setConfirmSubject(s)}
                      disabled={deletingID === s.id}
                      loading={deletingID === s.id}
                    >
                      {deletingID === s.id ? "deleting…" : "delete"}
                    </Button>
                  </div>
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table></div>

      <ConfirmModal
        open={!!confirmSubject}
        title="Delete Subject"
        message={confirmSubject ? `Delete subject "${confirmSubject.code} - ${confirmSubject.name}"?` : ""}
        variant="danger"
        confirmLabel="Delete"
        loading={deleting}
        onConfirm={() => confirmSubject && onDelete(confirmSubject)}
        onCancel={() => setConfirmSubject(null)}
      />
    </div>
  );
}
