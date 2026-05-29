import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { TrendingUp, TrendingDown, Minus, ChevronDown } from "lucide-react";
import { apiJson, downloadApiFile } from "../api/client";
import { useToast } from "../hooks/useToast";
import type { AbsenceStats, AbsenceTrends } from "../types";
import PageHeading from "../components/ui/PageHeading";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";

type Period = "week" | "month" | "quarter";

type Breakdown = { label: string; count: number };
type DashboardResponse = {
  period: string;
  stats: AbsenceStats;
  trends?: AbsenceTrends;
  subjects: Breakdown[];
  reasons: Breakdown[];
  heatmap: [string, number][];
  attention: { course_code: string; course_name: string; absence_count: number }[];
};

function StatCard({ label, count, prevCount }: { label: string; count: number; prevCount?: number }) {
  const diff = prevCount !== undefined ? count - prevCount : 0;
  return (
    <section className="rounded-sm border border-gray-200 bg-white p-4">
      <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
      <p className="mt-1 text-3xl font-semibold text-gray-900">{count}</p>
      {prevCount !== undefined && prevCount >= 0 ? (
        <div className="mt-1 flex items-center gap-1 text-xs">
          {diff > 0 ? (
            <><TrendingUp className="h-3.5 w-3.5 text-red-500" /><span className="text-red-600">+{diff}</span></>
          ) : diff < 0 ? (
            <><TrendingDown className="h-3.5 w-3.5 text-green-500" /><span className="text-green-600">{diff}</span></>
          ) : (
            <><Minus className="h-3.5 w-3.5 text-gray-400" /><span className="text-gray-500">No change</span></>
          )}
          <span className="text-gray-400">vs prev</span>
        </div>
      ) : null}
    </section>
  );
}

function BreakdownRows({ rows, total }: { rows: Breakdown[]; total: number }) {
  const max = Math.max(1, ...rows.map((r) => r.count));
  return (
    <div className="space-y-3">
      {rows.map((row) => (
        <div key={row.label} className="grid grid-cols-[90px_1fr_44px] items-center gap-3 text-sm">
          <span className="truncate font-medium">{row.label}</span>
          <div className="h-2 rounded-full bg-slate-100">
            <div className="h-2 rounded-full bg-[var(--color-wi-primary)]" style={{ width: `${Math.max(5, (row.count / max) * 100)}%` }} />
          </div>
          <span className="text-right text-gray-500">{total ? `${Math.round((row.count / total) * 100)}%` : row.count}</span>
        </div>
      ))}
      {rows.length === 0 ? <p className="text-sm text-gray-500">No data.</p> : null}
    </div>
  );
}

function HeatmapCell({ count, max }: { count: number; max: number }) {
  const intensity = max > 0 ? Math.min(1, count / max) : 0;
  const bg = intensity === 0 ? "bg-gray-50" : intensity < 0.25 ? "bg-blue-100" : intensity < 0.5 ? "bg-blue-300" : intensity < 0.75 ? "bg-blue-500" : "bg-blue-700";
  const text = intensity > 0.5 ? "text-white" : "text-gray-700";
  return (
    <div className={`flex items-center justify-center rounded-sm px-1.5 py-1 text-xs font-medium ${bg} ${text}`}>
      {count > 0 ? count : ""}
    </div>
  );
}

