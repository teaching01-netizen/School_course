# CRM XLSX Upload + Course-Level "Excel Filters" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Accept user-uploaded XLSX exports via web UI, store as snapshots in Postgres, and let Admins configure an "Excel-like" saved filter per course that authoritatively drives the course roster (add/remove/upsert students) with guardrails.

**Architecture:** A background job (DB-tracked + single-flight) parses the uploaded XLSX, atomically replaces all `crm_rows`, populates `crm_cycles`, then reconciles all unlocked courses with CRM filters enabled. Admin UI provides file upload and course-level filter configuration; course rosters become read-only while CRM filter is enabled.

**Tech Stack:** Go 1.25 (net/http, pgx, sqlc, goose), Postgres, React + Vite + Tailwind, server-driven polling UI.

**Design pivot (from original spec):** This plan replaces the Sage CRM HTTP scraping + scheduler approach with a user-upload XLSX approach. Key differences:
- ❌ No Sage CRM HTTP client (login, form scraping, download)
- ❌ No scheduled imports (07:00/15:00/22:00 removed)
- ❌ No `crm_import_jobs` single-flight lock tied to CRM sessions
- ✅ Upload endpoint + async job + polling replaces "Import from CRM now"
- ✅ Per-course roster lock toggle (prevents auto-reconcile) replaces cycle-level freeze
- ✅ `crm_cycles` simplified to filter dropdown source (populated from uploads)

---

## Preconditions / Local Dev

- Backend env (required for server): `DATABASE_URL`, `AUTH_PEPPER`
- No CRM env vars needed (no scraper)

Local dev run:

- `docker compose up -d db`
- `make -C backend migrate-up`
- `make -C backend dev`
- SPA: `npm run dev` (or `npm run dev:full` if repo supports it)

---

## File Map (what to create/modify)

**Backend DB**
- Create: `backend/db/migrations/00011_crm_import.sql`
- Create: `backend/db/queries/crm_cycles.sql`
- Create: `backend/db/queries/crm_rows.sql`
- Modify: `backend/db/queries/students.sql` (add upsert)

**Backend Go**
- Modify: `backend/internal/config/config.go` (optional — may add upload size limit)
- Create: `backend/internal/crmimport/types.go` (row structs + filter schema + validation)
- Create: `backend/internal/crmimport/xlsx_parse.go` (parse uploaded XLSX into rows)
- Create: `backend/internal/crmimport/import_service.go` (atomic replace + reconcile trigger)
- Create: `backend/internal/crmimport/reconcile_service.go` (authoritative roster reconcile + guardrails)
- Create: `backend/internal/crmimport/upload_service.go` (single-flight upload jobs + status)
- Create: `backend/internal/crmimport/*_test.go` (unit tests)

**Backend HTTP API**
- Modify: `backend/internal/httpapi/httpdeps/deps.go` (add upload service dependency)
- Modify: `backend/internal/httpapi/handler.go` (register crm routes)
- Create: `backend/internal/httpapi/crmhttp/routes.go` (upload + cycles + course filter endpoints)
- Modify: `backend/internal/httpapi/courseshttp/routes.go` (block manual roster edits when CRM-managed)

**Frontend**
- Create: `src/pages/CrmAdmin.tsx` (Admin-only upload page + job status)
- Modify: `src/App.tsx` (route)
- Modify: `src/components/Layout.tsx` (Admin-only nav item)
- Create: `src/components/crm/CrmFilterPanel.tsx` (course detail filter panel + lock toggle)
- Modify: `src/pages/CourseDetail.tsx` (mount panel + roster read-only UX)
- Modify: `src/types/index.ts` (types for CRM filter + endpoints if you keep centralized types)

**Docs**
- This plan file: updated to reflect upload approach

---

### Task 1: Add DB Schema (crm_rows, crm_cycles, course filter fields + lock)

**Files:**
- Create: `backend/db/migrations/00011_crm_import.sql`

- [ ] **Step 1: Create migration**

```sql
-- +goose Up

-- Cycles config: populated from distinct cycle_label values in uploaded data.
-- Provides dropdown options for course filter.
CREATE TABLE IF NOT EXISTS crm_cycles (
  id               text PRIMARY KEY,
  label            text NOT NULL,
  last_imported_at timestamptz NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

-- Imported CRM rows (single authoritative snapshot; replaced atomically on each upload).
CREATE TABLE IF NOT EXISTS crm_rows (
  row_hash             text NOT NULL,

  cycle_label          text NOT NULL,
  course_name          text NOT NULL,
  wcode                text NOT NULL,

  first_name           text NULL,
  last_name            text NULL,
  nickname             text NULL,
  secondary_school     text NULL,
  academic_level       text NULL,
  mobile_phone         text NULL,
  hours                integer NULL CHECK (hours IS NULL OR hours >= 0),
  teachers_raw         text NULL,
  primary_email        text NULL,
  parent_name          text NULL,
  parent_phone         text NULL,
  parent_email         text NULL,
  order_quote_updated_at timestamptz NULL,

  imported_at          timestamptz NOT NULL DEFAULT now(),

  PRIMARY KEY (row_hash)
);

CREATE INDEX IF NOT EXISTS crm_rows_course_name_idx ON crm_rows(course_name);
CREATE INDEX IF NOT EXISTS crm_rows_wcode_idx ON crm_rows(wcode);
CREATE INDEX IF NOT EXISTS crm_rows_cycle_label_idx ON crm_rows(cycle_label);

-- Course-level CRM filter definition + flags + lock.
ALTER TABLE courses
  ADD COLUMN IF NOT EXISTS crm_filter_enabled boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS crm_filter jsonb NULL,
  ADD COLUMN IF NOT EXISTS crm_filter_updated_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS crm_roster_locked boolean NOT NULL DEFAULT false;

-- +goose Down

ALTER TABLE courses
  DROP COLUMN IF EXISTS crm_roster_locked,
  DROP COLUMN IF EXISTS crm_filter_updated_at,
  DROP COLUMN IF EXISTS crm_filter,
  DROP COLUMN IF EXISTS crm_filter_enabled;

DROP TABLE IF EXISTS crm_rows;
DROP TABLE IF EXISTS crm_cycles;
```

- [ ] **Step 2: Run migrations locally**

Run: `make -C backend migrate-up`
Expected: exits 0, adds `00011_crm_import.sql` to schema.

---

### Task 2: Add SQLC Queries for CRM + Student Upsert

**Files:**
- Create: `backend/db/queries/crm_cycles.sql`
- Create: `backend/db/queries/crm_rows.sql`
- Modify: `backend/db/queries/students.sql`

- [ ] **Step 1: Add `crm_cycles` queries**

