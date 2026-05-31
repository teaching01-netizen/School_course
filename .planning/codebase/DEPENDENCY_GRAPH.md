# Go Package Dependency Graph

**Analysis Date:** 2026-05-30

## Mermaid Graph

```mermaid
flowchart LR
  subgraph external["External Dependencies"]
    pgx["github.com/jackc/pgx/v5"]
    goose["github.com/pressly/goose/v3"]
    uuid["github.com/google/uuid"]
    xcrypto["golang.org/x/crypto"]
    xnet["golang.org/x/net"]
    xtext["golang.org/x/text"]
    xsync["golang.org/x/sync"]
    xtime["golang.org/x/time"]
    excelize["github.com/xuri/excelize/v2"]
  end

  subgraph cmd["cmd Layer"]
    main["cmd/server"]
  end

  subgraph infra["Infrastructure / Config"]
    config["internal/config"]
    logging["internal/logging"]
    pg["internal/pg"]
    db["internal/db (sqlc)"]
  end

  subgraph http["HTTP Layer"]
    httpapi["internal/httpapi"]
    httpdeps["internal/httpapi/httpdeps"]
    httpadapter["internal/httpapi/httpadapter"]
    absenceshttp["internal/httpapi/absenceshttp"]
    activecourseshttp["internal/httpapi/activecourseshttp"]
    adminusershttp["internal/httpapi/adminusershttp"]
    audithttp["internal/httpapi/audithttp"]
    availabilityhttp["internal/httpapi/availabilityhttp"]
    corehttp["internal/httpapi/corehttp"]
    courselevelshttp["internal/httpapi/courselevelshttp"]
    courseshttp["internal/httpapi/courseshttp"]
    crmhttp["internal/httpapi/crmhttp"]
    roomshttp["internal/httpapi/roomshttp"]
    schedulinghttp["internal/httpapi/schedulinghttp"]
    serieshttp["internal/httpapi/serieshttp"]
    sessionshttp["internal/httpapi/sessionshttp"]
    sitinruleshttp["internal/httpapi/sitinruleshttp"]
    staffabsencehttp["internal/httpapi/staffabsencehttp"]
    studentshttp["internal/httpapi/studentshttp"]
    subjectshttp["internal/httpapi/subjectshttp"]
    usershttp["internal/httpapi/usershttp"]
  end

  subgraph domain["Domain Services"]
    auth["internal/auth"]
    scheduling["internal/scheduling"]
    series["internal/series"]
    idempotency["internal/idempotency"]
    smartsms["internal/smartsms"]
    users["internal/users"]
    devseed["internal/devseed"]
  end

  subgraph crm["CRM Import"]
    crmimport["internal/crmimport"]
    crmqueue["internal/crmimport/queue"]
    crmreconcile["internal/crmimport/reconcile"]
    crmtypes["internal/crmimport/crmtypes"]
    crmxlsx["internal/crmimport/xlsx"]
  end

  %% cmd layer edges
  main --> config
  main --> crmimport
  main --> crmqueue
  main --> crmreconcile
  main --> devseed
  main --> httpapi
  main --> logging
  main --> pg
  main --> httpapi

  %% infra edges
  db --> pgx
  pg --> pgx

  %% domain service edges
  devseed --> auth
  users --> auth
  users --> db
  idempotency --> db
  series --> db
  series --> pgx
  scheduling --> series
  scheduling --> db
  scheduling --> pgx

  %% crm import edges
  crmimport --> crmxlsx
  crmimport --> crmqueue
  crmreconcile --> crmtypes
  crmreconcile --> crmqueue
  crmimport --> pgx

  %% httpapi edges
  httpapi --> config
  httpapi --> auth
  httpapi --> crmimport
  httpapi --> crmqueue
  httpapi --> crmreconcile
  httpapi --> db
  httpapi --> httpdeps
  httpapi --> scheduling
  httpapi --> series
  httpapi --> smartsms
  httpapi --> users

  httpdeps --> crmimport
  httpdeps --> crmqueue
  httpdeps --> crmreconcile
  httpdeps --> db
  httpdeps --> httpadapter
  httpdeps --> scheduling
  httpdeps --> smartsms
  httpdeps --> users

  httpadapter --> auth
  httpadapter --> db
  httpadapter --> idempotency
  httpadapter --> scheduling

  %% http subroute edges (all route packages use httpdeps + httpadapter)
  absenceshttp --> httpdeps
  absenceshttp --> httpadapter
  absenceshttp --> db
  activecourseshttp --> httpdeps
  activecourseshttp --> httpadapter
  adminusershttp --> httpdeps
  adminusershttp --> httpadapter
  adminusershttp --> db
  adminusershttp --> users
  audithttp --> httpdeps
  audithttp --> httpadapter
  availabilityhttp --> httpdeps
  availabilityhttp --> httpadapter
  corehttp --> httpdeps
  corehttp --> httpadapter
  corehttp --> auth
  courselevelshttp --> httpdeps
  courselevelshttp --> httpadapter
  courseshttp --> httpdeps
  courseshttp --> httpadapter
  courseshttp --> db
  courseshttp --> scheduling
  crmhttp --> httpdeps
  crmhttp --> httpadapter
  roomshttp --> httpdeps
  roomshttp --> httpadapter
  schedulinghttp --> httpdeps
  schedulinghttp --> httpadapter
  schedulinghttp --> scheduling
  serieshttp --> httpdeps
  serieshttp --> httpadapter
  serieshttp --> db
  serieshttp --> scheduling
  sessionshttp --> httpdeps
  sessionshttp --> httpadapter
  sitinruleshttp --> httpdeps
  sitinruleshttp --> httpadapter
  staffabsencehttp --> httpdeps
  staffabsencehttp --> httpadapter
  studentshttp --> httpdeps
  studentshttp --> httpadapter
  subjectshttp --> httpdeps
  subjectshttp --> httpadapter
  usershttp --> httpdeps
  usershttp --> httpadapter

  %% style by layer
  classDef cmd fill:#1a1a2e,color:#e0e0e0,stroke:#16213e
  classDef infra fill:#0f3460,color:#e0e0e0,stroke:#533483
  classDef http fill:#533483,color:#e0e0e0,stroke:#e94560
  classDef domain fill:#e94560,color:#fff,stroke:#0f3460
  classDef crm fill:#2d6a4f,color:#fff,stroke:#1b4332
  classDef ext fill:#495057,color:#e0e0e0,stroke:#6c757d

  class main cmd
  class config,logging,pg,db infra
  class httpapi,httpdeps,httpadapter,absenceshttp,activecourseshttp,adminusershttp,audithttp,availabilityhttp,corehttp,courselevelshttp,courseshttp,crmhttp,roomshttp,schedulinghttp,serieshttp,sessionshttp,sitinruleshttp,staffabsencehttp,studentshttp,subjectshttp,usershttp http
  class auth,scheduling,series,idempotency,smartsms,users,devseed domain
  class crmimport,crmqueue,crmreconcile,crmtypes,crmxlsx crm
  class pgx,goose,uuid,xcrypto,xnet,xtext,xsync,xtime,excelize ext
```

