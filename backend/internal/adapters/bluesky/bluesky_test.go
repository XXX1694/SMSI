package bluesky

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/adapters/providertest"
)

func TestContract(t *testing.T) {
	providertest.Contract(t, newFake(t).adapter())
}

func TestVerifyStoresDIDAndHandleButNoSession(t *testing.T) {
	f := newFake(t)
	p, secret := connect(t, f.adapter())
	if p.ID != testDID || p.Username != testHandle || secret != testPassword {
		t.Fatalf("profile %+v secret %q", p, secret)
	}
	blob, _ := json.Marshal(p)
	for _, bad := range []string{"jwt", "h.", testPassword, "accessJwt", "refreshJwt"} {
		if strings.Contains(string(blob), bad) {
			t.Errorf("profile contains %q: %s", bad, blob)
		}
	}
	if _, custom := p.Metadata["pds"]; custom {
		t.Error("default PDS must not be stored")
	}
}

func TestVerifyBadPasswordIsAuthWithoutLeak(t *testing.T) {
	f := newFake(t)
	_, _, err := f.adapter().Verify(context.Background(), map[string]string{"handle": testHandle, "app_password": "wrong-wrong-wrong"}) // gitleaks:allow (fake test value)
	if provider.Classify(err) != provider.KindAuth {
		t.Fatalf("want auth, got %v", err)
	}
	if strings.Contains(err.Error(), "wrong-wrong") || strings.Contains(provider.SafeMessage(err), "wrong-wrong") {
		t.Errorf("error leaks the password: %v", err)
	}
}

func TestVerifyRejectsBadInput(t *testing.T) {
	a := newFake(t).adapter()
	for name, fields := range map[string]map[string]string{
		"no dot handle": {"handle": "alice", "app_password": "x"},
		"email":         {"handle": "a@b.com", "app_password": "x"},
		"no password":   {"handle": testHandle},
		"http pds":      {"handle": testHandle, "app_password": "x", "pds": "http://pds.example.com"},
		"pds with path": {"handle": testHandle, "app_password": "x", "pds": "https://pds.example.com/x"},
		"pds port 8443": {"handle": testHandle, "app_password": "x", "pds": "https://pds.example.com:8443"},
		"pds userinfo":  {"handle": testHandle, "app_password": "x", "pds": "https://u:p@pds.example.com"},
	} {
		if _, _, err := a.Verify(context.Background(), fields); provider.Classify(err) != provider.KindPermanent {
			t.Errorf("%s: want permanent, got %v", name, err)
		}
	}
}

func TestCustomPDSGoesThroughSafeClientAndIsStored(t *testing.T) {
	a := New(Config{}) // production clients
	if a.fixed == a.custom {
		t.Fatal("custom PDS must use its own client")
	}
	base, custom, err := a.baseFor("https://PDS.Example.com/")
	if err != nil || !custom || base != "https://pds.example.com" {
		t.Fatalf("base %q custom %v err %v", base, custom, err)
	}
	// A private address is refused by the SSRF guard before any connection.
	_, _, err = a.Verify(context.Background(), map[string]string{"handle": testHandle, "app_password": "x", "pds": "https://127.0.0.1"})
	if err == nil {
		t.Fatal("loopback PDS must be refused")
	}
	if strings.Contains(err.Error(), "x") && strings.Contains(err.Error(), "password") {
		t.Errorf("error mentions the password: %v", err)
	}
}

func TestSessionIsCachedRefreshedThenRecreated(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	ctx := context.Background()
	if _, err := a.Publish(ctx, req("one")); err != nil {
		t.Fatal(err)
	}
	r2 := req("two")
	r2.IdempotencyKey = "target-2"
	if _, err := a.Publish(ctx, r2); err != nil {
		t.Fatal(err)
	}
	if f.creates != 1 || f.refreshes != 0 {
		t.Fatalf("creates %d refreshes %d, want 1 and 0", f.creates, f.refreshes)
	}
	f.expireAccess() // the PDS rejects the cached token with ExpiredToken
	r3 := req("three")
	r3.IdempotencyKey = "target-3"
	if _, err := a.Publish(ctx, r3); err != nil {
		t.Fatal(err)
	}
	if f.creates != 1 || f.refreshes != 1 {
		t.Fatalf("expired token: creates %d refreshes %d, want 1 and 1", f.creates, f.refreshes)
	}
	// Refresh token gone too: fall back to createSession.
	f.expireAccess()
	f.mu.Lock()
	f.refresh = map[string]bool{}
	f.mu.Unlock()
	r4 := req("four")
	r4.IdempotencyKey = "target-4"
	if _, err := a.Publish(ctx, r4); err != nil {
		t.Fatal(err)
	}
	if f.creates != 2 {
		t.Fatalf("creates %d, want 2", f.creates)
	}
}

