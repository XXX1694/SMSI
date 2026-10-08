// Package config loads configuration from environment variables.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the full runtime configuration (API + worker).
type Config struct {
	Env            string // development | production | test
	HTTPAddr       string
	DatabaseURL    string
	DBMaxConns     int32
	RedisURL       string
	QueueName      string
	EncryptionKey  string // base64 32 bytes
	APIPublicURL   string // public base of the API (OAuth redirect base)
	WebBaseURL     string // frontend base for post-connect redirects
	MCPPublicURL   string
	GatewaySecret  string // MCP_GATEWAY_SECRET, shared with the MCP server; empty = never trust X-SocialOS-Client-IP
	CORSOrigins    []string
	CookieSecure   bool
	CookieDomain   string
	SessionTTL     time.Duration
	MetricsToken   string
	MigrateOnStart bool
	LogLevel       string
	LogFormat      string // json | text
	MockProviders  bool
	RateLimitRPS   float64
	RateLimitBurst int
	AuthRateRPS    float64
	AuthRateBurst  int
	// PasswordHashConcurrency caps argon2 hash/verify operations running at once; PasswordHashMemoryMiB caps their
	// combined memory (an old 64 MiB hash runs alone).
	PasswordHashConcurrency int
	PasswordHashMemoryMiB   int
	TrustProxy              bool
	TrustedProxies          []netip.Prefix // networks whose X-Forwarded-For is believed; empty = ignore the header (see proxies.go)
	Warnings                []string       // valid but suspicious settings; the binaries log them at startup (LogWarnings)
	WorkerConc              int
	ReconcileEvery          time.Duration
	StorageDriver           string // s3 | memory
	S3Endpoint              string
	S3PublicURL             string
	S3AccessKey             string
	S3SecretKey             string
	S3Bucket                string
	S3Region                string
	S3UseSSL                bool
	S3AutoCreate            bool
	LinkedInID              string
	LinkedInSecret          string
	LinkedInVersion         string
	LinkedInPKCE            bool
	TelegramToken           string
	// TelegramUpdatesMode selects how the bot receives the messages that prove
	// chat ownership: "polling" (worker long-polls getUpdates, needs no public
	// URL; default) or "webhook" (Telegram calls POST /api/v1/webhooks/telegram).
	TelegramUpdatesMode string
	// TelegramWebhookSecret is the secret_token Telegram echoes in the
	// X-Telegram-Bot-Api-Secret-Token header; required in webhook mode.
	TelegramWebhookSecret string

	// MailProvider selects how transactional mail leaves the system: "log"
	// (default; writes to the log, nothing is sent) or "smtp".
	MailProvider string
	SMTPHost     string
	SMTPPort     int
	SMTPTLS      string // starttls (default, port 587) | implicit (TLS from the first byte, port 465)
	SMTPUsername string
	SMTPPassword string
	MailFrom     string // RFC 5322 address, e.g. "SocialOS <no-reply@example.com>"
}

// Mail providers and SMTP TLS modes.
const (
	MailProviderLog  = "log"
	MailProviderSMTP = "smtp"
	SMTPTLSStartTLS  = "starttls"
	SMTPTLSImplicit  = "implicit"
)

// Telegram update intake modes.
const (
	TelegramModePolling = "polling"
	TelegramModeWebhook = "webhook"
)

