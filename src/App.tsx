import { BrowserRouter, Routes, Route, Navigate, Outlet } from "react-router-dom";
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
import CourseEdit from './pages/CourseEdit';
import Availability from './pages/Availability';
import Reports from './pages/Reports';
import Logs from './pages/Logs';
import SlotFinder from './pages/SlotFinder';
import CrmAdmin from './pages/CrmAdmin';
import CourseLevels from './pages/CourseLevels';
import AbsenceForm from './pages/AbsenceForm';
import Absences from './pages/Absences';
import AbsenceDetail from './pages/AbsenceDetail';
import AbsenceDashboard from './pages/AbsenceDashboard';
import AbsenceSettings from './pages/AbsenceSettings';
import OperationsCalendar from './pages/OperationsCalendar';
import OperationsHub from './pages/operations/OperationsHub';
import LeavePolicy from './pages/LeavePolicy';

function AppLayout() {
  return (
    <Layout>
      <Outlet />
    </Layout>
  );
}

function RequireAuth() {
  const { user, loading } = useAuth();
  if (loading) return null;
  if (!user) return <Navigate to="/login" replace />;
  return <AppLayout />;
}

function App() {
  return (
    <ToastProvider>
      <AuthProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route path="/absence" element={<AbsenceForm />} />
            <Route element={<RequireAuth />}>
              <Route path="/" element={<Home />} />
              <Route path="/courses" element={<Courses />} />
              <Route path="/courses/create" element={<CourseCreate />} />
              <Route path="/courses/:id" element={<CourseDetail />} />
              <Route path="/courses/:id/edit" element={<CourseEdit />} />
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
              <Route path="/admin/absence-settings" element={<AbsenceSettings />} />
              <Route path="/absences/calendar" element={<OperationsCalendar />} />
              <Route path="/operations/calendar" element={<Navigate to="/absences/calendar" replace />} />
              <Route path="/admin/operations" element={<OperationsHub />} />
              <Route path="/leave-policy" element={<LeavePolicy />} />
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </BrowserRouter>
      </AuthProvider>
    </ToastProvider>
  );
}

export default App;
