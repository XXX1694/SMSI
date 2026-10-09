package e2e

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/config"
	"github.com/socialos/backend/internal/domain/dataexport"
)

func (c *client) waitExport(id, want string) map[string]any {
	c.e.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for _, it := range c.must("GET", "/api/v1/account/exports", nil, 200)["items"].([]any) {
			if m := it.(map[string]any); m["id"] == id && m["status"] == want {
				return m
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.e.t.Fatalf("export %s never became %s", id, want)
	return nil
}

func (e *env) zipOf(userID, exportID string) map[string][]byte {
	e.t.Helper()
	uid, _ := uuid.Parse(userID)
	eid, _ := uuid.Parse(exportID)
	rc, err := e.storage.Get(context.Background(), dataexport.ObjectKey(uid, eid))
	if err != nil {
		e.t.Fatalf("archive not stored: %v", err)
	}
	b, _ := io.ReadAll(rc)
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		e.t.Fatalf("not a zip: %v", err)
	}
	out := map[string][]byte{"": b}
	for _, f := range zr.File {
		r, _ := f.Open()
		out[f.Name], _ = io.ReadAll(r)
		_ = r.Close()
	}
	return out
}

// The whole export flow: request, the worker builds the ZIP, the owner gets a link, the archive has the owner's data
// and none of the secrets, rules (one at a time, cooldown, session only) hold, and the archive expires.
func TestAccountExportFlow(t *testing.T) {
	clk := &steppingClock{}
	clk.set(time.Now().UTC())
	e := newEnv(t, envOpts{startWorker: true, clock: clk, mutate: func(c *config.Config) { c.ExportRetentionDays = 7; c.SessionTTL = 30 * 24 * time.Hour }})
	c := e.browser()
	me := c.register("exporter@example.com")
	uid := me["user"].(map[string]any)["id"].(string)
	acc := c.connectMock()
	img := c.upload("photo.png", pngBytes(t))
	if img.status != 201 {
		t.Fatalf("upload: %d %s", img.status, img.body)
	}
	mediaID := img.json(t)["id"].(string)
	post := c.must("POST", "/api/v1/posts", map[string]any{"content": "export me", "social_account_ids": []string{acc}, "media_ids": []string{mediaID}}, 201)
	rawKey := c.createKey("agent", "posts:read")

	// API keys can never export, whatever their scopes.
	k := e.apiKeyClient(rawKey)
	for _, tc := range []struct{ method, path string }{{"POST", "/api/v1/account/exports"}, {"GET", "/api/v1/account/exports"}} {
		if r := k.do(tc.method, tc.path, nil); r.status != http.StatusForbidden {
			t.Errorf("%s %s with a key: %d %s", tc.method, tc.path, r.status, r.body)
		}
	}

	req := c.must("POST", "/api/v1/account/exports", nil, 202)
	id := req["id"].(string)
	if req["status"] != "pending" {
		t.Fatalf("new export: %v", req)
	}
	if r := c.do("POST", "/api/v1/account/exports", nil); r.status != http.StatusConflict {
		// The worker may already be done; then the cooldown answers instead. Never a second export.
		if r.status != http.StatusTooManyRequests {
			t.Fatalf("second request: %d %s", r.status, r.body)
		}
	}
	ready := c.waitExport(id, "ready")
	if ready["size_bytes"].(float64) <= 0 || ready["expires_at"] == nil {
		t.Fatalf("ready export: %v", ready)
	}

	// 429 with Retry-After inside the cooldown.
	r := c.do("POST", "/api/v1/account/exports", nil)
	if r.status != http.StatusTooManyRequests || r.header.Get("Retry-After") == "" {
		t.Fatalf("cooldown: %d %v %s", r.status, r.header, r.body)
	}

	link := c.must("GET", "/api/v1/account/exports/"+id, nil, 200)
	if !strings.HasPrefix(link["url"].(string), "memory://") || link["url_expires_at"] == nil {
		t.Fatalf("no link: %v", link)
	}

	files := e.zipOf(uid, id)
	for _, name := range []string{"README.txt", "profile.json", "social_accounts.json", "posts.json", "media.json", "api_keys.json",
		"mcp_connections.json", "approvals.json", "audit_logs.json", "sign_in_methods.json"} {
		if _, ok := files[name]; !ok {
			t.Errorf("archive lacks %s", name)
		}
	}
	if _, ok := files["media/"+mediaID+".png"]; !ok {
		t.Errorf("archive lacks the media file")
	}
	var posts []map[string]any
	if err := json.Unmarshal(files["posts.json"], &posts); err != nil || len(posts) != 1 || posts[0]["id"] != post["id"] || len(posts[0]["targets"].([]any)) != 1 {
		t.Fatalf("posts.json: %v %s", err, files["posts.json"])
	}
	if !strings.Contains(string(files["api_keys.json"]), `"prefix"`) || !strings.Contains(string(files["audit_logs.json"]), "account.export_requested") {
		t.Fatalf("api_keys / audit content wrong: %s", files["api_keys.json"])
	}

	// No secret of any kind is in the archive: not the password, not the raw key, no hash, no stored credential.
	secrets := []string{"correct horse battery", rawKey, "password_hash", "key_hash", "token_hash", "access_token", "refresh_token", "csrf"}
	rows, err := e.app.DB.Pool.Query(context.Background(),
		`SELECT password_hash FROM users UNION ALL SELECT key_hash FROM api_keys UNION ALL SELECT token_hash FROM sessions
		 UNION ALL SELECT access_token_enc FROM oauth_credentials WHERE access_token_enc <> ''`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		secrets = append(secrets, s)
	}
	rows.Close()
	for _, f := range e.zipOf(uid, id) {
		for _, s := range secrets {
			if s != "" && bytes.Contains(f, []byte(s)) {
				t.Errorf("archive contains secret %.12q", s)
			}
		}
	}

	// Past the retention the link is gone and the sweep deletes the archive.
	clk.set(clk.Now().Add(8 * 24 * time.Hour))
	if r := c.do("GET", "/api/v1/account/exports/"+id, nil); r.status != http.StatusConflict {
		t.Fatalf("expired export gave %d %s", r.status, r.body)
	}
	if err := e.app.Services.Exports.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if uid2, _ := uuid.Parse(uid); e.storage.Has(dataexport.ObjectKey(uid2, uuid.MustParse(id))) {
		t.Fatal("expired archive still stored")
	}
	c.waitExport(id, "expired")
	c.must("POST", "/api/v1/account/exports", nil, 202) // a new one is allowed again
}

// Without a worker the export stays pending: a second request is refused with 409.
func TestAccountExportOneAtATime(t *testing.T) {
	e := newEnv(t, envOpts{})
	c := e.browser()
	c.register("once@example.com")
	c.must("POST", "/api/v1/account/exports", nil, 202)
	if r := c.do("POST", "/api/v1/account/exports", nil); r.status != http.StatusConflict || r.errCode(t) != "CONFLICT" {
		t.Fatalf("second export: %d %s", r.status, r.body)
	}
	// No CSRF header, no export.
	c2 := e.browser()
	c2.register("csrf@example.com")
	c2.csrf = ""
	if r := c2.do("POST", "/api/v1/account/exports", nil); r.status != http.StatusForbidden {
		t.Fatalf("export without CSRF: %d %s", r.status, r.body)
	}
}

// blockingBuilder never finishes a build until released.
type blockingBuilder struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingBuilder) Build(ctx context.Context, _ uuid.UUID) error {
	b.started <- struct{}{}
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return nil
}

// A build that runs for a long time must not take a worker slot from publishing: exports have their own queue.
func TestRunningExportDoesNotBlockPublishing(t *testing.T) {
	b := &blockingBuilder{started: make(chan struct{}, 4), release: make(chan struct{})}
	t.Cleanup(func() { close(b.release) })
	e := newEnv(t, envOpts{startWorker: true, exportTasks: b})
	c := e.browser()
	c.register("slow-export@example.com")
	acc := c.connectMock()
	c.must("POST", "/api/v1/account/exports", nil, 202)
	select {
	case <-b.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the export build never started")
	}
	p := c.must("POST", "/api/v1/posts", map[string]any{"content": "still goes out", "social_account_ids": []string{acc}}, 201)
	c.must("POST", "/api/v1/posts/"+p["id"].(string)+"/publish", nil, 202)
	c.waitStatus(p["id"].(string), "published", 15*time.Second)
}
