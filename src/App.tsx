import { useCallback, useEffect, useState, type ReactNode } from "react";
import { BrowserRouter, Routes, Route, Navigate, Outlet } from "react-router-dom";
import { QueryClientProvider, useQueryClient } from "@tanstack/react-query";
import { ToastProvider } from './hooks/useToast';
import { AuthProvider, useAuth } from "./hooks/useAuth";
import Layout from './components/Layout';
import Login from './pages/Login';
import Home from './pages/Home';
import Courses from './pages/Courses';
import CourseCreate from './pages/CourseCreate';
import CourseDetail from './pages/CourseDetail';
import Students from './pages/Students';
import StudentProfile from './pages/StudentProfile';
import Teachers from './pages/Teachers';
import TeacherCreate from './pages/TeacherCreate';
import TeacherProfile from './pages/TeacherProfile';
import Subjects from './pages/Subjects';
import SubjectCreate from './pages/SubjectCreate';
import Classrooms from './pages/Classrooms';
import Users from './pages/Users';
import Schedule from './pages/Schedule';
import Summary from './pages/Summary';
import Availability from './pages/Availability';
import Reports from './pages/Reports';
import Logs from './pages/Logs';
import SlotFinder from './pages/SlotFinder';
import CrmAdmin from './pages/CrmAdmin';
import CrmConflicts from './pages/CrmConflicts';
import CrossStudyPage from './pages/CrossStudyPage';
import CourseLevels from './pages/CourseLevels';
import AbsenceForm from './pages/AbsenceForm';
import Absences from './pages/Absences';
import AbsenceDetail from './pages/AbsenceDetail';
import AbsenceDashboard from './pages/AbsenceDashboard';
import AbsenceSettings from './pages/AbsenceSettings';
import OperationsCalendar from './pages/OperationsCalendar';
import OperationsHub from './pages/operations/OperationsHub';
import LeavePolicy from './pages/LeavePolicy';
import EmailReminders from './pages/EmailReminders';
import SitInTestPage from './pages/SitInTestPage';
import TeacherDashboard from './pages/TeacherDashboard';
import TeacherAbsenceDetail from './pages/TeacherAbsenceDetail';
import { clearCacheForUserChange, queryClient } from "./query/cache";
import { RealtimeProvider } from "./realtime/RealtimeProvider";
import { invalidateRealtimeBackedQueries, RealtimeQueryBridge } from "./realtime/queryBridge";

function AuthenticatedDataServices({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const client = useQueryClient();
  const userID = user?.id ?? null;
  const [cacheUserID, setCacheUserID] = useState<string | null>(null);

  useEffect(() => {
    if (cacheUserID === userID) return;
    clearCacheForUserChange(client, cacheUserID, userID);
    setCacheUserID(userID);
  }, [cacheUserID, client, userID]);

  const handleReconnect = useCallback(() => {
    void invalidateRealtimeBackedQueries(client);
  }, [client]);

  if (cacheUserID !== userID) return null;

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

function RequireAuth() {
  const { user, loading } = useAuth();
  if (loading) return null;
  if (!user) return <Navigate to="/login" replace />;
  return <AppLayout />;
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <AuthProvider>
          <AuthenticatedDataServices>
            <BrowserRouter>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route path="/absence" element={<AbsenceForm />} />
            <Route element={<RequireAuth />}>
              <Route path="/" element={<IndexRoute />} />
              <Route path="/teacher-dashboard" element={<TeacherDashboard />} />
              <Route path="/teacher-dashboard/absences/:id" element={<TeacherAbsenceDetail />} />
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
              <Route path="/leave-policy" element={<LeavePolicy />} />
              <Route path="/email-reminders" element={<EmailReminders />} />
              <Route path="/admin/sit-in-test" element={<SitInTestPage />} />
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
            </BrowserRouter>
          </AuthenticatedDataServices>
        </AuthProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
}

export default App;
