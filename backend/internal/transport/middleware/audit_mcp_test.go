package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/transport/httpx"
	"github.com/socialos/backend/internal/transport/middleware"
)

const gatewaySecret = "gw-secret-0123456789abcdef0123456789abcdef"

type mcpRig struct {
	h    http.Handler
	rec  *memAudit
	auth *fakeAuth
}

func newMCPRig(secret string) *mcpRig {
	keyActor := actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, ID: "k1", APIKeyID: uuid.New(), Label: "MCP: laptop"}
	f := &fakeAuth{keyActor: keyActor}
	rec := &memAudit{}
	r := chi.NewRouter()
	r.Use(middleware.Gateway(secret), middleware.Authenticate(f, nil),
		middleware.APIKeyAudit(rec, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))))
	r.Get("/me", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Post("/posts", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) })
	r.Post("/posts/{id}/publish", func(w http.ResponseWriter, r *http.Request) {
		httpx.ErrorCode(w, r, errs.InsufficientScope, "missing required scope posts:publish")
	})
	return &mcpRig{h: r, rec: rec, auth: f}
}

func (m *mcpRig) call(method, path string, hdr map[string]string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "198.51.100.9:4711"
	req.Header.Set("Authorization", "Bearer sk_live_good")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	out := httptest.NewRecorder()
	m.h.ServeHTTP(out, req)
	return out
}

func TestMCPToolCallIsOneAllowListedRow(t *testing.T) {
	m := newMCPRig(gatewaySecret)
	id := uuid.NewString()
	m.call("POST", "/posts/"+id+"/publish", map[string]string{"X-MCP-Tool": "publish_post"}, `{"content":"secret draft text"}`)
	if len(m.rec.entries) != 1 {
		t.Fatalf("exactly one row per tool call, got %d", len(m.rec.entries))
	}
	e := m.rec.entries[0]
	ids, _ := e.meta["target_ids"].(map[string]string)
	if e.action != "mcp.tool_call" || e.meta["tool"] != "publish_post" || e.meta["route"] != "/posts/{id}/publish" ||
		e.meta["status"] != http.StatusForbidden || e.meta["error_code"] != "INSUFFICIENT_SCOPE" || e.meta["method"] != "POST" ||
		e.meta["client"] != "MCP: laptop" || e.meta["credential_id"] != e.actor.APIKeyID.String() || e.meta["via_gateway"] != false ||
		ids["id"] != id {
		t.Fatalf("entry: %+v", e)
	}
	allowed := map[string]bool{"tool": true, "method": true, "route": true, "status": true, "error_code": true, "target_ids": true,
		"client": true, "credential_id": true, "via_gateway": true}
	for k := range e.meta {
		if !allowed[k] {
			t.Errorf("metadata key %q is not on the allow-list", k)
		}
	}
}

func TestRequestsWithoutAToolHeaderKeepTheAPIRequestRow(t *testing.T) {
	m := newMCPRig(gatewaySecret)
	m.call("POST", "/posts", nil, "")
	m.call("GET", "/me", nil, "") // the per-request introspection of the MCP server carries no tool name
	if len(m.rec.entries) != 2 || m.rec.entries[0].action != "api_key.request" || m.rec.entries[1].action != "api_key.request" {
		t.Fatalf("entries: %+v", m.rec.entries)
	}
	for _, e := range m.rec.entries {
		if _, ok := e.meta["tool"]; ok {
			t.Errorf("tool recorded without a header: %v", e.meta)
		}
	}
}

func TestBadToolNamesAreDropped(t *testing.T) {
	for _, bad := range []string{"", "Publish", "publish-post", "publish post", "tool2", "sk_live_abc123", strings.Repeat("a", 65), "a\"b", "../x"} {
		m := newMCPRig(gatewaySecret)
		hdr := map[string]string{}
		if bad != "" {
			hdr["X-MCP-Tool"] = bad
		}
		m.call("POST", "/posts", hdr, "")
		if len(m.rec.entries) != 1 || m.rec.entries[0].action != "api_key.request" || m.rec.entries[0].meta["tool"] != nil {
			t.Errorf("tool %q: %+v", bad, m.rec.entries)
		}
	}
	m := newMCPRig(gatewaySecret)
	m.call("POST", "/posts", map[string]string{"X-MCP-Tool": strings.Repeat("a", 64)}, "")
	if m.rec.entries[0].action != "mcp.tool_call" {
		t.Fatalf("64 chars is allowed: %+v", m.rec.entries[0])
	}
}

func TestToolHeaderFromASessionIsIgnored(t *testing.T) {
	m := newMCPRig(gatewaySecret)
	req := httptest.NewRequest("POST", "/posts", nil)
	req.Header.Set("X-MCP-Tool", "publish_post")
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: "good-session"})
	m.h.ServeHTTP(httptest.NewRecorder(), req)
	if len(m.rec.entries) != 0 {
		t.Fatalf("sessions are not audited here: %+v", m.rec.entries)
	}
}

