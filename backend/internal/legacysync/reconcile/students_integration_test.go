package reconcile

import (
	"context"
	"testing"
	"time"

	"warwick-institute/internal/legacysync/normalize"
)

func TestApplyStudentProfiles_FillsEmptyFieldsFromLegacyDirectory(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()

	wcode := "W6001" + suffixDigits(suffix)
	// Pre-existing student with a name only; the directory fills everything else.
	if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ($1, 'Existing Name', '')`, wcode); err != nil {
		t.Fatal(err)
	}

	student := normalize.LegacyStudent{
		WCode:    wcode,
		Name:     "Existing Name",
		Nickname: "Nicky",
		School:   "Bangkok Prep",
		Level:    "G1",
		Year:     "2025",
		Phone:    "081-0000001",
		Email:    "student@example.com",
	}

	changed, err := reconciler.ApplyStudentProfiles(ctx, []normalize.LegacyStudent{student}, FullReconcileOptions{ObservedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("ApplyStudentProfiles: %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}

	var nickname, school, level, year, phone, email string
	if err := pool.QueryRow(ctx, `
		SELECT nickname, school, level, year, student_phone, email
		FROM students WHERE wcode = $1`, wcode).Scan(&nickname, &school, &level, &year, &phone, &email); err != nil {
		t.Fatal(err)
	}
	if nickname != "Nicky" || school != "Bangkok Prep" || level != "G1" || year != "2025" || phone != "081-0000001" || email != "student@example.com" {
		t.Fatalf("profile = (%q %q %q %q %q %q), want directory values", nickname, school, level, year, phone, email)
	}

	// The student now carries a legacy external ref so later runs can
	// enumerate them for the per-wcode directory lookup.
	var refs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type='student' AND external_id=$2 AND state='active'`, suffix, wcode).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if refs != 1 {
		t.Fatalf("student external refs = %d, want 1", refs)
	}
}

func TestApplyStudentProfiles_NeverClobbersHumanEdits(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()

	wcode := "W6002" + suffixDigits(suffix)
	// A human/CRM edit: the stored nickname and school differ from the legacy
	// directory, and the full_name is authoritative.
	if _, err := pool.Exec(ctx, `
		INSERT INTO students (wcode, full_name, notes, nickname, school, student_phone)
		VALUES ($1, 'Human Name', '', 'HumanNick', 'St Mary''s', '081-9999999')`, wcode); err != nil {
		t.Fatal(err)
	}

	student := normalize.LegacyStudent{
		WCode:    wcode,
		Name:     "Legacy Name",
		Nickname: "LegacyNick",
		School:   "Bangkok Prep",
		Level:    "G1",
		Year:     "2025",
		Phone:    "081-0000001",
	}

	if _, err := reconciler.ApplyStudentProfiles(ctx, []normalize.LegacyStudent{student}, FullReconcileOptions{ObservedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("ApplyStudentProfiles: %v", err)
	}

	// Existing non-empty values are preserved.
	var nickname, school, name, phone string
	if err := pool.QueryRow(ctx, `SELECT full_name, nickname, school, student_phone FROM students WHERE wcode=$1`, wcode).Scan(&name, &nickname, &school, &phone); err != nil {
		t.Fatal(err)
	}
	if name != "Human Name" {
		t.Fatalf("full_name = %q, want Human Name (never overwritten)", name)
	}
	if nickname != "HumanNick" {
		t.Fatalf("nickname = %q, want HumanNick (human edit preserved)", nickname)
	}
	if school != "St Mary's" {
		t.Fatalf("school = %q, want St Mary's (human edit preserved)", school)
	}
	if phone != "081-9999999" {
		t.Fatalf("student_phone = %q, want 081-9999999 (human edit preserved)", phone)
	}

	// The legacy-only fields (level, year) that were empty still got filled.
	var level, year string
	if err := pool.QueryRow(ctx, `SELECT level, year FROM students WHERE wcode=$1`, wcode).Scan(&level, &year); err != nil {
		t.Fatal(err)
	}
	if level != "G1" || year != "2025" {
		t.Fatalf("level/year = (%q %q), want (G1 2025) filled from directory", level, year)
	}
}

func TestApplyStudentProfiles_ShadowModeIsNoop(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()

	wcode := "W6003" + suffixDigits(suffix)
	student := normalize.LegacyStudent{
		WCode:  wcode,
		Name:   "Shadow Name",
		School: "Bangkok Prep",
		Level:  "G1",
		Year:   "2025",
	}

	changed, err := reconciler.ApplyStudentProfiles(ctx, []normalize.LegacyStudent{student}, FullReconcileOptions{ObservedAt: time.Now().UTC(), ShadowMode: true})
	if err != nil {
		t.Fatalf("ApplyStudentProfiles (shadow): %v", err)
	}
	if changed != 0 {
		t.Fatalf("changed = %d, want 0 in shadow mode", changed)
	}
	var students int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM students WHERE wcode=$1`, wcode).Scan(&students); err != nil {
		t.Fatal(err)
	}
	if students != 0 {
		t.Fatal("shadow mode created a student")
	}
	var refs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_refs WHERE source=$1 AND entity_type='student'`, suffix).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if refs != 0 {
		t.Fatal("shadow mode recorded a student external ref")
	}
}
