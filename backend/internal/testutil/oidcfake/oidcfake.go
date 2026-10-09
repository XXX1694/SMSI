// Package oidcfake is an in-process OpenID Connect provider for tests: JWKS and token endpoints, with switches to
// make it misbehave the way a broken or hostile provider would. The adapter is configured statically, so there is
// no discovery document.
package oidcfake

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/socialos/backend/internal/infrastructure/crypto"
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
	// Nonce is the nonce the ID token echoes; tests set it to what the login sent.
	Nonce string
	Fault Fault
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
	// TokenRequests counts calls to the token endpoint; KeyFetches counts JWKS downloads.
	TokenRequests int
	KeyFetches    int

	t      testing.TB
	mu     sync.Mutex
	key    *rsa.PrivateKey
	kid    string
	keys   int
	grants map[string]Grant
}

// New starts the fake provider and stops it when the test ends.
func New(t testing.TB) *Fake {
	t.Helper()
	f := &Fake{t: t, ClientID: "fake-client-id", ClientSecret: "fake-client-secret", Now: time.Now, grants: map[string]Grant{}}
	f.Rotate()
	mux := http.NewServeMux()
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
	f.keys++
	f.key, f.kid = key, fmt.Sprintf("key-%d", f.keys)
}

// Code registers a grant and returns the authorization code that redeems it once.
func (f *Fake) Code(g Grant) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	code := "code-" + base64.RawURLEncoding.EncodeToString(b)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.grants[code] = g
	return code
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
	if r.PostForm.Get("client_id") != f.ClientID || r.PostForm.Get("client_secret") != f.ClientSecret {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}
	code := r.PostForm.Get("code")
	f.mu.Lock()
	g, ok := f.grants[code]
	delete(f.grants, code) // a code works once
	f.mu.Unlock()
	switch {
	case !ok || r.PostForm.Get("grant_type") != "authorization_code":
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	case g.CodeChallenge != "" && crypto.PKCEChallenge(r.PostForm.Get("code_verifier")) != g.CodeChallenge:
		http.Error(w, `{"error":"invalid_grant","error_description":"PKCE verification failed"}`, http.StatusBadRequest)
	case g.RedirectURI != "" && r.PostForm.Get("redirect_uri") != g.RedirectURI:
		http.Error(w, `{"error":"invalid_grant","error_description":"redirect_uri mismatch"}`, http.StatusBadRequest)
	case g.Fault == FaultTokenEndpoint:
		http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
	default:
		resp := map[string]any{"access_token": "fake-access-token", "token_type": "Bearer", "expires_in": 3600}
		if g.Fault != FaultNoIDToken {
			resp["id_token"] = f.idToken(g)
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, resp)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