func TestAccessExpiryByClockRefreshesBeforeUse(t *testing.T) {
	f := newFake(t)
	f.accessTTL = 10 * time.Minute
	a := f.adapter()
	if _, err := a.Publish(context.Background(), req("one")); err != nil {
		t.Fatal(err)
	}
	f.advance(11 * time.Minute)
	r := req("two")
	r.IdempotencyKey = "target-2"
	if _, err := a.Publish(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if f.refreshes != 1 || f.creates != 1 {
		t.Fatalf("refreshes %d creates %d", f.refreshes, f.creates)
	}
}

func TestRevokedPasswordIsAuthAfterOneRefreshAttempt(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	if _, err := a.Publish(context.Background(), req("one")); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.password = "rotated"
	f.mu.Unlock()
	f.expireAccess()
	f.mu.Lock()
	f.refresh = map[string]bool{}
	f.mu.Unlock()
	r := req("two")
	r.IdempotencyKey = "target-2"
	_, err := a.Publish(context.Background(), r)
	if provider.Classify(err) != provider.KindAuth {
		t.Fatalf("want auth, got %v", err)
	}
	if strings.Contains(err.Error(), testPassword) {
		t.Error("error leaks the password")
	}
}

func TestRateLimitedCreateSessionBacksOff(t *testing.T) {
	f := newFake(t)
	f.rateLimitAt = t0.Add(90 * time.Second).Unix()
	a := f.adapter()
	_, err := a.Publish(context.Background(), req("one"))
	if provider.Classify(err) != provider.KindRetryable || provider.RetryAfterOf(err) != 90*time.Second {
		t.Fatalf("want retryable 90s, got %v / %v", err, provider.RetryAfterOf(err))
	}
	calls := f.creates
	f.advance(30 * time.Second)
	_, err = a.Publish(context.Background(), req("one"))
	if provider.Classify(err) != provider.KindRetryable || provider.RetryAfterOf(err) != 60*time.Second {
		t.Fatalf("want retryable 60s left, got %v / %v", err, provider.RetryAfterOf(err))
	}
	if f.creates != calls {
		t.Fatal("createSession must not be called while rate limited")
	}
	f.advance(61 * time.Second)
	f.mu.Lock()
	f.rateLimitAt = 0
	f.mu.Unlock()
	if _, err := a.Publish(context.Background(), req("one")); err != nil {
		t.Fatalf("after the reset: %v", err)
	}
}

func TestServerAndRequestErrorMapping(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	f.failRecord = 503
	if _, err := a.Publish(context.Background(), req("x")); provider.Classify(err) != provider.KindRetryable {
		t.Errorf("503: %v", err)
	}
	f.failRecord = 400
	if _, err := a.Publish(context.Background(), req("x")); provider.Classify(err) != provider.KindPermanent {
		t.Errorf("InvalidRequest: %v", err)
	}
}

func TestChangedPasswordDropsCachedSession(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	if _, err := a.Publish(context.Background(), req("one")); err != nil {
		t.Fatal(err)
	}
	// Same password again: the cached session is reused.
	r2 := req("two")
	r2.IdempotencyKey = "target-2"
	if _, err := a.Publish(context.Background(), r2); err != nil {
		t.Fatal(err)
	}
	if f.creates != 1 {
		t.Fatalf("creates %d, want 1 while the password is unchanged", f.creates)
	}
	// A new password must log in again even though the cached token is still valid.
	f.mu.Lock()
	f.password = "rotated-rotated-pw" // gitleaks:allow (fake test value)
	f.mu.Unlock()
	r3 := req("three")
	r3.IdempotencyKey = "target-3"
	r3.AccessToken = "rotated-rotated-pw" // gitleaks:allow (fake test value)
	if _, err := a.Publish(context.Background(), r3); err != nil {
		t.Fatal(err)
	}
	if f.creates != 2 {
		t.Fatalf("creates %d, want 2 after the password changed", f.creates)
	}
	// And the old password is no longer served from the cache.
	r4 := req("four")
	r4.IdempotencyKey = "target-4"
	if _, err := a.Publish(context.Background(), r4); provider.Classify(err) != provider.KindAuth {
		t.Fatalf("old password: want auth, got %v", err)
	}
}
