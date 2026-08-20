import { lazy, Suspense, useCallback, type ReactNode } from "react";
import { BrowserRouter, Routes, Route, Navigate, Outlet } from "react-router-dom";
import { QueryClientProvider, useQueryClient } from "@tanstack/react-query";
import { ToastProvider } from './hooks/useToast';
import { AuthProvider, useAuth } from "./hooks/useAuth";
import Layout from './components/Layout';
import { queryClient } from "./query/cache";
import { RealtimeProvider } from "./realtime/RealtimeProvider";
import { invalidateRealtimeBackedQueries, RealtimeQueryBridge } from "./realtime/queryBridge";
import { useUserScopedQueryCache } from "./query/useUserScopedQueryCache";

const Login = lazy(() => import('./pages/Login'));
const Home = lazy(() => import('./pages/Home'));
const AssignRooms = lazy(() => import('./pages/AssignRooms'));
const Courses = lazy(() => import('./pages/Courses'));
const CourseCreate = lazy(() => import('./pages/CourseCreate'));
const CourseDetail = lazy(() => import('./pages/CourseDetail'));
const Students = lazy(() => import('./pages/Students'));
const StudentProfile = lazy(() => import('./pages/StudentProfile'));
const Teachers = lazy(() => import('./pages/Teachers'));
const TeacherCreate = lazy(() => import('./pages/TeacherCreate'));
const TeacherProfile = lazy(() => import('./pages/TeacherProfile'));
const Subjects = lazy(() => import('./pages/Subjects'));
const SubjectCreate = lazy(() => import('./pages/SubjectCreate'));
const Classrooms = lazy(() => import('./pages/Classrooms'));
const Users = lazy(() => import('./pages/Users'));
const Schedule = lazy(() => import('./pages/Schedule'));
const Summary = lazy(() => import('./pages/Summary'));
const Availability = lazy(() => import('./pages/Availability'));
const Reports = lazy(() => import('./pages/Reports'));
const Logs = lazy(() => import('./pages/Logs'));
const SlotFinder = lazy(() => import('./pages/SlotFinder'));
const CrmAdmin = lazy(() => import('./pages/CrmAdmin'));
const CrmConflicts = lazy(() => import('./pages/CrmConflicts'));
const CrossStudyPage = lazy(() => import('./pages/CrossStudyPage'));
const CourseLevels = lazy(() => import('./pages/CourseLevels'));
const AbsenceForm = lazy(() => import('./pages/AbsenceForm'));
const Absences = lazy(() => import('./pages/Absences'));
const AbsenceDetail = lazy(() => import('./pages/AbsenceDetail'));
const AbsenceDashboard = lazy(() => import('./pages/AbsenceDashboard'));
const AbsenceSettings = lazy(() => import('./pages/AbsenceSettings'));
const OperationsCalendar = lazy(() => import('./pages/OperationsCalendar'));
const OperationsHub = lazy(() => import('./pages/operations/OperationsHub'));
const SessionChanges = lazy(() => import('./pages/SessionChanges'));
const SessionChangeDetail = lazy(() => import('./pages/SessionChangeDetail'));
const LeavePolicy = lazy(() => import('./pages/LeavePolicy'));
const EmailReminders = lazy(() => import('./pages/EmailReminders'));
const SitInTestPage = lazy(() => import('./pages/SitInTestPage'));
const TeacherDashboard = lazy(() => import('./pages/TeacherDashboard'));
const TeacherAbsenceDetail = lazy(() => import('./pages/TeacherAbsenceDetail'));
const LegacySyncHealth = lazy(() => import('./pages/LegacySyncHealth'));
const LegacyAudit = lazy(() => import('./pages/LegacyAudit'));

function AuthenticatedDataServices({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const client = useQueryClient();
  const handleReconnect = useCallback(() => {
    void invalidateRealtimeBackedQueries(client);
  }, [client]);

  const cacheReady = useUserScopedQueryCache(user?.id ?? null);
  if (!cacheReady) return null;

  return (
    <RealtimeProvider enabled={user != null} onReconnect={handleReconnect}>
      <RealtimeQueryBridge />
      {children}
    </RealtimeProvider>
  );
}

function AppLayout() {
  return (
    <Layout>
      <Outlet />
    </Layout>
  );
}

function IndexRoute() {
  const { user } = useAuth();
  if (user?.role === 'Teacher') return <Navigate to="/teacher-dashboard" replace />;
  return <Home />;
}

function LoadingScreen() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-white">
      <span className="sr-only">Loading...</span>
      <div className="flex items-center gap-3 text-[13px] text-[var(--color-wi-text-light)]">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-[var(--color-wi-line)] border-t-[var(--color-wi-primary)]" />
        Loading…
      </div>
    </div>
  );
}

