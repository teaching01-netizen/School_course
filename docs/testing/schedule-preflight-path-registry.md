# Schedule preflight path registry

The executable registry is [`test-specs/schedule-preflight-paths.yaml`](../../test-specs/schedule-preflight-paths.yaml). It is intentionally JSON-compatible YAML so the validator can use the standard Node JSON parser without a dependency or fragile section parser.

`npm run test:schedule` executes every registered frontend test path. `npm run test:schedule:coverage` applies the dedicated Vitest configuration and enforces 100% statements, functions, branches, and lines for the shared scheduling/preflight modules listed in that configuration. `npm run check:scheduling-coverage` validates the registry and the generated JSON coverage summary.

Each test entry contains named test symbols. The registry checker verifies each symbol appears in its declared file. The backend conflict-value list is compared with every string value declared by `Err*` and `ConflictKind*` constants in `backend/internal/scheduling/errors.go`; frontend statuses are compared with `PreflightStatus` in `src/features/scheduling/hooks/usePreflight.ts`.
