package crypto

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/sync/semaphore"

	"github.com/socialos/backend/internal/domain/errs"
)

// Argon2Params configures argon2id.
type Argon2Params struct {
	Memory  uint32 // KiB
	Time    uint32
	Threads uint8
	KeyLen  uint32
	SaltLen uint32
}

// DefaultArgon2 is the OWASP-recommended argon2id configuration m=19 MiB, t=2, p=1
// (https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html). It replaces the former 64 MiB
// setting: every hash runs in memory the 160 MB API container can afford. Hashes created with other parameters keep
// verifying because Verify reads the parameters from the PHC string; NeedsRehash reports them as outdated.
var DefaultArgon2 = Argon2Params{Memory: 19 * 1024, Time: 2, Threads: 1, KeyLen: 32, SaltLen: 16}

// DefaultWait is how long a hash or verify waits for a free slot before it gives up with a retryable error.
const DefaultWait = 5 * time.Second

// DefaultMemoryBudgetKiB is the total argon2 memory that may be in use at once (48 MiB): two new hashes, or one 64 MiB
// legacy verification on its own. It keeps hashing well inside the 160 MB API container.
const DefaultMemoryBudgetKiB = 48 * 1024

// PasswordHasher hashes passwords with argon2id in PHC string format. At most `limit` operations run at once and
// their combined argon2 memory stays within a budget, so a burst of logins cannot exhaust RAM. An operation heavier
// than the whole budget (an old 64 MiB hash) runs alone.
type PasswordHasher struct {
	p      Argon2Params
	slot   chan struct{}
	mem    *semaphore.Weighted
	budget int64
	wait   time.Duration
	kdf    func(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte
}

// NewPasswordHasher returns a hasher with the given parameters that runs at most limit operations at once (min 1)
// within budgetKiB of argon2 memory (<= 0 means DefaultMemoryBudgetKiB).
func NewPasswordHasher(p Argon2Params, limit int, budgetKiB int64) *PasswordHasher {
	if limit < 1 {
		limit = 1
	}
	if budgetKiB <= 0 {
		budgetKiB = DefaultMemoryBudgetKiB
	}
	return &PasswordHasher{p: p, slot: make(chan struct{}, limit), mem: semaphore.NewWeighted(budgetKiB), budget: budgetKiB,
		wait: DefaultWait, kdf: argon2.IDKey}
}

// acquire takes a slot and memoryKiB of the budget, waiting until both free up, ctx ends or the wait budget runs
// out. Both failures are retryable.
func (h *PasswordHasher) acquire(ctx context.Context, memoryKiB uint32) (release func(), err error) {
	ctx, cancel := context.WithTimeout(ctx, h.wait)
	defer cancel()
	busy := func() error { return errs.Wrap(errs.RateLimited, "server is busy, retry shortly", ctx.Err()) }
	select {
	case h.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, busy()
	}
	w := min(int64(memoryKiB), h.budget)
	if err := h.mem.Acquire(ctx, w); err != nil {
		<-h.slot
		return nil, busy()
	}
	return func() { h.mem.Release(w); <-h.slot }, nil
}

var errBadHash = errors.New("crypto: malformed password hash")

// Hash returns $argon2id$v=19$m=..,t=..,p=..$salt$hash.
func (h *PasswordHasher) Hash(ctx context.Context, password string) (string, error) {
	release, err := h.acquire(ctx, h.p.Memory)
	if err != nil {
		return "", err
	}
	defer release()
	salt := make([]byte, h.p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := h.kdf([]byte(password), salt, h.p.Time, h.p.Memory, h.p.Threads, h.p.KeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, h.p.Memory, h.p.Time, h.p.Threads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Verify checks password against an encoded hash in constant time.
func (h *PasswordHasher) Verify(ctx context.Context, password, encoded string) (bool, error) {
	parts, p, err := parsePHC(encoded)
	if err != nil {
		return false, err
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false, errBadHash
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil {
		return false, errBadHash
	}
	release, err := h.acquire(ctx, p.Memory)
	if err != nil {
		return false, err
	}
	defer release()
	got := h.kdf([]byte(password), salt, p.Time, p.Memory, p.Threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// NeedsRehash reports whether encoded is a valid hash made with parameters other than the current ones.
func (h *PasswordHasher) NeedsRehash(encoded string) bool {
	_, p, err := parsePHC(encoded)
	return err == nil && (p.Memory != h.p.Memory || p.Time != h.p.Time || p.Threads != h.p.Threads)
}

func parsePHC(encoded string) ([]string, Argon2Params, error) {
	var p Argon2Params
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, p, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return nil, p, errBadHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return nil, p, errBadHash
	}
	return parts, p, nil
}
