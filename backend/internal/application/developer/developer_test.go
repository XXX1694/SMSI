package developer

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/errs"
)

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

type inlineTx struct{}

func (inlineTx) InTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type auditRec struct{ actions []string }

func (a *auditRec) Record(_ context.Context, _ actor.Actor, action, _, _ string, _ map[string]any) error {
	a.actions = append(a.actions, action)
	return nil
}

type memKeys struct {
	keys    []apikey.Key
	revoked map[uuid.UUID]time.Time
}

func (m *memKeys) Create(_ context.Context, k *apikey.Key) error {
	m.keys = append(m.keys, *k)
	return nil
}
func (m *memKeys) List(_ context.Context, uid uuid.UUID) ([]apikey.Key, error) {
	var out []apikey.Key
	for _, k := range m.keys {
		if k.UserID == uid {
			out = append(out, k)
		}
	}
	return out, nil
}
func (m *memKeys) Revoke(_ context.Context, uid, id uuid.UUID, at time.Time) error {
	for _, k := range m.keys {
		if k.UserID == uid && k.ID == id {
			if m.revoked == nil {
				m.revoked = map[uuid.UUID]time.Time{}
			}
			m.revoked[id] = at
			return nil
		}
	}
	return errs.NotFoundf("api key")
}

type memConns struct{ conns []MCPConnection }

func (m *memConns) Create(_ context.Context, c *MCPConnection) error {
	m.conns = append(m.conns, *c)
	return nil
}
func (m *memConns) List(context.Context, uuid.UUID) ([]MCPConnection, error) { return m.conns, nil }
func (m *memConns) Get(_ context.Context, uid, id uuid.UUID) (*MCPConnection, error) {
	for _, c := range m.conns {
		if c.UserID == uid && c.ID == id {
			return &c, nil
		}
	}
	return nil, errs.NotFoundf("mcp connection")
}
func (m *memConns) Revoke(context.Context, uuid.UUID, uuid.UUID, time.Time) error { return nil }

type memUsage struct {
	keys []KeyUsage
	days []DayUsage
	got  time.Time
	err  error
}

func (m *memUsage) KeyUsage(_ context.Context, _ uuid.UUID, since time.Time) ([]KeyUsage, error) {
	m.got = since
	return m.keys, m.err
}
func (m *memUsage) DailyUsage(context.Context, uuid.UUID, time.Time) ([]DayUsage, error) {
	return m.days, m.err
}

var now = time.Date(2026, 10, 7, 15, 30, 0, 0, time.UTC)

func newSvc() (*Service, *memKeys, *memConns, *memUsage, *auditRec) {
	k, c, u, a := &memKeys{}, &memConns{}, &memUsage{}, &auditRec{}
	return NewService(Deps{Keys: k, Connections: c, Usage: u, Tx: inlineTx{}, Audit: a, Clock: fakeClock{now},
		MCPPublicURL: "https://mcp.example.com/mcp", APIPublicURL: "https://api.example.com/api/v1"}), k, c, u, a
}

func session() actor.Actor {
	return actor.Actor{UserID: uuid.New(), Type: actor.TypeUser, ID: "u", SessionID: uuid.New(), EmailVerified: true}
}

