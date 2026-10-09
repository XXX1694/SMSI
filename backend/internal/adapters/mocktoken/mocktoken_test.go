package mocktoken

import (
	"context"
	"testing"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/adapters/providertest"
)

func TestContract(t *testing.T) { providertest.Contract(t, New()) }

func TestVerifyRejectsForeignKeys(t *testing.T) {
	_, _, err := New().Verify(context.Background(), map[string]string{"api_key": "nope"})
	if provider.Classify(err) != provider.KindAuth {
		t.Fatalf("want auth failure, got %v", err)
	}
}

func TestVerifyIsDeterministicAndHidesTheKey(t *testing.T) {
	p := New()
	a, secret, err := p.Verify(context.Background(), map[string]string{"api_key": "mt_abcdef123456"}) // gitleaks:allow (fake test value)
	if err != nil || secret != "mt_abcdef123456" {
		t.Fatalf("%v %q", err, secret)
	}
	b, _, _ := p.Verify(context.Background(), map[string]string{"api_key": "mt_abcdef123456"}) // gitleaks:allow (fake test value)
	if a.ID != b.ID || a.ID == "" || a.ID == secret {
		t.Fatalf("ids: %q %q", a.ID, b.ID)
	}
}

func TestPublishNeedsAValidKey(t *testing.T) {
	p := New()
	for _, tok := range []string{"", "bad", "mt_revoked1"} {
		_, err := p.Publish(context.Background(), provider.PublishRequest{IdempotencyKey: "k", AccessToken: tok, Title: "t", Text: "x"})
		if provider.Classify(err) != provider.KindAuth {
			t.Errorf("token %q: want auth failure, got %v", tok, err)
		}
	}
	res, err := p.Publish(context.Background(), provider.PublishRequest{IdempotencyKey: "k", AccessToken: "mt_ok123456", Title: "Hello", Text: "x"})
	if err != nil || res.Metadata["title"] != "Hello" {
		t.Fatalf("%v %+v", err, res)
	}
}
