package storage

import (
	"context"
	"testing"
	"time"

	"github.com/socialos/backend/internal/application/port"
)

// minio-go aborts a failed multipart upload with the context it got; if that were the request context, a client
// disconnect would cancel the abort and leave the parts behind.
func TestPutContextSurvivesRequestCancellation(t *testing.T) {
	req, cancelReq := context.WithCancel(context.Background())
	ctx, cancel := putContext(req, putTimeout)
	defer cancel()
	cancelReq()
	if ctx.Err() != nil {
		t.Fatal("cancelling the request must not cancel the put")
	}
	dl, ok := ctx.Deadline()
	if !ok || time.Until(dl) > putTimeout {
		t.Fatalf("the put needs an upper bound, got %v", dl)
	}
}

func TestPutContextHonoursAnExportSizedTimeout(t *testing.T) {
	ctx, cancel := putContext(context.Background(), port.PutTimeout(port.WithPutTimeout(context.Background(), 90*time.Minute), putTimeout))
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok || time.Until(dl) <= putTimeout {
		t.Fatalf("deadline %v must be beyond the %v default", dl, putTimeout)
	}
}
