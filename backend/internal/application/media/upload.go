package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"image"
	_ "image/gif"  // register decoder for dimensions
	_ "image/jpeg" // register decoder for dimensions
	_ "image/png"  // register decoder for dimensions
	"io"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp" // register decoder for dimensions

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	domain "github.com/socialos/backend/internal/domain/media"
)

// Defaults for the upload limiter (D-015): two uploads hold at most two part buffers, a waiter gives up after 5 s.
const (
	DefaultUploadConcurrency = 2
	DefaultUploadWait        = 5 * time.Second
	sniffBytes               = 3072 // what mimetype.DetectReader reads
)

// Option tunes the service.
type Option func(*Service)

// WithUploadLimit allows at most n uploads at once; a caller waits up to wait for a slot and then gets RATE_LIMITED.
func WithUploadLimit(n int, wait time.Duration) Option {
	return func(s *Service) {
		s.slots = make(chan struct{}, max(n, 1))
		if wait > 0 {
			s.wait = wait
		}
	}
}

// UploadInput is a file to upload. File is read once, front to back; its length is not known in advance.
type UploadInput struct {
	File         io.Reader
	OriginalName string
}

// Upload validates (sniffed MIME allow-list, size enforced while streaming), hashes and stores a file. Videos are
// streamed to storage without being buffered; images (at most 10 MB) are read into memory to get their dimensions.
func (s *Service) Upload(ctx context.Context, a actor.Actor, in UploadInput) (*WithURL, error) {
	if err := a.Require(apikey.MediaWrite); err != nil {
		return nil, err
	}
	release, err := s.acquire(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	defer release()

	head := make([]byte, sniffBytes)
	n, err := io.ReadFull(in.File, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, readFailure(err)
	}
	if n == 0 {
		return nil, errs.Validationf("file is empty").WithField("file", "empty")
	}
	head = head[:n]
	mime := mimetype.Detect(head).String()
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	kind, ext, ok := domain.Classify(mime)
	if !ok {
		return nil, errs.Validationf("unsupported file type %s", mime).WithField("file", "allowed: jpeg, png, webp, gif, mp4, mov")
	}
	m := &domain.Media{ID: uuid.New(), UserID: a.UserID, Kind: kind, MimeType: mime,
		OriginalName: sanitizeName(in.OriginalName), Status: domain.StatusReady}
	m.StorageKey = "users/" + a.UserID.String() + "/media/" + m.ID.String() + ext

	g := newGuard(io.MultiReader(bytes.NewReader(head), in.File), domain.MaxBytes(kind), kind)
	if kind == domain.KindImage {
		err = s.storeImage(ctx, m, g)
	} else {
		err = s.storage.Put(ctx, m.StorageKey, g, -1, mime)
	}
	if err != nil {
		// Whatever the store managed to write is removed: a violation must not leave an object behind.
		_ = s.storage.Delete(context.WithoutCancel(ctx), m.StorageKey)
		if cause := g.failure(); cause != nil {
			return nil, cause
		}
		if _, ok := errs.As(err); ok {
			return nil, err
		}
		return nil, errs.Wrap(errs.Internal, "storage upload failed", err)
	}
	m.SizeBytes, m.SHA256 = g.n, hex.EncodeToString(g.sum.Sum(nil))
	if err := s.createCounted(ctx, m); err != nil {
		_ = s.storage.Delete(context.WithoutCancel(ctx), m.StorageKey)
		return nil, err
	}
	_ = s.audit.Record(ctx, a, audit.ActionMediaUploaded, "media", m.ID.String(),
		map[string]any{"mime_type": mime, "size_bytes": m.SizeBytes})
	out := s.withURL(ctx, []domain.Media{*m})[0]
	return &out, nil
}

// acquire takes the user's single upload slot (a second concurrent upload by the same user is refused at once) and
// then a global slot, waiting briefly for it. Taken before the body is read, so a refused upload costs nothing.
func (s *Service) acquire(ctx context.Context, user uuid.UUID) (release func(), err error) {
	s.mu.Lock()
	if _, busy := s.active[user]; busy {
		s.mu.Unlock()
		return nil, errs.New(errs.RateLimited, "another upload of yours is still running, retry when it finishes")
	}
	s.active[user] = struct{}{}
	s.mu.Unlock()
	freeUser := func() { s.mu.Lock(); delete(s.active, user); s.mu.Unlock() }

	ctx, cancel := context.WithTimeout(ctx, s.wait)
	defer cancel()
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots; freeUser() }, nil
	case <-ctx.Done():
		freeUser()
		return nil, errs.Wrap(errs.RateLimited, "server is busy, retry shortly", ctx.Err())
	}
}

func (s *Service) storeImage(ctx context.Context, m *domain.Media, g *guard) error {
	buf, err := readBounded(g, int(g.limit))
	if err != nil {
		return err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		return errs.Validationf("image is corrupt or unreadable").WithField("file", "invalid image")
	}
	if cfg.Width > 20000 || cfg.Height > 20000 {
		return errs.Validationf("image dimensions too large").WithField("file", "max 20000px")
	}
	m.Width, m.Height = cfg.Width, cfg.Height
	return s.storage.Put(ctx, m.StorageKey, bytes.NewReader(buf), int64(len(buf)), m.MimeType)
}

// guard enforces the size limit while the bytes stream through it, and hashes and counts them.
type guard struct {
	r      io.Reader
	limit  int64
	kind   domain.Kind
	n      int64
	sum    hash.Hash
	over   bool
	srcErr error
}

func newGuard(r io.Reader, limit int64, kind domain.Kind) *guard {
	return &guard{r: r, limit: limit, kind: kind, sum: sha256.New()}
}

func (g *guard) Read(p []byte) (int, error) {
	if g.over {
		return 0, errTooLarge
	}
	n, err := g.r.Read(p)
	if g.n+int64(n) > g.limit {
		g.over = true
		return 0, errTooLarge
	}
	g.n += int64(n)
	g.sum.Write(p[:n])
	if err != nil && !errors.Is(err, io.EOF) {
		g.srcErr = err
	}
	return n, err
}

var errTooLarge = errors.New("media: file exceeds the size limit")

// failure maps what went wrong with the stream to a client error; nil means the storage itself failed.
func (g *guard) failure() error {
	if g.over {
		return errs.Validationf("%s exceeds %d MB limit", g.kind, domain.MaxBytes(g.kind)>>20).WithField("file", "too large")
	}
	if g.srcErr != nil {
		return readFailure(g.srcErr)
	}
	return nil
}

func readFailure(err error) error {
	return errs.Wrap(errs.Validation, "upload was interrupted or malformed", err).WithField("file", "unreadable")
}

// readBounded reads the whole stream into one buffer allocated up front at the limit, so it never regrows (a growing
// io.ReadAll would peak at about twice the size). The guard fails the read if the stream is longer than the limit.
func readBounded(g *guard, limit int) ([]byte, error) {
	buf := make([]byte, limit)
	n, err := io.ReadFull(g, buf)
	if err == nil {
		var one [1]byte
		if _, err = g.Read(one[:]); err == nil {
			err = errTooLarge // unreachable: the guard fails first; kept so a longer stream can never pass
		}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return buf[:n], nil
	}
	return nil, err
}
