// Package githubfake is an in-process GitHub for tests: the OAuth token endpoint plus /user and /user/emails
// (with pagination), and switches for the answers a real GitHub gives in odd situations.
package githubfake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// Email is one entry of /user/emails.
type Email struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// Account is what the fake knows about a user.
type Account struct {
	ID     int64
	Login  string
	Name   string
	Emails []Email
	// PublicEmail is the profile email, which the adapter must ignore (it can be unverified or someone else's).
	PublicEmail string
	// PKCEChallenge, when set, is checked against the S256 challenge of the verifier at the token endpoint.
	PKCEChallenge string
	// Faults.
	TokenError      bool // 200 with {"error":"bad_verification_code"}, as GitHub does
	UserStatus      int  // non-zero: /user answers this status
	EmailsStatus    int  // non-zero: /user/emails answers this status
	EmailsBadJSON   bool
	EmailsEndlessly bool   // every page claims another page follows
	LinkOffHost     string // non-empty: the Link header's next page points at this base URL instead of the fake
}

// Fake is a running fake GitHub.
type Fake struct {
	Server       *httptest.Server
	ClientID     string
	ClientSecret string
	// PageSize is how many emails /user/emails returns per page (default 2, small on purpose).
	PageSize int

	mu       sync.Mutex
	codes    map[string]*Account
	tokens   map[string]*Account
	n        int
	Requests []string // "METHOD path" of every request, in order
}

// New starts the fake and stops it when the test ends.
func New(t testing.TB) *Fake {
	f := &Fake{ClientID: "gh-client-id", ClientSecret: "gh-client-secret", PageSize: 2, codes: map[string]*Account{}, tokens: map[string]*Account{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", f.token)
	mux.HandleFunc("/user", f.user)
	mux.HandleFunc("/user/emails", f.emails)
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.Requests = append(f.Requests, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.Server.Close)
	return f
}

// AuthURL and TokenURL are the OAuth endpoints; APIURL is the REST base.
func (f *Fake) AuthURL() string  { return f.Server.URL + "/login/oauth/authorize" }
func (f *Fake) TokenURL() string { return f.Server.URL + "/login/oauth/access_token" }
func (f *Fake) APIURL() string   { return f.Server.URL }

// Code registers an account and returns the code that redeems it once.
func (f *Fake) Code(a *Account) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	code := fmt.Sprintf("gh-code-%d", f.n)
	f.codes[code] = a
	return code
}

func (f *Fake) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if r.PostForm.Get("client_id") != f.ClientID || r.PostForm.Get("client_secret") != f.ClientSecret {
		reply(w, r, map[string]string{"error": "incorrect_client_credentials"})
		return
	}
	f.mu.Lock()
	a, ok := f.codes[r.PostForm.Get("code")]
	delete(f.codes, r.PostForm.Get("code"))
	f.mu.Unlock()
	if !ok || a.TokenError {
		reply(w, r, map[string]string{"error": "bad_verification_code"})
		return
	}
	if a.PKCEChallenge != "" {
		if crypto.PKCEChallenge(r.PostForm.Get("code_verifier")) != a.PKCEChallenge {
			reply(w, r, map[string]string{"error": "bad_verification_code", "error_description": "PKCE"})
			return
		}
	}
	f.mu.Lock()
	f.n++
	tok := fmt.Sprintf("gho_faketoken%d", f.n) // gitleaks:allow (fake test value)
	f.tokens[tok] = a
	f.mu.Unlock()
	reply(w, r, map[string]string{"access_token": tok, "token_type": "bearer", "scope": "user:email"})
}

// reply answers like GitHub: form-encoded unless the client asks for JSON.
func reply(w http.ResponseWriter, r *http.Request, v map[string]string) {
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
		return
	}
	q := url.Values{}
	for k, val := range v {
		q.Set(k, val)
	}
	w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
	_, _ = w.Write([]byte(q.Encode()))
}

func (f *Fake) account(w http.ResponseWriter, r *http.Request) *Account {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	f.mu.Lock()
	a := f.tokens[h[min(len(prefix), len(h)):]]
	f.mu.Unlock()
	if len(h) < len(prefix) || h[:len(prefix)] != prefix || a == nil {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return nil
	}
	return a
}

func (f *Fake) user(w http.ResponseWriter, r *http.Request) {
	a := f.account(w, r)
	if a == nil {
		return
	}
	if a.UserStatus != 0 {
		http.Error(w, `{"message":"boom"}`, a.UserStatus)
		return
	}
	body := map[string]any{"id": a.ID, "login": a.Login, "name": a.Name}
	if a.PublicEmail != "" {
		body["email"] = a.PublicEmail
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (f *Fake) emails(w http.ResponseWriter, r *http.Request) {
	a := f.account(w, r)
	if a == nil {
		return
	}
	if a.EmailsStatus != 0 {
		http.Error(w, `{"message":"boom"}`, a.EmailsStatus)
		return
	}
	if a.EmailsBadJSON {
		_, _ = w.Write([]byte("<html>"))
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(page, 1)
	size := f.PageSize
	if pp, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && pp > 0 && pp < size {
		size = pp
	}
	lo, hi := min((page-1)*size, len(a.Emails)), min(page*size, len(a.Emails))
	if hi < len(a.Emails) || a.EmailsEndlessly {
		next := *r.URL
		q := next.Query()
		q.Set("page", strconv.Itoa(page+1))
		next.RawQuery = q.Encode()
		base := f.Server.URL
		if a.LinkOffHost != "" {
			base = a.LinkOffHost
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s%s>; rel="next"`, base, next.RequestURI()))
	}
	out := a.Emails[lo:hi]
	if a.EmailsEndlessly {
		out = a.Emails
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
