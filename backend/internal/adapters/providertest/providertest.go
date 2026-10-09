// Package providertest holds the shared contract every provider adapter must
// satisfy. Each adapter's test calls Contract(t, p) next to its own
// fake-server tests.
package providertest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

// sentinel is a recognisable credential: it must never appear in an error or a profile.
const sentinel = "contract-secret-0123456789"

// Contract checks that a provider declares honest, consistent capabilities and,
// for token providers, that Verify keeps credentials out of errors and profiles.
func Contract(t *testing.T, p provider.Provider) {
	t.Helper()
	if p.Name() == "" || p.DisplayName() == "" {
		t.Fatal("name and display name are required")
	}
	c := p.Capabilities()
	switch c.ConnectMethod {
	case provider.ConnectOAuth, provider.ConnectTelegram, provider.ConnectToken, provider.ConnectNone:
	default:
		t.Fatalf("unknown connect method %q", c.ConnectMethod)
	}
	if c.Notes == "" {
		t.Error("notes must say honestly what the provider does and does not support")
	}
	checkPublishing(t, p, c)
	if c.ConnectMethod != provider.ConnectToken {
		if len(c.ConnectFields) != 0 {
			t.Error("connect_fields are only for token providers")
		}
		return
	}
	checkTokenForm(t, p, c)
	checkVerifyLeaks(t, p, c)
}

func checkPublishing(t *testing.T, p provider.Provider, c provider.Capabilities) {
	t.Helper()
	_, isPublisher := p.(provider.Publisher)
	if !p.Supported() {
		if c.CanPublishText || c.CanPublishImage || c.CanPublishVideo {
			t.Error("an unsupported provider must not claim to publish")
		}
		return
	}
	if !isPublisher {
		t.Error("a supported provider must implement Publisher")
	}
	if c.CanPublishText && c.MaxTextLength <= 0 {
		t.Error("a text provider must declare max_text_length")
	}
	if (c.CanPublishImage || c.CanPublishVideo) && c.MaxMediaCount <= 0 {
		t.Error("a media provider must declare max_media_count")
	}
	if c.MaxImageBytes > 0 && !c.CanPublishImage {
		t.Error("max_image_bytes without image support")
	}
	if c.MaxCaptionLength > 0 && c.MaxTextLength > 0 && c.MaxCaptionLength > c.MaxTextLength {
		t.Error("caption limit exceeds text limit")
	}
	if c.MaxImageBytes < 0 || c.MaxMediaCount < 0 || c.MaxTextLength < 0 {
		t.Error("limits must not be negative")
	}
}

func checkTokenForm(t *testing.T, p provider.Provider, c provider.Capabilities) {
	t.Helper()
	if _, ok := p.(provider.TokenConnector); !ok {
		t.Fatal("a token provider must implement TokenConnector")
	}
	var secrets, required int
	seen := map[string]bool{}
	for _, f := range c.ConnectFields {
		if f.Name == "" || f.Label == "" || seen[f.Name] {
			t.Errorf("field %q needs a unique name and a label", f.Name)
		}
		seen[f.Name] = true
		switch f.Kind {
		case provider.FieldText, provider.FieldURL:
		case provider.FieldSecret:
			if !f.Secret {
				t.Errorf("field %q has kind secret but is not marked Secret", f.Name)
			}
		default:
			t.Errorf("field %q has unknown kind %q", f.Name, f.Kind)
		}
		if f.Secret {
			secrets++
		}
		if f.Required {
			required++
		}
	}
	if secrets == 0 {
		t.Error("a token provider needs at least one field marked Secret")
	}
	if required == 0 {
		t.Error("a token provider needs at least one required field")
	}
}

// checkVerifyLeaks submits a sentinel in every field. Verify may accept or
// reject it, but neither the error nor the profile may contain it.
func checkVerifyLeaks(t *testing.T, p provider.Provider, c provider.Capabilities) {
	t.Helper()
	fields := map[string]string{}
	for _, f := range c.ConnectFields {
		switch f.Kind {
		case provider.FieldURL:
			fields[f.Name] = "https://example.com/" + sentinel
		default:
			fields[f.Name] = sentinel
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	prof, secret, err := p.(provider.TokenConnector).Verify(ctx, fields)
	if err != nil {
		if strings.Contains(err.Error(), sentinel) || strings.Contains(provider.SafeMessage(err), sentinel) {
			t.Errorf("Verify error leaks the credential: %v", err)
		}
		return
	}
	if prof.ID == "" || secret == "" {
		t.Error("a successful Verify returns a profile id and a credential")
	}
	blob, _ := json.Marshal(prof)
	if strings.Contains(string(blob), sentinel) || (len(secret) >= 8 && strings.Contains(string(blob), secret)) {
		t.Errorf("profile leaks the credential: %s", blob)
	}
}
