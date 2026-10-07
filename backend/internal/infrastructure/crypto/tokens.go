package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"strings"
)

// RandomToken returns n random bytes encoded as URL-safe base64 (no padding).
func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SHA256Hex returns the hex SHA-256 digest of s.
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ConstantTimeEqual compares two strings in constant time.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// PKCEChallenge returns the S256 code challenge for a verifier (RFC 7636).
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func randomBase62(n int) (string, error) {
	var sb strings.Builder
	max := big.NewInt(int64(len(base62)))
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		sb.WriteByte(base62[v.Int64()])
	}
	return sb.String(), nil
}

// APIKeyLivePrefix marks production API keys.
const APIKeyLivePrefix = "sk_live_"

const (
	apiKeyIDLen     = 8
	apiKeySecretLen = 40
)

// GeneratedAPIKey is a freshly minted key. Raw must be shown once and discarded.
type GeneratedAPIKey struct {
	Raw    string // sk_live_<8 id chars><40 secret chars>
	Prefix string // sk_live_<8 id chars>, safe to display
	Hash   string // SHA-256 hex of Raw, stored
}

// GenerateAPIKey mints a new API key (~238 bits of entropy).
func GenerateAPIKey() (GeneratedAPIKey, error) {
	id, err := randomBase62(apiKeyIDLen)
	if err != nil {
		return GeneratedAPIKey{}, err
	}
	secret, err := randomBase62(apiKeySecretLen)
	if err != nil {
		return GeneratedAPIKey{}, err
	}
	raw := APIKeyLivePrefix + id + secret
	return GeneratedAPIKey{Raw: raw, Prefix: APIKeyLivePrefix + id, Hash: SHA256Hex(raw)}, nil
}

// LooksLikeAPIKey performs a cheap format check before hashing/lookup.
func LooksLikeAPIKey(raw string) bool {
	if !strings.HasPrefix(raw, APIKeyLivePrefix) {
		return false
	}
	body := raw[len(APIKeyLivePrefix):]
	if len(body) != apiKeyIDLen+apiKeySecretLen {
		return false
	}
	for i := 0; i < len(body); i++ {
		if !strings.ContainsRune(base62, rune(body[i])) {
			return false
		}
	}
	return true
}
