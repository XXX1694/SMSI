package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func validEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
}

func TestLoadDefaults(t *testing.T) {
	validEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Env != "development" || c.HTTPAddr != ":8080" || c.QueueName != "publish" || c.StorageDriver != "s3" ||
		c.SessionTTL != 7*24*time.Hour || c.WorkerConc != 10 || c.CookieSecure || c.MockProviders || c.TrustProxy {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.RateLimitRPS != 10 || c.RateLimitBurst != 40 || c.AuthRateBurst != 10 || c.MigrateOnStart {
		t.Fatalf("rate limit defaults: %+v", c)
	}
	if len(c.CORSOrigins) != 1 || c.CORSOrigins[0] != "http://localhost:3000" {
		t.Fatalf("cors default: %v", c.CORSOrigins)
	}
	if c.Production() {
		t.Fatal("development is not production")
	}
}

func TestLoadParsesOverrides(t *testing.T) {
	validEnv(t)
	t.Setenv("APP_ENV", "staging")
	t.Setenv("API_PUBLIC_URL", "https://api.example.com///")
	t.Setenv("WEB_BASE_URL", "https://app.example.com/")
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://app.example.com/ , ,https://admin.example.com")
	t.Setenv("SESSION_TTL", "90m")
	t.Setenv("RATE_LIMIT_RPS", "2.5")
	t.Setenv("WORKER_CONCURRENCY", "3")
	t.Setenv("SOCIAL_MOCK_PROVIDERS", "true")
	t.Setenv("TRUST_PROXY", "1")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.APIPublicURL != "https://api.example.com" || c.WebBaseURL != "https://app.example.com" {
		t.Fatalf("trailing slashes must be trimmed: %q %q", c.APIPublicURL, c.WebBaseURL)
	}
	if got := strings.Join(c.CORSOrigins, "|"); got != "https://app.example.com|https://admin.example.com" {
		t.Fatalf("origins: %q", got)
	}
	if c.SessionTTL != 90*time.Minute || c.RateLimitRPS != 2.5 || c.WorkerConc != 3 || !c.MockProviders || !c.TrustProxy {
		t.Fatalf("parsed values: %+v", c)
	}
}

func TestLoadFallsBackOnMalformedNumbers(t *testing.T) {
	validEnv(t)
	t.Setenv("WORKER_CONCURRENCY", "many")
	t.Setenv("SESSION_TTL", "a week")
	t.Setenv("RATE_LIMIT_RPS", "fast")
	t.Setenv("COOKIE_SECURE", "maybe")
	t.Setenv("HTTP_ADDR", "   ")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.WorkerConc != 10 || c.SessionTTL != 7*24*time.Hour || c.RateLimitRPS != 10 || c.CookieSecure || c.HTTPAddr != ":8080" {
		t.Fatalf("malformed values must fall back to defaults: %+v", c)
	}
}

func TestValidate(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(make([]byte, 32))
	for name, tc := range map[string]struct {
		env  map[string]string
		want string // substring of the error, "" for success
	}{
		"ok":                    {map[string]string{}, ""},
		"missing database":      {map[string]string{"DATABASE_URL": " "}, "DATABASE_URL is required"},
		"missing key":           {map[string]string{"ENCRYPTION_KEY": ""}, "ENCRYPTION_KEY"},
		"key not base64":        {map[string]string{"ENCRYPTION_KEY": "%%%not base64%%%"}, "ENCRYPTION_KEY"},
		"key too short":         {map[string]string{"ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 16))}, "ENCRYPTION_KEY"},
		"key too long":          {map[string]string{"ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 33))}, "ENCRYPTION_KEY"},
		"wildcard cors":         {map[string]string{"CORS_ALLOWED_ORIGINS": "https://a.example,*"}, "CORS_ALLOWED_ORIGINS"},
		"bad storage driver":    {map[string]string{"STORAGE_DRIVER": "ftp"}, "STORAGE_DRIVER must be"},
		"prod insecure cookies": {map[string]string{"APP_ENV": "production"}, "COOKIE_SECURE"},
		"prod memory storage":   {map[string]string{"APP_ENV": "production", "COOKIE_SECURE": "true", "STORAGE_DRIVER": "memory"}, "not allowed in production"},
		"prod mock providers":   {map[string]string{"APP_ENV": "production", "COOKIE_SECURE": "true", "SOCIAL_MOCK_PROVIDERS": "true"}, "SOCIAL_MOCK_PROVIDERS"},
		"prod ok":               {map[string]string{"APP_ENV": "production", "COOKIE_SECURE": "true"}, ""},
		"dev may use memory":    {map[string]string{"STORAGE_DRIVER": "memory", "SOCIAL_MOCK_PROVIDERS": "true"}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			t.Setenv("ENCRYPTION_KEY", good)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			c, err := Load()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
			if c == nil {
				t.Fatal("Load should still return the config alongside the error")
			}
		})
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("APP_ENV", "production")
	_, err := Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"DATABASE_URL", "ENCRYPTION_KEY", "COOKIE_SECURE"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s: %v", want, err)
		}
	}
}

func TestErrorNeverEchoesSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://admin:hunter2@db/x")
	t.Setenv("ENCRYPTION_KEY", "super-secret-but-invalid")
	_, err := Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("configuration error leaks a secret: %v", err)
	}
}

func TestTelegramUpdatesModeDefaultsToPolling(t *testing.T) {
	validEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.TelegramUpdatesMode != TelegramModePolling || c.TelegramWebhookSecret != "" {
		t.Fatalf("defaults: %q %q", c.TelegramUpdatesMode, c.TelegramWebhookSecret)
	}
	t.Setenv("TELEGRAM_BOT_TOKEN", "1:abc")
	if _, err := Load(); err != nil {
		t.Fatalf("polling needs no webhook secret: %v", err)
	}
}

func TestTelegramWebhookModeNeedsAValidSecret(t *testing.T) {
	validEnv(t)
	t.Setenv("TELEGRAM_UPDATES_MODE", "WEBHOOK") // case-insensitive
	if _, err := Load(); err != nil {
		t.Fatalf("webhook mode without a bot token is harmless (telegram is off): %v", err)
	}
	t.Setenv("TELEGRAM_BOT_TOKEN", "1:abc")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TELEGRAM_WEBHOOK_SECRET is required") {
		t.Fatalf("missing secret: %v", err)
	}
	for _, bad := range []string{"has space", "semi;colon", strings.Repeat("a", 257)} {
		t.Setenv("TELEGRAM_WEBHOOK_SECRET", bad)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TELEGRAM_WEBHOOK_SECRET must be") {
			t.Fatalf("secret %.20q: %v", bad, err)
		}
	}
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "short-dev-secret")
	c, err := Load()
	if err != nil || c.TelegramUpdatesMode != TelegramModeWebhook || c.TelegramWebhookSecret != "short-dev-secret" {
		t.Fatalf("%+v %v", c, err)
	}
	// Production demands a long secret.
	t.Setenv("APP_ENV", "production")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("STORAGE_DRIVER", "s3")
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "tooshort")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "at least 16") {
		t.Fatalf("production short secret: %v", err)
	}
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "0123456789abcdef0123456789abcdef")
	if _, err := Load(); err != nil {
		t.Fatalf("production long secret: %v", err)
	}
}

