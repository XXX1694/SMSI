package github_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/socialos/backend/internal/adapters/identity/github"
	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/infrastructure/crypto"
	"github.com/socialos/backend/internal/testutil/githubfake"
)

const (
	verifier    = "verifier-0123456789012345678901234567890123456789"
	redirectURI = "https://api.example.test/api/v1/auth/oauth/github/callback"
)

func setup(t *testing.T) (*githubfake.Fake, *github.Adapter) {
	t.Helper()
	f := githubfake.New(t)
	a, err := github.New(github.Config{ClientID: f.ClientID, ClientSecret: f.ClientSecret, AuthURL: f.AuthURL(), TokenURL: f.TokenURL(), APIURL: f.APIURL()})
	if err != nil {
		t.Fatal(err)
	}
	return f, a
}

func account(emails ...githubfake.Email) *githubfake.Account {
	return &githubfake.Account{ID: 4242, Login: "ann", Name: "Ann A", Emails: emails, PKCEChallenge: crypto.PKCEChallenge(verifier),
		PublicEmail: "public@evil.test"}
}

func exchange(a *github.Adapter, code string) (identity.Claims, error) {
	return a.Exchange(context.Background(), auth.ExchangeRequest{Code: code, CodeVerifier: verifier, RedirectURI: redirectURI})
}

func TestPrimaryVerifiedEmailIsAuthoritative(t *testing.T) {
	f, a := setup(t)
	acc := account(githubfake.Email{Email: "Other@x.test", Verified: true}, githubfake.Email{Email: "Ann@X.test", Primary: true, Verified: true})
	c, err := exchange(a, f.Code(acc))
	if err != nil {
		t.Fatal(err)
	}
	want := identity.Claims{Provider: identity.GitHub, Subject: "4242", Email: "ann@x.test", EmailVerified: true, EmailPrimary: true, DisplayName: "Ann A"}
	if c != want || !c.AuthoritativeEmail() {
		t.Fatalf("got %+v", c)
	}
}

func TestPrimaryOnALaterPageIsFound(t *testing.T) {
	f, a := setup(t)
	acc := account(githubfake.Email{Email: "a@x.test", Verified: true}, githubfake.Email{Email: "b@x.test", Verified: true},
		githubfake.Email{Email: "c@x.test", Verified: true}, githubfake.Email{Email: "d@x.test", Verified: true},
		githubfake.Email{Email: "main@x.test", Primary: true, Verified: true})
	c, err := exchange(a, f.Code(acc))
	if err != nil || c.Email != "main@x.test" || !c.EmailVerified {
		t.Fatalf("%+v %v", c, err)
	}
	pages := 0
	for _, r := range f.Requests {
		if r == "GET /user/emails" {
			pages++
		}
	}
	if pages != 3 {
		t.Fatalf("followed %d pages, want 3: %v", pages, f.Requests)
	}
}

func TestUnverifiedOrMissingPrimaryIsNotAuthoritative(t *testing.T) {
	for name, emails := range map[string][]githubfake.Email{
		"primary unverified":       {{Email: "p@x.test", Primary: true}, {Email: "v@x.test", Verified: true}},
		"verified but not primary": {{Email: "v@x.test", Verified: true}},
		"no addresses":             nil,
	} {
		t.Run(name, func(t *testing.T) {
			f, a := setup(t)
			c, err := exchange(a, f.Code(account(emails...)))
			if err != nil {
				t.Fatal(err)
			}
			if c.EmailVerified || c.AuthoritativeEmail() || strings.Contains(c.Email, "public@evil") || c.Subject != "4242" {
				t.Fatalf("%+v", c)
			}
			if name == "primary unverified" && c.Email != "p@x.test" {
				t.Fatalf("the primary address should be reported (unverified): %+v", c)
			}
		})
	}
}

func TestNameFallsBackToLogin(t *testing.T) {
	f, a := setup(t)
	acc := account(githubfake.Email{Email: "p@x.test", Primary: true, Verified: true})
	acc.Name = ""
	if c, err := exchange(a, f.Code(acc)); err != nil || c.DisplayName != "ann" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestFailuresAreProviderErrors(t *testing.T) {
	good := githubfake.Email{Email: "p@x.test", Primary: true, Verified: true}
	for name, mut := range map[string]func(*githubfake.Account){
		"token error with 200": func(a *githubfake.Account) { a.TokenError = true },
		"user 500":             func(a *githubfake.Account) { a.UserStatus = 500 },
		"user 403":             func(a *githubfake.Account) { a.UserStatus = 403 },
		"emails 500":           func(a *githubfake.Account) { a.EmailsStatus = 500 },
		"emails bad json":      func(a *githubfake.Account) { a.EmailsBadJSON = true },
		"no id":                func(a *githubfake.Account) { a.ID = 0 },
		"wrong pkce":           func(a *githubfake.Account) { a.PKCEChallenge = crypto.PKCEChallenge("other") },
	} {
		t.Run(name, func(t *testing.T) {
			f, a := setup(t)
			acc := account(good)
			mut(acc)
			c, err := exchange(a, f.Code(acc))
			if !errs.Is(err, errs.ProviderError) || c != (identity.Claims{}) {
				t.Fatalf("want PROVIDER_ERROR, got %+v %v", c, err)
			}
			if strings.Contains(err.Error(), "gho_") {
				t.Fatalf("access token in the error: %v", err)
			}
		})
	}
}

func TestEndlessPaginationStops(t *testing.T) {
	f, a := setup(t)
	acc := account(githubfake.Email{Email: "x@x.test", Verified: true})
	acc.EmailsEndlessly = true
	c, err := exchange(a, f.Code(acc))
	if err != nil || c.Email != "" {
		t.Fatalf("%+v %v", c, err)
	}
	if n := len(f.Requests); n > 10 {
		t.Fatalf("%d requests: pagination is not bounded", n)
	}
}

func TestCodeIsSingleUseAndUnknownCodeFails(t *testing.T) {
	f, a := setup(t)
	code := f.Code(account(githubfake.Email{Email: "p@x.test", Primary: true, Verified: true}))
	if _, err := exchange(a, code); err != nil {
		t.Fatal(err)
	}
	if _, err := exchange(a, code); !errs.Is(err, errs.ProviderError) {
		t.Fatalf("code redeemed twice: %v", err)
	}
}

func TestAuthorizeURL(t *testing.T) {
	_, a := setup(t)
	u, err := url.Parse(a.AuthorizeURL(auth.AuthorizeRequest{State: "st", Nonce: "ignored", CodeVerifier: verifier, RedirectURI: redirectURI}))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	for k, want := range map[string]string{"response_type": "code", "client_id": "gh-client-id", "state": "st", "redirect_uri": redirectURI,
		"code_challenge": crypto.PKCEChallenge(verifier), "code_challenge_method": "S256", "scope": "user:email"} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
	if q.Has("nonce") || a.ID() != identity.GitHub {
		t.Fatal("github has no nonce and must identify as github")
	}
}

func TestNewRequiresCredentials(t *testing.T) {
	if _, err := github.New(github.Config{ClientID: "id"}); err == nil {
		t.Fatal("missing secret accepted")
	}
	if a, err := github.New(github.Config{ClientID: "id", ClientSecret: "s"}); err != nil || a == nil {
		t.Fatalf("defaults should point at github.com: %v", err)
	}
}
