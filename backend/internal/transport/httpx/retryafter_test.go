package httpx

import (
	"net/http/httptest"
	"testing"

	"github.com/socialos/backend/internal/domain/errs"
)

func TestRateLimitedErrorsCarryRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, httptest.NewRequest("POST", "/", nil), errs.New(errs.RateLimited, "server is busy, retry shortly"))
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "5" {
		t.Fatalf("status %d Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	rec = httptest.NewRecorder()
	rec.Header().Set("Retry-After", "42")
	Error(rec, httptest.NewRequest("POST", "/", nil), errs.New(errs.RateLimited, "x"))
	if rec.Header().Get("Retry-After") != "42" {
		t.Fatal("an explicit Retry-After must not be overwritten")
	}
}
