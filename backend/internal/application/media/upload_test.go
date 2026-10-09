package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	domain "github.com/socialos/backend/internal/domain/media"
)

const mib = 1 << 20

var mp4Header = []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0, 0, 0, 0, 'm', 'p', '4', '2', 'i', 's', 'o', 'm'}

// stream yields head followed by pad zero bytes without holding them, and counts what was read.
type stream struct {
	head []byte
	pad  int64
	read atomic.Int64
	fail error // returned once everything else was read
}

func (s *stream) Read(p []byte) (int, error) {
	if len(s.head) > 0 {
		n := copy(p, s.head)
		s.head = s.head[n:]
		s.read.Add(int64(n))
		return n, nil
	}
	if s.pad <= 0 {
		if s.fail != nil {
			return 0, s.fail
		}
		return 0, io.EOF
	}
	n := min(int64(len(p)), s.pad)
	clear(p[:n])
	s.pad -= n
	s.read.Add(n)
	return int(n), nil
}

// sink is a Storage that drains uploads in 32 KiB reads, like a network client would.
type sink struct {
	mu      sync.Mutex
	objects map[string]int64
	deleted []string
	puts    atomic.Int32
	inPut   chan struct{} // closed-over signal: a Put started
	hold    chan struct{} // when set, Put blocks until it is closed
	partial bool          // keep the bytes of a failed Put, like a store that is not atomic
}

func newSink() *sink { return &sink{objects: map[string]int64{}} }

func (s *sink) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	s.puts.Add(1)
	if s.inPut != nil {
		s.inPut <- struct{}{}
	}
	if s.hold != nil {
		<-s.hold
	}
	buf := make([]byte, 32<<10)
	var n int64
	for {
		m, err := r.Read(buf)
		n += int64(m)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if s.partial {
				s.mu.Lock()
				s.objects[key] = n
				s.mu.Unlock()
			}
			return err
		}
	}
	s.mu.Lock()
	s.objects[key] = n
	s.mu.Unlock()
	return nil
}

func (s *sink) Get(context.Context, string) (io.ReadCloser, error) { return nil, errors.New("n/a") }
func (s *sink) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	delete(s.objects, key)
	s.deleted = append(s.deleted, key)
	s.mu.Unlock()
	return nil
}
func (s *sink) PresignGet(context.Context, string, time.Duration) (string, error) { return "u", nil }
func (s *sink) Ping(context.Context) error                                        { return nil }
func (s *sink) count() int                                                        { s.mu.Lock(); defer s.mu.Unlock(); return len(s.objects) }

type fakeRepo struct {
	Repo
	mu   sync.Mutex
	rows []*domain.Media
}

func (r *fakeRepo) Create(_ context.Context, m *domain.Media) error {
	r.mu.Lock()
	r.rows = append(r.rows, m)
	r.mu.Unlock()
	return nil
}

type noAudit struct{}

func (noAudit) Record(context.Context, actor.Actor, string, string, string, map[string]any) error {
	return nil
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(1_700_000_000, 0) }

func newSvc(st Storage, opts ...Option) (*Service, *fakeRepo) {
	repo := &fakeRepo{}
	return NewService(repo, st, noAudit{}, fixedClock{}, opts...), repo
}

func owner() actor.Actor { return actor.Actor{UserID: uuid.New(), Type: actor.TypeUser} }

func wantCode(t *testing.T, err error, code errs.Code) *errs.Error {
	t.Helper()
	e, ok := errs.As(err)
	if !ok || e.Code != code {
		t.Fatalf("error = %v, want code %s", err, code)
	}
	return e
}

