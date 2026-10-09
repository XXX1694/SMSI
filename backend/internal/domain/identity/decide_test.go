package identity

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAuthoritativeEmail(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    Claims
		want bool
	}{
		{"google gmail verified", Claims{Provider: Google, Issuer: GoogleIssuer, Email: "a@gmail.com", EmailVerified: true}, true},
		{"google gmail uppercase", Claims{Provider: Google, Issuer: GoogleIssuer, Email: "A@GMAIL.COM", EmailVerified: true}, true},
		{"google gmail unverified", Claims{Provider: Google, Issuer: GoogleIssuer, Email: "a@gmail.com"}, false},
		{"google workspace verified with hd", Claims{Provider: Google, Issuer: GoogleIssuer, Email: "a@corp.example", EmailVerified: true, HostedDomain: "corp.example"}, true},
		{"google custom domain verified without hd", Claims{Provider: Google, Issuer: GoogleIssuer, Email: "a@corp.example", EmailVerified: true}, false},
		{"google workspace unverified with hd", Claims{Provider: Google, Issuer: GoogleIssuer, Email: "a@corp.example", HostedDomain: "corp.example"}, false},
		{"google-labelled provider with another issuer is never authoritative", Claims{Provider: Google, Issuer: "https://keycloak.example/realms/x", Email: "a@gmail.com", EmailVerified: true, HostedDomain: "x"}, false},
		{"google without issuer is not authoritative", Claims{Provider: Google, Email: "a@gmail.com", EmailVerified: true}, false},
		{"google lookalike of gmail", Claims{Provider: Google, Issuer: GoogleIssuer, Email: "a@notgmail.com", EmailVerified: true}, false},
		{"google gmail as a local part", Claims{Provider: Google, Issuer: GoogleIssuer, Email: "a@gmail.com.evil.test", EmailVerified: true}, false},
		{"google empty email", Claims{Provider: Google, Issuer: GoogleIssuer, EmailVerified: true, HostedDomain: "x"}, false},
		{"github primary verified", Claims{Provider: GitHub, Email: "a@x.test", EmailVerified: true, EmailPrimary: true}, true},
		{"github verified not primary", Claims{Provider: GitHub, Email: "a@x.test", EmailVerified: true}, false},
		{"github primary unverified", Claims{Provider: GitHub, Email: "a@x.test", EmailPrimary: true}, false},
		{"github hd is meaningless", Claims{Provider: GitHub, Email: "a@x.test", EmailVerified: true, HostedDomain: "x"}, false},
		{"unknown provider", Claims{Provider: "evil", Email: "a@gmail.com", EmailVerified: true, EmailPrimary: true}, false},
	} {
		if got := tc.c.AuthoritativeEmail(); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDecide(t *testing.T) {
	owner, other := uuid.New(), uuid.New()
	active := &Owner{UserID: owner, Active: true}
	inactive := &Owner{UserID: owner}
	verifiedLocal := &Owner{UserID: other, Active: true, EmailVerified: true}
	unverifiedLocal := &Owner{UserID: other, Active: true}
	disabledLocal := &Owner{UserID: other, EmailVerified: true}

	gmail := Claims{Provider: Google, Issuer: GoogleIssuer, Subject: "g1", Email: "a@gmail.com", EmailVerified: true}
	workspace := Claims{Provider: Google, Issuer: GoogleIssuer, Subject: "g1", Email: "a@corp.example", EmailVerified: true, HostedDomain: "corp.example"}
	customNoHD := Claims{Provider: Google, Issuer: GoogleIssuer, Subject: "g1", Email: "a@corp.example", EmailVerified: true}
	ghPrimary := Claims{Provider: GitHub, Subject: "42", Email: "a@x.test", EmailVerified: true, EmailPrimary: true}
	ghSecondary := Claims{Provider: GitHub, Subject: "42", Email: "a@x.test", EmailVerified: true}
	ghNoEmail := Claims{Provider: GitHub, Subject: "42"}
	ghUnverified := Claims{Provider: GitHub, Subject: "42", Email: "a@x.test", EmailPrimary: true}

	for _, tc := range []struct {
		name       string
		claims     Claims
		byIdentity *Owner
		byEmail    *Owner
		want       Decision
	}{
		// Rule 1: a known identity wins over everything else.
		{"known identity signs in", gmail, active, nil, Decision{Action: SignIn, UserID: owner}},
		{"known identity signs in although email is now unverified", ghUnverified, active, nil, Decision{Action: SignIn, UserID: owner}},
		{"known identity signs in although email now belongs to another account", gmail, active, verifiedLocal, Decision{Action: SignIn, UserID: owner}},
		{"known identity of an inactive user is refused", gmail, inactive, nil, Decision{Action: Refuse, Reason: ReasonAccountUnavailable}},

		// Rule 2: auto-link needs an authoritative provider email and a verified local email.
		{"google gmail links a verified local account", gmail, nil, verifiedLocal, Decision{Action: LinkAndSignIn, UserID: other}},
		{"google workspace links a verified local account", workspace, nil, verifiedLocal, Decision{Action: LinkAndSignIn, UserID: other}},
		{"github primary verified links a verified local account", ghPrimary, nil, verifiedLocal, Decision{Action: LinkAndSignIn, UserID: other}},
		{"unverified local account is never linked (pre-hijacking)", gmail, nil, unverifiedLocal, Decision{Action: Refuse, Reason: ReasonAccountExists}},
		{"unverified local account is never linked, github", ghPrimary, nil, unverifiedLocal, Decision{Action: Refuse, Reason: ReasonAccountExists}},
		{"google custom domain without hd is not authoritative", customNoHD, nil, verifiedLocal, Decision{Action: Refuse, Reason: ReasonAccountExists}},
		{"github non-primary is not authoritative", ghSecondary, nil, verifiedLocal, Decision{Action: Refuse, Reason: ReasonAccountExists}},
		{"unverified provider email never links", ghUnverified, nil, verifiedLocal, Decision{Action: Refuse, Reason: ReasonAccountExists}},
		{"disabled local account is refused", gmail, nil, disabledLocal, Decision{Action: Refuse, Reason: ReasonAccountUnavailable}},

		// Rule 3: no match starts a sign-up, only with a verified email.
		{"new user, authoritative email", gmail, nil, nil, Decision{Action: SignUp}},
		{"new user, github primary", ghPrimary, nil, nil, Decision{Action: SignUp}},
		{"new user, verified but not authoritative email is refused (custom domain can change hands)", customNoHD, nil, nil, Decision{Action: Refuse, Reason: ReasonEmailUnverified}},
		{"new user, github non-primary verified email is refused", ghSecondary, nil, nil, Decision{Action: Refuse, Reason: ReasonEmailUnverified}},
		{"new user without any email", ghNoEmail, nil, nil, Decision{Action: Refuse, Reason: ReasonEmailUnverified}},
		{"new user with unverified email", ghUnverified, nil, nil, Decision{Action: Refuse, Reason: ReasonEmailUnverified}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(tc.claims, tc.byIdentity, tc.byEmail); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestReasonMessagesNameTheNextStep(t *testing.T) {
	if m := ReasonEmailUnverified.Message(GitHub); !strings.Contains(m, "Verify your primary email on GitHub") {
		t.Fatalf("github: %q", m)
	}
	if m := ReasonAccountExists.Message(Google); !strings.Contains(m, "Sign in with your password") || !strings.Contains(m, "Google") {
		t.Fatalf("exists: %q", m)
	}
	for _, r := range []Reason{ReasonEmailUnverified, ReasonAccountExists, ReasonAccountUnavailable, ReasonNone} {
		if r.Message(Google) == "" {
			t.Fatalf("no message for %q", r)
		}
	}
}