function RequireAuth() {
  const { user, loading } = useAuth();
  if (loading) return <LoadingScreen />;
  if (!user) return <Navigate to="/login" replace />;
  return <AppLayout />;
}

function RequireTeacherOrAdmin() {
  const { user, loading } = useAuth();
  if (loading) return <LoadingScreen />;
  if (user?.role === 'Teacher' || user?.role === 'Admin') return <Outlet />;
  return <Navigate to="/login" replace />;
}

function RequireAdmin() {
  const { user, loading } = useAuth();
  if (loading) return <LoadingScreen />;
  if (user?.role === 'Admin') return <Outlet />;
  if (!user) return <Navigate to="/login" replace />;
  return <Navigate to="/teacher-dashboard" replace />;
}

const loadingFallback = (
  <div className="flex min-h-screen items-center justify-center bg-white">
    <span className="sr-only">Loading...</span>
    <div className="flex items-center gap-3 text-[13px] text-[var(--color-wi-text-light)]">
      <div className="h-4 w-4 animate-spin rounded-full border-2 border-[var(--color-wi-line)] border-t-[var(--color-wi-primary)]" />
      Loading…
    </div>
  </div>
);

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <AuthProvider>
          <AuthenticatedDataServices>
            <BrowserRouter>
          <Suspense fallback={loadingFallback}>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route path="/absence" element={<AbsenceForm />} />
            <Route element={<RequireAuth />}>
              <Route path="/" element={<IndexRoute />} />
              <Route element={<RequireTeacherOrAdmin />}>
                <Route path="/teacher-dashboard" element={<TeacherDashboard />} />
                <Route path="/teacher-dashboard/absences/:id" element={<TeacherAbsenceDetail />} />
              </Route>
              <Route element={<RequireAdmin />}>
                <Route path="/courses" element={<Courses />} />
                <Route path="/courses/create" element={<CourseCreate />} />
                <Route path="/courses/:id" element={<CourseDetail />} />
                <Route path="/students" element={<Students />} />
                <Route path="/students/:wcode" element={<StudentProfile />} />
                <Route path="/teachers" element={<Teachers />} />
                <Route path="/teachers/create" element={<TeacherCreate />} />
                <Route path="/teachers/:id" element={<TeacherProfile />} />
                <Route path="/subjects" element={<Subjects />} />
                <Route path="/subjects/create" element={<SubjectCreate />} />
                <Route path="/classrooms" element={<Classrooms />} />
                <Route path="/users" element={<Users />} />
                <Route path="/schedule" element={<Schedule />} />
                <Route path="/assign" element={<AssignRooms />} />
                <Route path="/summary" element={<Summary />} />
                <Route path="/availability" element={<Availability />} />
                <Route path="/reports" element={<Reports />} />
                <Route path="/absences" element={<Absences />} />
                <Route path="/absences/board" element={<Navigate to="/absences?view=board" replace />} />
                <Route path="/absences/dashboard" element={<AbsenceDashboard />} />
                <Route path="/absences/:id" element={<AbsenceDetail />} />
                <Route path="/logs" element={<Logs />} />
                <Route path="/slot-finder" element={<SlotFinder />} />
                <Route path="/course-levels" element={<CourseLevels />} />
                <Route path="/crm" element={<CrmAdmin />} />
                <Route path="/crm/conflicts" element={<CrmConflicts />} />
                <Route path="/crm/cross-study" element={<CrossStudyPage />} />
                <Route path="/admin/absence-settings" element={<AbsenceSettings />} />
                <Route path="/absences/calendar" element={<OperationsCalendar />} />
                <Route path="/operations/calendar" element={<Navigate to="/absences/calendar" replace />} />
                <Route path="/admin/operations" element={<OperationsHub />} />
                <Route path="/operations/schedule-impact" element={<SessionChanges />} />
                <Route path="/operations/session-changes" element={<Navigate to="/operations/schedule-impact?view=history" replace />} />
                <Route path="/operations/session-changes/:id" element={<SessionChangeDetail />} />
                <Route path="/leave-policy" element={<LeavePolicy />} />
                <Route path="/email-reminders" element={<EmailReminders />} />
                <Route path="/admin/sit-in-test" element={<SitInTestPage />} />
                <Route path="/admin/legacy-sync" element={<LegacySyncHealth />} />
                <Route path="/admin/legacy-sync/audit" element={<LegacyAudit />} />
              </Route>
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
          </Suspense>
            </BrowserRouter>
          </AuthenticatedDataServices>
        </AuthProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
}

export default App;