// A 64 MiB video must stream through with a bounded heap: this is what keeps the 160 MB API container alive.
func TestUploadStreamsVideoWithBoundedHeap(t *testing.T) {
	const size = 64 * mib
	st := newSink()
	svc, repo := newSvc(st)

	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	var peak atomic.Uint64
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		var ms runtime.MemStats
		for {
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Millisecond):
				runtime.ReadMemStats(&ms)
				if ms.HeapInuse > peak.Load() {
					peak.Store(ms.HeapInuse)
				}
			}
		}
	}()
	got, err := svc.Upload(context.Background(), owner(), UploadInput{File: &stream{head: mp4Header, pad: size - int64(len(mp4Header))}, OriginalName: "clip.mp4"})
	close(stop)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if got.SizeBytes != size || got.Kind != domain.KindVideo || got.MimeType != "video/mp4" || len(got.SHA256) != 64 {
		t.Fatalf("media: %+v", got.Media)
	}
	if n := st.objects[got.StorageKey]; n != size || len(repo.rows) != 1 {
		t.Fatalf("stored %d bytes, %d rows", n, len(repo.rows))
	}
	if grew := int64(peak.Load()) - int64(base.HeapInuse); grew > 16*mib {
		t.Fatalf("heap grew by %d MiB while streaming %d MiB; the upload is being buffered", grew>>20, size>>20)
	}
}

func TestUploadRejectsOversizeMidStreamAndRemovesObject(t *testing.T) {
	for name, tc := range map[string]struct {
		head  []byte
		total int64
		limit string
	}{
		"video over 100 MB": {mp4Header, 100*mib + 1, "100 MB"},
		"image over 10 MB":  {[]byte("\x89PNG\r\n\x1a\n"), 10*mib + 1, "10 MB"},
	} {
		t.Run(name, func(t *testing.T) {
			st := newSink()
			st.partial = true // the store keeps what it received until told to delete
			svc, repo := newSvc(st)
			src := &stream{head: tc.head, pad: tc.total - int64(len(tc.head))}
			_, err := svc.Upload(context.Background(), owner(), UploadInput{File: src, OriginalName: "big"})
			e := wantCode(t, err, errs.Validation)
			if e.Fields["file"] != "too large" || !bytes.Contains([]byte(e.Message), []byte(tc.limit)) {
				t.Fatalf("error: %+v", e)
			}
			if st.count() != 0 || len(repo.rows) != 0 {
				t.Fatalf("left behind: %d objects, %d rows", st.count(), len(repo.rows))
			}
			if src.read.Load() > tc.total-int64(len(tc.head))/2 && src.pad > 0 {
				t.Fatalf("kept reading after the limit")
			}
		})
	}
}

func TestUploadRejectsWrongTypeAfterSniffingOnly(t *testing.T) {
	st := newSink()
	svc, repo := newSvc(st)
	src := &stream{head: []byte("%PDF-1.7\n"), pad: 80 * mib}
	_, err := svc.Upload(context.Background(), owner(), UploadInput{File: src, OriginalName: "movie.mp4"})
	_ = wantCode(t, err, errs.Validation)
	if st.puts.Load() != 0 || len(repo.rows) != 0 {
		t.Fatalf("a wrong type reached storage: %d puts", st.puts.Load())
	}
	if got := src.read.Load(); got > 64<<10 {
		t.Fatalf("read %d bytes of a rejected file; only the sniff window is needed", got)
	}
	_, err = svc.Upload(context.Background(), owner(), UploadInput{File: bytes.NewReader(nil)})
	_ = wantCode(t, err, errs.Validation)
}

func TestUploadBrokenSourceIsAClientErrorAndLeavesNothing(t *testing.T) {
	st := newSink()
	st.partial = true
	svc, repo := newSvc(st)
	src := &stream{head: mp4Header, pad: 6 * mib, fail: io.ErrUnexpectedEOF} // client hung up mid-body
	_, err := svc.Upload(context.Background(), owner(), UploadInput{File: src, OriginalName: "x.mp4"})
	_ = wantCode(t, err, errs.Validation)
	if st.count() != 0 || len(repo.rows) != 0 {
		t.Fatalf("left behind: %d objects, %d rows", st.count(), len(repo.rows))
	}
	// A failing store is internal, and also cleaned up.
	svc, _ = newSvc(failingStore{newSink()})
	_, err = svc.Upload(context.Background(), owner(), UploadInput{File: &stream{head: mp4Header, pad: 10}})
	_ = wantCode(t, err, errs.Internal)
}

