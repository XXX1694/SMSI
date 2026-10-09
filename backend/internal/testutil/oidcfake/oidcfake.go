// Package oidcfake is an in-process OpenID Connect provider for tests: discovery, JWKS and token endpoints, with
// switches to make it misbehave the way a broken or hostile provider would.
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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// Fault selects how the next ID token is wrong. FaultNone is a valid token.
type Fault string

const (
	FaultNone          Fault = ""
	FaultWrongAudience Fault = "wrong_audience"
	FaultWrongIssuer   Fault = "wrong_issuer"
	FaultExpired       Fault = "expired"
	FaultAlgNone       Fault = "alg_none"           // unsigned token with "alg":"none"
	FaultHS256PubKey   Fault = "hs256_public_key"   // HS256 token whose secret is the provider's public key (algorithm confusion)
	FaultWrongNonce    Fault = "wrong_nonce"        // validly signed, but for another login
	FaultNoNonce       Fault = "no_nonce"           // validly signed, nonce claim missing
	FaultUnknownKey    Fault = "unknown_key"        // signed by a key the JWKS never published
	FaultBadSignature  Fault = "bad_signature"      // right key id, payload altered after signing
	FaultNoIDToken     Fault = "no_id_token"        // token response without id_token
	FaultNoSubject     Fault = "no_subject"         // signed, "sub" empty
	FaultTwoAudiences  Fault = "two_audiences"      // aud = [client, other] without azp
	FaultTokenEndpoint Fault = "token_endpoint_500" // the token endpoint fails
)

// Grant is what the provider will answer for one authorization code.
type Grant struct {
	Subject       string
	Email         string
	EmailVerified bool
	HostedDomain  string
	Name          string
	// Nonce is the nonce the ID token echoes; tests set it to what the login sent (or leave "" with FaultNoNonce).
	Nonce string
	// EmailVerifiedAsString makes the token carry "email_verified":"true" like some providers do.
	EmailVerifiedAsString bool
	Fault                 Fault
	// CodeChallenge, when set, is checked against the S256 challenge of the verifier presented at the token endpoint.
	CodeChallenge string
	// RedirectURI, when set, must match the redirect_uri of the token request.
	RedirectURI string
}

// Fake is a running fake provider.
type Fake struct {
	Server       *httptest.Server
	ClientID     string
	ClientSecret string
	// Now is the clock used for iat and exp.
	Now func() time.Time

	t      testing.TB
	mu     sync.Mutex
	key    *rsa.PrivateKey
	kid    string
	grants map[string]Grant
	seq    int
	// TokenRequests counts calls to the token endpoint; KeyFetches counts JWKS downloads.
	TokenRequests int
	KeyFetches    int
}

// New starts the fake provider and stops it when the test ends.
func New(t testing.TB) *Fake {
	t.Helper()
	f := &Fake{t: t, ClientID: "fake-client-id", ClientSecret: "fake-client-secret", Now: time.Now, grants: map[string]Grant{}}
	f.Rotate()
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", f.discovery)
	mux.HandleFunc("/jwks", f.jwks)
	mux.HandleFunc("/token", f.token)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)
	return f
}

// Issuer is the iss of valid tokens.
func (f *Fake) Issuer() string { return f.Server.URL }

// AuthURL is the authorization endpoint (nothing is served there; tests drive the callback themselves).
func (f *Fake) AuthURL() string { return f.Server.URL + "/authorize" }

// TokenURL is the token endpoint.
func (f *Fake) TokenURL() string { return f.Server.URL + "/token" }

// JWKSURL is the key set endpoint.
func (f *Fake) JWKSURL() string { return f.Server.URL + "/jwks" }

// Rotate replaces the published signing key. Tokens signed with the previous key stop verifying once a client
// refetches the key set.
func (f *Fake) Rotate() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	f.key, f.kid = key, fmt.Sprintf("key-%d", f.seq)
}

// Code registers a grant and returns the authorization code that redeems it once.
func (f *Fake) Code(g Grant) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	code := "code-" + randString()
	f.grants[code] = g
	return code
}

func randString() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (f *Fake) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"issuer": f.Issuer(), "authorization_endpoint": f.AuthURL(), "token_endpoint": f.TokenURL(), "jwks_uri": f.JWKSURL(),
		"id_token_signing_alg_values_supported": []string{"RS256"}, "subject_types_supported": []string{"public"},
		"response_types_supported": []string{"code"},
	})
}

