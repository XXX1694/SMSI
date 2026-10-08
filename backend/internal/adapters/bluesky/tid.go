package bluesky

import (
	"crypto/sha256"
	"encoding/binary"
	"time"
)

const tidAlphabet = "234567abcdefghijklmnopqrstuvwxyz"

// rkeyFor derives a record key in TID syntax (13 chars of the base32-sortable
// alphabet, top bit zero) from the idempotency key, so every attempt of one
// target writes the same record and Lookup can find it [U: TID layout from the
// atproto TID spec]. Without a key it falls back to a time-based TID.
func rkeyFor(idempotencyKey string, now time.Time) string {
	var v uint64
	if idempotencyKey == "" {
		v = uint64(now.UnixMicro()) << 10
	} else {
		sum := sha256.Sum256([]byte("socialos:bluesky:rkey:" + idempotencyKey))
		v = binary.BigEndian.Uint64(sum[:8])
	}
	v &^= 1 << 63
	var out [13]byte
	for i := 12; i >= 0; i-- {
		out[i] = tidAlphabet[v&31]
		v >>= 5
	}
	return string(out[:])
}
