import { useRef, useState, useEffect } from "react";

interface ActiveCourseSelectorProps {
  subjectId: string;
  courses: Array<{ id: string; code: string; name: string; cycleLabel: string }>;
  activeCourseId: string | null;
  disabled?: boolean;
  onSelect: (courseId: string) => void;
}

export default function ActiveCourseSelector({
  subjectId,
  courses,
  activeCourseId,
  disabled = false,
  onSelect,
}: ActiveCourseSelectorProps) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function handleMouseDown(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleMouseDown);
    return () => document.removeEventListener("mousedown", handleMouseDown);
  }, [open]);

  if (courses.length === 0) {
    return (
      <span className="text-xs text-gray-400 select-none">
        No courses
      </span>
    );
  }

  const activeCourse = courses.find((c) => c.id === activeCourseId);
  const activeInOtherGroup = activeCourseId !== null && !activeCourse;

  let triggerContent: React.ReactNode;

  if (activeInOtherGroup) {
    triggerContent = (
      <span className="text-xs text-gray-500">
        Active: {activeCourseId} (in group)
      </span>
    );
  } else if (activeCourse) {
    triggerContent = (
      <>
        <span className="w-1.5 h-1.5 rounded-full bg-green-500 inline-block shrink-0" />
        <span className="text-xs font-medium text-gray-700 truncate">
          {activeCourse.code}
        </span>
      </>
    );
  } else {
    triggerContent = (
      <span className="text-xs font-medium text-amber-600">
        Not set
      </span>
    );
  }

  return (
    <div className="relative" ref={containerRef}>
      <button
        type="button"
        disabled={disabled}
        onClick={() => setOpen((prev) => !prev)}
        className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded-sm transition-colors duration-150
          ${disabled ? "opacity-50 cursor-not-allowed" : "hover:bg-gray-100 cursor-pointer"}
        `}
        aria-label={`Active course selector for subject ${subjectId}`}
        aria-expanded={open}
      >
        {triggerContent}
        <svg
          className={`w-3 h-3 text-gray-400 transition-transform duration-150 ${open ? "rotate-180" : ""}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
          aria-hidden="true"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {open && (
        <div className="absolute left-0 top-full mt-1 z-50 w-56 bg-white border border-gray-200 rounded-sm shadow-lg">
          <div className="py-1">
            {courses.map((course) => {
              const isActive = course.id === activeCourseId;
              return (
                <button
                  key={course.id}
                  type="button"
                  onClick={() => {
                    onSelect(course.id);
                    setOpen(false);
                  }}
                  className={`w-full flex items-center gap-2 px-3 py-1.5 text-xs text-left transition-colors duration-150
                    ${isActive ? "bg-[var(--color-wi-primary)]/5 font-medium" : "hover:bg-gray-50"}
                  `}
                >
                  <span className="w-4 shrink-0 flex items-center justify-center">
                    {isActive && (
                      <svg
                        className="w-3 h-3 text-green-600"
                        fill="currentColor"
                        viewBox="0 0 20 20"
                        aria-hidden="true"
                      >
                        <path
                          fillRule="evenodd"
                          d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                          clipRule="evenodd"
                        />
                      </svg>
                    )}
                  </span>
                  <span className="flex-1 truncate text-gray-700">{course.code}</span>
                  <span className="shrink-0 text-gray-400">{course.cycleLabel}</span>
                </button>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
