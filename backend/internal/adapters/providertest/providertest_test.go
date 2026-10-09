package providertest

import (
	"testing"

	"github.com/socialos/backend/internal/adapters/mock"
	"github.com/socialos/backend/internal/adapters/mocktoken"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/adapters/stubs"
)

func TestExistingProvidersSatisfyTheContract(t *testing.T) {
	Contract(t, mock.New())
	for _, s := range stubs.All() {
		Contract(t, s)
	}
}

type fakeTokenProvider struct {
	*mocktoken.Provider
	fields []provider.ConnectField
}

func (f fakeTokenProvider) Capabilities() provider.Capabilities {
	c := f.Provider.Capabilities()
	c.ConnectFields = f.fields
	return c
}

func TestContractDemandsASecretFieldOnTokenProviders(t *testing.T) {
	t.Run("mocktoken passes", func(t *testing.T) { Contract(t, mocktoken.New()) })
	for name, fields := range map[string][]provider.ConnectField{
		"no secret field":         {{Name: "k", Label: "K", Kind: provider.FieldText, Required: true}},
		"secret kind not flagged": {{Name: "k", Label: "K", Kind: provider.FieldSecret, Required: true}},
	} {
		inner := &testing.T{}
		done := make(chan struct{})
		go func() { defer close(done); Contract(inner, fakeTokenProvider{mocktoken.New(), fields}) }()
		<-done
		if !inner.Failed() {
			t.Errorf("%s: contract must fail", name)
		}
	}
}
