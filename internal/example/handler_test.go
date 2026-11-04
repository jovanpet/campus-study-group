package example

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestMux() *http.ServeMux {
	mux := http.NewServeMux()
	NewHandler(NewService(&fakeRepo{})).RegisterRoutes(mux)
	return mux
}

func TestHandler(t *testing.T) {
	mux := newTestMux()

	tests := []struct {
		method, path, body string
		wantStatus         int
	}{
		{http.MethodPost, "/examples", `{"name":"calculus"}`, http.StatusCreated},
		{http.MethodPost, "/examples", `{"name":""}`, http.StatusBadRequest},
		{http.MethodPost, "/examples", `{"nope":1}`, http.StatusBadRequest},
		{http.MethodGet, "/examples/1", "", http.StatusOK},
		{http.MethodGet, "/examples/99", "", http.StatusNotFound},
		{http.MethodGet, "/examples/abc", "", http.StatusBadRequest},
		{http.MethodGet, "/examples", "", http.StatusOK},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tt.wantStatus {
			t.Errorf("%s %s %s = %d, want %d (body: %s)", tt.method, tt.path, tt.body, rec.Code, tt.wantStatus, rec.Body)
		}
	}
}
