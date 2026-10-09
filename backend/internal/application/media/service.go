// Package media handles uploads (validated, sniffed, hashed) and media lookup.
package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/gif"  // register decoder for dimensions
	_ "image/jpeg" // register decoder for dimensions
	_ "image/png"  // register decoder for dimensions
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp" // register decoder for dimensions

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
	quota   port.QuotaGate
	tx      port.TxRunner
}

// NewService creates the media service.
func NewService(repo Repo, storage Storage, audit port.AuditRecorder, clock port.Clock) *Service {
	return &Service{repo: repo, storage: storage, audit: audit, clock: clock}
}

// WithQuota makes uploads count against the storage limit. tx must be the runner that backs the quota gate.
func (s *Service) WithQuota(q port.QuotaGate, tx port.TxRunner) *Service {
	s.quota, s.tx = q, tx
	return s
}

// Storage returns the storage port (used by the publisher to stream bytes).
func (s *Service) Storage() Storage { return s.storage }

// UploadInput is a file to upload. File must be seekable (multipart temp file).
type UploadInput struct {
	File         io.ReadSeeker
	Size         int64
	OriginalName string
}

// Upload validates (size, sniffed MIME allow-list), hashes and stores a file.
func (s *Service) Upload(ctx context.Context, a actor.Actor, in UploadInput) (*WithURL, error) {
	if err := a.Require(apikey.MediaWrite); err != nil {
		return nil, err
	}
	if in.Size <= 0 {
		return nil, errs.Validationf("file is empty").WithField("file", "empty")
	}
	mt, err := mimetype.DetectReader(in.File)
	if err != nil {
		return nil, errs.Validationf("cannot read file")
	}
	mime := mt.String()
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	kind, ext, ok := domain.Classify(mime)
	if !ok {
		return nil, errs.Validationf("unsupported file type %s", mime).WithField("file", "allowed: jpeg, png, webp, gif, mp4, mov")
	}
	if in.Size > domain.MaxBytes(kind) {
		return nil, errs.Validationf("%s exceeds %d MB limit", kind, domain.MaxBytes(kind)>>20).WithField("file", "too large")
	}
	m := &domain.Media{
		ID: uuid.New(), UserID: a.UserID, Kind: kind, MimeType: mime, SizeBytes: in.Size,
		OriginalName: sanitizeName(in.OriginalName), Status: domain.StatusReady,
	}
	m.StorageKey = "users/" + a.UserID.String() + "/media/" + m.ID.String() + ext
	if s.quota != nil {
		// Cheap early refusal, before the bytes are stored; the authoritative check runs under the lock below.
		if err := s.quota.EnforceMedia(ctx, a.UserID, in.Size); err != nil {
			return nil, err
		}
	}
	if kind == domain.KindImage {
		if err := s.readDimensions(in.File, m); err != nil {
			return nil, err
		}
	}
	if _, err := in.File.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	h := sha256.New()
	if err := s.storage.Put(ctx, m.StorageKey, io.TeeReader(in.File, h), in.Size, mime); err != nil {
		return nil, errs.Wrap(errs.Internal, "storage upload failed", err)
	}
	m.SHA256 = hex.EncodeToString(h.Sum(nil))
	if err := s.createCounted(ctx, m); err != nil {
		_ = s.storage.Delete(context.WithoutCancel(ctx), m.StorageKey)
		return nil, err
	}
	_ = s.audit.Record(ctx, a, audit.ActionMediaUploaded, "media", m.ID.String(),
		map[string]any{"mime_type": mime, "size_bytes": in.Size})
	out := s.withURL(ctx, []domain.Media{*m})[0]
	return &out, nil
}

// createCounted inserts the row. With a quota gate, the check and the insert run in one transaction under the user's
// lock, so concurrent uploads cannot overshoot the limit together.
func (s *Service) createCounted(ctx context.Context, m *domain.Media) error {
	if s.quota == nil || s.tx == nil {
		return s.repo.Create(ctx, m)
	}
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.quota.EnforceMedia(ctx, m.UserID, m.SizeBytes); err != nil {
			return err
		}
		return s.repo.Create(ctx, m)
	})
}

func (s *Service) readDimensions(f io.ReadSeeker, m *domain.Media) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return errs.Validationf("image is corrupt or unreadable").WithField("file", "invalid image")
	}
	if cfg.Width > 20000 || cfg.Height > 20000 {
		return errs.Validationf("image dimensions too large").WithField("file", "max 20000px")
	}
	m.Width, m.Height = cfg.Width, cfg.Height
	return nil
}

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