func (f *Fake) jwks(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	f.KeyFetches++
	pub := jose.JSONWebKey{Key: &f.key.PublicKey, KeyID: f.kid, Algorithm: "RS256", Use: "sig"}
	f.mu.Unlock()
	writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{pub}})
}

func (f *Fake) token(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.TokenRequests++
	f.mu.Unlock()
	if err := r.ParseForm(); err != nil || r.Method != http.MethodPost {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}
	id, secret := r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	if u, p, ok := r.BasicAuth(); ok {
		id, secret = u, p
		if v, err := url.QueryUnescape(u); err == nil {
			id = v
		}
		if v, err := url.QueryUnescape(p); err == nil {
			secret = v
		}
	}
	if id != f.ClientID || secret != f.ClientSecret {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	g, ok := f.grants[r.PostForm.Get("code")]
	delete(f.grants, r.PostForm.Get("code")) // a code works once
	f.mu.Unlock()
	if !ok || r.PostForm.Get("grant_type") != "authorization_code" {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	if g.CodeChallenge != "" && s256(r.PostForm.Get("code_verifier")) != g.CodeChallenge {
		http.Error(w, `{"error":"invalid_grant","error_description":"PKCE verification failed"}`, http.StatusBadRequest)
		return
	}
	if g.RedirectURI != "" && r.PostForm.Get("redirect_uri") != g.RedirectURI {
		http.Error(w, `{"error":"invalid_grant","error_description":"redirect_uri mismatch"}`, http.StatusBadRequest)
		return
	}
	if g.Fault == FaultTokenEndpoint {
		http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
		return
	}
	resp := map[string]any{"access_token": "fake-access-token", "token_type": "Bearer", "expires_in": 3600}
	if g.Fault != FaultNoIDToken {
		resp["id_token"] = f.idToken(g)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, resp)
}

func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (f *Fake) idToken(g Grant) string {
	now := f.Now()
	claims := map[string]any{
		"iss": f.Issuer(), "aud": f.ClientID, "sub": g.Subject, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"nonce": g.Nonce, "email": g.Email, "email_verified": g.EmailVerified, "name": g.Name,
	}
	if g.EmailVerifiedAsString {
		claims["email_verified"] = map[bool]string{true: "true", false: "false"}[g.EmailVerified]
	}
	if g.HostedDomain != "" {
		claims["hd"] = g.HostedDomain
	}
	switch g.Fault {
	case FaultWrongAudience:
		claims["aud"] = "another-client"
	case FaultWrongIssuer:
		claims["iss"] = "https://evil.example"
	case FaultExpired:
		claims["iat"], claims["exp"] = now.Add(-2*time.Hour).Unix(), now.Add(-time.Hour).Unix()
	case FaultWrongNonce:
		claims["nonce"] = "nonce-of-another-login"
	case FaultNoNonce:
		delete(claims, "nonce")
	case FaultNoSubject:
		claims["sub"] = ""
	case FaultTwoAudiences:
		claims["aud"] = []string{f.ClientID, "another-client"}
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	key, kid := f.key, f.kid
	f.mu.Unlock()
	switch g.Fault {
	case FaultAlgNone:
		return b64(`{"alg":"none","typ":"JWT"}`) + "." + b64(string(payload)) + "."
	case FaultHS256PubKey:
		der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			f.t.Fatal(err)
		}
		secret := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
		head := b64(`{"alg":"HS256","typ":"JWT","kid":"` + kid + `"}`)
		signing := head + "." + b64(string(payload))
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(signing))
		return signing + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
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
	if g.Fault == FaultBadSignature {
		// Swap the payload for another one and keep the old signature.
		forged, _ := json.Marshal(map[string]any{"iss": f.Issuer(), "aud": f.ClientID, "sub": "attacker", "exp": now.Add(time.Hour).Unix(), "nonce": g.Nonce})
		parts := splitDots(compact)
		return parts[0] + "." + b64(string(forged)) + "." + parts[2]
	}
	return compact
}

func splitDots(s string) [3]string {
	var out [3]string
	n := 0
	start := 0
	for i := 0; i < len(s) && n < 2; i++ {
		if s[i] == '.' {
			out[n] = s[start:i]
			n++
			start = i + 1
		}
	}
	out[2] = s[start:]
	return out
}

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
