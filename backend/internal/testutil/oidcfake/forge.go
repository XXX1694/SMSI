package oidcfake

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"

	"github.com/go-jose/go-jose/v4"
)

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

// sign produces the compact JWT for payload, forged the way fault says (FaultNone: a properly signed RS256 token).
func (f *Fake) sign(fault Fault, payload []byte) string {
	f.mu.Lock()
	key, kid := f.key, f.kid
	f.mu.Unlock()
	switch fault {
	case FaultAlgNone:
		return b64(`{"alg":"none","typ":"JWT"}`) + "." + b64(string(payload)) + "."
	case FaultHS256PubKey:
		return f.hs256WithPublicKey(&key.PublicKey, kid, payload)
	case FaultUnknownKey:
		other, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			f.t.Fatal(err)
		}
		key, kid = other, "never-published"
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: kid}}, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	jws, err := signer.Sign(payload)
	if err != nil {
		f.t.Fatal(err)
	}
	compact, err := jws.CompactSerialize()
	if err != nil {
		f.t.Fatal(err)
	}
	if fault == FaultBadSignature {
		return f.swapPayload(compact)
	}
	return compact
}

// hs256WithPublicKey is the classic algorithm-confusion token: HMAC-SHA256 keyed with the PEM of the public key.
func (f *Fake) hs256WithPublicKey(pub *rsa.PublicKey, kid string, payload []byte) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		f.t.Fatal(err)
	}
	secret := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	signing := b64(`{"alg":"HS256","typ":"JWT","kid":"`+kid+`"}`) + "." + b64(string(payload))
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	return signing + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// swapPayload keeps the header and signature of a token and replaces its claims by another subject's.
func (f *Fake) swapPayload(compact string) string {
	parts := strings.Split(compact, ".")
	forged, err := json.Marshal(map[string]any{"iss": f.Issuer(), "aud": f.ClientID, "sub": "attacker", "exp": f.Now().Add(3600e9).Unix()})
	if err != nil {
		f.t.Fatal(err)
	}
	return parts[0] + "." + b64(string(forged)) + "." + parts[2]
}
