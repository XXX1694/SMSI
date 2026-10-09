// Package media handles uploads (validated, sniffed, hashed) and media lookup.
package media

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	domain "github.com/socialos/backend/internal/domain/media"
)

// Repo persists media rows (tenant-scoped).
type Repo interface {
	Create(ctx context.Context, m *domain.Media) error
	Get(ctx context.Context, userID, id uuid.UUID) (*domain.Media, error)
	GetMany(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) ([]domain.Media, error)
	List(ctx context.Context, userID uuid.UUID, page port.Page) ([]domain.Media, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
	IsReferenced(ctx context.Context, userID, id uuid.UUID) (bool, error)
}

// Storage is the object storage port (S3/MinIO/R2 or in-memory).
type Storage interface {
	// Put stores r under key. size is the exact length, or -1 when unknown (the store streams in bounded parts and
	// must leave no object behind when r fails).
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	Ping(ctx context.Context) error
}

// URLTTL is the lifetime of presigned download URLs.
const URLTTL = 15 * time.Minute

// Service implements media use cases.
type Service struct {
	repo    Repo
	storage Storage
	audit   port.AuditRecorder
	clock   port.Clock
	slots   chan struct{} // bounds concurrent uploads (D-015)
	wait    time.Duration
}

// NewService creates the media service.
func NewService(repo Repo, storage Storage, audit port.AuditRecorder, clock port.Clock, opts ...Option) *Service {
	s := &Service{repo: repo, storage: storage, audit: audit, clock: clock,
		slots: make(chan struct{}, DefaultUploadConcurrency), wait: DefaultUploadWait}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Storage returns the storage port (used by the publisher to stream bytes).
func (s *Service) Storage() Storage { return s.storage }

func sanitizeName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '"' {
			return -1
		}
		return r
	}, n)
	for utf8.RuneCountInString(n) > 200 {
		_, size := utf8.DecodeLastRuneInString(n)
		n = n[:len(n)-size]
	}
	if n == "." || n == "/" {
		return ""
	}
	return n
}

// WithURL pairs media with a short-lived download URL.
type WithURL struct {
	domain.Media
	URL string
}

// Get returns media with a presigned URL.
func (s *Service) Get(ctx context.Context, a actor.Actor, id uuid.UUID) (*WithURL, error) {
	if err := a.Require(apikey.PostsRead); err != nil {
		return nil, err
	}
	m, err := s.repo.Get(ctx, a.UserID, id)
	if err != nil {
		return nil, err
	}
	u, err := s.storage.PresignGet(ctx, m.StorageKey, URLTTL)
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "cannot sign media url", err)
	}
	return &WithURL{Media: *m, URL: u}, nil
}

// withURL presigns a download URL; signing is a local computation and a
// failure only means the item is returned without a URL.
func (s *Service) withURL(ctx context.Context, items []domain.Media) []WithURL {
	out := make([]WithURL, len(items))
	for i := range items {
		out[i].Media = items[i]
		if u, err := s.storage.PresignGet(ctx, items[i].StorageKey, URLTTL); err == nil {
			out[i].URL = u
		}
	}
	return out
}

// ByIDs resolves the tenant's media in the given order (unknown ids are skipped),
// each with a short-lived URL. Used to embed media in post details.
func (s *Service) ByIDs(ctx context.Context, a actor.Actor, ids []uuid.UUID) ([]WithURL, error) {
	if err := a.Require(apikey.PostsRead); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []WithURL{}, nil
	}
	found, err := s.repo.GetMany(ctx, a.UserID, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]domain.Media, len(found))
	for _, m := range found {
		byID[m.ID] = m
	}
	ordered := make([]domain.Media, 0, len(ids))
	for _, id := range ids {
		if m, ok := byID[id]; ok {
			ordered = append(ordered, m)
		}
	}
	return s.withURL(ctx, ordered), nil
}

// List returns the tenant's media, newest first, each with a short-lived URL.
func (s *Service) List(ctx context.Context, a actor.Actor, page port.Page) (port.Result[WithURL], error) {
	if err := a.Require(apikey.PostsRead); err != nil {
		return port.Result[WithURL]{}, err
	}
	items, err := s.repo.List(ctx, a.UserID, port.Page{Limit: page.Limit + 1, Cursor: page.Cursor})
	if err != nil {
		return port.Result[WithURL]{}, err
	}
	res := port.Paginate(items, page.Limit, func(m domain.Media) port.Cursor { return port.Cursor{At: m.CreatedAt, ID: m.ID} })
	return port.Result[WithURL]{Items: s.withURL(ctx, res.Items), NextCursor: res.NextCursor}, nil
}

// Delete removes unreferenced media and its object.
func (s *Service) Delete(ctx context.Context, a actor.Actor, id uuid.UUID) error {
	if err := a.Require(apikey.MediaWrite); err != nil {
		return err
	}
	m, err := s.repo.Get(ctx, a.UserID, id)
	if err != nil {
		return err
	}
	used, err := s.repo.IsReferenced(ctx, a.UserID, id)
	if err != nil {
		return err
	}
	if used {
		return errs.New(errs.Conflict, "media is attached to a post")
	}
	if err := s.repo.Delete(ctx, a.UserID, id); err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, m.StorageKey); err != nil {
		return errs.Wrap(errs.Internal, "storage delete failed", err)
	}
	return s.audit.Record(ctx, a, audit.ActionMediaDeleted, "media", id.String(), nil)
}
