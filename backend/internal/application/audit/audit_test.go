package audit

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	domain "github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
)

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

type fakeRepo struct {
	inserted  []domain.Entry
	listed    []domain.Entry
	gotPage   port.Page
	gotAction string
}

func (f *fakeRepo) Insert(_ context.Context, e *domain.Entry) error {
	f.inserted = append(f.inserted, *e)
	return nil
}
func (f *fakeRepo) List(_ context.Context, _ uuid.UUID, action string, p port.Page) ([]domain.Entry, error) {
	f.gotPage, f.gotAction = p, action
	return f.listed, nil
}

var now = time.Date(2026, 10, 7, 15, 30, 0, 0, time.UTC)

func TestRecordAttributesTheActor(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, fakeClock{now})
	uid, kid := uuid.New(), uuid.New()
	a := actor.Actor{UserID: uid, Type: actor.TypeAPIKey, ID: kid.String(), Label: "mcp agent", RequestID: "req-1", IP: "203.0.113.9"}
	if err := s.Record(context.Background(), a, "post.created", "post", "p1", map[string]any{"x": 1}); err != nil {
		t.Fatal(err)
	}
	e := repo.inserted[0]
	if e.UserID != uid || e.ActorType != "api_key" || e.ActorID != kid.String() || e.ActorLabel != "mcp agent" || e.Action != "post.created" ||
		e.ResourceType != "post" || e.ResourceID != "p1" || e.RequestID != "req-1" || e.IP != "203.0.113.9" || !e.CreatedAt.Equal(now) || e.Metadata["x"] != 1 {
		t.Fatalf("entry: %+v", e)
	}
	if err := s.Record(context.Background(), a, "x", "y", "z", nil); err != nil || repo.inserted[1].Metadata == nil {
		t.Fatalf("nil metadata must become an empty object: %+v %v", repo.inserted[1], err)
	}
}

func TestListIsSessionOnlyAndPaginates(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, fakeClock{now})
	for i := 0; i < 4; i++ {
		repo.listed = append(repo.listed, domain.Entry{ID: uuid.New(), CreatedAt: now.Add(-time.Duration(i) * time.Minute)})
	}
	sess := actor.Actor{UserID: uuid.New(), Type: actor.TypeUser, SessionID: uuid.New()}
	res, err := s.List(context.Background(), sess, "", port.Page{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if repo.gotPage.Limit != 4 {
		t.Fatalf("repo must be asked for limit+1 rows, got %d", repo.gotPage.Limit)
	}
	if len(res.Items) != 3 || res.NextCursor == "" {
		t.Fatalf("page: %d items, cursor %q", len(res.Items), res.NextCursor)
	}
	cur, err := port.DecodeCursor(res.NextCursor)
	if err != nil || cur.ID != res.Items[2].ID || !cur.At.Equal(res.Items[2].CreatedAt) {
		t.Fatalf("cursor must point at the last returned row: %+v %v", cur, err)
	}
	repo.listed = repo.listed[:2]
	if res, _ := s.List(context.Background(), sess, "", port.Page{Limit: 3}); len(res.Items) != 2 || res.NextCursor != "" {
		t.Fatalf("last page: %+v", res)
	}

	if _, err := s.List(context.Background(), sess, "mcp.tool_call", port.Page{Limit: 3}); err != nil || repo.gotAction != "mcp.tool_call" {
		t.Fatalf("the action filter must reach the repo: %q %v", repo.gotAction, err)
	}

	key := actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey}
	if _, err := s.List(context.Background(), key, "", port.Page{Limit: 3}); !errs.Is(err, errs.Forbidden) {
		t.Fatalf("API keys must not read the audit log: %v", err)
	}
	if _, err := s.List(context.Background(), actor.Actor{}, "", port.Page{Limit: 3}); !errs.Is(err, errs.Unauthenticated) {
		t.Fatalf("anonymous: %v", err)
	}
}
