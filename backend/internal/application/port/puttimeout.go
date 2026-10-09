package port

import (
	"context"
	"time"
)

type putTimeoutKey struct{}

// WithPutTimeout asks the object store to bound the upload that ctx is passed to by d instead of its default. It is
// for uploads that legitimately run long (the data export, which streams for up to its whole build budget).
func WithPutTimeout(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, putTimeoutKey{}, d)
}

// PutTimeout returns the upload bound set by WithPutTimeout, or def when none was set.
func PutTimeout(ctx context.Context, def time.Duration) time.Duration {
	if d, ok := ctx.Value(putTimeoutKey{}).(time.Duration); ok && d > 0 {
		return d
	}
	return def
}