```sql
-- name: CrmCycleUpsert :one
INSERT INTO crm_cycles (id, label)
VALUES ($1, $2)
ON CONFLICT (id) DO UPDATE
SET label = EXCLUDED.label,
    updated_at = now()
RETURNING id, label, last_imported_at, created_at, updated_at;

-- name: CrmCyclesList :many
SELECT id, label, last_imported_at, created_at, updated_at
FROM crm_cycles
ORDER BY label ASC;

-- name: CrmCyclesUpsertFromUpload :many
-- Upsert all distinct cycle values from uploaded rows; return all cycles.
WITH distinct_cycles AS (
  SELECT DISTINCT cycle_label AS label FROM crm_rows
),
upserted AS (
  INSERT INTO crm_cycles (id, label)
  SELECT label, label FROM distinct_cycles
  ON CONFLICT (id) DO UPDATE
  SET label = EXCLUDED.label,
      updated_at = now()
  RETURNING id
)
UPDATE crm_cycles SET last_imported_at = now()
WHERE id IN (SELECT id FROM upserted);

-- name: CrmCyclesDeleteUnused :exec
DELETE FROM crm_cycles
WHERE id NOT IN (SELECT DISTINCT cycle_label FROM crm_rows);
```

- [ ] **Step 2: Add `crm_rows` queries**

```sql
-- name: CrmRowsDeleteAll :exec
DELETE FROM crm_rows;

-- name: CrmRowsInsert :one
INSERT INTO crm_rows (
  row_hash, cycle_label, course_name, wcode,
  first_name, last_name, nickname, secondary_school, academic_level,
  mobile_phone, hours, teachers_raw, primary_email,
  parent_name, parent_phone, parent_email, order_quote_updated_at
) VALUES (
  $1,$2,$3,$4,
  NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),
  NULLIF($10,''),$11,NULLIF($12,''),NULLIF($13,''),
  NULLIF($14,''),NULLIF($15,''),NULLIF($16,''),$17
) RETURNING row_hash;

-- name: CrmRowsCount :one
SELECT COUNT(*)::bigint FROM crm_rows;

-- name: CrmDistinctOptions :one
SELECT
  (SELECT jsonb_agg(DISTINCT cycle_label ORDER BY cycle_label) FROM crm_rows) AS cycle_labels,
  (SELECT jsonb_agg(DISTINCT course_name ORDER BY course_name) FROM crm_rows) AS course_names,
  (SELECT jsonb_agg(DISTINCT academic_level ORDER BY academic_level) FROM crm_rows WHERE academic_level IS NOT NULL AND btrim(academic_level) <> '') AS academic_levels,
  (SELECT jsonb_agg(DISTINCT secondary_school ORDER BY secondary_school) FROM crm_rows WHERE secondary_school IS NOT NULL AND btrim(secondary_school) <> '') AS secondary_schools;
```

- [ ] **Step 3: Add student upsert-by-wcode (do not overwrite notes)**

Append to `backend/db/queries/students.sql`:

```sql
-- name: StudentUpsertNameByWCode :one
INSERT INTO students (wcode, full_name, notes)
VALUES ($1, $2, '')
ON CONFLICT (wcode) DO UPDATE
SET full_name = EXCLUDED.full_name,
    updated_at = now()
RETURNING id, wcode, full_name, notes, created_at, updated_at;
```

- [ ] **Step 4: Generate sqlc + gofmt**

Run: `make -C backend sqlc && make -C backend fmt`
Expected: sqlc exits 0; Go formats cleanly.

---

### Task 3: Add CRM Import Types (row model + filter schema + validation)

**Files:**
- Create: `backend/internal/crmimport/types.go`

- [ ] **Step 1: Create types + filter validation**

```go
package crmimport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type BlankMode string

const (
	BlankModeAny       BlankMode = "any"
	BlankModeOnlyBlank BlankMode = "only_blank"
	BlankModeOnlyValue BlankMode = "only_value"
)

func (m BlankMode) Valid() bool {
	return m == BlankModeAny || m == BlankModeOnlyBlank || m == BlankModeOnlyValue
}

type UploadJobStatus string

const (
	UploadJobRunning   UploadJobStatus = "Running"
	UploadJobSucceeded UploadJobStatus = "Succeeded"
	UploadJobFailed    UploadJobStatus = "Failed"
)

// CourseFilter is stored on courses.crm_filter as JSON.
// v1 supports:
// - multi-select equals for cycle_label/course_name/academic_level/secondary_school
// - contains for teachers_raw
// - blank mode toggles per supported column
type CourseFilter struct {
	CycleLabels            []string  `json:"cycle_labels"`
	CycleBlankMode         BlankMode `json:"cycle_blank_mode"`
	CourseNameValues       []string  `json:"course_name_values"`
	CourseNameBlankMode    BlankMode `json:"course_name_blank_mode"`
	AcademicLevelValues    []string  `json:"academic_level_values"`
	AcademicLevelBlankMode BlankMode `json:"academic_level_blank_mode"`
	SecondarySchoolValues  []string  `json:"secondary_school_values"`
	SecondarySchoolBlankMode BlankMode `json:"secondary_school_blank_mode"`
	TeachersContains       string    `json:"teachers_contains"`
	TeachersBlankMode      BlankMode `json:"teachers_blank_mode"`
}

func (f *CourseFilter) Normalize() {
	trimList := func(in []string) []string {
		out := make([]string, 0, len(in))
		seen := map[string]struct{}{}
		for _, v := range in {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
		sort.Strings(out)
		return out
	}
	f.CycleLabels = trimList(f.CycleLabels)
	f.CourseNameValues = trimList(f.CourseNameValues)
	f.AcademicLevelValues = trimList(f.AcademicLevelValues)
	f.SecondarySchoolValues = trimList(f.SecondarySchoolValues)
	f.TeachersContains = strings.TrimSpace(f.TeachersContains)
	if f.CycleBlankMode == "" {
		f.CycleBlankMode = BlankModeAny
	}
	if f.CourseNameBlankMode == "" {
		f.CourseNameBlankMode = BlankModeAny
	}
	if f.AcademicLevelBlankMode == "" {
		f.AcademicLevelBlankMode = BlankModeAny
	}
	if f.SecondarySchoolBlankMode == "" {
		f.SecondarySchoolBlankMode = BlankModeAny
	}
	if f.TeachersBlankMode == "" {
		f.TeachersBlankMode = BlankModeAny
	}
}

func (f CourseFilter) Validate() error {
	if !f.CycleBlankMode.Valid() || !f.CourseNameBlankMode.Valid() || !f.AcademicLevelBlankMode.Valid() || !f.SecondarySchoolBlankMode.Valid() || !f.TeachersBlankMode.Valid() {
		return fmt.Errorf("invalid blank mode")
	}
	return nil
}

type Row struct {
	CycleLabel string
	CourseName string
	WCode      string

	FirstName       string
	LastName        string
	Nickname        string
	SecondarySchool string
	AcademicLevel   string
	MobilePhone     string
	Hours           *int32
	TeachersRaw     string
	PrimaryEmail    string
	ParentName      string
	ParentPhone     string
	ParentEmail     string
	OrderQuoteUpdatedAt *time.Time
}

func (r Row) Hash() string {
	parts := []string{
		strings.TrimSpace(r.WCode),
		strings.TrimSpace(r.CourseName),
		strings.TrimSpace(r.CycleLabel),
		strings.TrimSpace(r.TeachersRaw),
	}
	if r.Hours != nil {
		parts = append(parts, fmt.Sprintf("hours=%d", *r.Hours))
	}
	if r.OrderQuoteUpdatedAt != nil {
		parts = append(parts, "updated_at="+r.OrderQuoteUpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:])
}

type UploadResult struct {
	RowsParsed       int            `json:"rows_parsed"`
	RowsStored       int            `json:"rows_stored"`
	CyclesFound      []string       `json:"cycles_found"`
	CoursesReconciled int           `json:"courses_reconciled"`
	StudentsAdded    int            `json:"students_added"`
	StudentsRemoved  int            `json:"students_removed"`
}
```

