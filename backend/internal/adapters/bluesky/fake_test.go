package bluesky

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

const (
	testHandle   = "alice.bsky.social"
	testDID      = "did:plc:alice123"
	testPassword = "pw-pw-pw-pw-secret" // gitleaks:allow (fake test value)
)

var t0 = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func jwtAt(exp time.Time, n int) string {
	p, _ := json.Marshal(map[string]any{"exp": exp.Unix(), "n": n})
	return "h." + base64.RawURLEncoding.EncodeToString(p) + ".s"
}

// fakePDS implements the XRPC subset the adapter calls.
type fakePDS struct {
	mu            sync.Mutex
	srv           *httptest.Server
	now           time.Time
	password      string
	n             int
	access        map[string]bool // valid access tokens
	refresh       map[string]bool // valid refresh tokens
	records       map[string]map[string]any
	blobs         int
	creates       int
	refreshes     int
	createRecords int
	deletes       []string
	rateLimitAt   int64 // when set, createSession answers 429 with this RateLimit-Reset
	failRecord    int   // status to answer createRecord with (0 = ok)
	accessTTL     time.Duration
}

func newFake(t *testing.T) *fakePDS {
	f := &fakePDS{now: t0, password: testPassword, access: map[string]bool{}, refresh: map[string]bool{},
		records: map[string]map[string]any{}, accessTTL: time.Hour}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /xrpc/com.atproto.server.createSession", f.createSession)
	mux.HandleFunc("POST /xrpc/com.atproto.server.refreshSession", f.refreshSession)
	mux.HandleFunc("POST /xrpc/com.atproto.repo.createRecord", f.authed(f.createRecord))
	mux.HandleFunc("POST /xrpc/com.atproto.repo.uploadBlob", f.authed(f.uploadBlob))
	mux.HandleFunc("GET /xrpc/com.atproto.repo.getRecord", f.authed(f.getRecord))
	mux.HandleFunc("POST /xrpc/com.atproto.repo.deleteRecord", f.authed(f.deleteRecord))
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakePDS) adapter() *Adapter {
	return New(Config{PDSURL: f.srv.URL, HTTPClient: f.srv.Client(), Now: func() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.now }})
}

func writeErr(w http.ResponseWriter, status int, name, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": name, "message": msg})
}

func (f *fakePDS) issue() map[string]string {
	f.n++
	a, r := jwtAt(f.now.Add(f.accessTTL), f.n), jwtAt(f.now.Add(60*24*time.Hour), f.n+1000)
	f.access[a], f.refresh[r] = true, true
	return map[string]string{"accessJwt": a, "refreshJwt": r, "handle": testHandle, "did": testDID}
}

func (f *fakePDS) createSession(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	if f.rateLimitAt != 0 {
		w.Header().Set("RateLimit-Reset", strconv.FormatInt(f.rateLimitAt, 10))
		writeErr(w, 429, "RateLimitExceeded", "Rate Limit Exceeded")
		return
	}
	var in map[string]string
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in["password"] != f.password || (in["identifier"] != testHandle && in["identifier"] != testDID) {
		writeErr(w, 401, "AuthenticationRequired", "Invalid identifier or password")
		return
	}
	_ = json.NewEncoder(w).Encode(f.issue())
}

func (f *fakePDS) refreshSession(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshes++
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !f.refresh[tok] {
		writeErr(w, 400, "ExpiredToken", "Token has expired")
		return
	}
	delete(f.refresh, tok)
	_ = json.NewEncoder(w).Encode(f.issue())
}

// authed rejects unknown or expired access tokens like a PDS does.
func (f *fakePDS) authed(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		ok := f.access[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		f.mu.Unlock()
		if !ok {
			writeErr(w, 400, "ExpiredToken", "Token has expired")
			return
		}
		h(w, r)
	}
}

func (f *fakePDS) createRecord(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Repo, Collection, Rkey string
		Record                 map[string]any
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createRecords++
	switch {
	case f.failRecord != 0:
		name := "InternalServerError"
		if f.failRecord == 400 {
			name = "InvalidRequest"
		}
		writeErr(w, f.failRecord, name, "boom")
	case f.records[in.Rkey] != nil:
		writeErr(w, 400, "InvalidRequest", "Could not create record: record already exists")
	case in.Collection != postType || in.Repo != testDID:
		writeErr(w, 400, "InvalidRequest", "bad collection or repo")
	default:
		f.records[in.Rkey] = in.Record
		_ = json.NewEncoder(w).Encode(map[string]string{"uri": "at://" + testDID + "/" + postType + "/" + in.Rkey, "cid": "bafycid"})
	}
}

func (f *fakePDS) uploadBlob(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.blobs++
	n := f.blobs
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"blob": map[string]any{"$type": "blob", "mimeType": r.Header.Get("Content-Type"),
		"size": len(b), "ref": map[string]string{"$link": fmt.Sprintf("bafyblob%d", n)}}})
}

func (f *fakePDS) getRecord(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rk := r.URL.Query().Get("rkey")
	if f.records[rk] == nil {
		writeErr(w, 400, "RecordNotFound", "Could not locate record")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"uri": "at://" + testDID + "/" + postType + "/" + rk, "cid": "bafycid"})
}

func (f *fakePDS) deleteRecord(w http.ResponseWriter, r *http.Request) {
	var in struct{ Rkey string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, in.Rkey)
	delete(f.records, in.Rkey)
	_, _ = w.Write([]byte("{}"))
}

// expireAccess makes every issued access token expire (the PDS answers ExpiredToken).
func (f *fakePDS) expireAccess()           { f.mu.Lock(); f.access = map[string]bool{}; f.mu.Unlock() }
func (f *fakePDS) advance(d time.Duration) { f.mu.Lock(); f.now = f.now.Add(d); f.mu.Unlock() }

func account() provider.AccountRef {
	return provider.AccountRef{ID: "acc-1", ProviderAccountID: testDID, Username: testHandle,
		Metadata: map[string]any{"handle": testHandle, "did": testDID}}
}

func req(text string) provider.PublishRequest {
	return provider.PublishRequest{IdempotencyKey: "target-1", Account: account(), AccessToken: testPassword, Text: text}
}

func connect(t *testing.T, a *Adapter) (provider.Profile, string) {
	t.Helper()
	p, secret, err := a.Verify(context.Background(), map[string]string{"handle": "@Alice.bsky.social", "app_password": testPassword})
	if err != nil {
		t.Fatal(err)
	}
	return p, secret
}