func TestClientIPHeaderNeedsTheGatewaySecret(t *testing.T) {
	cases := []struct {
		name   string
		secret string
		hdr    map[string]string
		want   string
		via    bool
	}{
		{"forged without secret", gatewaySecret, map[string]string{"X-SocialOS-Client-IP": "203.0.113.50"}, "198.51.100.9", false},
		{"wrong secret", gatewaySecret, map[string]string{"X-SocialOS-Gateway": "nope", "X-SocialOS-Client-IP": "203.0.113.50"}, "198.51.100.9", false},
		{"prefix of the secret", gatewaySecret, map[string]string{"X-SocialOS-Gateway": gatewaySecret[:10], "X-SocialOS-Client-IP": "203.0.113.50"}, "198.51.100.9", false},
		{"right secret", gatewaySecret, map[string]string{"X-SocialOS-Gateway": gatewaySecret, "X-SocialOS-Client-IP": "203.0.113.50"}, "203.0.113.50", true},
		{"right secret, v4-mapped", gatewaySecret, map[string]string{"X-SocialOS-Gateway": gatewaySecret, "X-SocialOS-Client-IP": "::ffff:203.0.113.50"}, "203.0.113.50", true},
		{"right secret, garbage address", gatewaySecret, map[string]string{"X-SocialOS-Gateway": gatewaySecret, "X-SocialOS-Client-IP": "not-an-ip"}, "198.51.100.9", false},
		{"no secret configured, empty header", "", map[string]string{"X-SocialOS-Gateway": "", "X-SocialOS-Client-IP": "203.0.113.50"}, "198.51.100.9", false},
		{"no secret configured", "", map[string]string{"X-SocialOS-Gateway": gatewaySecret, "X-SocialOS-Client-IP": "203.0.113.50"}, "198.51.100.9", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newMCPRig(tc.secret)
			tc.hdr["X-MCP-Tool"] = "list_posts"
			m.call("POST", "/posts", tc.hdr, "")
			if m.auth.lastInfo.IP != tc.want {
				t.Fatalf("client ip %q, want %q", m.auth.lastInfo.IP, tc.want)
			}
			if got := m.rec.entries[0].meta["via_gateway"]; got != tc.via {
				t.Fatalf("via_gateway %v, want %v", got, tc.via)
			}
		})
	}
}

// TestMCPMetadataNeverHoldsSecretsOrRequestData runs requests that carry every kind of sensitive input and walks all
// metadata values recorded for them.
func TestMCPMetadataNeverHoldsSecretsOrRequestData(t *testing.T) {
	m := newMCPRig(gatewaySecret)
	body := `{"content":"launch day body text","code_verifier":"verifier-123"}`
	query := "?token=sat_leak&refresh=srt_leak&q=search+term"
	hdr := map[string]string{
		"X-MCP-Tool": "publish_post", "X-SocialOS-Gateway": gatewaySecret, "X-SocialOS-Client-IP": "203.0.113.50",
		"Authorization": "Bearer sk_live_good", "Cookie": "socialos_session=abc",
	}
	id := uuid.NewString()
	m.call("POST", "/posts/"+id+"/publish"+query, hdr, body)
	m.call("POST", "/posts"+query, hdr, body)
	m.call("POST", "/posts/not-a-uuid-sat_x/publish"+query, hdr, body)
	if len(m.rec.entries) != 3 {
		t.Fatalf("entries: %d", len(m.rec.entries))
	}
	forbidden := regexp.MustCompile(`sk_live_|sat_|srt_|sac_|scs_|Bearer|code_verifier|launch day|search|verifier-123|` + regexp.QuoteMeta(gatewaySecret))
	for _, e := range m.rec.entries {
		if e.action != "mcp.tool_call" {
			t.Fatalf("action %q", e.action)
		}
		walk(t, "meta", e.meta, func(path, s string) {
			if forbidden.MatchString(s) {
				t.Errorf("%s = %q contains a forbidden value", path, s)
			}
		})
		if strings.Contains(string(mustJSON(e.meta)), "?") {
			t.Errorf("a query string leaked: %s", mustJSON(e.meta))
		}
	}
	if _, ok := m.rec.entries[2].meta["target_ids"]; ok {
		t.Errorf("a non-uuid path segment must not be copied: %v", m.rec.entries[2].meta)
	}
}

// walk calls visit for every string reachable in v (map keys included).
func walk(t *testing.T, path string, v any, visit func(path, s string)) {
	t.Helper()
	switch x := v.(type) {
	case string:
		visit(path, x)
	case map[string]any:
		for k, vv := range x {
			visit(path+"#key", k)
			walk(t, path+"."+k, vv, visit)
		}
	case map[string]string:
		for k, vv := range x {
			visit(path+"#key", k)
			visit(path+"."+k, vv)
		}
	case nil, bool, int, int64, float64:
	default:
		var generic any
		if err := json.Unmarshal(mustJSON(v), &generic); err != nil {
			t.Fatalf("%s: %T not serialisable", path, v)
		}
		if _, same := generic.(string); same || generic == nil {
			return
		}
		walk(t, path, generic, visit)
	}
}