- [ ] **Step 2: Add a tiny unit test for filter normalization**

Create `backend/internal/crmimport/types_test.go`:

```go
package crmimport

import "testing"

func TestCourseFilterNormalizeAndValidate(t *testing.T) {
	f := CourseFilter{
		CycleLabels: []string{" May-Aug 2026 ", "May-Aug 2026"},
	}
	f.Normalize()
	if err := f.Validate(); err != nil {
		t.Fatalf("expected valid filter, got %v", err)
	}
	if len(f.CycleLabels) != 1 || f.CycleLabels[0] != "May-Aug 2026" {
		t.Fatalf("expected deduped cycle labels, got %#v", f.CycleLabels)
	}
}
```

- [ ] **Step 3: Run backend tests**

Run: `make -C backend test`
Expected: PASS (DB integration tests may skip unless `TEST_DATABASE_URL` is set).

---

### Task 4: Implement XLSX Parsing (header discovery + row extraction)

**Files:**
- Create: `backend/internal/crmimport/xlsx_parse.go`
- Test: `backend/internal/crmimport/xlsx_parse_test.go`

- [ ] **Step 1: Add XLSX parser using `excelize`**

Add dependency:

Run: `cd backend && go get github.com/xuri/excelize/v2@v2.9.0`
Expected: `go.mod` + `go.sum` updated.

Create `backend/internal/crmimport/xlsx_parse.go`:

```go
package crmimport

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type ParsedXLSX struct {
	Rows []Row
}

var requiredHeaders = []string{
	"Student Id",
	"First Name",
	"Last Name",
	"Course Name",
	"Cycle",
}

func ParseXLSX(xlsxBytes []byte, instituteLoc *time.Location) (ParsedXLSX, error) {
	f, err := excelize.OpenReader(bytes.NewReader(xlsxBytes))
	if err != nil {
		return ParsedXLSX{}, fmt.Errorf("open xlsx: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return ParsedXLSX{}, fmt.Errorf("xlsx has no sheets")
	}
	sheet := sheets[0]

	// Scan first N rows to find header row containing required headers.
	const scanRows = 20
	headerRowIdx := -1
	var headerCells []string
	for i := 1; i <= scanRows; i++ {
		row, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
		if err != nil {
			return ParsedXLSX{}, fmt.Errorf("read rows: %w", err)
		}
		if i-1 >= len(row) {
			break
		}
		cells := row[i-1]
		if looksLikeHeaderRow(cells) {
			headerRowIdx = i
			headerCells = cells
			break
		}
	}
	if headerRowIdx == -1 {
		return ParsedXLSX{}, fmt.Errorf("header row not found in first %d rows", scanRows)
	}

	colByHeader := map[string]int{}
	for idx, h := range headerCells {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		colByHeader[h] = idx
	}
	for _, rh := range requiredHeaders {
		if _, ok := colByHeader[rh]; !ok {
			return ParsedXLSX{}, fmt.Errorf("missing required header %q", rh)
		}
	}

	allRows, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return ParsedXLSX{}, fmt.Errorf("read rows: %w", err)
	}

	var out []Row
	for i := headerRowIdx + 1; i <= len(allRows); i++ {
		cells := allRows[i-1]
		get := func(header string) string {
			idx := colByHeader[header]
			if idx < 0 || idx >= len(cells) {
				return ""
			}
			return strings.TrimSpace(cells[idx])
		}

		wcode := get("Student Id")
		courseName := get("Course Name")
		cycleLabel := get("Cycle")
		if wcode == "" && courseName == "" && cycleLabel == "" {
			continue
		}
		if wcode == "" || courseName == "" || cycleLabel == "" {
			continue
		}

		var hoursPtr *int32
		if s := get("Hours"); s != "" {
			if n, err := strconv.Atoi(strings.ReplaceAll(s, ",", "")); err == nil && n >= 0 {
				n32 := int32(n)
				hoursPtr = &n32
			}
		}

		var updatedAtPtr *time.Time
		if s := get("Order Quote Updated Date"); s != "" {
			if t, err := time.ParseInLocation("02/01/2006 03:04 PM", s, instituteLoc); err == nil {
				u := t.UTC()
				updatedAtPtr = &u
			}
		}

		r := Row{
			CycleLabel: cycleLabel,
			CourseName: courseName,
			WCode:      wcode,

			FirstName:       get("First Name"),
			LastName:        get("Last Name"),
			Nickname:        get("Nickname"),
			SecondarySchool: get("Secondary School"),
			AcademicLevel:   get("Academic level"),
			MobilePhone:     get("Mobile Phone"),
			Hours:           hoursPtr,
			TeachersRaw:     get("Teacher(s)"),
			PrimaryEmail:    get("Primary E-mail"),
			ParentName:      get("Parent Name"),
			ParentPhone:     get("phoneparent"),
			ParentEmail:     get("emailparent"),
			OrderQuoteUpdatedAt: updatedAtPtr,
		}
		out = append(out, r)
	}

	if len(out) == 0 {
		return ParsedXLSX{}, fmt.Errorf("xlsx contained 0 usable data rows")
	}
	return ParsedXLSX{Rows: out}, nil
}

func looksLikeHeaderRow(cells []string) bool {
	need := map[string]bool{"Student Id": false, "Course Name": false, "Cycle": false}
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if _, ok := need[c]; ok {
			need[c] = true
		}
	}
	for _, ok := range need {
		if !ok {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Unit test using generated XLSX in-memory**

Create `backend/internal/crmimport/xlsx_parse_test.go`:

```go
package crmimport

