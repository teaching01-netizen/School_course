import { useEffect, useState } from "react";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import EmptyState from "../components/ui/EmptyState";

type AuditItem = {
  id: number;
  created_at: string;
  actor_user_id: string | null;
  action: string;
  payload: unknown;
};

export default function Logs() {
  const { addToast } = useToast();
  const [items, setItems] = useState<AuditItem[]>([]);
  const [loading, setLoading] = useState(true);

  const load = async () => {
    try {
      setLoading(true);
      const res = await apiJson<AuditItem[]>("/api/v1/audit?limit=200", { method: "GET" });
      setItems(res);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load audit log");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div>
      <PageHeading>Log</PageHeading>

      <div className="flex items-center gap-2 mb-3">
        <Button variant="secondary" size="md" onClick={() => void load()}>Refresh</Button>
      </div>

      {loading ? (
        <LoadingSkeleton type="table" lines={3} />
      ) : items.length === 0 ? (
        <EmptyState message="No audit entries" />
      ) : (
        <div className="border border-gray-200 rounded-sm overflow-x-auto">
          <table className="w-full text-[13px]">
            <thead>
              <tr className="border-b border-gray-200 bg-gray-50">
                <th className="text-left py-2 px-2 font-semibold">Time</th>
                <th className="text-left py-2 px-2 font-semibold">Actor</th>
                <th className="text-left py-2 px-2 font-semibold">Action</th>
                <th className="text-left py-2 px-2 font-semibold">Payload</th>
              </tr>
            </thead>
            <tbody>
              {items.map((it) => (
                <tr key={it.id} className="border-b border-gray-100 last:border-b-0 align-top">
                  <td className="py-2 px-2 font-mono text-xs text-gray-600 whitespace-nowrap">{it.created_at}</td>
                  <td className="py-2 px-2 font-mono text-xs text-gray-600">{it.actor_user_id ?? "—"}</td>
                  <td className="py-2 px-2 text-gray-800 whitespace-nowrap">{it.action}</td>
                  <td className="py-2 px-2">
                    <pre className="text-[11px] bg-gray-50 border border-gray-200 rounded-sm p-2 overflow-auto max-h-40">
                      {JSON.stringify(it.payload ?? {}, null, 2)}
                    </pre>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

