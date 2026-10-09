package port_test

import (
	"context"
	"testing"
	"time"

	"github.com/socialos/backend/internal/application/port"
)

func TestPutTimeout(t *testing.T) {
	ctx := context.Background()
	if got := port.PutTimeout(ctx, time.Minute); got != time.Minute {
		t.Fatalf("default = %v", got)
	}
	if got := port.PutTimeout(port.WithPutTimeout(ctx, time.Hour), time.Minute); got != time.Hour {
		t.Fatalf("override = %v", got)
	}
	if got := port.PutTimeout(port.WithPutTimeout(ctx, 0), time.Minute); got != time.Minute {
		t.Fatalf("zero override must fall back, got %v", got)
	}
}