import (
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestParseXLSX_HeaderDiscoveryAndRows(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Bangkok")
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)

	// Title rows.
	_ = f.SetCellValue(sheet, "A1", "Total Students Each Cycle (New 2021)")
	_ = f.SetCellValue(sheet, "A2", "metadata")

	// Header row at row 4.
	headers := []string{
		"Order Quote Updated Date", "Student Id", "First Name", "Last Name", "Nickname", "Secondary School", "Academic level",
		"Mobile Phone", "Course Name", "Cycle", "Hours", "Teacher(s)", "Primary E-mail", "Parent Name", "phoneparent", "emailparent",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		_ = f.SetCellValue(sheet, cell, h)
	}
	// One data row.
	row := []any{"22/02/2026 01:58 PM", "W250084", "Jitirada", "Limsitisarn", "Jib", ".Satit Chula", "M.4", "092-6766560", "SAT Verbal", "May-Aug 2026", 56, "Nice", "a@b.com", "Parent", "081", "p@x.com"}
	for i, v := range row {
		cell, _ := excelize.CoordinatesToCellName(i+1, 5)
		_ = f.SetCellValue(sheet, cell, v)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	parsed, err := ParseXLSX(buf.Bytes(), loc)
	if err != nil {
		t.Fatalf("ParseXLSX: %v", err)
	}
	if len(parsed.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(parsed.Rows))
	}
	if parsed.Rows[0].WCode != "W250084" {
		t.Fatalf("unexpected wcode: %q", parsed.Rows[0].WCode)
	}
}

func TestParseXLSX_RejectsMissingHeaders(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	// Only partial headers.
	_ = f.SetCellValue(sheet, "A1", "Student Id")
	_ = f.SetCellValue(sheet, "B1", "Course Name")
	// Missing "Cycle"
	buf, _ := f.WriteToBuffer()
	_, err := ParseXLSX(buf.Bytes(), time.UTC)
	if err == nil {
		t.Fatal("expected error for missing headers")
	}
}

func TestParseXLSX_EmptyFile(t *testing.T) {
	f := excelize.NewFile()
	buf, _ := f.WriteToBuffer()
	_, err := ParseXLSX(buf.Bytes(), time.UTC)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}
```

- [ ] **Step 3: Run backend tests**

Run: `make -C backend test`
Expected: PASS.

---

### Task 5: Implement Atomic Import (replace all rows + upsert cycles)

**Files:**
- Create: `backend/internal/crmimport/import_service.go`
- Test: `backend/internal/crmimport/import_service_test.go` (DB integration, optional)

- [ ] **Step 1: Implement importer**

```go
package crmimport

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
)

type ImportService struct {
	db           *pgxpool.Pool
	q            *sqldb.Queries
	instituteLoc *time.Location
}

func NewImportService(db *pgxpool.Pool, instituteTZ string) (*ImportService, error) {
	loc, err := time.LoadLocation(instituteTZ)
	if err != nil {
		return nil, err
	}
	return &ImportService{db: db, q: sqldb.New(db), instituteLoc: loc}, nil
}

