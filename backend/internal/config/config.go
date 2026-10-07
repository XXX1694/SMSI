// Package config loads configuration from environment variables.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the full runtime configuration (API + worker).
type Config struct {
	Env             string // development | production | test
	HTTPAddr        string
	DatabaseURL     string
	DBMaxConns      int32
	RedisURL        string
	QueueName       string
	EncryptionKey   string // base64 32 bytes
	APIPublicURL    string // public base of the API (OAuth redirect base)
	WebBaseURL      string // frontend base for post-connect redirects
	MCPPublicURL    string
	CORSOrigins     []string
	CookieSecure    bool
	CookieDomain    string
	SessionTTL      time.Duration
	MetricsToken    string
	MigrateOnStart  bool
	LogLevel        string
	LogFormat       string // json | text
	MockProviders   bool
	RateLimitRPS    float64
	RateLimitBurst  int
	AuthRateRPS     float64
	AuthRateBurst   int
	TrustProxy      bool
	WorkerConc      int
	ReconcileEvery  time.Duration
	StorageDriver   string // s3 | memory
	S3Endpoint      string
	S3PublicURL     string
	S3AccessKey     string
	S3SecretKey     string
	S3Bucket        string
	S3Region        string
	S3UseSSL        bool
	S3AutoCreate    bool
	LinkedInID      string
	LinkedInSecret  string
	LinkedInVersion string
	LinkedInPKCE    bool
	TelegramToken   string
}

// Load reads and validates configuration.
func Load() (*Config, error) {
	c := &Config{
		Env:             env("APP_ENV", "development"),
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		DatabaseURL:     env("DATABASE_URL", ""),
		DBMaxConns:      int32(envInt("DB_MAX_CONNS", 20)),
		RedisURL:        env("REDIS_URL", "redis://localhost:6379/0"),
		QueueName:       env("QUEUE_NAME", "publish"),
		EncryptionKey:   env("ENCRYPTION_KEY", ""),
		APIPublicURL:    strings.TrimRight(env("API_PUBLIC_URL", "http://localhost:8080"), "/"),
		WebBaseURL:      strings.TrimRight(env("WEB_BASE_URL", "http://localhost:3000"), "/"),
		MCPPublicURL:    env("MCP_PUBLIC_URL", "http://localhost:3333/mcp"),
		CORSOrigins:     list(env("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		CookieSecure:    envBool("COOKIE_SECURE", false),
		CookieDomain:    env("COOKIE_DOMAIN", ""),
		SessionTTL:      envDuration("SESSION_TTL", 7*24*time.Hour),
		MetricsToken:    env("METRICS_TOKEN", ""),
		MigrateOnStart:  envBool("MIGRATE_ON_START", false),
		LogLevel:        env("LOG_LEVEL", "info"),
		LogFormat:       env("LOG_FORMAT", "json"),
		MockProviders:   envBool("SOCIAL_MOCK_PROVIDERS", false),
		RateLimitRPS:    envFloat("RATE_LIMIT_RPS", 10),
		RateLimitBurst:  envInt("RATE_LIMIT_BURST", 40),
		AuthRateRPS:     envFloat("AUTH_RATE_LIMIT_RPS", 0.2),
		AuthRateBurst:   envInt("AUTH_RATE_LIMIT_BURST", 10),
		TrustProxy:      envBool("TRUST_PROXY", false),
		WorkerConc:      envInt("WORKER_CONCURRENCY", 10),
		ReconcileEvery:  envDuration("RECONCILE_INTERVAL", time.Minute),
		StorageDriver:   env("STORAGE_DRIVER", "s3"),
		S3Endpoint:      env("S3_ENDPOINT", "localhost:9000"),
		S3PublicURL:     env("S3_PUBLIC_ENDPOINT", ""),
		S3AccessKey:     env("S3_ACCESS_KEY", ""),
		S3SecretKey:     env("S3_SECRET_KEY", ""),
		S3Bucket:        env("S3_BUCKET", "socialos-media"),
		S3Region:        env("S3_REGION", "us-east-1"),
		S3UseSSL:        envBool("S3_USE_SSL", false),
		S3AutoCreate:    envBool("S3_AUTO_CREATE_BUCKET", true),
		LinkedInID:      env("LINKEDIN_CLIENT_ID", ""),
		LinkedInSecret:  env("LINKEDIN_CLIENT_SECRET", ""),
		LinkedInVersion: env("LINKEDIN_API_VERSION", "202606"),
		LinkedInPKCE:    envBool("LINKEDIN_USE_PKCE", true),
		TelegramToken:   env("TELEGRAM_BOT_TOKEN", ""),
	}
	return c, c.validate()
}

// Production reports whether APP_ENV=production.
func (c *Config) Production() bool { return c.Env == "production" }

func (c *Config) validate() error {
	var problems []string
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
	if c.StorageDriver != "s3" && c.StorageDriver != "memory" {
		problems = append(problems, "STORAGE_DRIVER must be s3 or memory")
	}
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