// Load reads and validates configuration.
func Load() (*Config, error) {
	c := &Config{
		Env:                     env("APP_ENV", "development"),
		HTTPAddr:                env("HTTP_ADDR", ":8080"),
		DatabaseURL:             env("DATABASE_URL", ""),
		DBMaxConns:              int32(envInt("DB_MAX_CONNS", 20)),
		RedisURL:                env("REDIS_URL", "redis://localhost:6379/0"),
		QueueName:               env("QUEUE_NAME", "publish"),
		EncryptionKey:           env("ENCRYPTION_KEY", ""),
		APIPublicURL:            strings.TrimRight(env("API_PUBLIC_URL", "http://localhost:8080"), "/"),
		WebBaseURL:              strings.TrimRight(env("WEB_BASE_URL", "http://localhost:3000"), "/"),
		MCPPublicURL:            env("MCP_PUBLIC_URL", "http://localhost:3333/mcp"),
		GatewaySecret:           env("MCP_GATEWAY_SECRET", ""),
		CORSOrigins:             list(env("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		CookieSecure:            envBool("COOKIE_SECURE", false),
		CookieDomain:            env("COOKIE_DOMAIN", ""),
		SessionTTL:              envDuration("SESSION_TTL", 7*24*time.Hour),
		MetricsToken:            env("METRICS_TOKEN", ""),
		MigrateOnStart:          envBool("MIGRATE_ON_START", false),
		LogLevel:                env("LOG_LEVEL", "info"),
		LogFormat:               env("LOG_FORMAT", "json"),
		MockProviders:           envBool("SOCIAL_MOCK_PROVIDERS", false),
		RateLimitRPS:            envFloat("RATE_LIMIT_RPS", 10),
		RateLimitBurst:          envInt("RATE_LIMIT_BURST", 40),
		AuthRateRPS:             envFloat("AUTH_RATE_LIMIT_RPS", 0.2),
		AuthRateBurst:           envInt("AUTH_RATE_LIMIT_BURST", 10),
		PasswordHashConcurrency: envInt("PASSWORD_HASH_CONCURRENCY", 2),
		PasswordHashMemoryMiB:   envInt("PASSWORD_HASH_MEMORY_MIB", 48),
		TrustProxy:              envBool("TRUST_PROXY", false),
		WorkerConc:              envInt("WORKER_CONCURRENCY", 10),
		ReconcileEvery:          envDuration("RECONCILE_INTERVAL", time.Minute),
		StorageDriver:           env("STORAGE_DRIVER", "s3"),
		S3Endpoint:              env("S3_ENDPOINT", "localhost:9000"),
		S3PublicURL:             env("S3_PUBLIC_ENDPOINT", ""),
		S3AccessKey:             env("S3_ACCESS_KEY", ""),
		S3SecretKey:             env("S3_SECRET_KEY", ""),
		S3Bucket:                env("S3_BUCKET", "socialos-media"),
		S3Region:                env("S3_REGION", "us-east-1"),
		S3UseSSL:                envBool("S3_USE_SSL", false),
		S3AutoCreate:            envBool("S3_AUTO_CREATE_BUCKET", true),
		LinkedInID:              env("LINKEDIN_CLIENT_ID", ""),
		LinkedInSecret:          env("LINKEDIN_CLIENT_SECRET", ""),
		LinkedInVersion:         env("LINKEDIN_API_VERSION", "202606"),
		LinkedInPKCE:            envBool("LINKEDIN_USE_PKCE", true),
		TelegramToken:           env("TELEGRAM_BOT_TOKEN", ""),

		TelegramUpdatesMode:   strings.ToLower(env("TELEGRAM_UPDATES_MODE", TelegramModePolling)),
		TelegramWebhookSecret: env("TELEGRAM_WEBHOOK_SECRET", ""),

		MailProvider: strings.ToLower(env("MAIL_PROVIDER", MailProviderLog)),
		SMTPHost:     env("SMTP_HOST", ""),
		SMTPPort:     envInt("SMTP_PORT", 587),
		SMTPTLS:      strings.ToLower(env("SMTP_TLS", SMTPTLSStartTLS)),
		SMTPUsername: env("SMTP_USERNAME", ""),
		SMTPPassword: env("SMTP_PASSWORD", ""),
		MailFrom:     env("MAIL_FROM", ""),
	}
	proxies, warnings, perr := resolveTrustedProxies(c.TrustProxy, env("TRUSTED_PROXIES", ""))
	c.TrustedProxies, c.Warnings = proxies, warnings
	if c.Production() && c.MailProvider == MailProviderLog {
		c.Warnings = append(c.Warnings, "MAIL_PROVIDER=log in production: no email is sent, so verification and password-reset mail never reaches users (set MAIL_PROVIDER=smtp)")
	}
	return c, c.validate(perr)
}

// Production reports whether APP_ENV=production.
func (c *Config) Production() bool { return c.Env == "production" }

// LogWarnings writes the non-fatal configuration warnings found by Load. The logger is built from the loaded
// configuration, so it cannot be used inside Load itself.
func (c *Config) LogWarnings(log *slog.Logger) {
	for _, w := range c.Warnings {
		log.Warn("configuration: " + w)
	}
}

func (c *Config) validate(extra ...error) error {
	var problems []string
	for _, e := range extra {
		if e != nil {
			problems = append(problems, e.Error())
		}
	}
	if c.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}
	if key, err := base64.StdEncoding.DecodeString(c.EncryptionKey); err != nil || len(key) != 32 {
		problems = append(problems, "ENCRYPTION_KEY must be base64 of 32 random bytes (openssl rand -base64 32)")
	}
	for _, o := range c.CORSOrigins {
		if o == "*" {
			problems = append(problems, "CORS_ALLOWED_ORIGINS must list explicit origins; \"*\" cannot be combined with credentialed requests")
		}
	}
	if n := len(c.GatewaySecret); n > 0 && n < 32 {
		problems = append(problems, "MCP_GATEWAY_SECRET must be at least 32 characters (openssl rand -hex 32)")
	}
	if c.PasswordHashConcurrency < 1 {
		problems = append(problems, "PASSWORD_HASH_CONCURRENCY must be at least 1")
	}
	if c.PasswordHashMemoryMiB < 1 {
		problems = append(problems, "PASSWORD_HASH_MEMORY_MIB must be at least 1")
	}
	if c.StorageDriver != "s3" && c.StorageDriver != "memory" {
		problems = append(problems, "STORAGE_DRIVER must be s3 or memory")
	}
	switch c.TelegramUpdatesMode {
	case TelegramModePolling:
	case TelegramModeWebhook:
		if c.TelegramToken != "" {
			if c.TelegramWebhookSecret == "" {
				problems = append(problems, "TELEGRAM_WEBHOOK_SECRET is required when TELEGRAM_UPDATES_MODE=webhook (openssl rand -hex 32)")
			} else if !validWebhookSecret(c.TelegramWebhookSecret) {
				problems = append(problems, "TELEGRAM_WEBHOOK_SECRET must be 1-256 characters of A-Z a-z 0-9 _ - (Telegram's secret_token rule)")
			} else if c.Production() && len(c.TelegramWebhookSecret) < 16 {
				problems = append(problems, "TELEGRAM_WEBHOOK_SECRET must be at least 16 characters in production")
			}
		}
	default:
		problems = append(problems, "TELEGRAM_UPDATES_MODE must be polling or webhook")
	}
	problems = append(problems, c.validateMail()...)
	if c.Production() {
		if !c.CookieSecure {
			problems = append(problems, "COOKIE_SECURE must be true in production")
		}
		if c.StorageDriver == "memory" {
			problems = append(problems, "STORAGE_DRIVER=memory is not allowed in production")
		}
		if c.MockProviders {
			problems = append(problems, "SOCIAL_MOCK_PROVIDERS must be false in production")
		}
	}
	if len(problems) > 0 {
		return errors.New("config: " + strings.Join(problems, "; "))
	}
	return nil
}

func (c *Config) validateMail() []string {
	switch c.MailProvider {
	case MailProviderLog:
		return nil
	case MailProviderSMTP:
	default:
		return []string{"MAIL_PROVIDER must be log or smtp"}
	}
	var p []string
	for _, f := range []struct{ name, val string }{{"SMTP_HOST", c.SMTPHost}, {"SMTP_USERNAME", c.SMTPUsername},
		{"SMTP_PASSWORD", c.SMTPPassword}, {"MAIL_FROM", c.MailFrom}} {
		if f.val == "" {
			p = append(p, f.name+" is required when MAIL_PROVIDER=smtp")
		}
	}
	if c.SMTPPort < 1 || c.SMTPPort > 65535 {
		p = append(p, "SMTP_PORT must be 1-65535")
	}
	if c.SMTPTLS != SMTPTLSStartTLS && c.SMTPTLS != SMTPTLSImplicit {
		p = append(p, "SMTP_TLS must be starttls or implicit")
	}
	if c.MailFrom != "" {
		if _, err := mail.ParseAddress(c.MailFrom); err != nil {
			p = append(p, "MAIL_FROM must be an email address, optionally with a display name")
		}
	}
	return p
}

func validWebhookSecret(s string) bool {
	if s == "" || len(s) > 256 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envInt(key string, def int) int {
	v, err := strconv.Atoi(env(key, strconv.Itoa(def)))
	if err != nil {
		return def
	}
	return v
}

func envFloat(key string, def float64) float64 {
	v, err := strconv.ParseFloat(env(key, fmt.Sprint(def)), 64)
	if err != nil {
		return def
	}
	return v
}

func envBool(key string, def bool) bool {
	v, err := strconv.ParseBool(env(key, strconv.FormatBool(def)))
	if err != nil {
		return def
	}
	return v
}

func envDuration(key string, def time.Duration) time.Duration {
	v, err := time.ParseDuration(env(key, def.String()))
	if err != nil {
		return def
	}
	return v
}

func list(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.TrimRight(p, "/"))
		}
	}
	return out
}
