interface LoadingSkeletonProps {
  lines?: number;
  type?: "table" | "card" | "text";
}

export default function LoadingSkeleton({ lines = 3, type = "table" }: LoadingSkeletonProps) {
  const rows = Array.from({ length: lines });

  if (type === "text") {
    return (
      <div className="space-y-2 py-4" role="status" aria-label="Loading">
        {rows.map((_, i) => (
          <div
            key={i}
            className="h-4 bg-gray-200 rounded animate-pulse"
            style={{ width: `${[80, 65, 75, 50][i % 4]}%` }}
          />
        ))}
        <span className="sr-only">Loading...</span>
      </div>
    );
  }

  if (type === "card") {
    return (
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 py-4" role="status" aria-label="Loading">
        {rows.map((_, i) => (
          <div key={i} className="border border-gray-200 rounded-sm p-4 space-y-3">
            <div className="h-4 bg-gray-200 rounded animate-pulse w-3/4" />
            <div className="h-3 bg-gray-200 rounded animate-pulse w-1/2" />
            <div className="h-3 bg-gray-200 rounded animate-pulse w-full" />
          </div>
        ))}
        <span className="sr-only">Loading...</span>
      </div>
    );
  }

  // table skeleton (default)
  return (
    <div className="space-y-1 py-4" role="status" aria-label="Loading">
      {rows.map((_, i) => (
        <div key={i} className="flex gap-4 py-3 px-2">
          <div className="h-4 bg-gray-200 rounded animate-pulse w-1/6" />
          <div className="h-4 bg-gray-200 rounded animate-pulse w-1/4" />
          <div className="h-4 bg-gray-200 rounded animate-pulse w-1/5" />
          <div className="h-4 bg-gray-200 rounded animate-pulse w-1/6" />
          <div className="h-4 bg-gray-200 rounded animate-pulse w-1/6" />
        </div>
      ))}
      <span className="sr-only">Loading...</span>
    </div>
  );
}
