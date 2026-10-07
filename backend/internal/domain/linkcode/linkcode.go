// Package linkcode defines the one-time code a user posts into a chat
// (Telegram channel or group) to prove that they control it.
//
// The plaintext code is shown to its owner exactly once; only its SHA-256 hash
// is stored. The package is pure: no I/O besides the system random source.
package linkcode

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

const (
	// Prefix starts every code.
	Prefix = "SOS-"
	// Alphabet has no look-alike characters (no 0/O, 1/I/L).
	Alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	// RandomLen is the number of random characters after the prefix (~39 bits).
	RandomLen = 8
	// TTL is how long a code can be redeemed.
	TTL = 15 * time.Minute
	// MaxActive is the number of unredeemed, unexpired codes one user may hold;
	// starting another link retires the oldest.
	MaxActive = 3
)

// Status of a link attempt as seen by its owner.
type Status string

const (
	StatusPending   Status = "pending"
	StatusConnected Status = "connected"
	StatusExpired   Status = "expired"
)

// Code is the stored form of a link code. It never holds the plaintext.
type Code struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	Hash            string
	ExpiresAt       time.Time
	UsedAt          *time.Time
	ChatID          string
	SocialAccountID *uuid.UUID
	CreatedAt       time.Time
}

// Used reports whether the code was already redeemed.
func (c *Code) Used() bool { return c.UsedAt != nil }

// Expired reports whether the code can no longer be redeemed because of time.
func (c *Code) Expired(now time.Time) bool { return !now.Before(c.ExpiresAt) }

// Usable reports whether the code can be redeemed now: unused and unexpired.
func (c *Code) Usable(now time.Time) bool { return !c.Used() && !c.Expired(now) }

// StatusAt derives the owner-facing status.
func (c *Code) StatusAt(now time.Time) Status {
	switch {
	case c.Used():
		return StatusConnected
	case c.Expired(now):
		return StatusExpired
	default:
		return StatusPending
	}
}

// Generate returns a new code such as "SOS-7KQ2M9XA" from the system random source.
func Generate() (string, error) { return generate(rand.Reader) }

func generate(r io.Reader) (string, error) {
	// Rejection sampling keeps the distribution uniform over the alphabet.
	const limit = 256 - 256%len(Alphabet)
	out := make([]byte, 0, RandomLen)
	buf := make([]byte, 32)
	for len(out) < RandomLen {
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			out = append(out, Alphabet[int(b)%len(Alphabet)])
			if len(out) == RandomLen {
				break
			}
		}
	}
	return Prefix + string(out), nil
}

// Invisible characters Telegram clients may leave around pasted text.
const (
	zeroWidthSpace = rune(0x200b)
	byteOrderMark  = rune(0xfeff)
)

// ErrMalformed is returned for text that cannot be a link code.
var ErrMalformed = errors.New("linkcode: malformed")

// Normalize trims surrounding whitespace, upper-cases and validates the shape
// of a candidate message. The whole message must be the code: a code buried
// in other text is not accepted.
func Normalize(s string) (string, error) {
	// A code is 12 characters; anything much longer cannot match, which also
	// keeps arbitrary chat traffic from being hashed.
	if len(s) > 64 {
		return "", ErrMalformed
	}
	s = strings.ToUpper(strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == zeroWidthSpace || r == byteOrderMark }))
	if len(s) != len(Prefix)+RandomLen || !strings.HasPrefix(s, Prefix) {
		return "", ErrMalformed
	}
	for i := len(Prefix); i < len(s); i++ {
		if strings.IndexByte(Alphabet, s[i]) < 0 {
			return "", ErrMalformed
		}
	}
	return s, nil
}

// Hash is the stored digest of a normalised code.
func Hash(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