export default function AbsenceDashboard() {
  const { addToast } = useToast();
  const [period, setPeriod] = useState<Period>("month");
  const [month, setMonth] = useState(() => new Date().toISOString().slice(0, 7));
  const [data, setData] = useState<DashboardResponse | null>(null);
  const [showCancelled, setShowCancelled] = useState(false);

  useEffect(() => {
    const params = new URLSearchParams({ month, period });
    void apiJson<DashboardResponse>(`/api/v1/absences/dashboard?${params}`, { method: "GET" })
      .then(setData)
      .catch((err: unknown) => addToast("error", err instanceof Error ? err.message : "Failed to load dashboard"));
  }, [addToast, month, period]);

  async function exportReport() {
    const start = `${month}-01`;
    const end = new Date(Date.UTC(Number(month.slice(0, 4)), Number(month.slice(5, 7)), 0)).toISOString().slice(0, 10);
    try {
      await downloadApiFile(`/api/v1/absences/export?date_from=${start}&date_to=${end}`);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Export failed");
    }
  }

  if (!data) return <LoadingSkeleton type="table" lines={5} />;

  const trends = data.trends;
  const statCards: Array<{ label: string; count: number; prevCount: number }> = [
    { label: "Total", count: data.stats.total_count, prevCount: trends?.prev_total_count ?? -1 },
    { label: "Pending", count: data.stats.pending_count, prevCount: trends?.prev_pending_count ?? -1 },
    { label: "Reviewed", count: data.stats.reviewed_count, prevCount: trends?.prev_reviewed_count ?? -1 },
    { label: "Actioned", count: data.stats.actioned_count, prevCount: trends?.prev_actioned_count ?? -1 },
  ];

  return (
    <div className="w-full space-y-5">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <Link className="text-sm text-[var(--color-wi-primary)] hover:underline" to="/absences">Back to Records</Link>
          <PageHeading>Absence Dashboard</PageHeading>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex rounded-sm border border-gray-300 bg-white text-sm">
            {(["week", "month", "quarter"] as Period[]).map((p) => (
              <button
                key={p}
                onClick={() => setPeriod(p)}
                className={`px-3 py-1.5 font-medium ${period === p ? "bg-gray-100 text-gray-900" : "text-gray-500 hover:text-gray-700"}`}
              >
                {p === "week" ? "Week" : p === "month" ? "Month" : "Quarter"}
              </button>
            ))}
          </div>
          <label className="flex items-center gap-2 text-sm text-gray-600">
            <input className="ml-2" type="month" value={month} onChange={(e) => setMonth(e.target.value)} />
          </label>
          <Button variant="secondary" size="sm" onClick={() => void exportReport()}>Export</Button>
        </div>
      </header>

      <div className="grid gap-3 sm:grid-cols-4">
        {statCards.map(({ label, count, prevCount }) => (
          <StatCard key={label} label={label} count={count} prevCount={prevCount >= 0 ? prevCount : undefined} />
        ))}
      </div>

      <div>
        <button onClick={() => setShowCancelled(!showCancelled)} className="flex items-center gap-2 text-sm font-medium text-gray-600 hover:text-gray-900">
          <ChevronDown className={`h-4 w-4 transition-transform ${showCancelled ? "rotate-180" : ""}`} />
          Cancelled history — {data.stats.cancelled_count}
        </button>
        {showCancelled ? (
          <div className="mt-2 grid gap-3 sm:grid-cols-4">
            <StatCard label="Cancelled" count={data.stats.cancelled_count} prevCount={trends?.prev_cancelled_count ?? -1} />
          </div>
        ) : null}
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <section className="rounded-sm border border-gray-200 bg-white p-5">
          <h2 className="mb-4 text-base font-semibold">Absences by Subject</h2>
          <BreakdownRows rows={data.subjects} total={data.stats.total_count} />
        </section>
        <section className="rounded-sm border border-gray-200 bg-white p-5">
          <h2 className="mb-4 text-base font-semibold">Reasons ({period})</h2>
          <BreakdownRows rows={data.reasons} total={data.stats.total_count} />
        </section>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        {data.attention?.length > 0 ? (
          <section className="rounded-sm border border-gray-200 bg-white p-5">
            <h2 className="mb-4 text-base font-semibold">Courses Needing Attention</h2>
            <div className="space-y-2">
              {data.attention.map((course) => (
                <div key={course.course_code} className="flex items-center justify-between rounded-sm bg-amber-50 px-3 py-2 text-sm">
                  <Link to={`/courses/${course.course_code}`} className="font-medium text-[var(--color-wi-primary)] hover:underline">{course.course_code}</Link>
                  <span className="text-amber-700">{course.absence_count} absence{course.absence_count !== 1 ? "s" : ""}</span>
                </div>
              ))}
            </div>
          </section>
        ) : null}

        {data.heatmap && data.heatmap.length > 0 ? (
          <section className="rounded-sm border border-gray-200 bg-white p-5">
            <h2 className="mb-4 text-base font-semibold">Absence Heatmap ({period})</h2>
            <div className="grid grid-cols-7 gap-1">
              {data.heatmap.map(([date, count]) => (
                <div key={date} className="text-center">
                  <div className="text-[10px] text-gray-400 mb-0.5">
                    {new Date(date + "T00:00:00").toLocaleDateString("en-GB", { day: "numeric", month: "short" })}
                  </div>
                  <HeatmapCell count={count} max={Math.max(1, ...data.heatmap.map(([_, c]) => c))} />
                </div>
              ))}
            </div>
          </section>
        ) : null}
      </div>
    </div>
  );
}