import { Component, type ErrorInfo, type ReactNode } from "react";

// Cross-browser message formats for a failed dynamic import / chunk load.
// After a deploy, users with the previous build still open request chunk
// hashes that no longer exist; the server 404s them (staticHandler) and the
// browser rejects the import with one of these messages.
const CHUNK_ERROR_PATTERNS: readonly RegExp[] = [
  /failed to fetch dynamically imported module/i,
  /error loading dynamically imported module/i,
  /importing a module script failed/i,
  /loading chunk \d+ failed/i,
  /loading css chunk \d+ failed/i,
];

export function isChunkLoadError(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error ?? "");
  return CHUNK_ERROR_PATTERNS.some((pattern) => pattern.test(message));
}

const RELOAD_GUARD_KEY = "wi.chunk-reload-at";
const RELOAD_COOLDOWN_MS = 30_000;

// index.html is served with no-cache, so a reload transparently picks up the
// new build's chunk hashes and re-renders the same route. The cooldown guard
// keeps a genuinely broken deploy from turning auto-recovery into a reload
// loop; when it trips (or storage is unavailable) the manual screen takes over.
export function reloadForNewBuild(): boolean {
  try {
    const last = Number(sessionStorage.getItem(RELOAD_GUARD_KEY) ?? 0);
    if (Number.isFinite(last) && Date.now() - last < RELOAD_COOLDOWN_MS) return false;
    sessionStorage.setItem(RELOAD_GUARD_KEY, String(Date.now()));
  } catch {
    return false;
  }
  window.location.reload();
  return true;
}

type ErrorBoundaryProps = {
  children: ReactNode;
  // Injectable so tests can observe recovery without navigating jsdom.
  recoverFromChunkError?: () => boolean;
};

type ErrorBoundaryState = { error: Error | null; recovering: boolean };

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null, recovering: false };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error, recovering: false };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("[ErrorBoundary] Unhandled UI error:", error, info.componentStack);
    if (isChunkLoadError(error)) {
      const recovering = (this.props.recoverFromChunkError ?? reloadForNewBuild)();
      if (recovering) this.setState({ recovering: true });
    }
  }

  render() {
    if (this.state.recovering) return <UpdatingScreen />;
    if (this.state.error) return <ErrorScreen error={this.state.error} />;
    return this.props.children;
  }
}

function UpdatingScreen() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-white" role="status">
      <span className="sr-only">A new version is available, reloading…</span>
      <div className="flex items-center gap-3 text-[13px] text-[var(--color-wi-text-light)]">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-[var(--color-wi-line)] border-t-[var(--color-wi-primary)]" />
        Updating… relaunching the app
      </div>
    </div>
  );
}

function ErrorScreen({ error }: { error: Error }) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-white px-6" role="alert">
      <div className="w-full max-w-sm space-y-4 text-center">
        <h1 className="text-lg font-semibold text-[var(--color-wi-text)]">Something went wrong</h1>
        <p className="text-[13px] text-[var(--color-wi-text-light)]">
          The page hit an unexpected error. Reloading usually fixes it.
        </p>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="inline-flex items-center justify-center rounded-md bg-[var(--color-wi-primary)] px-4 py-2 text-[13px] font-medium text-white hover:opacity-90"
        >
          Reload page
        </button>
        {import.meta.env.DEV ? (
          <pre className="overflow-x-auto rounded-md bg-[var(--color-wi-line)] p-3 text-left text-xs text-[var(--color-wi-text)]">
            {error.message}
          </pre>
        ) : null}
      </div>
    </div>
  );
}