type failingStore struct{ *sink }

func (failingStore) Put(context.Context, string, io.Reader, int64, string) error {
	return errors.New("connection refused")
}

func TestUploadConcurrencyLimitAnswersRateLimited(t *testing.T) {
	st := newSink()
	st.hold, st.inPut = make(chan struct{}), make(chan struct{}, 4)
	svc, _ := newSvc(st, WithUploadLimit(1, 30*time.Millisecond))
	first := make(chan error, 1)
	go func() {
		_, err := svc.Upload(context.Background(), owner(), UploadInput{File: &stream{head: mp4Header, pad: 100}})
		first <- err
	}()
	<-st.inPut // the first upload owns the only slot

	waited := time.Now()
	src := &stream{head: mp4Header, pad: 100}
	_, err := svc.Upload(context.Background(), owner(), UploadInput{File: src})
	_ = wantCode(t, err, errs.RateLimited)
	if time.Since(waited) < 25*time.Millisecond {
		t.Fatal("the second upload should wait briefly for a slot before giving up")
	}
	if src.read.Load() != 0 {
		t.Fatal("a refused upload must not read its body")
	}

	close(st.hold)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	// The slot is free again.
	if _, err := svc.Upload(context.Background(), owner(), UploadInput{File: &stream{head: mp4Header, pad: 100}}); err != nil {
		t.Fatalf("after release: %v", err)
	}
}

func TestUploadNeedsScope(t *testing.T) {
	svc, _ := newSvc(newSink())
	_, err := svc.Upload(context.Background(), actor.Actor{}, UploadInput{File: bytes.NewReader(mp4Header)})
	_ = wantCode(t, err, errs.Unauthenticated)
}

// One user cannot hold the global slots: a second concurrent upload by the same user is refused at once, another
// user still gets in, and the slot comes back when the first finishes.
func TestUploadOneSlotPerUser(t *testing.T) {
	st := newSink()
	st.hold, st.inPut = make(chan struct{}), make(chan struct{}, 4)
	svc, _ := newSvc(st, WithUploadLimit(2, 30*time.Millisecond))
	alice, bob := owner(), owner()
	first := make(chan error, 2)
	go func() {
		_, err := svc.Upload(context.Background(), alice, UploadInput{File: &stream{head: mp4Header, pad: 100}})
		first <- err
	}()
	<-st.inPut

	started := time.Now()
	src := &stream{head: mp4Header, pad: 100}
	_, err := svc.Upload(context.Background(), alice, UploadInput{File: src})
	_ = wantCode(t, err, errs.RateLimited)
	if time.Since(started) > 20*time.Millisecond || src.read.Load() != 0 {
		t.Fatal("the same user's second upload must be refused immediately, without reading")
	}
	go func() {
		_, err := svc.Upload(context.Background(), bob, UploadInput{File: &stream{head: mp4Header, pad: 100}})
		first <- err
	}()
	<-st.inPut // bob got the second global slot

	close(st.hold)
	for range 2 {
		if err := <-first; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Upload(context.Background(), alice, UploadInput{File: &stream{head: mp4Header, pad: 100}}); err != nil {
		t.Fatalf("slot not released: %v", err)
	}
	// A failed upload frees the user's slot too.
	_, err = svc.Upload(context.Background(), alice, UploadInput{File: bytes.NewReader([]byte("%PDF-1.7"))})
	_ = wantCode(t, err, errs.Validation)
	if _, err := svc.Upload(context.Background(), alice, UploadInput{File: &stream{head: mp4Header, pad: 100}}); err != nil {
		t.Fatalf("slot leaked after a rejected upload: %v", err)
	}
}
