package account

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/dataexport"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/infrastructure/storage"
)

type rig struct {
	svc   *ExportService
	repo  *memExports
	data  *memData
	store *storage.Memory
	queue *queued
	audit *auditLog
	clock *clockAt
	owner uuid.UUID
}

func newRig(t *testing.T) *rig {
	t.Helper()
	clk := &clockAt{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	r := &rig{repo: newMemExports(clk), data: &memData{rows: map[Dataset][]Row{}}, store: storage.NewMemory(),
		queue: &queued{}, audit: &auditLog{}, clock: clk, owner: uuid.New()}
	r.data.rows[DatasetProfile] = []Row{row(r.owner, map[string]any{"id": r.owner, "email": "a@example.com"})}
	r.svc = NewExportService(Deps{Exports: r.repo, Data: r.data, Store: r.store, Queue: r.queue, Tx: passTx{}, Audit: r.audit, Clock: clk,
		Retention: 7 * 24 * time.Hour})
	return r
}

func (r *rig) session() actor.Actor {
	return actor.Actor{UserID: r.owner, Type: actor.TypeUser, ID: r.owner.String(), SessionID: uuid.New()}
}

func (r *rig) key() actor.Actor {
	return actor.Actor{UserID: r.owner, Type: actor.TypeAPIKey, ID: "k", APIKeyID: uuid.New()}
}

// wantCode fails unless err carries code; the error is returned for further checks.
func wantCode(t *testing.T, err error, code errs.Code) *errs.Error {
	t.Helper()
	e, ok := errs.As(err)
	if !ok || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	return e
}

func TestRequestRulesAndQueue(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()

	// API keys can never request an export.
	_, err := r.svc.Request(ctx, r.key())
	_ = wantCode(t, err, errs.Forbidden)

	e, err := r.svc.Request(ctx, r.session())
	if err != nil || e.Status != dataexport.StatusPending {
		t.Fatalf("request: %v %+v", err, e)
	}
	if len(r.queue.ids) != 1 || r.queue.ids[0] != e.ID {
		t.Fatalf("not queued: %v", r.queue.ids)
	}
	if got := r.audit.actions(); len(got) != 1 || got[0] != audit.ActionExportRequested {
		t.Fatalf("audit: %v", got)
	}
	// One at a time.
	_, err = r.svc.Request(ctx, r.session())
	_ = wantCode(t, err, errs.Conflict)

	// After a successful build the 24 h cooldown applies and says how long to wait.
	if err := r.svc.Build(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	r.clock.t = r.clock.t.Add(2 * time.Hour)
	_, err = r.svc.Request(ctx, r.session())
	if got := wantCode(t, err, errs.RateLimited); got.RetryAfter != 22*time.Hour {
		t.Fatalf("retry after = %v", got.RetryAfter)
	}
	// Later a new export replaces the old one, which is expired at once and swept.
	r.clock.t = r.clock.t.Add(23 * time.Hour)
	n, err := r.svc.Request(ctx, r.session())
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	old, _ := r.repo.Get(ctx, r.owner, e.ID)
	if old.Status != dataexport.StatusExpired || n.ID == e.ID {
		t.Fatalf("old export not expired: %+v", old)
	}
	if err := r.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if r.store.Has(old.StorageKey) {
		t.Fatal("expired archive still stored")
	}
}

func TestQueueFailureClosesTheRow(t *testing.T) {
	r := newRig(t)
	r.queue.err = errBoom
	_, err := r.svc.Request(context.Background(), r.session())
	_ = wantCode(t, err, errs.Internal)
	items, _ := r.svc.List(context.Background(), r.session())
	if len(items) != 1 || items[0].Status != dataexport.StatusFailed || items[0].ErrorCode != dataexport.ErrQueueFailed {
		t.Fatalf("row not failed: %+v", items)
	}
	r.queue.err = nil
	if _, err := r.svc.Request(context.Background(), r.session()); err != nil {
		t.Fatalf("user blocked after queue failure: %v", err)
	}
}

func readZip(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		out[f.Name], _ = io.ReadAll(rc)
		_ = rc.Close()
	}
	return out
}

func keys(m map[string][]byte) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestBuildWritesEveryFileAndMedia(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	// More rows than one batch, so paging is exercised.
	for i := 0; i < batchSize+5; i++ {
		id := uuid.New()
		r.data.rows[DatasetPosts] = append(r.data.rows[DatasetPosts], row(id, map[string]any{"id": id, "content": "post"}))
	}
	sort.Slice(r.data.rows[DatasetPosts], func(i, j int) bool {
		return string(r.data.rows[DatasetPosts][i].ID[:]) < string(r.data.rows[DatasetPosts][j].ID[:])
	})
	m := MediaFile{ID: uuid.New(), Kind: "image", MimeType: "image/png", SizeBytes: 4, StorageKey: "users/x/media/a.png", SHA256: "abc"}
	gone := MediaFile{ID: uuid.New(), Kind: "image", MimeType: "image/png", StorageKey: "users/x/media/gone.png"}
	r.data.media = []MediaFile{m, gone}
	sort.Slice(r.data.media, func(i, j int) bool { return string(r.data.media[i].ID[:]) < string(r.data.media[j].ID[:]) })
	_ = r.store.Put(ctx, m.StorageKey, strings.NewReader("PNG!"), 4, "image/png")

	e, _ := r.svc.Request(ctx, r.session())
	if err := r.svc.Build(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := r.repo.Get(ctx, r.owner, e.ID)
	if got.Status != dataexport.StatusReady || got.StorageKey != dataexport.ObjectKey(r.owner, e.ID) || got.SizeBytes == 0 {
		t.Fatalf("not ready: %+v", got)
	}
	if want := r.clock.t.Add(7 * 24 * time.Hour); !got.ExpiresAt.Equal(want) {
		t.Fatalf("expires %v want %v", got.ExpiresAt, want)
	}
	rc, _ := r.store.Get(ctx, got.StorageKey)
	b, _ := io.ReadAll(rc)
	if int64(len(b)) != got.SizeBytes {
		t.Fatalf("size %d != stored %d", got.SizeBytes, len(b))
	}
	files := readZip(t, b)
	want := []string{"README.txt", "api_keys.json", "approvals.json", "audit_logs.json", "mcp_connections.json", "media.json",
		"media/" + m.ID.String() + ".png", "media/MISSING.txt", "posts.json", "profile.json", "social_accounts.json"}
	sort.Strings(want)
	if got := keys(files); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("files %v\nwant %v", got, want)
	}
	var posts []map[string]any
	if err := json.Unmarshal(files["posts.json"], &posts); err != nil || len(posts) != batchSize+5 {
		t.Fatalf("posts.json: %v, %d rows", err, len(posts))
	}
	var empty []any
	if err := json.Unmarshal(files["api_keys.json"], &empty); err != nil || len(empty) != 0 {
		t.Fatalf("empty dataset is not []: %q", files["api_keys.json"])
	}
	if string(files["media/"+m.ID.String()+".png"]) != "PNG!" || !strings.Contains(string(files["media/MISSING.txt"]), gone.ID.String()) {
		t.Fatal("media content or missing list wrong")
	}
	if strings.Contains(string(files["media.json"]), "users/x") {
		t.Fatal("storage keys must not be exported")
	}
	for _, id := range r.data.asked {
		if id != r.owner {
			t.Fatalf("data read for another user: %v", id)
		}
	}
	if a := r.audit.actions(); a[len(a)-1] != audit.ActionExportReady {
		t.Fatalf("audit: %v", a)
	}
}

func TestBuildFailureLeavesNoObjectAndCanBeRetriedByTheUser(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	m := MediaFile{ID: uuid.New(), Kind: "image", MimeType: "image/png", StorageKey: "k"}
	r.data.media = []MediaFile{m}
	r.svc.d.Store = failingStore{r.store}
	e, _ := r.svc.Request(ctx, r.session())
	if err := r.svc.Build(ctx, e.ID); err != nil {
		t.Fatalf("a failed build is recorded, not returned: %v", err)
	}
	got, _ := r.repo.Get(ctx, r.owner, e.ID)
	if got.Status != dataexport.StatusFailed || got.ErrorCode != dataexport.ErrBuildFailed {
		t.Fatalf("not failed: %+v", got)
	}
	if r.store.Has(dataexport.ObjectKey(r.owner, e.ID)) {
		t.Fatal("object left behind")
	}
	if _, err := r.svc.Request(ctx, r.session()); err != nil {
		t.Fatalf("user cannot retry: %v", err)
	}
	if a := r.audit.actions(); !contains(a, audit.ActionExportFailed) {
		t.Fatalf("no failure audit: %v", a)
	}
}

// failingStore refuses Put like a store that is down; everything else works.
type failingStore struct{ *storage.Memory }

func (failingStore) Put(context.Context, string, io.Reader, int64, string) error { return errBoom }

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestBuildIsClaimedOnce(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	e, _ := r.svc.Request(ctx, r.session())
	if err := r.svc.Build(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	puts := r.store.Len()
	if err := r.svc.Build(ctx, e.ID); err != nil || r.store.Len() != puts {
		t.Fatalf("second delivery built again: %v", err)
	}
	if err := r.svc.Build(ctx, uuid.New()); err != nil {
		t.Fatalf("unknown id must be a no-op: %v", err)
	}
}

func TestDownloadLinkRules(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	e, _ := r.svc.Request(ctx, r.session())

	// Not ready yet.
	_, err := r.svc.Download(ctx, r.session(), e.ID)
	_ = wantCode(t, err, errs.Conflict)
	_ = r.svc.Build(ctx, e.ID)

	// Another user and an API key get nothing.
	other := actor.Actor{UserID: uuid.New(), Type: actor.TypeUser, SessionID: uuid.New()}
	_, err = r.svc.Download(ctx, other, e.ID)
	_ = wantCode(t, err, errs.NotFound)
	_, err = r.svc.Download(ctx, r.key(), e.ID)
	_ = wantCode(t, err, errs.Forbidden)

	l, err := r.svc.Download(ctx, r.session(), e.ID)
	if err != nil || !strings.HasPrefix(l.URL, "memory://") || !l.ExpiresAt.Equal(r.clock.t.Add(LinkTTL)) {
		t.Fatalf("link: %v %+v", err, l)
	}
	if a := r.audit.actions(); a[len(a)-1] != audit.ActionExportDownloaded {
		t.Fatalf("audit: %v", a)
	}

	// Past its expiry the export is reported expired and gives no link, even before the sweep ran.
	r.clock.t = r.clock.t.Add(8 * 24 * time.Hour)
	_, err = r.svc.Download(ctx, r.session(), e.ID)
	_ = wantCode(t, err, errs.Conflict)
	items, _ := r.svc.List(ctx, r.session())
	if items[0].Status != dataexport.StatusExpired {
		t.Fatalf("status %s", items[0].Status)
	}
}

func TestSweepDeletesExpiredRepairsStuckAndRequeuesLost(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	done, _ := r.svc.Request(ctx, r.session())
	_ = r.svc.Build(ctx, done.ID)
	r.clock.t = r.clock.t.Add(8 * 24 * time.Hour)

	lost, _ := r.svc.Request(ctx, r.session()) // pending, its task is never delivered
	r.queue.ids = nil
	r.clock.t = r.clock.t.Add(11 * time.Minute)

	if err := r.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if r.store.Has(dataexport.ObjectKey(r.owner, done.ID)) {
		t.Fatal("expired archive not deleted")
	}
	if len(r.queue.ids) != 1 || r.queue.ids[0] != lost.ID {
		t.Fatalf("lost export not re-queued: %v", r.queue.ids)
	}

	// A worker that died mid-build leaves the row running; after two hours it fails so the user can ask again.
	if _, err := r.repo.Claim(ctx, lost.ID); err != nil {
		t.Fatal(err)
	}
	r.clock.t = r.clock.t.Add(3 * time.Hour)
	_ = r.svc.Sweep(ctx)
	got, _ := r.repo.Get(ctx, r.owner, lost.ID)
	if got.Status != dataexport.StatusFailed || got.ErrorCode != dataexport.ErrTimedOut {
		t.Fatalf("stuck export: %+v", got)
	}
}

func TestShutdownMidBuildFailsTheExportAsInterrupted(t *testing.T) {
	r := newRig(t)
	e, _ := r.svc.Request(context.Background(), r.session())
	ctx, cancel := context.WithCancel(context.Background())
	r.data.onRows = cancel // the worker is told to stop while the archive is being written
	if err := r.svc.Build(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := r.repo.Get(context.Background(), r.owner, e.ID)
	if got.Status != dataexport.StatusFailed || got.ErrorCode != dataexport.ErrInterrupted {
		t.Fatalf("export left %+v", got)
	}
	if r.store.Has(dataexport.ObjectKey(r.owner, e.ID)) {
		t.Fatal("partial archive kept")
	}
}
