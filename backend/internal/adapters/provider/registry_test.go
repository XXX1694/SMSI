package provider

import (
	"context"
	"testing"

	"github.com/socialos/backend/internal/domain/errs"
)

type tokenStub struct {
	method    string
	supported bool
	configure bool
	connector bool
}

func (s tokenStub) Name() string        { return "tok" }
func (s tokenStub) DisplayName() string { return "Tok" }
func (s tokenStub) Supported() bool     { return s.supported }
func (s tokenStub) Configured() bool    { return s.configure }
func (s tokenStub) Capabilities() Capabilities {
	return Capabilities{ConnectMethod: s.method}
}

type tokenStubWithVerify struct{ tokenStub }

func (tokenStubWithVerify) Verify(context.Context, map[string]string) (Profile, string, error) {
	return Profile{}, "", nil
}

func TestRegistryTokenConnector(t *testing.T) {
	ok := tokenStubWithVerify{tokenStub{ConnectToken, true, true, true}}
	if _, c, err := NewRegistry(ok).TokenConnector("tok"); err != nil || c == nil {
		t.Fatalf("%v", err)
	}
	for name, p := range map[string]Provider{
		"no Verify method": tokenStub{ConnectToken, true, true, false},
		"oauth method":     tokenStubWithVerify{tokenStub{ConnectOAuth, true, true, true}},
		"unsupported":      tokenStubWithVerify{tokenStub{ConnectToken, false, true, true}},
		"not configured":   tokenStubWithVerify{tokenStub{ConnectToken, true, false, true}},
	} {
		_, _, err := NewRegistry(p).TokenConnector("tok")
		if !errs.Is(err, errs.ProviderNotAvailable) {
			t.Errorf("%s: want PROVIDER_NOT_AVAILABLE, got %v", name, err)
		}
	}
	if _, _, err := NewRegistry().TokenConnector("tok"); !errs.Is(err, errs.ProviderNotAvailable) {
		t.Errorf("unknown provider: %v", err)
	}
}
