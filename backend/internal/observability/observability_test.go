package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactsSensitiveAttributes(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(contextHandler{slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: redact})})
	log.Info("event",
		"password", "hunter2", "access_token", "AT-1", "refreshToken", "RT-1", "client_secret", "S3CR3T", "Authorization", "Bearer sk_live_x",
		"Cookie", "socialos_session=abc", "api_key", "sk_live_y", "apikey", "sk_live_z", "code_verifier", "V",
		"user_id", "42", "action", "login")
	out := buf.String()
	for _, leaked := range []string{"hunter2", "AT-1", "RT-1", "S3CR3T", "sk_live", "socialos_session", `"V"`} {
		if strings.Contains(out, leaked) {
			t.Errorf("log line leaks %q: %s", leaked, out)
		}
	}
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["user_id"] != "42" || rec["action"] != "login" {
		t.Errorf("harmless attributes must survive: %v", rec)
	}
	for _, k := range []string{"password", "access_token", "refreshToken", "client_secret", "Authorization", "Cookie", "api_key", "apikey", "code_verifier"} {
		if rec[k] != "[REDACTED]" {
			t.Errorf("%s = %v", k, rec[k])
		}
	}
}

func TestContextAttributes(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(contextHandler{slog.NewJSONHandler(&buf, nil)})
	ctx := WithAttrs(context.Background(), slog.String("request_id", "r-1"))
	ctx = WithAttrs(ctx, slog.String("user_id", "u-1"))
	log.InfoContext(ctx, "hello")
	log.InfoContext(context.Background(), "no ctx")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if !strings.Contains(lines[0], `"request_id":"r-1"`) || !strings.Contains(lines[0], `"user_id":"u-1"`) {
		t.Fatalf("context attributes missing: %s", lines[0])
	}
	if strings.Contains(lines[1], "request_id") {
		t.Fatalf("attributes must not leak between requests: %s", lines[1])
	}
	// WithAttrs does not mutate the parent context's attributes.
	parent := WithAttrs(context.Background(), slog.String("a", "1"))
	_ = WithAttrs(parent, slog.String("b", "2"))
	buf.Reset()
	log.InfoContext(parent, "p")
	if strings.Contains(buf.String(), `"b"`) {
		t.Fatalf("child attributes leaked into the parent: %s", buf.String())
	}
}

func TestNewLoggerLevelsAndFormats(t *testing.T) {
	if NewLogger("debug", "text") == nil || NewLogger("nonsense", "json") == nil {
		t.Fatal("logger")
	}
	if !NewLogger("debug", "json").Enabled(context.Background(), slog.LevelDebug) {
		t.Error("debug level")
	}
	if NewLogger("nonsense", "json").Enabled(context.Background(), slog.LevelDebug) {
		t.Error("unknown levels fall back to info")
	}
	if !NewLogger("nonsense", "json").Enabled(context.Background(), slog.LevelInfo) {
		t.Error("info must be enabled by default")
	}
}

func TestMetricsRegistry(t *testing.T) {
	m := NewMetrics()
	m.HTTPRequests.WithLabelValues("/posts", "GET", "200").Inc()
	m.PublishOutcomes.WithLabelValues("mock", "published").Add(2)
	m.RateLimited.Inc()
	families, err := m.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, f := range families {
		for _, mt := range f.GetMetric() {
			switch {
			case mt.GetCounter() != nil:
				got[f.GetName()] += mt.GetCounter().GetValue()
			}
		}
	}
	if got["socialos_publish_attempts_total"] != 2 || got["socialos_http_requests_total"] != 1 || got["socialos_http_rate_limited_total"] != 1 {
		t.Fatalf("counters: %v", got)
	}
	// Two registries are independent (tests build many apps in one process).
	if NewMetrics().Registry == m.Registry {
		t.Fatal("registries must not be shared")
	}
}