## Key Findings

### 1. Layering is clean top-to-bottom with one notable inversion
`cmd/server` → `httpapi` → domain services → `db` → pgx is the dominant flow. **Exception:** `users` package imports `auth` (`internal/users/password_hasher_auth.go:3`), which inverts the typical domain layering — `auth` should arguably depend on `users`, not the reverse. The `users.AdminProvisioningService` uses `auth.HashPassword` as a hashing utility, creating a coupling that would block extracting `users` as an independent module.

### 2. `scheduling` → `series` dependency respects the module boundary defined in CONTEXT.md
`scheduling` imports `series` as a dependency (via an interface `SeriesService` at `internal/scheduling/service.go:29`), but `series` never imports `scheduling`. This matches the documented contract: "scheduling owns all scheduling writes; series is an implementation detail behind scheduling" and is the **only** cross-module dependency between domain services outside the CRM subtree.

### 3. `crmimport` subtree is self-contained with sibling subpackage coupling
`crmimport` root imports `xlsx`; `reconcile` imports `crmtypes` + `queue`; `upload_v2` imports `queue` + `xlsx`. No CRM subpackage depends on any domain service outside its own subtree. However, `queue` and `reconcile` share a two-way awareness (reconcile imports queue types; queue does not import reconcile). The `crmimport` package is the only domain service directly constructed in `cmd/server` (not wired through `httpapi`), confirming its standalone-worker architecture.

### Key Source Files with Line Numbers

| File | Line | Purpose |
|------|------|---------|
| `internal/httpapi/handler.go` | 43–75 | Dependency injection: constructs `authSvc`, `seriesSvc`, `schedulingSvc`, `deps` struct |
| `internal/httpapi/handler.go` | 91–108 | Route registration: 18 `Register(mux, deps)` calls |
| `internal/httpapi/httpdeps/deps.go` | 21–35 | `Deps` struct — all injected dependencies in one type |
| `internal/httpapi/httpadapter/adapter.go` | 22–25 | HTTP adapter imports: auth, db, idempotency, scheduling |
| `internal/scheduling/service.go` | 17–18, 29 | Imports `series` via `SeriesService` interface |
| `internal/scheduling/service.go` | 59 | `scheduling.NewService(db, tz, seriesSvc)` — series injected |
| `internal/series/service.go` | 15 | Only internal dep is `db` |
| `internal/users/password_hasher_auth.go` | 3 | `users` → `auth` import inversion |
| `internal/users/store_sqlc.go` | 10 | `users` → `db` |
| `internal/crmimport/reconcile/reconcile.go` | 14–15 | `reconcile` imports `crmtypes` + `queue` |
| `internal/idempotency/idempotency.go` | 18 | `idempotency` imports `db` |
| `internal/devseed/admin.go` | 11 | `devseed` imports `auth` |
| `cmd/server/main.go` | 54–83 | CRM service construction + worker wiring |
| `cmd/server/main.go` | 87 | `httpapi.NewHandler(log, cfg, dbpool, uploadV2, reconcileV2, worker)` |
