package courselevelshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"warwick-institute/internal/auth"
	"warwick-institute/internal/httpapi/httpdeps"
)

type fakeAuth struct {
	user auth.AuthenticatedUser
	err  error
}

func (f fakeAuth) RequireUser(ctx context.Context, r *http.Request) (auth.AuthenticatedUser, error) {
	return f.user, f.err
}

func (fakeAuth) HandleLogin(w http.ResponseWriter, r *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(w http.ResponseWriter, r *http.Request) error { return nil }

func TestRegister_PutLevel_BadID_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("PUT", "/api/v1/admin/courses/not-a-uuid/level",
		strings.NewReader(`{"level": 3, "cycle_id": "cy2025a"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_id" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_id")
	}
}

func TestRegister_PutLevel_NegativeLevel_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("PUT",
		"/api/v1/admin/courses/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/level",
		strings.NewReader(`{"level": -1}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_level" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_level")
	}
}

func TestRegister_PutLevel_StringLevel_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("PUT",
		"/api/v1/admin/courses/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/level",
		strings.NewReader(`{"level": "abc"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_json" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_json")
	}
}

func TestCourseLevelDTO_JSONIncludesRootCourseFields(t *testing.T) {
	id := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	rootID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	dto := courseLevelDTO{
		ID:                  id,
		Code:                "C101",
		Name:                "Course 101",
		SubjectID:           id,
		SubjectCode:         "SUBJ",
		SubjectName:         "Subject",
		CycleID:             strPtr("cy2025a"),
		CycleLabel:          strPtr("Cycle 2025 A"),
		Level:               int16Ptr(1),
		RootCourseGroupID:   &rootID,
		RootCourseGroupName: strPtr("Root Course 101"),
	}

	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}

	checkField := func(name string, expected any) {
		v, ok := m[name]
		if !ok {
			t.Errorf("missing JSON field %q", name)
			return
		}
		if v != expected {
			t.Errorf("field %q = %v, want %v", name, v, expected)
		}
	}

	checkField("id", id)
	checkField("code", "C101")
	checkField("name", "Course 101")
	checkField("subject_id", id)
	checkField("subject_code", "SUBJ")
	checkField("subject_name", "Subject")
	checkField("cycle_id", "cy2025a")
	checkField("cycle_label", "Cycle 2025 A")
	checkField("level", float64(1))
	checkField("root_course_group_id", rootID)
	checkField("root_course_group_name", "Root Course 101")

	// Verify root_course_group fields are nil when no root_course_group_id set
	nilDTO := courseLevelDTO{
		ID:          id,
		Code:        "C102",
		Name:        "Course 102",
		SubjectID:   id,
		SubjectCode: "SUBJ",
		SubjectName: "Subject",
	}
	b2, err := json.Marshal(nilDTO)
	if err != nil {
		t.Fatal(err)
	}
	var m2 map[string]any
	if err := json.Unmarshal(b2, &m2); err != nil {
		t.Fatal(err)
	}
	checkNilField := func(name string) {
		v, ok := m2[name]
		if !ok {
			t.Errorf("missing JSON field %q", name)
			return
		}
		if v != nil {
			t.Errorf("field %q = %v, want nil", name, v)
		}
	}
	checkNilField("root_course_group_id")
	checkNilField("root_course_group_name")
}

func strPtr(s string) *string { return &s }
func int16Ptr(v int16) *int16 { return &v }

func TestRegister_GetLevels_Unauthenticated_Returns401(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{err: context.DeadlineExceeded},
	})

	req := httptest.NewRequest("GET", "/api/v1/admin/course-levels", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestRegister_PutRootCourseGroup_BadID_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("PUT", "/api/v1/admin/courses/not-a-uuid/root-course-group",
		strings.NewReader(`{"root_course_group_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_id" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_id")
	}
}

func TestRegister_PutRootCourseGroup_BadJSON_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("PUT",
		"/api/v1/admin/courses/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/root-course-group",
		strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_json" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_json")
	}
}

func TestRegister_PutRootCourseGroup_BadGroupID_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	courseID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	req := httptest.NewRequest("PUT",
		"/api/v1/admin/courses/"+courseID+"/root-course-group",
		strings.NewReader(`{"root_course_group_id": "not-a-uuid"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_root_course_group_id" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_root_course_group_id")
	}
}

func TestRegister_PutRootCourseGroup_Unauthenticated_Returns401(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{err: context.DeadlineExceeded},
	})

	req := httptest.NewRequest("PUT", "/api/v1/admin/courses/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/root-course-group",
		strings.NewReader(`{"root_course_group_id": null}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestRegister_GetRootCourseGroups_Unauthenticated_Returns401(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{err: context.DeadlineExceeded},
	})

	req := httptest.NewRequest("GET", "/api/v1/admin/root-course-groups", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestRegister_PostRootCourseGroups_BadJSON_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("POST", "/api/v1/admin/root-course-groups",
		strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_json" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_json")
	}
}

func TestRegister_PostRootCourseGroups_MissingFields_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("POST", "/api/v1/admin/root-course-groups",
		strings.NewReader(`{"name": ""}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "missing_fields" {
		t.Fatalf("code = %q, want %q", got.Code, "missing_fields")
	}
}

func TestRegister_PutRootCourseGroups_BadID_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("PUT", "/api/v1/admin/root-course-groups/not-a-uuid",
		strings.NewReader(`{"name": "Root 101"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_id" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_id")
	}
}

func TestRegister_DeleteRootCourseGroup_BadID_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/root-course-groups/not-a-uuid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_id" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_id")
	}
}

func TestRegister_DeleteRootCourseGroup_Unauthenticated_Returns401(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{err: context.DeadlineExceeded},
	})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/root-course-groups/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestRegister_PutRootCourseGroups_Unauthenticated_Returns401(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{err: context.DeadlineExceeded},
	})

	req := httptest.NewRequest("PUT", "/api/v1/admin/root-course-groups/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		strings.NewReader(`{"name": "Root 101"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}
