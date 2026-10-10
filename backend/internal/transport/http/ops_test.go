package http_test

// Ops endpoints that need no services, so these run in `make test` without Postgres or Redis.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/socialos/backend/internal/buildinfo"
	httptransport "github.com/socialos/backend/internal/transport/http"
)

func TestVersionIsPublicAndCacheable(t *testing.T) {
	router := httptransport.NewRouter(httptransport.Services{}, httptransport.Options{})
	for _, path := range []string{"/api/v1/version", "/version"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s without credentials: %d %s", path, rec.Code, rec.Body)
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=60" {
			t.Errorf("GET %s: Cache-Control = %q", path, got)
		}
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("GET %s: security headers missing (X-Content-Type-Options = %q)", path, got)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("GET %s: body %s: %v", path, rec.Body, err)
		}
		// Exactly these keys: nothing toolchain- or host-specific leaks into a public endpoint.
		want := buildinfo.Get()
		expected := map[string]string{"version": want.Version, "commit": want.Commit, "built_at": want.BuiltAt}
		if len(body) != len(expected) {
			t.Errorf("GET %s: keys %v, want exactly version, commit, built_at", path, body)
		}
		for k, v := range expected {
			if body[k] != v || v == "" {
				t.Errorf("GET %s: %s = %q, want %q (non-empty)", path, k, body[k], v)
			}
		}
	}
}

func TestVersionRejectsOtherMethods(t *testing.T) {
	router := httptransport.NewRouter(httptransport.Services{}, httptransport.Options{})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/version", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("POST /api/v1/version: %d, want an error", rec.Code)
	}
}
