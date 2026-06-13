type Props = {
  count: number | null;
};

export default function StudentStatusBadge({ count }: Props) {
  const hasStudents = count !== null && count > 0;

  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        className={`inline-block h-2 w-2 rounded-full ${
          hasStudents ? "bg-green-500" : "bg-gray-300"
        }`}
      />
      <span
        className={`text-xs ${
          hasStudents ? "text-green-700 font-medium" : "text-gray-400"
        }`}
      >
        {hasStudents ? `${count} Enrolled` : "No students"}
      </span>
    </span>
  );
}
