package config

import (
	"strings"
	"testing"
)

func TestSignInProvidersAreOffByDefault(t *testing.T) {
	validEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.GoogleSignIn() || c.GitHubSignIn() {
		t.Fatalf("providers must be disabled without credentials: %+v", c.SignInConfig)
	}
}

func TestSignInProviderNeedsAWholePair(t *testing.T) {
	for _, tc := range []struct{ id, secret, name string }{
		{"GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET", "GOOGLE"},
		{"GITHUB_CLIENT_ID", "GITHUB_CLIENT_SECRET", "GITHUB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validEnv(t)
			t.Setenv(tc.id, "the-id")
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tc.id) {
				t.Fatalf("id without secret must fail, got %v", err)
			}
			t.Setenv(tc.id, "")
			t.Setenv(tc.secret, "the-secret") // gitleaks:allow (fake test value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tc.id) {
				t.Fatalf("secret without id must fail, got %v", err)
			}
			t.Setenv(tc.id, "the-id")
			c, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if c.GoogleSignIn() != (tc.name == "GOOGLE") || c.GitHubSignIn() != (tc.name == "GITHUB") {
				t.Fatalf("only %s may be enabled: %+v", tc.name, c.SignInConfig)
			}
		})
	}
}

func TestSignInWhitespaceOnlyCountsAsEmpty(t *testing.T) {
	validEnv(t)
	t.Setenv("GITHUB_CLIENT_ID", "  ")
	t.Setenv("GITHUB_CLIENT_SECRET", "")
	if c, err := Load(); err != nil || c.GitHubSignIn() {
		t.Fatalf("%v %v", c, err)
	}
}
