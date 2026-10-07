package mock

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/socialos/backend/internal/adapters/provider"
)

func req(key, text string) provider.PublishRequest {
	return provider.PublishRequest{IdempotencyKey: key, Text: text, Account: provider.AccountRef{ID: "a"}}
}

func kindOf(err error) provider.Kind {
	var pe *provider.Error
	if errors.As(err, &pe) {
		return pe.Kind
	}
	return provider.Classify(err)
}

func TestIdentityAndLabel(t *testing.T) {
	p := New()
	c := p.Capabilities()
	if p.Name() != "mock" || !p.Supported() || !p.Configured() || !strings.Contains(c.Notes, "MOCK") || c.RequiresApproval || c.ConnectMethod != provider.ConnectOAuth {
		t.Fatalf("%s %+v", p.Name(), c)
	}
	if c.SafeToRetryAfterUnknown {
		t.Fatal("the mock must model a network that needs lookup after an unknown outcome")
	}
	var _ provider.OAuth = p
	var _ provider.Publisher = p
	var _ provider.Lookuper = p
}

func TestOAuthFlow(t *testing.T) {
	p := New()
	ctx := context.Background()
	u, err := url.Parse(p.AuthorizeURL(provider.AuthorizeParams{State: "st", CodeChallenge: "ch", RedirectURI: "https://api.test/cb"}))
	if err != nil || u.Query().Get("state") != "st" || !strings.HasPrefix(u.Query().Get("code"), "mock-code-") || u.Host != "api.test" {
		t.Fatalf("%v %v", u, err)
	}
	if u2, _ := url.Parse(p.AuthorizeURL(provider.AuthorizeParams{State: "s", CodeChallenge: "c", RedirectURI: "https://api.test/cb?x=1"})); u2.Query().Get("x") != "1" || u2.Query().Get("state") != "s" {
		t.Fatalf("existing query must be kept: %v", u2)
	}
	tok, err := p.Exchange(ctx, u.Query().Get("code"), "verifier", "")
	if err != nil || tok.AccessToken == "" || tok.RefreshToken == "" || tok.ExpiresAt == nil {
		t.Fatalf("%+v %v", tok, err)
	}
	for name, tc := range map[string][2]string{"bad code": {"stolen", "v"}, "no verifier": {"mock-code-x", ""}} {
		if _, err := p.Exchange(ctx, tc[0], tc[1], ""); kindOf(err) != provider.KindPermanent {
			t.Errorf("%s: %v", name, err)
		}
	}
	prof, err := p.Profile(ctx, tok.AccessToken)
	if err != nil || prof.ID == "" || prof.Username == "" {
		t.Fatalf("%+v %v", prof, err)
	}
	if _, err := p.Profile(ctx, ""); kindOf(err) != provider.KindAuth {
		t.Fatalf("missing token: %v", err)
	}
	if fresh, err := p.Refresh(ctx, tok.RefreshToken); err != nil || fresh.AccessToken == tok.AccessToken || fresh.RefreshToken != tok.RefreshToken {
		t.Fatalf("refresh: %+v %v", fresh, err)
	}
	if _, err := p.Refresh(ctx, "garbage"); kindOf(err) != provider.KindAuth {
		t.Fatalf("bad refresh token: %v", err)
	}
}

func TestPublishIsIdempotentPerKey(t *testing.T) {
	p := New()
	ctx := context.Background()
	a, err := p.Publish(ctx, req("k1", "hello"))
	if err != nil || a.ExternalID == "" || a.URL == "" {
		t.Fatalf("%+v %v", a, err)
	}
	b, _ := p.Publish(ctx, req("k1", "hello"))
	c, _ := p.Publish(ctx, req("k2", "hello"))
	if a.ExternalID != b.ExternalID || a.ExternalID == c.ExternalID {
		t.Fatalf("same key same post, different key different post: %v %v %v", a.ExternalID, b.ExternalID, c.ExternalID)
	}
	if p.Calls("k1") != 2 || p.Calls("k2") != 1 || p.Calls("nope") != 0 {
		t.Fatalf("calls: %d %d", p.Calls("k1"), p.Calls("k2"))
	}
}

func TestControlMarkers(t *testing.T) {
	p := New()
	ctx := context.Background()
	if _, err := p.Publish(ctx, req("f", "x #mock-fail")); kindOf(err) != provider.KindPermanent {
		t.Fatalf("#mock-fail: %v", err)
	}
	if _, err := p.Publish(ctx, req("f", "x #mock-fail")); kindOf(err) != provider.KindPermanent {
		t.Fatalf("#mock-fail is permanent every time: %v", err)
	}
	if _, err := p.Publish(ctx, req("a", "x #mock-auth")); kindOf(err) != provider.KindAuth {
		t.Fatalf("#mock-auth: %v", err)
	}
	if _, err := p.Publish(ctx, req("r", "x #mock-retry")); kindOf(err) != provider.KindRetryable {
		t.Fatalf("#mock-retry first attempt: %v", err)
	}
	if _, err := p.Publish(ctx, req("r", "x #mock-retry")); err != nil {
		t.Fatalf("#mock-retry second attempt must succeed: %v", err)
	}
	// #mock-unknown: the post IS recorded, but the caller sees a timeout; a lookup finds it, a second publish is a no-op.
	if _, err := p.Publish(ctx, req("u", "x #mock-unknown")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("#mock-unknown: %v", err)
	}
	res, found, err := p.Lookup(ctx, req("u", "x #mock-unknown"))
	if err != nil || !found || res.ExternalID == "" {
		t.Fatalf("lookup must find the recorded post: %+v %v %v", res, found, err)
	}
	if _, found, _ := p.Lookup(ctx, req("never", "x")); found {
		t.Fatal("lookup of an unknown key")
	}
	again, err := p.Publish(ctx, req("u", "x #mock-unknown"))
	if err != nil || again.ExternalID != res.ExternalID {
		t.Fatalf("second publish: %+v %v", again, err)
	}
}

func TestDelete(t *testing.T) {
	p := New()
	ctx := context.Background()
	res, _ := p.Publish(ctx, req("d", "x"))
	if err := p.Delete(ctx, provider.DeleteRequest{ExternalID: res.ExternalID}); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := p.Lookup(ctx, req("d", "x")); found {
		t.Fatal("deleted post still found")
	}
	if err := p.Delete(ctx, provider.DeleteRequest{ExternalID: "missing"}); err != nil {
		t.Fatalf("deleting a missing post is not an error: %v", err)
	}
}

func TestLoginHintSelectsDistinctAccount(t *testing.T) {
	p := New()
	profileFor := func(hint string) provider.Profile {
		t.Helper()
		u, err := url.Parse(p.AuthorizeURL(provider.AuthorizeParams{State: "s", CodeChallenge: "c", RedirectURI: "https://api.test/cb", LoginHint: hint}))
		if err != nil {
			t.Fatal(err)
		}
		tok, err := p.Exchange(context.Background(), u.Query().Get("code"), "verifier", "")
		if err != nil {
			t.Fatal(err)
		}
		prof, err := p.Profile(context.Background(), tok.AccessToken)
		if err != nil {
			t.Fatal(err)
		}
		return prof
	}
	if got := profileFor("").ID; got != "mock-user-1" {
		t.Fatalf("default identity = %q", got)
	}
	a, b := profileFor("linkedin"), profileFor("telegram")
	if a.ID == b.ID || a.ID != "mock-linkedin" {
		t.Fatalf("hinted identities not distinct: %q %q", a.ID, b.ID)
	}
}
