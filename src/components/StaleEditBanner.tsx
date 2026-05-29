import Button from "./ui/Button";

interface StaleEditBannerProps {
  entityType: "session" | "series" | "absence";
  serverCopy: Record<string, unknown>;
  localCopy: Record<string, unknown>;
  fields: string[];
  onAcceptServer: () => void;
  onRetry: () => void;
  onCancel: () => void;
}

const entityLabels: Record<StaleEditBannerProps["entityType"], string> = {
  session: "Session",
  series: "Series",
  absence: "Absence",
};

export function StaleEditBanner({
  entityType,
  serverCopy,
  localCopy,
  fields,
  onAcceptServer,
  onRetry,
  onCancel,
}: StaleEditBannerProps) {
  return (
    <div role="alert" className="rounded border border-amber-300 bg-amber-50 p-4">
      <h2 className="mb-3 text-sm font-semibold text-amber-900">
        {entityLabels[entityType]} modified by another user
      </h2>

      <table className="mb-4 w-full text-sm">
        <thead>
          <tr className="border-b border-amber-200 text-left text-amber-800">
            <th className="py-1 pr-4">Field</th>
            <th className="py-1 pr-4">Your changes</th>
            <th className="py-1">Server version</th>
          </tr>
        </thead>
        <tbody>
          {fields.map((field) => {
            const localVal = String(localCopy[field] ?? "");
            const serverVal = String(serverCopy[field] ?? "");
            const changed = localVal !== serverVal;

            return (
              <tr
                key={field}
                className={changed ? "bg-red-50" : ""}
              >
                <td className="py-1 pr-4 font-medium">{field}</td>
                <td className="py-1 pr-4">{localVal}</td>
                <td className="py-1">{serverVal}</td>
              </tr>
            );
          })}
        </tbody>
      </table>

      <div className="flex gap-2">
        <Button variant="primary" size="sm" onClick={onAcceptServer}>
          Accept server version
        </Button>
        <Button variant="secondary" size="sm" onClick={onRetry}>
          Retry with my changes
        </Button>
        <Button variant="ghost" size="sm" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
