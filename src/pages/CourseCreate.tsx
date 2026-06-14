import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { useFormValidation } from "../hooks/useFormValidation";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import Input from "../components/ui/Input";
import Select from "../components/ui/Select";
import FormField from "../components/ui/FormField";
import FormErrorSummary from "../components/ui/FormErrorSummary";
import MultiTeacherSelect from "../components/MultiTeacherSelect";

type Teacher = { id: string; username: string; role: "Admin" | "Teacher" };
type Subject = { id: string; code: string; name: string };

const schema = {
  year: [{ type: "required" as const, message: "Year is required" }],
  teacherID: [{ type: "required" as const, message: "At least one teacher is required" }],
  subjectID: [{ type: "required" as const, message: "Subject is required" }],
  hour: [{ type: "required" as const, message: "Hour is required" }],
  studentCount: [{ type: "min" as const, value: 1, message: "Student count must be at least 1" }],
};

export default function CourseCreate() {
  const navigate = useNavigate();
  const { addToast } = useToast();

  const [year, setYear] = useState(() => String(new Date().getFullYear() % 100));
  const [teacherIDs, setTeacherIDs] = useState<string[]>([]);
  const [subjectID, setSubjectID] = useState("");
  const [hour, setHour] = useState(0);
  const [studentCount, setStudentCount] = useState(0);
  const [courseType, setCourseType] = useState<"Private" | "Group">("Private");

  const [teachers, setTeachers] = useState<Teacher[]>([]);
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [loadingOptions, setLoadingOptions] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  const formValues = { year, teacherID: teacherIDs[0] || "", subjectID, hour, studentCount };
  const { errors, validate, validateAll, touched, touch } = useFormValidation(schema, formValues);
  const teacherOptions = useMemo(() => teachers.map((t) => ({ value: t.id, label: t.username, keywords: t.id })), [teachers]);

  useEffect(() => {
    (async () => {
      try {
        setLoadingOptions(true);
        const [t, s] = await Promise.all([
          apiJson<Teacher[]>("/api/v1/users?role=Teacher", { method: "GET" }),
          apiJson<Subject[]>("/api/v1/subjects", { method: "GET" }),
        ]);
        setTeachers(t);
        setSubjects(s);
      } catch {
        // Non-blocking: the page still renders with empty option lists.
      } finally {
        setLoadingOptions(false);
      }
    })();
  }, []);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!validateAll()) return;
    try {
      setSubmitting(true);
      await apiJson("/api/v1/courses", {
        method: "POST",
        body: JSON.stringify({
          year: Number.parseInt(year, 10),
          teacher_id: teacherIDs[0] || "",
          teacher_ids: teacherIDs,
          subject_id: subjectID,
          hour,
          student_count: studentCount,
          course_type: courseType,
        }),
      });
      addToast("success", "Course created");
      navigate("/courses");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Create failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="w-full">
      <PageHeading className="text-center">New Course</PageHeading>

      <form onSubmit={onSubmit} className="max-w-xl mx-auto space-y-4">
        <FormErrorSummary errors={errors} touched={touched} />

        <FormField name="year" label="Year" error={errors.year} touched={touched.year} required>
          <Input size="md" value={year} onChange={(e) => setYear(e.target.value)} inputMode="numeric" onBlur={() => { touch("year"); validate("year"); }} />
        </FormField>

        <FormField name="teacherID" label="Teacher" error={errors.teacherID} touched={touched.teacherID} required>
          <MultiTeacherSelect
            value={teacherIDs}
            onChange={setTeacherIDs}
            options={teacherOptions}
            placeholder="Select teachers…"
          />
        </FormField>

        <FormField name="subjectID" label="Subject" error={errors.subjectID} touched={touched.subjectID} required>
          <Select
            size="md"
            value={subjectID}
            onChange={(e) => setSubjectID(e.target.value)}
            disabled={loadingOptions}
            onBlur={() => { touch("subjectID"); validate("subjectID"); }}
          >
            <option value="">-- Select Subject --</option>
            {subjects.map((s) => (
              <option key={s.id} value={s.id}>
                {s.code} — {s.name}
              </option>
            ))}
          </Select>
        </FormField>

        <FormField name="hour" label="Hour" error={errors.hour} touched={touched.hour} required>
          <Input size="md" value={String(hour)} onChange={(e) => setHour(Number(e.target.value))} inputMode="numeric" onBlur={() => { touch("hour"); validate("hour"); }} />
        </FormField>

        <FormField name="studentCount" label="Student" error={errors.studentCount} touched={touched.studentCount}>
          <Input size="md" value={String(studentCount)} onChange={(e) => setStudentCount(Number(e.target.value))} inputMode="numeric" onBlur={() => { touch("studentCount"); validate("studentCount"); }} />
        </FormField>

        <FormField name="courseType" label="Type">
          <Select
            size="md"
            value={courseType}
            onChange={(e) => setCourseType(e.target.value === "Group" ? "Group" : "Private")}
          >
            <option value="Private">Private</option>
            <option value="Group">Group</option>
          </Select>
        </FormField>

        <div className="flex gap-3 mt-6">
          <Button type="submit" variant="primary" size="lg" loading={submitting}>
            {submitting ? "Saving…" : "Save"}
          </Button>
          <Button type="button" variant="secondary" size="lg" onClick={() => navigate("/courses")}>
            Back
          </Button>
        </div>
      </form>
    </div>
  );
}
