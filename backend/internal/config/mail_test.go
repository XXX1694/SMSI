package config

import (
	"strings"
	"testing"
)

func TestMailConfigValidation(t *testing.T) {
	smtpOK := map[string]string{"MAIL_PROVIDER": "smtp", "SMTP_HOST": "smtp.resend.com", "SMTP_PORT": "465", "SMTP_TLS": "implicit",
		"SMTP_USERNAME": "resend", "SMTP_PASSWORD": "re_test_key", "MAIL_FROM": "SocialOS <no-reply@example.com>"}
	with := func(over map[string]string) map[string]string {
		m := map[string]string{}
		for k, v := range smtpOK {
			m[k] = v
		}
		for k, v := range over {
			m[k] = v
		}
		return m
	}
	for _, tc := range []struct {
		name    string
		env     map[string]string
		wantErr string // substring; empty = valid
	}{
		{"default is log", nil, ""},
		{"explicit log", map[string]string{"MAIL_PROVIDER": "log"}, ""},
		{"provider case-insensitive", with(map[string]string{"MAIL_PROVIDER": "SMTP"}), ""},
		{"unknown provider", map[string]string{"MAIL_PROVIDER": "sendgrid"}, "MAIL_PROVIDER must be log or smtp"},
		{"smtp complete (resend 465)", smtpOK, ""},
		{"smtp starttls 587 default", with(map[string]string{"SMTP_TLS": "", "SMTP_PORT": ""}), ""},
		{"smtp missing host", with(map[string]string{"SMTP_HOST": ""}), "SMTP_HOST is required"},
		{"smtp missing username", with(map[string]string{"SMTP_USERNAME": ""}), "SMTP_USERNAME is required"},
		{"smtp missing password", with(map[string]string{"SMTP_PASSWORD": ""}), "SMTP_PASSWORD is required"},
		{"smtp missing from", with(map[string]string{"MAIL_FROM": ""}), "MAIL_FROM is required"},
		{"smtp bad from", with(map[string]string{"MAIL_FROM": "not-an-address"}), "MAIL_FROM must be an email address"},
		{"smtp bad tls", with(map[string]string{"SMTP_TLS": "none"}), "SMTP_TLS must be starttls or implicit"},
		{"smtp bad port", with(map[string]string{"SMTP_PORT": "70000"}), "SMTP_PORT must be 1-65535"},
		{"smtp fields ignored with log", map[string]string{"SMTP_TLS": "none", "MAIL_FROM": "x"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			c, err := Load()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			case err == nil && c.MailProvider == "":
				t.Fatal("provider must default to log")
			}
		})
	}
}

func TestMailDefaults(t *testing.T) {
	validEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.MailProvider != MailProviderLog || c.SMTPPort != 587 || c.SMTPTLS != SMTPTLSStartTLS {
		t.Fatalf("defaults: %+v", c)
	}
}

func TestWarningsLogProviderInProduction(t *testing.T) {
	for _, tc := range []struct {
		env, provider string
		want          bool
	}{{"production", "log", true}, {"development", "log", false}, {"production", "smtp", false}} {
		validEnv(t)
		t.Setenv("APP_ENV", tc.env)
		t.Setenv("MAIL_PROVIDER", tc.provider)
		if tc.env == "production" {
			t.Setenv("COOKIE_SECURE", "true")
			t.Setenv("STORAGE_DRIVER", "s3")
			t.Setenv("SMTP_HOST", "h")
			t.Setenv("SMTP_USERNAME", "u")
			t.Setenv("SMTP_PASSWORD", "p")
			t.Setenv("MAIL_FROM", "a@example.com")
		}
		c, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		got := false
		for _, w := range c.Warnings {
			got = got || strings.Contains(w, "MAIL_PROVIDER=log")
		}
		if got != tc.want {
			t.Errorf("%s/%s: warning = %v, want %v", tc.env, tc.provider, got, tc.want)
		}
	}
}
