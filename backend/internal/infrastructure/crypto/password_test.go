package crypto

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/socialos/backend/internal/domain/errs"
	"golang.org/x/crypto/argon2"
)

var fastParams = Argon2Params{Memory: 8 * 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16}

func TestPasswordHasher(t *testing.T) {
	h := NewPasswordHasher(fastParams, 2)
	ctx := context.Background()
	enc, err := h.Hash(ctx, "correct horse battery")
	if err != nil || !strings.HasPrefix(enc, "$argon2id$v=19$m=8192,t=1,p=1$") {
		t.Fatalf("hash %q %v", enc, err)
	}
	if ok, err := h.Verify(ctx, "correct horse battery", enc); !ok || err != nil {
		t.Fatal("verify failed")
	}
	if ok, _ := h.Verify(ctx, "wrong", enc); ok {
		t.Fatal("wrong password accepted")
	}
	if _, err := h.Verify(ctx, "x", "$bcrypt$foo"); err == nil {
		t.Fatal("malformed hash accepted")
	}
}

func TestDefaultParamsAreOWASPMinimum(t *testing.T) {
	enc, err := NewPasswordHasher(DefaultArgon2, 1).Hash(context.Background(), "pw")
	if err != nil || !strings.HasPrefix(enc, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("new hashes must carry m=19456,t=2,p=1, got %q %v", enc, err)
	}
}

// legacy64MiB is the configuration used before the memory bound: still stored in existing databases.
var legacy64MiB = Argon2Params{Memory: 64 * 1024, Time: 2, Threads: 2, KeyLen: 32, SaltLen: 16}

func TestOldParameterHashesStillVerifyAndAreFlagged(t *testing.T) {
	ctx := context.Background()
	old, err := NewPasswordHasher(legacy64MiB, 1).Hash(ctx, "old-secret")
	if err != nil || !strings.Contains(old, "m=65536,t=2,p=2") {
		t.Fatalf("legacy hash %q %v", old, err)
	}
	h := NewPasswordHasher(DefaultArgon2, 1)
	if ok, err := h.Verify(ctx, "old-secret", old); !ok || err != nil {
		t.Fatalf("legacy hash must verify: %v %v", ok, err)
	}
	if !h.NeedsRehash(old) {
		t.Fatal("legacy parameters must be flagged for rehash")
	}
	fresh, _ := h.Hash(ctx, "old-secret")
	if h.NeedsRehash(fresh) || h.NeedsRehash("$bcrypt$junk") || h.NeedsRehash("") {
		t.Fatal("current or malformed hashes must not be flagged")
	}
}

func TestConcurrencyNeverExceedsLimit(t *testing.T) {
	const limit, workers = 2, 8
	h := NewPasswordHasher(fastParams, limit)
	var inFlight, peak atomic.Int32
	entered := make(chan struct{}, workers)
	gate := make(chan struct{})
	h.kdf = func(pw, salt []byte, tm, mem uint32, th uint8, kl uint32) []byte {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		entered <- struct{}{}
		<-gate
		inFlight.Add(-1)
		return argon2.IDKey(pw, salt, tm, mem, th, kl)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := h.Hash(context.Background(), "pw"); err != nil {
				t.Error(err)
			}
		}()
	}
	for i := 0; i < limit; i++ {
		<-entered // the limit is saturated; the other workers are parked on the semaphore
	}
	if got := inFlight.Load(); got != limit {
		t.Fatalf("in flight = %d, want %d", got, limit)
	}
	close(gate)
	wg.Wait()
	if peak.Load() > limit {
		t.Fatalf("peak concurrency %d exceeds limit %d", peak.Load(), limit)
	}
}

func TestWaitHonoursContextCancellation(t *testing.T) {
	h := NewPasswordHasher(fastParams, 1)
	h.slot <- struct{}{} // occupy the only slot
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.Hash(ctx, "pw")
	if errs.CodeOf(err) != errs.RateLimited || !errors.Is(err, context.Canceled) {
		t.Fatalf("want retryable RATE_LIMITED wrapping context.Canceled, got %v", err)
	}
	enc, _ := NewPasswordHasher(fastParams, 1).Hash(context.Background(), "pw")
	if _, err := h.Verify(ctx, "pw", enc); errs.CodeOf(err) != errs.RateLimited {
		t.Fatalf("verify must also give up while waiting, got %v", err)
	}
}

func TestWaitTimeoutIsRetryable(t *testing.T) {
	h := NewPasswordHasher(fastParams, 1)
	h.wait = 1 // nanosecond
	h.slot <- struct{}{}
	if _, err := h.Hash(context.Background(), "pw"); errs.CodeOf(err) != errs.RateLimited {
		t.Fatalf("want RATE_LIMITED after wait budget, got %v", err)
	}
}

func BenchmarkHashDefault(b *testing.B) {
	h := NewPasswordHasher(DefaultArgon2, 1)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := h.Hash(context.Background(), "benchmark-password"); err != nil {
			b.Fatal(err)
		}
	}
}
