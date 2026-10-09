package apikey

import (
	"testing"
	"time"

	"github.com/socialos/backend/internal/domain/errs"
)

func TestDefaultScopesAreSafe(t *testing.T) {
	for _, s := range DefaultScopes() {
		if s.Dangerous() {
			t.Fatalf("default scope %s is dangerous", s)
		}
	}
	for _, s := range []Scope{PostsPublish, PostsDelete, SocialDisconnect, SocialConnect} {
		if !s.Dangerous() {
			t.Fatalf("%s should be dangerous", s)
		}
	}
	if RiskOf(SocialDisconnect) != RiskCritical {
		t.Fatal("disconnect must be critical")
	}
	if RiskOf(SocialConnect) != RiskCritical {
		t.Fatal("connect must be critical")
	}
	for _, s := range DefaultScopes() {
		if s == SocialConnect {
			t.Fatal("social:connect must never be a default scope")
		}
	}
}

func TestParseScopes(t *testing.T) {
	got, err := ParseScopes([]string{"posts:read", "posts:read", "social:read"})
	if err != nil || len(got) != 2 || got[0] != PostsRead {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := ParseScopes([]string{"admin:all"}); !errs.Is(err, errs.Validation) {
		t.Fatalf("expected validation error, got %v", err)
	}
	def, _ := ParseScopes(nil)
	if len(def) != len(DefaultScopes()) {
		t.Fatal("empty should yield defaults")
	}
	if len(FromStrings([]string{"posts:read", "nope"})) != 1 {
		t.Fatal("FromStrings should drop unknown")
	}
}

func TestKeyUsable(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Minute), now.Add(time.Hour)
	if !(&Key{}).Usable(now) || !(&Key{ExpiresAt: &future}).Usable(now) {
		t.Fatal("should be usable")
	}
	if (&Key{ExpiresAt: &past}).Usable(now) || (&Key{RevokedAt: &past}).Usable(now) {
		t.Fatal("should not be usable")
	}
}
