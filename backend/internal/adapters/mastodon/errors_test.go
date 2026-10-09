package mastodon

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

func TestErrorMapping(t *testing.T) {
	reset := time.Date(2026, 10, 9, 12, 5, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for _, tc := range []struct {
		name    string
		status  int
		header  map[string]string
		body    string
		kind    provider.Kind
		message string
		after   time.Duration
	}{
		{"401 invalid token", 401, nil, `{"error":"The access token is invalid"}`, provider.KindAuth, "access token is invalid", 0},
		{"403 missing scope", 403, nil, `{"error":"This action is outside the authorized scopes"}`, provider.KindAuth, "authorized scopes", 0},
		{"422 keeps the message", 422, nil, `{"error":"Validation failed: Text character limit of 500 exceeded"}`, provider.KindPermanent, "character limit of 500", 0},
		{"429 honours X-RateLimit-Reset", 429, map[string]string{"X-RateLimit-Reset": reset}, `{"error":"Too many requests"}`, provider.KindRetryable, "Too many", 5 * time.Minute},
		{"429 falls back to Retry-After", 429, map[string]string{"Retry-After": "7"}, `{}`, provider.KindRetryable, "Too Many", 7 * time.Second},
		{"429 without hints", 429, nil, `{}`, provider.KindRetryable, "Too Many", 0},
		{"500", 500, nil, `{"error":"boom"}`, provider.KindRetryable, "boom", 0},
		{"503 html", 503, nil, `<html>`, provider.KindRetryable, "Service Unavailable", 0},
		{"400", 400, nil, `{"error":"bad"}`, provider.KindPermanent, "bad", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t)
			f.on("POST /api/v1/statuses", func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tc.header {
					w.Header().Set(k, v)
				}
				writeJSON(w, tc.status, tc.body)
			})
			_, err := f.adapter().Publish(context.Background(), publishReq("x"))
			if provider.Classify(err) != tc.kind || !strings.Contains(err.Error(), tc.message) || provider.RetryAfterOf(err) != tc.after {
				t.Fatalf("kind=%v after=%v err=%v", provider.Classify(err), provider.RetryAfterOf(err), err)
			}
		})
	}
}

func TestMediaErrorsAreMappedToo(t *testing.T) {
	f := newFake(t)
	f.on("POST /api/v2/media", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 422, `{"error":"Validation failed: File content type is invalid"}`)
	})
	_, err := f.adapter().Publish(context.Background(), publishReq("x", imageFile("x")))
	if provider.Classify(err) != provider.KindPermanent || !strings.Contains(err.Error(), "content type is invalid") {
		t.Fatalf("got %v", err)
	}
}

func TestTimeoutAfterSendIsUnknown(t *testing.T) {
	f := newFake(t)
	f.on("POST /api/v1/statuses", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	})
	a := f.adapter()
	a.http = f.client(100 * time.Millisecond)
	_, err := a.Publish(context.Background(), publishReq("x"))
	if provider.Classify(err) != provider.KindUnknown {
		t.Fatalf("want Unknown, got %v (%v)", provider.Classify(err), err)
	}
	if !a.Capabilities().SafeToRetryAfterUnknown {
		t.Fatal("an unknown outcome is only retryable because of the Idempotency-Key")
	}
}

func TestNoSecretInErrors(t *testing.T) {
	f := newFake(t)
	// A hostile server echoes the credential back in its error text.
	f.on("POST /api/v1/statuses", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 422, `{"error":"bad `+strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")+` token"}`)
	})
	f.on("GET /api/v1/accounts/verify_credentials", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 401, `{"error":"nope `+strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")+`"}`)
	})
	a := f.adapter()
	_, err := a.Publish(context.Background(), publishReq("x"))
	_, _, err2 := a.Verify(context.Background(), fieldsFor("https://example.com"))
	for _, e := range []error{err, err2} {
		if e == nil || strings.Contains(e.Error(), testToken) || strings.Contains(provider.SafeMessage(e), testToken) {
			t.Fatalf("secret leaked or no error: %v", e)
		}
	}
}