func (s *ImportService) ImportUpload(ctx context.Context, rows []Row) (UploadResult, error) {
	if len(rows) == 0 {
		return UploadResult{}, fmt.Errorf("0 rows to import")
	}

	// Deduplicate within this upload.
	seen := map[string]struct{}{}
	deduped := make([]Row, 0, len(rows))
	for _, r := range rows {
		h := r.Hash()
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		deduped = append(deduped, r)
	}

	cycleSet := map[string]struct{}{}
	for _, r := range deduped {
		cycleSet[r.CycleLabel] = struct{}{}
	}
	cycleLabels := make([]string, 0, len(cycleSet))
	for c := range cycleSet {
		cycleLabels = append(cycleLabels, c)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	// Atomically replace all rows.
	if err := qtx.CrmRowsDeleteAll(ctx); err != nil {
		return UploadResult{}, err
	}

	for _, r := range deduped {
		var hours pgtype.Int4
		if r.Hours != nil {
			hours = pgtype.Int4{Int32: *r.Hours, Valid: true}
		}
		var updated pgtype.Timestamptz
		if r.OrderQuoteUpdatedAt != nil {
			updated = pgtype.Timestamptz{Time: r.OrderQuoteUpdatedAt.UTC(), Valid: true}
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO crm_rows (
				row_hash, cycle_label, course_name, wcode,
				first_name, last_name, nickname, secondary_school, academic_level,
				mobile_phone, hours, teachers_raw, primary_email,
				parent_name, parent_phone, parent_email, order_quote_updated_at
			) VALUES (
				$1,$2,$3,$4,
				NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),
				NULLIF($10,''),$11,NULLIF($12,''),NULLIF($13,''),
				NULLIF($14,''),NULLIF($15,''),NULLIF($16,''),$17
			)
		`,
			r.Hash(), r.CycleLabel, r.CourseName, r.WCode,
			r.FirstName, r.LastName, r.Nickname, r.SecondarySchool, r.AcademicLevel,
			r.MobilePhone, hours, r.TeachersRaw, r.PrimaryEmail,
			r.ParentName, r.ParentPhone, r.ParentEmail, updated,
		); err != nil {
			return UploadResult{}, fmt.Errorf("insert row: %w", err)
		}
	}

	// Upsert crm_cycles from distinct labels.
	if _, err := qtx.CrmCyclesUpsertFromUpload(ctx); err != nil {
		return UploadResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return UploadResult{}, err
	}

	return UploadResult{
		RowsParsed:  len(rows),
		RowsStored:  len(deduped),
		CyclesFound: cycleLabels,
	}, nil
}
```

- [ ] **Step 2: (Optional) DB integration test for "atomic replace"**

Create `backend/internal/crmimport/import_service_test.go` using `TEST_DATABASE_URL`. Insert a row, call ImportUpload, verify old rows gone and new rows present.

- [ ] **Step 3: Run backend tests**

Run: `make -C backend test`
Expected: PASS (integration may skip).

---

### Task 6: Implement Roster Reconcile (authoritative + guardrails + audit)

**Files:**
- Create: `backend/internal/crmimport/reconcile_service.go`
- Test: `backend/internal/crmimport/reconcile_service_integration_test.go` (DB integration)
- Modify: `backend/internal/httpapi/courseshttp/routes.go` (block manual edits)

- [ ] **Step 1: Implement reconcile service**

```go
package crmimport

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
)

type ReconcileService struct {
	db *pgxpool.Pool
	q  *sqldb.Queries
}

func NewReconcileService(db *pgxpool.Pool) *ReconcileService {
	return &ReconcileService{db: db, q: sqldb.New(db)}
}

type ReconcileResult struct {
	DesiredStudents int `json:"desired_students"`
	Added           int `json:"added"`
	Removed         int `json:"removed"`
}

type reconcileStudent struct {
	WCode     string
	FirstName string
	LastName  string
}

func (s *ReconcileService) ReconcileCourse(ctx context.Context, courseID pgtype.UUID, filter CourseFilter) (ReconcileResult, error) {
	filter.Normalize()
	if err := filter.Validate(); err != nil {
		return ReconcileResult{}, err
	}

	desired, err := s.queryDesiredStudents(ctx, filter)
	if err != nil {
		return ReconcileResult{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	current, err := qtx.CourseStudentsList(ctx, courseID)
	if err != nil {
		return ReconcileResult{}, err
	}
	currentSet := map[uuid.UUID]struct{}{}
	for _, it := range current {
		id, _ := uuid.FromBytes(it.StudentID.Bytes[:])
		currentSet[id] = struct{}{}
	}

	// Upsert students + build desired student_id set.
	desiredIDs := map[uuid.UUID]pgtype.UUID{}
	for _, d := range desired {
		fullName := strings.TrimSpace(strings.TrimSpace(d.FirstName) + " " + strings.TrimSpace(d.LastName))
		if fullName == "" {
			fullName = d.WCode
		}
		st, err := qtx.StudentUpsertNameByWCode(ctx, sqldb.StudentUpsertNameByWCodeParams{
			Wcode:    d.WCode,
			FullName: fullName,
		})
		if err != nil {
			return ReconcileResult{}, err
		}
		id, _ := uuid.FromBytes(st.ID.Bytes[:])
		desiredIDs[id] = st.ID
	}

	added := 0
	removed := 0

	for id, pgid := range desiredIDs {
		if _, ok := currentSet[id]; ok {
			continue
		}
		if err := qtx.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: courseID, StudentID: pgid}); err != nil {
			return ReconcileResult{}, err
		}
		added++
	}

	for _, it := range current {
		id, _ := uuid.FromBytes(it.StudentID.Bytes[:])
		if _, ok := desiredIDs[id]; ok {
			continue
		}
		if err := qtx.CourseStudentRemove(ctx, sqldb.CourseStudentRemoveParams{CourseID: courseID, StudentID: it.StudentID}); err != nil {
			return ReconcileResult{}, err
		}
		removed++
	}

	if err := tx.Commit(ctx); err != nil {
		return ReconcileResult{}, err
	}
	return ReconcileResult{DesiredStudents: len(desired), Added: added, Removed: removed}, nil
}

func (s *ReconcileService) queryDesiredStudents(ctx context.Context, filter CourseFilter) ([]reconcileStudent, error) {
	conds := []string{"1=1"}
	args := []any{}
	argN := 1
	addIn := func(col string, values []string) {
		if len(values) == 0 {
			return
		}
		conds = append(conds, fmt.Sprintf("%s = ANY($%d)", col, argN))
		args = append(args, values)
		argN++
	}
	addBlankMode := func(col string, mode BlankMode) {
		switch mode {
		case BlankModeAny:
		case BlankModeOnlyBlank:
			conds = append(conds, fmt.Sprintf("(%s IS NULL OR btrim(%s) = '')", col, col))
		case BlankModeOnlyValue:
			conds = append(conds, fmt.Sprintf("(%s IS NOT NULL AND btrim(%s) <> '')", col, col))
		}
	}

	addIn("cycle_label", filter.CycleLabels)
	addBlankMode("cycle_label", filter.CycleBlankMode)
	addIn("course_name", filter.CourseNameValues)
	addBlankMode("course_name", filter.CourseNameBlankMode)
	addIn("academic_level", filter.AcademicLevelValues)
	addBlankMode("academic_level", filter.AcademicLevelBlankMode)
	addIn("secondary_school", filter.SecondarySchoolValues)
	addBlankMode("secondary_school", filter.SecondarySchoolBlankMode)

	if filter.TeachersContains != "" {
		conds = append(conds, fmt.Sprintf("teachers_raw ILIKE $%d", argN))
		args = append(args, "%"+filter.TeachersContains+"%")
		argN++
	}
	addBlankMode("teachers_raw", filter.TeachersBlankMode)

	sql := `
		SELECT DISTINCT ON (wcode)
			wcode,
			COALESCE(first_name, '') AS first_name,
			COALESCE(last_name, '') AS last_name
		FROM crm_rows
		WHERE ` + strings.Join(conds, " AND ") + `
		ORDER BY wcode, order_quote_updated_at DESC NULLS LAST
	`

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []reconcileStudent
	for rows.Next() {
		var r reconcileStudent
		if err := rows.Scan(&r.WCode, &r.FirstName, &r.LastName); err != nil {
			return nil, err
		}
		if strings.TrimSpace(r.WCode) == "" {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReconcileAllUnlockedCourses reconciles all courses with CRM filter enabled and not locked.
func (s *ReconcileService) ReconcileAllUnlockedCourses(ctx context.Context) (totalAdded, totalRemoved int, reconciledCount int, _ error) {
	rows, err := s.db.Query(ctx, `SELECT id, crm_filter FROM courses WHERE crm_filter_enabled = true AND crm_roster_locked = false AND deleted_at IS NULL`)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid pgtype.UUID
		var filterJSON []byte
		if err := rows.Scan(&cid, &filterJSON); err != nil {
			return 0, 0, 0, err
		}
		var f CourseFilter
		if err := json.Unmarshal(filterJSON, &f); err != nil {
			return 0, 0, 0, fmt.Errorf("bad course filter json: %w", err)
		}
		res, err := s.ReconcileCourse(ctx, cid, f)
		if err != nil {
			return 0, 0, 0, err
		}
		totalAdded += res.Added
		totalRemoved += res.Removed
		reconciledCount++
	}
	return totalAdded, totalRemoved, reconciledCount, rows.Err()
}
```

- [ ] **Step 2: Enforce "CRM-managed rosters are read-only" in course roster endpoints**

Modify `backend/internal/httpapi/courseshttp/routes.go` in `handleCourseStudentsAdd` and `handleCourseStudentsRemove`:

```go
var enabled bool
err := s.deps.DB.QueryRow(r.Context(), `SELECT crm_filter_enabled FROM courses WHERE id = $1`, courseID).Scan(&enabled)
if err != nil {
  s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
  return
}
if enabled {
  s.a.WriteErr(w, http.StatusConflict, "crm_managed_roster", "Roster is managed by CRM filter. Disable CRM filter to edit manually.")
  return
}
```

- [ ] **Step 3: DB integration test (recommended)**

Create `backend/internal/crmimport/reconcile_service_integration_test.go` using `TEST_DATABASE_URL`:
- Create a course with filter disabled
- Insert 2 students into roster manually
- Enable CRM filter with filter matching 1 student
- Call `ReconcileCourse` and assert removed=1, added=0

- [ ] **Step 4: Run backend tests**

Run: `make -C backend test`
Expected: PASS (integration may skip).

---

### Task 7: Implement Upload Job Service (single-flight + status tracking)

**Files:**
- Create: `backend/internal/crmimport/upload_service.go`

- [ ] **Step 1: Implement upload service with in-memory job tracking**

Since we removed the `crm_import_jobs` table, we track the upload job in-memory with a simple mutex-protected state. A lightweight approach for single-flight.

```go
package crmimport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UploadJobState struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Result    *UploadResult `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

type UploadService struct {
	log        *slog.Logger
	db         *pgxpool.Pool
	importSvc  *ImportService
	reconcileSvc *ReconcileService

	mu     sync.Mutex
	job    *UploadJobState
}

func NewUploadService(log *slog.Logger, db *pgxpool.Pool, importSvc *ImportService, reconcileSvc *ReconcileService) *UploadService {
	return &UploadService{
		log:          log,
		db:           db,
		importSvc:    importSvc,
		reconcileSvc: reconcileSvc,
	}
}

func (s *UploadService) StartUpload(ctx context.Context, file multipart.File, filename string, filesize int64) (*UploadJobState, error) {
	s.mu.Lock()
	if s.job != nil && s.job.Status == "Running" {
		s.mu.Unlock()
		return nil, fmt.Errorf("upload already in progress")
	}
	job := &UploadJobState{
		ID:        fmt.Sprintf("upload-%d", time.Now().UnixNano()),
		Status:    "Running",
		Message:   "Starting upload…",
		StartedAt: time.Now(),
	}
	s.job = job
	s.mu.Unlock()

	go s.runUpload(ctx, file, filename, job)
	return job, nil
}

func (s *UploadService) GetJob() *UploadJobState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil {
		return nil
	}
	cp := *s.job
	return &cp
}

func (s *UploadService) runUpload(ctx context.Context, file multipart.File, filename string, job *UploadJobState) {
	setMsg := func(msg string) {
		s.mu.Lock()
		job.Message = msg
		s.mu.Unlock()
	}

	// Read the file (limited to maxUploadSize — e.g., 50MB).
	const maxUploadSize = 50 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(file, maxUploadSize))
	if err != nil {
		s.failJob(job, fmt.Sprintf("read file: %v", err))
		return
	}
	_ = file.Close()

	// Validate XLSX magic bytes.
	if len(data) < 4 || string(data[:2]) != "PK" {
		s.failJob(job, "file is not a valid XLSX (bad signature)")
		return
	}

	setMsg("Parsing XLSX…")

	loc, _ := time.LoadLocation("Asia/Bangkok")
	parsed, err := ParseXLSX(data, loc)
	if err != nil {
		s.failJob(job, fmt.Sprintf("parse error: %v", err))
		return
	}

	setMsg(fmt.Sprintf("Importing %d rows…", len(parsed.Rows)))

	result, err := s.importSvc.ImportUpload(ctx, parsed.Rows)
	if err != nil {
		s.failJob(job, fmt.Sprintf("import error: %v", err))
		return
	}

	setMsg("Reconciling courses…")

	added, removed, reconciled, err := s.reconcileSvc.ReconcileAllUnlockedCourses(ctx)
	if err != nil {
		// Log but don't fail the upload — the data is in crm_rows.
		s.log.Error("reconcile error after upload", "error", err)
		setMsg("Upload succeeded, but some courses failed to reconcile")
	}

	result.CoursesReconciled = reconciled
	result.StudentsAdded = added
	result.StudentsRemoved = removed

	s.mu.Lock()
	job.Status = "Succeeded"
	job.Message = "Upload complete"
	job.Result = &result
	s.mu.Unlock()
}

func (s *UploadService) failJob(job *UploadJobState, errMsg string) {
	s.mu.Lock()
	job.Status = "Failed"
	job.Message = "Failed"
	job.Error = errMsg
	s.mu.Unlock()
}
```

- [ ] **Step 2: Run `make -C backend test`**

Expected: PASS.

---

### Task 8: Add HTTP API for Upload + Cycles + Course Filters

**Files:**
- Modify: `backend/internal/httpapi/httpdeps/deps.go`
- Modify: `backend/internal/httpapi/handler.go`
- Create: `backend/internal/httpapi/crmhttp/routes.go`

- [ ] **Step 1: Extend deps**

Modify `backend/internal/httpapi/httpdeps/deps.go`:

```go
import "warwick-institute/internal/crmimport"

type Deps struct {
  // existing...
  CRMImport *crmimport.UploadService
}
```

- [ ] **Step 2: Register routes**

Modify `backend/internal/httpapi/handler.go`:

```go
import "warwick-institute/internal/httpapi/crmhttp"
// ...
crmhttp.Register(mux, deps)
```

- [ ] **Step 3: Implement routes**

Create `backend/internal/httpapi/crmhttp/routes.go`:

```go
package crmhttp

import (
	"encoding/json"
	"net/http"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/crmimport"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("POST /api/v1/crm/upload", s.handleUpload)
	mux.HandleFunc("GET /api/v1/crm/upload/status", s.handleUploadStatus)

	mux.HandleFunc("GET /api/v1/crm/cycles", s.handleCyclesList)
	mux.HandleFunc("GET /api/v1/crm/options", s.handleCrmOptions)

	mux.HandleFunc("GET /api/v1/courses/{id}/crm-filter", s.handleCourseFilterGet)
	mux.HandleFunc("PUT /api/v1/courses/{id}/crm-filter", s.handleCourseFilterPut)
	mux.HandleFunc("POST /api/v1/courses/{id}/crm-filter/preview", s.handleCourseFilterPreview)
	mux.HandleFunc("POST /api/v1/courses/{id}/crm-filter/lock", s.handleCourseFilterLockToggle)
}

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	if s.deps.CRMImport == nil {
		s.a.WriteErr(w, http.StatusServiceUnavailable, "not_configured", "CRM import not configured")
		return
	}

	// Limit upload size to 50MB
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_upload", "Invalid upload: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_upload", "Missing file field")
		return
	}
	defer file.Close()

	job, err := s.deps.CRMImport.StartUpload(r.Context(), file, header.Filename, header.Size)
	if err != nil {
		if err.Error() == "upload already in progress" {
			s.a.WriteErr(w, http.StatusConflict, "upload_running", "Upload already in progress")
			return
		}
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusAccepted, job)
}

