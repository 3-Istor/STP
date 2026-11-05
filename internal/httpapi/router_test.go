package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		wantStatus      int
		wantBody        string
		wantContentType string
	}{
		{"get returns ok", http.MethodGet, http.StatusOK, `{"status":"ok"}` + "\n", "application/json"},
		{"post is not allowed", http.MethodPost, http.StatusMethodNotAllowed, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewRouter(Services{}).ServeHTTP(rec, httptest.NewRequest(tt.method, "/healthz", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
			if tt.wantContentType != "" && rec.Header().Get("Content-Type") != tt.wantContentType {
				t.Errorf("Content-Type = %q, want %q", rec.Header().Get("Content-Type"), tt.wantContentType)
			}
		})
	}
}

// Une route /api/v1 inconnue répond en JSON ; une mauvaise méthode sur une route connue reste en 405.
func TestUnknownAPIRouteAndWrongMethod(t *testing.T) {
	tests := []struct {
		name, method, path string
		wantStatus         int
		wantJSON           bool
	}{
		{"route inconnue", http.MethodGet, "/api/v1/nope", http.StatusNotFound, true},
		{"mauvaise méthode", http.MethodPut, "/api/v1/boms/7", http.StatusMethodNotAllowed, false},
		{"hors API", http.MethodGet, "/nope", http.StatusNotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewRouter(Services{BOMs: &fakeBOMs{}, Snapshots: &fakeSnapshots{}}).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Content-Type") == "application/json"; got != tt.wantJSON {
				t.Errorf("JSON = %v, want %v (body %q)", got, tt.wantJSON, rec.Body)
			}
		})
	}
}
