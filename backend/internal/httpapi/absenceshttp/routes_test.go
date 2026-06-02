package absenceshttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestDispatchDelete_RouteRegistered(t *testing.T) {
	server := &server{}
	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/absences/"+fakeID.String(), nil)
	w := httptest.NewRecorder()

	server.handleAbsencesDispatch(w, req)

	// Should NOT return 404 — route exists
	if w.Code == http.StatusNotFound {
		t.Fatalf("DELETE /api/v1/absences/{id} should route to a handler, got 404")
	}
}

func TestDispatchDelete_WrongMethod(t *testing.T) {
	server := &server{}
	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/absences/"+fakeID.String(), nil)
	w := httptest.NewRecorder()

	server.handleAbsencesDispatch(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("PATCH /api/v1/absences/{id} should return 404, got %d", w.Code)
	}
}

func TestDispatchDelete_NotFoundOnSubpath(t *testing.T) {
	server := &server{}
	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/absences/"+fakeID.String()+"/unknown-subpath", nil)
	w := httptest.NewRecorder()

	server.handleAbsencesDispatch(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("DELETE /api/v1/absences/{id}/unknown-subpath should return 404, got %d", w.Code)
	}
}

func TestDispatchGet_RoutesToHandler(t *testing.T) {
	server := &server{}
	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/absences/"+fakeID.String(), nil)
	w := httptest.NewRecorder()

	server.handleAbsencesDispatch(w, req)

	// Should NOT return 404 — GET /absences/{id} is a valid route
	if w.Code == http.StatusNotFound {
		t.Fatalf("GET /api/v1/absences/{id} should route to a handler, got 404")
	}
}

func TestDispatchPost_OnIDPathReturns405(t *testing.T) {
	server := &server{}
	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/"+fakeID.String(), nil)
	w := httptest.NewRecorder()

	server.handleAbsencesDispatch(w, req)

	// POST on /absences/{id} is explicitly handled as method not allowed
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/v1/absences/{id} should return 405, got %d", w.Code)
	}
}
