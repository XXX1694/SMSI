package crypto

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/domain/errs"
)

var fastParams = Argon2Params{Memory: 8 * 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16}

func TestPasswordHasher(t *testing.T) {
	h := NewPasswordHasher(fastParams, 2, 0)
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
	enc, err := NewPasswordHasher(DefaultArgon2, 1, 0).Hash(context.Background(), "pw")
	if err != nil || !strings.HasPrefix(enc, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("new hashes must carry m=19456,t=2,p=1, got %q %v", enc, err)
	}
}

// legacy64MiB is the configuration used before the memory bound: still stored in existing databases.
var legacy64MiB = Argon2Params{Memory: 64 * 1024, Time: 2, Threads: 2, KeyLen: 32, SaltLen: 16}

func TestOldParameterHashesStillVerifyAndAreFlagged(t *testing.T) {
	ctx := context.Background()
	old, err := NewPasswordHasher(legacy64MiB, 1, 0).Hash(ctx, "old-secret")
	if err != nil || !strings.Contains(old, "m=65536,t=2,p=2") {
		t.Fatalf("legacy hash %q %v", old, err)
	}
	h := NewPasswordHasher(DefaultArgon2, 1, 0)
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

// trackingKDF records how many operations and how much argon2 memory are in flight at once and parks every call
// until gate closes, so the semaphore is the only thing that can hold the numbers down.
type tracking struct {
	mu                         sync.Mutex
	n, mem, peakN, peakSharedM int64
	entered                    chan struct{}
	gate                       chan struct{}
}

func (k *tracking) kdf(pw, salt []byte, tm, mem uint32, th uint8, kl uint32) []byte {
	k.mu.Lock()
	k.n++
	k.mem += int64(mem)
	k.peakN = max(k.peakN, k.n)
	if k.n > 1 {
		k.peakSharedM = max(k.peakSharedM, k.mem)
	}
	k.mu.Unlock()
	k.entered <- struct{}{}
	<-k.gate
	k.mu.Lock()
	k.n--
	k.mem -= int64(mem)
	k.mu.Unlock()
	return make([]byte, kl)
}

func TestConcurrencyAndMemoryBudgetAreNeverExceeded(t *testing.T) {
	const workers, budgetKiB = 20, 48 * 1024
	h := NewPasswordHasher(DefaultArgon2, 2, budgetKiB)
	k := &tracking{entered: make(chan struct{}, workers), gate: make(chan struct{})}
	h.kdf = k.kdf
	h.wait = time.Minute
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// alternate new (19 MiB) hashes and legacy (64 MiB, heavier than the whole budget) verifications
			if i%2 == 0 {
				if _, err := h.Hash(context.Background(), "pw"); err != nil {
					t.Error(err)
				}
				return
			}
			enc := "$argon2id$v=19$m=65536,t=2,p=2$c2FsdHNhbHRzYWx0c2FsdA$" + base64.RawStdEncoding.EncodeToString(make([]byte, 32))
			if _, err := h.Verify(context.Background(), "pw", enc); err != nil {
				t.Error(err)
			}
		}()
	}
	for i := 0; i < workers; i++ {
		<-k.entered
		k.gate <- struct{}{} // release one at a time so operations overlap for real
	}
	wg.Wait()
	if k.peakN > 2 {
		t.Fatalf("peak concurrency %d exceeds limit 2", k.peakN)
	}
	if k.peakSharedM > budgetKiB {
		t.Fatalf("operations shared %d KiB, budget is %d KiB", k.peakSharedM, budgetKiB)
	}
}

func TestOversizedHashRunsAlone(t *testing.T) {
	h := NewPasswordHasher(DefaultArgon2, 2, 48*1024)
	k := &tracking{entered: make(chan struct{}, 4), gate: make(chan struct{})}
	h.kdf = k.kdf
	enc := "$argon2id$v=19$m=65536,t=2,p=2$c2FsdHNhbHRzYWx0c2FsdA$" + base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = h.Verify(context.Background(), "pw", enc) }()
	}
	<-k.entered
	// the second 64 MiB verification must not start while the first holds the whole budget
	select {
	case <-k.entered:
		t.Fatal("two 64 MiB verifications ran together")
	case <-time.After(50 * time.Millisecond):
	}
	k.gate <- struct{}{}
	<-k.entered
	k.gate <- struct{}{}
	wg.Wait()
}

func TestWaitHonoursContextCancellation(t *testing.T) {
	h := NewPasswordHasher(fastParams, 1, 0)
	h.slot <- struct{}{} // occupy the only slot
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.Hash(ctx, "pw")
	if errs.CodeOf(err) != errs.RateLimited || !errors.Is(err, context.Canceled) {
		t.Fatalf("want retryable RATE_LIMITED wrapping context.Canceled, got %v", err)
	}
	enc, _ := NewPasswordHasher(fastParams, 1, 0).Hash(context.Background(), "pw")
	if _, err := h.Verify(ctx, "pw", enc); errs.CodeOf(err) != errs.RateLimited {
		t.Fatalf("verify must also give up while waiting, got %v", err)
	}
}

func TestWaitTimeoutIsRetryable(t *testing.T) {
	h := NewPasswordHasher(fastParams, 1, 0)
	h.wait = time.Nanosecond
	h.slot <- struct{}{}
	if _, err := h.Hash(context.Background(), "pw"); errs.CodeOf(err) != errs.RateLimited {
		t.Fatalf("want RATE_LIMITED after wait budget, got %v", err)
	}
}

func BenchmarkHashDefault(b *testing.B) {
	h := NewPasswordHasher(DefaultArgon2, 1, 0)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := h.Hash(context.Background(), "benchmark-password"); err != nil {
			b.Fatal(err)
		}
	}
}
