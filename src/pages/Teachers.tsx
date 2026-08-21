import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { useToast } from '../hooks/useToast';
import { useApiQuery } from '@/hooks/useApiQuery';
import { useApiMutation } from '@/hooks/useApiMutation';
import PageHeading from "../components/ui/PageHeading";
import SearchInput from "../components/ui/SearchInput";
import Button from "../components/ui/Button";
import ConfirmModal from "../components/ConfirmModal";
import EmptyState from "../components/ui/EmptyState";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";

type Teacher = { id: string; username: string; full_name: string | null; role: 'Admin' | 'Teacher' };

export default function Teachers() {
  const [search, setSearch] = useState('');
  const [deletingID, setDeletingID] = useState<string | null>(null);
  const [confirmTeacher, setConfirmTeacher] = useState<Teacher | null>(null);
  const { addToast } = useToast();

  const { data: teachers, loading, error, refetch } = useApiQuery<Teacher[]>('/api/v1/users?role=Teacher');
  const { mutate: deleteUser, loading: deleting } = useApiMutation<unknown, unknown>('DELETE');

  useEffect(() => {
    if (!error) return;
    if (error.status === 401) {
      addToast('error', 'Please sign in to view teachers');
    } else {
      addToast('error', error.message);
    }
  }, [error]);

  const filtered = useMemo(() => {
    let data = [...(teachers ?? [])];
    if (search) {
      const q = search.toLowerCase();
      data = data.filter((t) => (t.full_name ?? "").toLowerCase().includes(q) || t.username.toLowerCase().includes(q) || t.id.toLowerCase().includes(q));
    }
    return data;
  }, [search, teachers]);

  const onDelete = async (teacher: Teacher) => {
    if (deleting || deletingID) return;
    setDeletingID(teacher.id);
    try {
      await deleteUser({}, `/api/v1/admin/users/${teacher.id}`);
      addToast('success', 'Teacher deleted');
      refetch();
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Delete failed');
    } finally {
      setDeletingID(null);
      setConfirmTeacher(null);
    }
  };

  return (
    <div>
      <PageHeading>Teachers</PageHeading>
      <div className="flex flex-wrap items-center gap-2 mb-4">
        <SearchInput value={search} onChange={setSearch} placeholder="Search by ID or username..." />
        <Button variant="secondary" size="sm" onClick={() => void refetch()}>Reload</Button>
        <Link to="/teachers/create" className="px-3 py-1 text-sm rounded-sm bg-[var(--color-wi-green)] hover:bg-[var(--color-wi-green-dark)] text-white">Create Teacher</Link>
      </div>
      <div className="overflow-x-auto data-table-wrapper"><table className="w-full text-[13px]">
        <caption className="sr-only">List of teachers</caption>
        <thead>
          <tr className="border-b var(--color-wi-line)">
            <th scope="col" className="text-left py-2 px-2 font-semibold">ID</th>
            <th scope="col" className="text-left py-2 px-2 font-semibold">Name</th>
            <th scope="col" className="text-left py-2 px-2 font-semibold">Username</th>
            <th scope="col" className="text-left py-2 px-2 font-semibold"></th>
          </tr>
        </thead>
        <tbody>
          {loading ? (
            <tr><td colSpan={4}><LoadingSkeleton type="table" lines={3} /></td></tr>
          ) : error ? (
            <tr><td colSpan={4}><EmptyState message={`Failed to load teachers (${error.message}). Press Reload to try again.`} /></td></tr>
          ) : filtered.length === 0 ? (
            <tr><td colSpan={4}><EmptyState message="No teachers found." /></td></tr>
          ) : filtered.map((t) => (
              <tr key={t.id} className="border-b var(--color-wi-line) hover:bg-[var(--color-wi-row-alt)]">
                <td className="py-2 px-2 font-mono text-xs text-[var(--color-wi-text-light)]">{t.id}</td>
                <td className="py-2 px-2 text-xs text-[var(--color-wi-text)]">{t.full_name || '—'}</td>
                <td className="py-2 px-2 font-mono text-xs text-[var(--color-wi-text-light)]">{t.username}</td>
                <td className="py-2 px-2">
                  <div className="flex items-center gap-2">
                    <Link to={`/teachers/${t.id}`} className="px-2 py-0.5 text-xs bg-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary-dark)] text-white rounded-sm inline-block">
                      detail
                    </Link>
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => setConfirmTeacher(t)}
                      disabled={deletingID === t.id}
                      loading={deletingID === t.id}
                    >
                      {deletingID === t.id ? 'deleting…' : 'delete'}
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
        </tbody>
      </table></div>

      <ConfirmModal
        open={!!confirmTeacher}
        title="Delete Teacher"
        message={confirmTeacher ? `Delete teacher "${confirmTeacher.username}"?` : ""}
        variant="danger"
        confirmLabel="Delete"
        loading={deleting}
        onConfirm={() => confirmTeacher && onDelete(confirmTeacher)}
        onCancel={() => setConfirmTeacher(null)}
      />
    </div>
  );
}
