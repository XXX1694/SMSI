package providertest

import (
	"testing"

	"github.com/socialos/backend/internal/adapters/mock"
	"github.com/socialos/backend/internal/adapters/stubs"
)

func TestExistingProvidersSatisfyTheContract(t *testing.T) {
	Contract(t, mock.New())
	for _, s := range stubs.All() {
		Contract(t, s)
	}
}
