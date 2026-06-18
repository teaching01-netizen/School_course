import type { TeacherDashboardResponse } from '../../types';
import SummaryBar from './SummaryBar';
import TabBar from './TabBar';
import TeacherDashboardAlerts from './TeacherDashboardAlerts';
import TeacherDashboardTable from './TeacherDashboardTable';

const DASHBOARD_TABS = [
  { id: 'alerts', label: "Today's Alerts" },
  { id: 'schedule', label: 'Schedule' },
] as const;

export type DashboardTab = (typeof DASHBOARD_TABS)[number]['id'];

type DashboardViewProps = {
  data: TeacherDashboardResponse;
  weekStart: Date;
  activeTab: DashboardTab;
  onTabChange: (tab: DashboardTab) => void;
};

export default function DashboardView({
  data,
  weekStart,
  activeTab,
  onTabChange,
}: DashboardViewProps) {
  return (
    <>
      <SummaryBar
        totalSessions={data.summary.total_sessions}
        totalAbsences={data.summary.total_absences}
        totalSitIns={data.summary.total_sit_ins}
      />

      <TabBar tabs={DASHBOARD_TABS} activeTab={activeTab} onChange={onTabChange} />

      {activeTab === 'alerts' ? (
        <TeacherDashboardAlerts sessions={data.sessions} />
      ) : (
        <div className="rounded-sm border border-gray-200 bg-white">
          <TeacherDashboardTable sessions={data.sessions} weekStart={weekStart} />
        </div>
      )}

      {data.sessions.length === 0 ? (
        <p className="py-8 text-center text-sm text-gray-400">No sessions this week.</p>
      ) : null}
    </>
  );
}
