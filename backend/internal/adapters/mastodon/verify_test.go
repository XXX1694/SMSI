package mastodon

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/adapters/providertest"
)

func TestContract(t *testing.T) { providertest.Contract(t, New(Config{})) }

func TestCapabilitiesAreHonest(t *testing.T) {
	c := New(Config{}).Capabilities()
	if !c.SafeToRetryAfterUnknown || c.CanPublishVideo || !c.CanDelete || c.ConnectMethod != provider.ConnectToken {
		t.Fatalf("%+v", c)
	}
}

func fieldsFor(url string) map[string]string {
	return map[string]string{"instance_url": url, "access_token": testToken}
}

func TestVerifyReadsProfileAndLimits(t *testing.T) {
	f := newFake(t)
	f.standardRoutes()
	prof, secret, err := f.adapter().Verify(context.Background(), fieldsFor("https://Example.com/"))
	if err != nil {
		t.Fatal(err)
	}
	if secret != testToken || prof.ID != "example.com:1234" || prof.Username != "@alice@example.com" || prof.DisplayName != "Alice" {
		t.Fatalf("%+v %q", prof, secret)
	}
	limits := prof.Metadata["limits"].(map[string]any)
	if limits["max_characters"] != 800 || limits["max_media"] != 3 || limits["max_image_bytes"] != int64(8388608) {
		t.Fatalf("limits: %v", limits)
	}
	if prof.Metadata["host"] != "example.com" || prof.Metadata["account_id"] != "1234" || prof.Metadata["handle"] != "@alice@example.com" {
		t.Fatalf("metadata: %v", prof.Metadata)
	}
	// The stored limits tighten the capabilities, never loosen them.
	c := New(Config{}).Capabilities().WithAccountLimits(prof.Metadata)
	if c.MaxTextLength != 500 || c.MaxMediaCount != 3 || c.MaxImageBytes != 8388608 {
		t.Fatalf("caps: %+v", c)
	}
	blob, _ := json.Marshal(prof)
	if strings.Contains(string(blob), testToken) {
		t.Fatal("profile leaks the token")
	}
}

func TestVerifyFallsBackToV1InstanceAndToNoLimits(t *testing.T) {
	f := newFake(t)
	f.standardRoutes()
	delete(f.routes, "GET /api/v2/instance")
	f.on("GET /api/v1/instance", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, `{"configuration":{"statuses":{"max_characters":1500}}}`)
	})
	prof, _, err := f.adapter().Verify(context.Background(), fieldsFor("https://example.com"))
	if err != nil || prof.Metadata["limits"].(map[string]any)["max_characters"] != 1500 {
		t.Fatalf("%v %+v", err, prof)
	}
	delete(f.routes, "GET /api/v1/instance")
	prof, _, err = f.adapter().Verify(context.Background(), fieldsFor("https://example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if _, has := prof.Metadata["limits"]; has {
		t.Fatalf("no limits expected: %v", prof.Metadata)
	}
}

func TestVerifyRejectsBadInput(t *testing.T) {
	f := newFake(t)
	f.standardRoutes()
	a := f.adapter()
	for name, in := range map[string]map[string]string{
		"http":        fieldsFor("http://example.com"),
		"path":        fieldsFor("https://example.com/mastodon"),
		"query":       fieldsFor("https://example.com/?x=1"),
		"credentials": fieldsFor("https://user:pw@example.com"),
		"port 8443":   fieldsFor("https://example.com:8443"),
		"port 22":     fieldsFor("https://example.com:22"),
		"no host":     fieldsFor("https://"),
		"empty":       fieldsFor(""),
		"no token":    {"instance_url": "https://example.com"},
	} {
		if _, _, err := a.Verify(context.Background(), in); provider.Classify(err) != provider.KindPermanent {
			t.Errorf("%s: want permanent, got %v", name, err)
		}
	}
	if n := len(f.requests("GET /api/v1/accounts/verify_credentials")); n != 0 {
		t.Fatalf("invalid input must not reach the network, got %d calls", n)
	}
}

func TestVerifyMapsAuthAndNonMastodon(t *testing.T) {
	f := newFake(t)
	f.standardRoutes()
	bad := map[string]string{"instance_url": "https://example.com", "access_token": "revoked-" + testToken}
	_, _, err := f.adapter().Verify(context.Background(), bad)
	if provider.Classify(err) != provider.KindAuth || strings.Contains(err.Error(), bad["access_token"]) {
		t.Fatalf("want auth without the token, got %v", err)
	}
	f.on("GET /api/v1/accounts/verify_credentials", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, `<html>`) })
	if _, _, err := f.adapter().Verify(context.Background(), fieldsFor("https://example.com")); provider.CodeOf(err) != "NOT_MASTODON" {
		t.Fatalf("got %v", err)
	}
}

// The production client refuses private destinations before any request is made.
func TestDefaultClientBlocksPrivateInstances(t *testing.T) {
	a := New(Config{})
	for _, host := range []string{"https://127.0.0.1", "https://[::1]", "https://169.254.169.254", "https://10.0.0.5:443", "https://[fec0::1]"} {
		_, _, err := a.Verify(context.Background(), fieldsFor(host))
		if provider.Classify(err) != provider.KindPermanent || provider.CodeOf(err) != "INSTANCE_NOT_ALLOWED" {
			t.Errorf("%s: want INSTANCE_NOT_ALLOWED, got %v", host, err)
		}
	}
}