func TestCreateKeyReturnsRawSecretOnceAndStoresOnlyHash(t *testing.T) {
	s, keys, _, _, rec := newSvc()
	a := session()
	key, raw, err := s.CreateKey(context.Background(), a, CreateKeyInput{Name: "  CI  ", Scopes: []string{"posts:read", "posts:write"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, "sk_live_") || len(raw) != len("sk_live_")+48 || !strings.HasPrefix(raw, key.Prefix) {
		t.Fatalf("raw key %q prefix %q", raw, key.Prefix)
	}
	stored := keys.keys[0]
	if stored.Name != "CI" || stored.KeyHash == "" || strings.Contains(stored.KeyHash, raw) || stored.UserID != a.UserID {
		t.Fatalf("stored key: %+v", stored)
	}
	if len(rec.actions) != 1 || rec.actions[0] != "api_key.created" {
		t.Fatalf("audit: %v", rec.actions)
	}
	_, raw2, _ := s.CreateKey(context.Background(), a, CreateKeyInput{Name: "again"})
	if raw2 == raw {
		t.Fatal("keys must be unique")
	}
}

func TestCreateKeyValidation(t *testing.T) {
	s, keys, _, _, _ := newSvc()
	past, future := now.Add(-time.Second), now.Add(time.Hour)
	for name, in := range map[string]CreateKeyInput{
		"empty name":      {Name: "   "},
		"long name":       {Name: strings.Repeat("n", 101)},
		"unknown scope":   {Name: "k", Scopes: []string{"posts:everything"}},
		"expired already": {Name: "k", ExpiresAt: &past},
		"expires now":     {Name: "k", ExpiresAt: &now},
	} {
		if _, _, err := s.CreateKey(context.Background(), session(), in); !errs.Is(err, errs.Validation) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(keys.keys) != 0 {
		t.Fatal("nothing may be stored on validation failure")
	}
	if _, _, err := s.CreateKey(context.Background(), session(), CreateKeyInput{Name: "ok", ExpiresAt: &future}); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultScopesExcludeDangerousOnes(t *testing.T) {
	s, keys, _, _, _ := newSvc()
	if _, _, err := s.CreateKey(context.Background(), session(), CreateKeyInput{Name: "defaults"}); err != nil {
		t.Fatal(err)
	}
	if len(keys.keys[0].Scopes) == 0 {
		t.Fatal("a key without scopes would be useless; defaults expected")
	}
	for _, sc := range keys.keys[0].Scopes {
		if sc.Dangerous() {
			t.Errorf("default scope %s is dangerous", sc)
		}
	}
}

func TestKeyManagementIsSessionOnly(t *testing.T) {
	s, _, _, _, _ := newSvc()
	ctx := context.Background()
	apiKey := actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, ID: "k", Scopes: apikey.AllScopes()}
	anon := actor.Actor{}
	for name, a := range map[string]actor.Actor{"api key with every scope": apiKey, "anonymous": anon} {
		want := errs.Forbidden
		if a.UserID == uuid.Nil {
			want = errs.Unauthenticated
		}
		if _, _, err := s.CreateKey(ctx, a, CreateKeyInput{Name: "x"}); !errs.Is(err, want) {
			t.Errorf("%s CreateKey: %v", name, err)
		}
		if _, err := s.ListKeys(ctx, a); !errs.Is(err, want) {
			t.Errorf("%s ListKeys: %v", name, err)
		}
		if err := s.RevokeKey(ctx, a, uuid.New()); !errs.Is(err, want) {
			t.Errorf("%s RevokeKey: %v", name, err)
		}
		if _, err := s.CreateMCPConnection(ctx, a, CreateMCPInput{Name: "x"}); !errs.Is(err, want) {
			t.Errorf("%s CreateMCPConnection: %v", name, err)
		}
		if _, err := s.ListMCPConnections(ctx, a); !errs.Is(err, want) {
			t.Errorf("%s ListMCPConnections: %v", name, err)
		}
		if err := s.RevokeMCPConnection(ctx, a, uuid.New()); !errs.Is(err, want) {
			t.Errorf("%s RevokeMCPConnection: %v", name, err)
		}
		if _, err := s.Usage(ctx, a); !errs.Is(err, want) {
			t.Errorf("%s Usage: %v", name, err)
		}
	}
}

func TestRevokeKeyIsTenantScoped(t *testing.T) {
	s, keys, _, _, rec := newSvc()
	owner, other := session(), session()
	k, _, _ := s.CreateKey(context.Background(), owner, CreateKeyInput{Name: "mine"})
	if err := s.RevokeKey(context.Background(), other, k.ID); !errs.Is(err, errs.NotFound) {
		t.Fatalf("revoking someone else's key: %v", err)
	}
	if len(keys.revoked) != 0 {
		t.Fatal("foreign revoke must not touch the key")
	}
	if err := s.RevokeKey(context.Background(), owner, k.ID); err != nil {
		t.Fatal(err)
	}
	if keys.revoked[k.ID] != now || rec.actions[len(rec.actions)-1] != "api_key.revoked" {
		t.Fatalf("revoked=%v audit=%v", keys.revoked, rec.actions)
	}
	if list, _ := s.ListKeys(context.Background(), other); len(list) != 0 {
		t.Fatal("other tenant lists keys")
	}
}

func TestMCPConfigIsReadyToPaste(t *testing.T) {
	s, keys, conns, _, _ := newSvc()
	got, err := s.CreateMCPConnection(context.Background(), session(), CreateMCPInput{Name: "Cursor", ClientName: " cursor ", Scopes: []string{"posts:read"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Connection.ClientName != "cursor" || got.Connection.Name != "Cursor" || got.Connection.KeyPrefix == "" || len(conns.conns) != 1 {
		t.Fatalf("connection: %+v", got.Connection)
	}
	if keys.keys[0].Name != "MCP: Cursor" || got.Connection.APIKeyID != keys.keys[0].ID {
		t.Fatalf("dedicated key: %+v", keys.keys[0])
	}
	raw := got.RawKey
	httpSrv := got.Config["http"].(map[string]any)["mcpServers"].(map[string]any)["socialos"].(map[string]any)
	if httpSrv["type"] != "http" || httpSrv["url"] != "https://mcp.example.com/mcp" || httpSrv["headers"].(map[string]string)["Authorization"] != "Bearer "+raw {
		t.Fatalf("http: %v", httpSrv)
	}
	stdio := got.Config["stdio"].(map[string]any)["mcpServers"].(map[string]any)["socialos"].(map[string]any)
	env := stdio["env"].(map[string]string)
	args := stdio["args"].([]string)
	if stdio["command"] != "npx" || env["SOCIALOS_AUTH_HEADER"] != "Bearer "+raw || args[1] != MCPRemotePackage || args[2] != "https://mcp.example.com/mcp" {
		t.Fatalf("stdio: %v", stdio)
	}
	assertNoUnpinnedNpx(t, got.Config)
	if _, ok := got.Config["mcpServers"].(map[string]any)["socialos"]; !ok {
		t.Fatal("top-level mcpServers must be pasteable as-is")
	}
	cmd := got.Config["claude_code"].(string)
	if !strings.HasPrefix(cmd, "claude mcp add --transport http socialos https://mcp.example.com/mcp") || !strings.Contains(cmd, `"Authorization: Bearer `+raw+`"`) {
		t.Fatalf("claude_code: %s", cmd)
	}
	if _, err := s.CreateMCPConnection(context.Background(), session(), CreateMCPInput{Name: "x", ClientName: strings.Repeat("c", 101)}); !errs.Is(err, errs.Validation) {
		t.Fatalf("long client name: %v", err)
	}
}

func TestUsageWindowAndTotals(t *testing.T) {
	s, _, _, usage, _ := newSvc()
	usage.keys = []KeyUsage{{Name: "a", Requests: 5}, {Name: "b", Requests: 7}}
	usage.days = []DayUsage{{Day: now, Requests: 12}}
	rep, err := s.Usage(context.Background(), session())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Total != 12 || rep.WindowDays != 30 || len(rep.Keys) != 2 || len(rep.Days) != 1 {
		t.Fatalf("%+v", rep)
	}
	// 30 calendar days inclusive of today, anchored at UTC midnight.
	if want := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC); !usage.got.Equal(want) {
		t.Fatalf("window starts %v, want %v", usage.got, want)
	}
	usage.err = errors.New("db down")
	if _, err := s.Usage(context.Background(), session()); err == nil {
		t.Fatal("repo errors must propagate")
	}
}

func TestUnverifiedOwnerCannotCreateKeysOrConnections(t *testing.T) {
	s, keys, conns, _, _ := newSvc()
	a := session()
	a.EmailVerified = false
	if _, _, err := s.CreateKey(context.Background(), a, CreateKeyInput{Name: "k"}); !errs.Is(err, errs.EmailNotVerified) {
		t.Fatalf("CreateKey: %v", err)
	}
	if _, err := s.CreateMCPConnection(context.Background(), a, CreateMCPInput{Name: "c"}); !errs.Is(err, errs.EmailNotVerified) {
		t.Fatalf("CreateMCPConnection: %v", err)
	}
	if len(keys.keys) != 0 || len(conns.conns) != 0 {
		t.Fatal("something was created")
	}
}

var pinnedNpx = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*@\d+\.\d+\.\d+$`)

// assertNoUnpinnedNpx fails when a generated config mentions the unpublished socialos-mcp package or runs an npx
// package without an exact version (a squattable name would receive the API key).
func assertNoUnpinnedNpx(t *testing.T, cfg map[string]any) {
	t.Helper()
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "socialos-mcp") {
		t.Fatalf("config references the unpublished socialos-mcp package: %s", b)
	}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if x["command"] == "npx" {
				var pkg string
				for _, a := range x["args"].([]string) {
					if !strings.HasPrefix(a, "-") {
						pkg = a
						break
					}
				}
				if !pinnedNpx.MatchString(pkg) {
					t.Fatalf("npx package %q is not pinned to an exact version", pkg)
				}
			}
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(cfg)
}