func TestTelegramUpdatesModeRejectsUnknownValues(t *testing.T) {
	validEnv(t)
	t.Setenv("TELEGRAM_UPDATES_MODE", "carrier-pigeon")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TELEGRAM_UPDATES_MODE must be polling or webhook") {
		t.Fatalf("got %v", err)
	}
}

func TestMCPGatewaySecret(t *testing.T) {
	validEnv(t)
	if c, err := Load(); err != nil || c.GatewaySecret != "" {
		t.Fatalf("unset secret is valid and means never trust the header: %v %q", err, c.GatewaySecret)
	}
	t.Setenv("MCP_GATEWAY_SECRET", "short")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MCP_GATEWAY_SECRET") {
		t.Fatalf("a short secret must stop startup: %v", err)
	}
	t.Setenv("MCP_GATEWAY_SECRET", strings.Repeat("a", 32))
	if c, err := Load(); err != nil || c.GatewaySecret == "" {
		t.Fatalf("32 chars are fine: %v", err)
	}
}

func TestApprovalSettingsDefaultAndAreValidated(t *testing.T) {
	validEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AgentMinScheduleLead != 5*time.Minute || c.ApprovalTTL != 10*time.Minute || c.ApprovalMaxPending != 10 {
		t.Fatalf("defaults: %v %v %d", c.AgentMinScheduleLead, c.ApprovalTTL, c.ApprovalMaxPending)
	}
	t.Setenv("AGENT_MIN_SCHEDULE_LEAD", "0s") // 0 switches the rule off
	t.Setenv("APPROVAL_TTL", "30m")
	t.Setenv("APPROVAL_MAX_PENDING", "3")
	if c, err = Load(); err != nil || c.AgentMinScheduleLead != 0 || c.ApprovalTTL != 30*time.Minute || c.ApprovalMaxPending != 3 {
		t.Fatalf("overrides: %+v %v", c, err)
	}
	for env, val := range map[string]string{"AGENT_MIN_SCHEDULE_LEAD": "-1m", "APPROVAL_TTL": "10s", "APPROVAL_MAX_PENDING": "0"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(env, val)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), env) {
				t.Fatalf("%s=%s should be rejected, got %v", env, val, err)
			}
		})
	}
}
