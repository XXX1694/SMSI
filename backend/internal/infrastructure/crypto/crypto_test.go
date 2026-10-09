package crypto

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func testCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := NewCipherFromBase64(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCipherRoundTrip(t *testing.T) {
	c := testCipher(t)
	enc, err := c.Encrypt("secret-access-token")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "v1:") || strings.Contains(enc, "secret") {
		t.Fatalf("bad ciphertext %q", enc)
	}
	enc2, _ := c.Encrypt("secret-access-token")
	if enc == enc2 {
		t.Fatal("nonce reuse: identical ciphertexts")
	}
	dec, err := c.Decrypt(enc)
	if err != nil || dec != "secret-access-token" {
		t.Fatalf("got %q %v", dec, err)
	}
	if e, _ := c.Encrypt(""); e != "" {
		t.Fatal("empty should stay empty")
	}
}

func TestCipherRejectsTampering(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("token")
	raw, _ := base64.StdEncoding.DecodeString(enc[3:])
	raw[len(raw)-1] ^= 1
	tampered := "v1:" + base64.StdEncoding.EncodeToString(raw)
	for _, bad := range []string{tampered, "v2:" + enc[3:], "garbage", "v1:!!!"} {
		if _, err := c.Decrypt(bad); err != ErrDecrypt {
			t.Fatalf("expected ErrDecrypt for %q, got %v", bad, err)
		}
	}
	other, _ := NewCipher(bytes.Repeat([]byte{8}, 32))
	if _, err := other.Decrypt(enc); err != ErrDecrypt {
		t.Fatal("wrong key must fail")
	}
	if _, err := NewCipher([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestAPIKeyGeneration(t *testing.T) {
	k, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(k.Raw, "sk_live_") || !strings.HasPrefix(k.Raw, k.Prefix) || len(k.Prefix) != 16 {
		t.Fatalf("bad key %+v", k)
	}
	if k.Hash != SHA256Hex(k.Raw) || len(k.Hash) != 64 || strings.Contains(k.Hash, k.Raw) {
		t.Fatal("hash mismatch")
	}
	if !LooksLikeAPIKey(k.Raw) {
		t.Fatal("generated key fails format check")
	}
	k2, _ := GenerateAPIKey()
	if k2.Raw == k.Raw || k2.Hash == k.Hash {
		t.Fatal("keys not unique")
	}
	for _, bad := range []string{"", "sk_test_abc", k.Raw + "x", "sk_live_" + strings.Repeat("!", 48)} {
		if LooksLikeAPIKey(bad) {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestPKCEChallenge(t *testing.T) {
	// RFC 7636 Appendix B test vector.
	if got := PKCEChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("got %s", got)
	}
	tok, _ := RandomToken(32)
	if len(tok) != 43 {
		t.Fatalf("token len %d", len(tok))
	}
	if !ConstantTimeEqual("a", "a") || ConstantTimeEqual("a", "b") {
		t.Fatal("constant time equal wrong")
	}
}

func TestSubkeyIsDeterministicPerPurposeAndDiffersFromTheMaster(t *testing.T) {
	master := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	a, err := Subkey(master, "approval-fingerprint")
	if err != nil || len(a) != 32 {
		t.Fatalf("%v %d", err, len(a))
	}
	again, _ := Subkey(master, "approval-fingerprint")
	other, _ := Subkey(master, "something-else")
	if !bytes.Equal(a, again) || bytes.Equal(a, other) || bytes.Equal(a, bytes.Repeat([]byte{7}, 32)) {
		t.Fatal("subkeys must be stable, purpose-bound and not the master key")
	}
	if _, err := Subkey("not base64!", "x"); err == nil {
		t.Fatal("a bad master key must fail")
	}
}