func (s *server) handleUploadStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	job := s.deps.CRMImport.GetJob()
	if job == nil {
		s.a.WriteJSON(w, http.StatusOK, map[string]any{"status": "no_upload"})
		return
	}
	s.a.WriteJSON(w, http.StatusOK, job)
}

func (s *server) handleCyclesList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	items, err := s.deps.Q.CrmCyclesList(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, items)
}

func (s *server) handleCrmOptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	row, err := s.deps.Q.CrmDistinctOptions(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, row)
}

func (s *server) handleCourseFilterGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var enabled bool
	var locked bool
	var filterJSON []byte
	err = s.deps.DB.QueryRow(r.Context(),
		`SELECT crm_filter_enabled, crm_roster_locked, COALESCE(crm_filter,'{}'::jsonb) FROM courses WHERE id=$1`,
		courseID,
	).Scan(&enabled, &locked, &filterJSON)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled": enabled,
		"locked":  locked,
		"filter":  json.RawMessage(filterJSON),
	})
}

func (s *server) handleCourseFilterPut(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var body struct {
		Enabled bool            `json:"enabled"`
		Filter  json.RawMessage `json:"filter"`
	}
	if err := s.a.DecodeJSON(r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	_, err = s.deps.DB.Exec(r.Context(), `
		UPDATE courses
		SET crm_filter_enabled=$2,
		    crm_filter=$3::jsonb,
		    crm_filter_updated_at=now()
		WHERE id=$1
	`, courseID, body.Enabled, body.Filter)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}

	// If enabled and not locked, reconcile immediately.
	if body.Enabled {
		var locked bool
		_ = s.deps.DB.QueryRow(r.Context(), `SELECT crm_roster_locked FROM courses WHERE id=$1`, courseID).Scan(&locked)
		if !locked {
			var f crmimport.CourseFilter
			if err := json.Unmarshal(body.Filter, &f); err == nil {
				res, reconcileErr := s.deps.CRMReconcile.ReconcileCourse(r.Context(), courseID, f)
				if reconcileErr != nil {
					s.a.WriteErr(w, http.StatusInternalServerError, "reconcile_error", reconcileErr.Error())
					return
				}
				s.a.WriteJSON(w, http.StatusOK, map[string]any{
					"ok":     true,
					"result": res,
				})
				return
			}
		}
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *server) handleCourseFilterLockToggle(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var body struct {
		Locked bool `json:"locked"`
	}
	if err := s.a.DecodeJSON(r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	_, err = s.deps.DB.Exec(r.Context(), `UPDATE courses SET crm_roster_locked=$2 WHERE id=$1`, courseID, body.Locked)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}

	// If unlocking, reconcile immediately.
	if !body.Locked {
		var filterJSON []byte
		_ = s.deps.DB.QueryRow(r.Context(), `SELECT crm_filter FROM courses WHERE id=$1`, courseID).Scan(&filterJSON)
		var f crmimport.CourseFilter
		if err := json.Unmarshal(filterJSON, &f); err == nil {
			res, reconcileErr := s.deps.CRMReconcile.ReconcileCourse(r.Context(), courseID, f)
			if reconcileErr != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "reconcile_error", reconcileErr.Error())
				return
			}
			s.a.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "result": res})
			return
		}
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *server) handleCourseFilterPreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	// v1: placeholder — count distinct wcodes from crm_rows matching filter.
	// Implement as a count query similar to the reconcile query.
	var body struct {
		Filter json.RawMessage `json:"filter"`
	}
	if err := s.a.DecodeJSON(r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	var f crmimport.CourseFilter
	if err := json.Unmarshal(body.Filter, &f); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_filter", "Invalid filter")
		return
	}
	// Use ReconcileService's queryDesiredStudents to get count
	count, err := s.deps.CRMReconcile.PreviewCount(r.Context(), f)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"distinct_students": count})
}
```

---

### Task 9: Frontend Admin Upload Page

**Files:**
- Create: `src/pages/CrmAdmin.tsx`
- Modify: `src/App.tsx`
- Modify: `src/components/Layout.tsx`

- [ ] **Step 1: Create `CrmAdmin` page with upload + job polling + summary**

```tsx
import { useEffect, useState, useRef } from "react";
import { apiJson, apiUpload } from "../api/client";
import { useToast } from "../hooks/useToast";

