package studentshttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

func newStudentsHTTPServer(t *testing.T, pool *pgxpool.Pool) *server {
	t.Helper()
	s := &server{
		deps: httpdeps.Deps{DB: pool, Q: sqldb.New(pool), Auth: routeAuth{user: auth.AuthenticatedUser{Role: "Admin"}}},
		a:    httpadapter.New(routeAuth{user: auth.AuthenticatedUser{Role: "Admin"}}, nil),
	}
	return s
}

// TestStudentUpdateAndGetCarryProfileFields pins the full student profile
// shape end to end: the directory/CRM fields (nickname, school, level, year,
// phone, email) round-trip through update, and both the by-wcode GET and the
// attendee-visible shape serve them.
func TestStudentUpdateAndGetCarryProfileFields(t *testing.T) {
	pool := requireStudentsTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wcode := "w" + strings.ToLower(time.Now().UTC().Format("150405")) + strings.ToLower(fmt.Sprintf("%d", time.Now().UnixNano()%10000))
	if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ($1, 'Original Name', '')`, wcode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM students WHERE wcode = $1`, wcode)
	})
	s := newStudentsHTTPServer(t, pool)

	// PUT the full profile.
	updateBody := `{
		"wcode": "` + wcode + `",
		"full_name": "Alice A. Alpha",
		"notes": "note",
		"nickname": "Ali",
		"school": "Bangkok Prep",
		"level": "G1",
		"year": "2026",
		"student_phone": "081-1234567",
		"email": "alice@example.com"
	}`
	listResp := func() *httptest.ResponseRecorder {
		// Scope the list to exactly this student: the shared test DB accumulates
		// rows from every package, so an unscoped 50-row list may not reach it.
		request := httptest.NewRequest(http.MethodGet, "/api/v1/students?limit=50&q="+wcode, nil)
		response := httptest.NewRecorder()
		s.handleStudentsList(response, request)
		return response
	}
	var id string
	{
		// Find the row first via the list (its DTO shape must carry the
		// profile fields, all empty initially).
		resp := listResp()
		if resp.Code != http.StatusOK {
			t.Fatalf("list = %d, want 200; body=%s", resp.Code, resp.Body.String())
		}
		var page struct {
			Items []studentDTO `json:"items"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		var found *studentDTO
		for i := range page.Items {
			if strings.EqualFold(page.Items[i].Wcode, wcode) {
				found = &page.Items[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("student %s not in list %+v", wcode, page.Items)
		}
		id = found.ID
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/students/"+id, strings.NewReader(updateBody))
	updateReq.SetPathValue("id", id)
	updateReq.Header.Set("Idempotency-Key", "test-idem-"+wcode)
	updateResp := httptest.NewRecorder()
	s.handleStudentsUpdate(updateResp, updateReq)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update = %d, want 200; body=%s", updateResp.Code, updateResp.Body.String())
	}

	// By-wcode GET must reflect every saved field.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/students/by-wcode?wcode="+wcode, nil)
	getResp := httptest.NewRecorder()
	s.handleStudentsGetByWCode(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("by-wcode = %d, want 200; body=%s", getResp.Code, getResp.Body.String())
	}
	var got studentDTO
	if err := json.Unmarshal(getResp.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.FullName != "Alice A. Alpha" || got.Nickname != "Ali" || got.School != "Bangkok Prep" || got.Level != "G1" || got.Year != "2026" || got.StudentPhone != "081-1234567" || got.Email != "alice@example.com" {
		t.Fatalf("profile after update = %+v, want all seven fields", got)
	}
}

// TestStudentCreateSavesFullProfile pins that creating a student through the
// API persists the same profile fields the sync and the edit screen use.
func TestStudentCreateSavesFullProfile(t *testing.T) {
	pool := requireStudentsTestDB(t)

	wcode := "w" + strings.ToLower(time.Now().UTC().Format("150405")) + strings.ToLower(fmt.Sprintf("%d", time.Now().UnixNano()%10000))
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM students WHERE wcode = $1`, wcode)
	})
	s := newStudentsHTTPServer(t, pool)

	createBody := `{
		"wcode": "` + wcode + `",
		"full_name": "Bob B. Beta",
		"notes": "",
		"nickname": "Bo",
		"school": "Harrow Bangkok",
		"level": "G2",
		"year": "2026",
		"student_phone": "081-7654321",
		"email": "bob@example.com"
	}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/students", strings.NewReader(createBody))
	createReq.Header.Set("Idempotency-Key", "test-idem-create-"+wcode)
	createResp := httptest.NewRecorder()
	s.handleStudentsCreate(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201; body=%s", createResp.Code, createResp.Body.String())
	}
	var created studentDTO
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Nickname != "Bo" || created.School != "Harrow Bangkok" || created.Level != "G2" || created.Year != "2026" || created.StudentPhone != "081-7654321" || created.Email != "bob@example.com" {
		t.Fatalf("created = %+v, want profile fields on create response", created)
	}

	// The saved row must survive a reload through the by-wcode GET.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/students/by-wcode?wcode="+wcode, nil)
	getResp := httptest.NewRecorder()
	s.handleStudentsGetByWCode(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("by-wcode = %d, want 200; body=%s", getResp.Code, getResp.Body.String())
	}
	var got studentDTO
	if err := json.Unmarshal(getResp.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Nickname != "Bo" || got.School != "Harrow Bangkok" || got.Level != "G2" || got.Year != "2026" || got.StudentPhone != "081-7654321" || got.Email != "bob@example.com" {
		t.Fatalf("profile after create = %+v, want all profile fields", got)
	}
}
