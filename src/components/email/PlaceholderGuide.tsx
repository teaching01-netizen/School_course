import { useState } from "react";

const PLACEHOLDERS = [
  { token: "{{student_name}}", description: "Student's full name" },
  { token: "{{student_nickname}}", description: "Student's nickname" },
  { token: "{{course_code}}", description: "Missed course code" },
  { token: "{{course_name}}", description: "Missed course name" },
  { token: "{{sit_in_course_code}}", description: "Sit-in course code" },
  { token: "{{sit_in_course_name}}", description: "Sit-in course name" },
  { token: "{{sit_in_date}}", description: "Sit-in session date" },
  { token: "{{sit_in_time}}", description: "Sit-in session time range" },
  { token: "{{absence_date_range}}", description: "Absence date range" },
  { token: "{{institute_name}}", description: "Institute display name" },
  { token: "{{today_date}}", description: "Today's date" },
];

export default function PlaceholderGuide({ onInsert }: { onInsert: (token: string) => void }) {
  const [open, setOpen] = useState(false);

  return (
    <div className="border border-gray-200 rounded-sm">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center justify-between w-full px-3 py-2 text-sm font-medium text-gray-700 bg-gray-50 hover:bg-gray-100"
      >
        <span>Available placeholders</span>
        <span className="text-xs text-gray-400">{open ? "▲" : "▼"}</span>
      </button>
      {open && (
        <div className="p-2 space-y-1">
          {PLACEHOLDERS.map((p) => (
            <button
              key={p.token}
              type="button"
              onClick={() => onInsert(p.token)}
              className="flex items-center justify-between w-full px-2 py-1.5 text-xs rounded hover:bg-blue-50 text-left group"
            >
              <code className="text-blue-600 font-mono text-xs">{p.token}</code>
              <span className="text-gray-500 group-hover:text-gray-700">{p.description}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