type UploadJob = {
  id: string;
  status: "Running" | "Succeeded" | "Failed";
  message: string;
  result?: {
    rows_parsed: number;
    rows_stored: number;
    cycles_found: string[];
    courses_reconciled: number;
    students_added: number;
    students_removed: number;
  } | null;
  error?: string;
  started_at: string;
};

export default function CrmAdmin() {
  const { addToast } = useToast();
  const [job, setJob] = useState<UploadJob | null>(null);
  const [uploading, setUploading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const poll = async () => {
    try {
      const res = await apiJson<UploadJob>("/api/v1/crm/upload/status", { method: "GET" });
      if (res && "status" in res && res.status !== "no_upload") {
        setJob(res);
      }
    } catch {
      // ignore
    }
  };

  useEffect(() => {
    void poll();
  }, []);

  // Poll while running
  useEffect(() => {
    if (!job || job.status !== "Running") return;
    const t = setInterval(poll, 1500);
    return () => clearInterval(t);
  }, [job]);

  const handleUpload = async () => {
    const file = fileRef.current?.files?.[0];
    if (!file) {
      addToast("error", "Please select a file");
      return;
    }
    if (!file.name.endsWith(".xlsx")) {
      addToast("error", "Please select an XLSX file");
      return;
    }
    try {
      setUploading(true);
      const res = await apiUpload<UploadJob>("/api/v1/crm/upload", file);
      setJob(res);
      addToast("success", "Upload started");
    } catch (err: any) {
      addToast("error", err?.message ?? "Upload failed");
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  };

  return (
    <div>
      <h1 className="text-[32px] font-bold text-gray-800 mb-3">CRM Import</h1>

      <div className="border border-gray-200 rounded-sm p-4 mb-4">
        <div className="text-sm font-semibold mb-2">Upload CRM Export XLSX</div>
        <div className="flex items-center gap-2">
          <input
            ref={fileRef}
            type="file"
            accept=".xlsx"
            className="text-sm"
            disabled={job?.status === "Running"}
          />
          <button
            onClick={handleUpload}
            disabled={uploading || job?.status === "Running"}
            className="px-3 py-1 text-sm rounded-sm bg-[var(--color-wi-green)] hover:bg-[var(--color-wi-green-dark)] text-white disabled:opacity-60"
          >
            {uploading ? "Uploading…" : job?.status === "Running" ? "Upload in progress…" : "Upload"}
          </button>
        </div>
      </div>

      {job && (
        <div className="border border-gray-200 rounded-sm p-4 mb-4">
          <div className="text-sm font-semibold mb-2">
            Status:{" "}
            <span className={
              job.status === "Succeeded" ? "text-green-600" :
              job.status === "Failed" ? "text-red-600" :
              "text-blue-600"
            }>
              {job.status}
            </span>
          </div>
          <div className="text-xs text-gray-600 mb-2">{job.message}</div>

          {job.status === "Succeeded" && job.result && (
            <div className="grid grid-cols-2 gap-x-6 gap-y-1 text-xs">
              <div>Rows parsed: <span className="font-semibold">{job.result.rows_parsed}</span></div>
              <div>Rows stored (after dedupe): <span className="font-semibold">{job.result.rows_stored}</span></div>
              <div>Cycles found: <span className="font-semibold">{job.result.cycles_found.join(", ")}</span></div>
              <div>Courses reconciled: <span className="font-semibold">{job.result.courses_reconciled}</span></div>
              <div>Students added: <span className="font-semibold text-green-600">+{job.result.students_added}</span></div>
              <div>Students removed: <span className="font-semibold text-red-600">-{job.result.students_removed}</span></div>
            </div>
          )}

          {job.status === "Failed" && (
            <div className="text-xs text-red-600 mt-1">{job.error}</div>
          )}
        </div>
      )}

      <div className="text-xs text-gray-400">
        Uploaded XLSX completely replaces all existing CRM data.
        Courses with CRM filter enabled and not locked will be auto-reconciled.
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Add route in `src/App.tsx`**

```tsx
import CrmAdmin from "./pages/CrmAdmin";
// ...
<Route path="/crm" element={<CrmAdmin />} />
```

- [ ] **Step 3: Add admin-only nav item in `src/components/Layout.tsx`**

Add a `CRM` nav item (path `/crm`) shown only if `user?.role === "Admin"`.

- [ ] **Step 4: Add `apiUpload` helper to `src/api/client.ts`**

```ts
export async function apiUpload<T>(path: string, file: File): Promise<T> {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch(path, {
    method: "POST",
    headers: { "Authorization": `Bearer ${getToken()}` },
    body: form,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: { message: res.statusText } }));
    throw new Error(err?.error?.message ?? res.statusText);
  }
  return res.json();
}
```

---

### Task 10: Frontend Course Filter Panel + Lock Toggle

**Files:**
- Create: `src/components/crm/CrmFilterPanel.tsx`
- Modify: `src/pages/CourseDetail.tsx`

- [ ] **Step 1: Implement `CrmFilterPanel` (v1 filter controls + save + lock)**

Key features:

- Load saved filter via `GET /api/v1/courses/:id/crm-filter`
- Load options via `GET /api/v1/crm/options`
- Multi-select dropdowns for cycle, course name, academic level, secondary school
- Text search for teacher(s)
- Blank/non-blank toggles per column
- Preview count: "This filter matches N distinct students"
- Save via `PUT /api/v1/courses/:id/crm-filter`
- Lock toggle via `POST /api/v1/courses/:id/crm-filter/lock`
- When locked: show banner "Roster is locked — won't auto-update on future uploads"

- [ ] **Step 2: In `CourseDetail`, hide manual roster controls when CRM filter is enabled**

Add in `CourseDetail.tsx`:
- state `crmEnabled: boolean`
- load `/api/v1/courses/:id/crm-filter` on mount (admin only)
- if `crmEnabled`, hide:
  - "Add by WCode" input/button
  - remove buttons per student
  - confirm message: "Roster is managed by CRM filter"

- [ ] **Step 3: Show lock banner when locked**

When `locked` is true, show a banner above the roster indicating the roster is frozen.

---

### Task 11: Add Preview Count to Reconcile Service

**Files:**
- Modify: `backend/internal/crmimport/reconcile_service.go`

- [ ] **Step 1: Add `PreviewCount` method**

```go
func (s *ReconcileService) PreviewCount(ctx context.Context, filter CourseFilter) (int, error) {
	filter.Normalize()
	if err := filter.Validate(); err != nil {
		return 0, err
	}
	conds := []string{"1=1"}
	args := []any{}
	argN := 1
	// ... same condition building as queryDesiredStudents ...
	sql := `SELECT COUNT(DISTINCT wcode) FROM crm_rows WHERE ` + strings.Join(conds, " AND ")
	var count int
	err := s.db.QueryRow(ctx, sql, args...).Scan(&count)
	return count, err
}
```

---

### Task 12: Wire Everything in main.go

**Files:**
- Modify: `backend/cmd/server/main.go`

- [ ] **Step 1: Initialize crmimport services (when upload config is valid)**

```go
// Import service
importSvc, err := crmimport.NewImportService(dbpool, cfg.InstituteTZ)
if err != nil {
  log.Fatal("init import service", "error", err)
}
reconcileSvc := crmimport.NewReconcileService(dbpool)
uploadSvc := crmimport.NewUploadService(logger, dbpool, importSvc, reconcileSvc)
```

- [ ] **Step 2: Pass to deps**

```go
deps.CRMImport = uploadSvc
deps.CRMReconcile = reconcileSvc
```

- [ ] **Step 3: Start server (no scheduler needed)**

---

## Self-Review Checklist (before implementing)

- [ ] No Sage CRM HTTP client code (no client.go, html_forms.go, scheduler.go)
- [ ] Upload endpoint accepts multipart file, validates XLSX magic bytes
- [ ] Atomic replace: all crm_rows deleted before insert in single transaction
- [ ] crm_cycles populated from distinct cycle values in uploaded data
- [ ] Auto-reconcile runs for all unlocked courses after successful upload
- [ ] Per-course lock toggle prevents auto-reconcile; unlock triggers reconcile
- [ ] Roster read-only when CRM filter enabled (manual add/remove blocked)
- [ ] Single-flight: only one upload job at a time
- [ ] XLSX parsing validates required headers before processing
- [ ] Upload summary shown to admin after completion
- [ ] Spec coverage: compare against `docs/superpowers/specs/2026-05-22-crm-xlsx-import-design.md`

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-22-crm-xlsx-import-and-course-filters.md`.

Two execution options:

1. **Subagent-Driven (recommended)** — use superpowers:subagent-driven-development
2. **Inline Execution** — use superpowers:executing-plans

Which approach?
