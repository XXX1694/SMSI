package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/testutil"
)

// memStore is an in-memory PollerStore with a real lease semantics: one holder at a time.
type memStore struct {
	mu        sync.Mutex
	offset    int64
	holder    *memLease
	acquires  int
	commits   []int64
	acquireEr error
	offsetEr  error
}

type memLease struct {
	s       *memStore
	lost    bool
	renewed int
}

func (s *memStore) Acquire(_ context.Context, _ time.Duration) (Lease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.acquireEr != nil {
		return nil, false, s.acquireEr
	}
	if s.holder != nil && !s.holder.lost {
		return nil, false, nil
	}
	s.acquires++
	s.holder = &memLease{s: s}
	return s.holder, true, nil
}

func (s *memStore) Offset(context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.offset, s.offsetEr
}

func (s *memStore) stolen() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.holder.lost = true
}

func (s *memStore) stored() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.offset
}

func (l *memLease) Renew(context.Context, time.Duration) (bool, error) {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	l.renewed++
	return !l.lost, nil
}

func (l *memLease) Commit(_ context.Context, off int64) error {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	if l.lost {
		return errors.New("lease lost")
	}
	l.s.offset = off
	l.s.commits = append(l.s.commits, off)
	return nil
}

func (l *memLease) Release(context.Context) error {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	if l.s.holder == l {
		l.s.holder = nil
	}
	return nil
}

// scriptedSource serves a queue of updates like getUpdates does: everything with
// an id below the requested offset is confirmed and dropped.
type scriptedSource struct {
	mu      sync.Mutex
	queue   []int64
	offsets []int64
	errs    []error // returned one per call before any update is served
	block   time.Duration
}

func (s *scriptedSource) push(ids ...int64) {
	s.mu.Lock()
	s.queue = append(s.queue, ids...)
	s.mu.Unlock()
}

func (s *scriptedSource) GetUpdates(ctx context.Context, offset int64, _ time.Duration) ([]json.RawMessage, error) {
	s.mu.Lock()
	s.offsets = append(s.offsets, offset)
	if len(s.errs) > 0 {
		err := s.errs[0]
		s.errs = s.errs[1:]
		s.mu.Unlock()
		return nil, err
	}
	var keep []int64
	var out []json.RawMessage
	for _, id := range s.queue {
		if id < offset {
			continue
		}
		keep = append(keep, id)
		out = append(out, json.RawMessage(fmt.Sprintf(`{"update_id":%d}`, id)))
	}
	s.queue = keep
	s.mu.Unlock()
	if len(out) == 0 {
		select { // emulate the long poll
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5*time.Millisecond + s.block):
		}
	}
	return out, nil
}

func (s *scriptedSource) lastOffset() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.offsets) == 0 {
		return -1
	}
	return s.offsets[len(s.offsets)-1]
}

func (s *scriptedSource) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.offsets)
}

type recorder struct {
	mu      sync.Mutex
	seen    []int64
	failFor map[int64]int // update id -> remaining failures
	panicOn int64
}

func (r *recorder) handle(_ context.Context, raw []byte) error {
	id, _ := UpdateID(raw)
	r.mu.Lock()
	defer r.mu.Unlock()
	if id == r.panicOn && id != 0 {
		panic("boom")
	}
	if n := r.failFor[id]; n > 0 {
		r.failFor[id] = n - 1
		return errors.New("database unavailable")
	}
	r.seen = append(r.seen, id)
	return nil
}

func (r *recorder) ids() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int64(nil), r.seen...)
}

var fast = PollerOptions{Timeout: time.Second, LockTTL: 90 * time.Millisecond, RetryMin: time.Millisecond, RetryMax: 8 * time.Millisecond,
	StandbyInterval: 10 * time.Millisecond, MaxAttempts: 3}

func start(t *testing.T, src UpdateSource, st PollerStore, h UpdateHandler, o PollerOptions) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	p := NewPoller(src, st, h, testutil.Logger(), o)
	go func() { defer close(done); p.Run(ctx) }()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("poller did not stop after cancel")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestPollerAdvancesOffsetAndHandlesEachUpdateOnce(t *testing.T) {
	src, st, rec := &scriptedSource{}, &memStore{offset: 100}, &recorder{}
	src.push(100, 101, 105)
	stop := start(t, src, st, rec.handle, fast)
	waitFor(t, "updates handled", func() bool { return len(rec.ids()) == 3 })
	waitFor(t, "offset stored", func() bool { return st.stored() == 106 })
	waitFor(t, "offset confirmed to Telegram", func() bool { return src.lastOffset() == 106 })
	src.push(106)
	waitFor(t, "a later update", func() bool { return len(rec.ids()) == 4 && st.stored() == 107 })
	stop()
	if got := rec.ids(); fmt.Sprint(got) != "[100 101 105 106]" {
		t.Fatalf("handled %v, each update must be handled once and in order", got)
	}
	if st.holder != nil {
		t.Fatal("the lease must be released on shutdown")
	}
	if src.offsets[0] != 100 {
		t.Fatalf("polling must resume from the stored offset, first call used %d", src.offsets[0])
	}
}

func TestPollerRecoversFromGetUpdatesErrors(t *testing.T) {
	src := &scriptedSource{errs: []error{
		&provider.Error{Kind: provider.KindRetryable, Provider: Name, HTTPStatus: 502, Message: "bad gateway"},
		&provider.Error{Kind: provider.KindRetryable, Provider: Name, HTTPStatus: 429, Message: "slow down", RetryAfter: 20 * time.Millisecond},
		&provider.Error{Kind: provider.KindPermanent, Provider: Name, HTTPStatus: 409, Message: "webhook is active"},
		errors.New("connection reset by peer"),
	}}
	src.push(1)
	st, rec := &memStore{}, &recorder{}
	start(t, src, st, rec.handle, fast)
	waitFor(t, "the update after 4 failed polls", func() bool { return len(rec.ids()) == 1 && st.stored() == 2 })
	if src.calls() < 5 {
		t.Fatalf("expected the loop to retry, only %d calls", src.calls())
	}
	if st.acquires != 1 {
		t.Fatalf("poll errors must not drop the lease (acquired %d times)", st.acquires)
	}
}

func TestPollerBacksOffBetweenFailures(t *testing.T) {
	bo := newBackoff(10*time.Millisecond, 80*time.Millisecond)
	for i, ceil := range []time.Duration{10, 20, 40, 80, 80, 80} {
		d := bo.next()
		ceil *= time.Millisecond
		if d < ceil/2 || d > ceil {
			t.Errorf("step %d: %v outside [%v, %v]", i, d, ceil/2, ceil)
		}
	}
	bo.reset()
	if d := bo.next(); d > 10*time.Millisecond || d < 5*time.Millisecond {
		t.Errorf("after reset the delay restarts from the minimum, got %v", d)
	}
}

func TestPollerRetriesTransientHandlerFailureWithoutLosingTheUpdate(t *testing.T) {
	src, st := &scriptedSource{}, &memStore{}
	rec := &recorder{failFor: map[int64]int{2: 2}} // update 2 fails twice, then works
	src.push(1, 2, 3)
	start(t, src, st, rec.handle, fast)
	waitFor(t, "all updates handled", func() bool { return len(rec.ids()) == 3 })
	if got := rec.ids(); fmt.Sprint(got) != "[1 2 3]" {
		t.Fatalf("handled %v: update 2 must be retried before 3 is processed", got)
	}
	waitFor(t, "offset", func() bool { return st.stored() == 4 })
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.commits[0] != 2 {
		t.Fatalf("update 1 must be confirmed before the failing update 2 blocks the queue, commits %v", st.commits)
	}
}

func TestPollerDropsAPoisonUpdateAfterMaxAttempts(t *testing.T) {
	src, st := &scriptedSource{}, &memStore{}
	rec := &recorder{failFor: map[int64]int{5: 1000}}
	src.push(5, 6)
	start(t, src, st, rec.handle, fast)
	waitFor(t, "update after the poison one", func() bool { return len(rec.ids()) == 1 && st.stored() == 7 })
	if got := rec.ids(); got[0] != 6 {
		t.Fatalf("handled %v", got)
	}
}

func TestPollerSurvivesAPanickingHandler(t *testing.T) {
	src, st := &scriptedSource{}, &memStore{}
	rec := &recorder{panicOn: 3}
	src.push(3, 4)
	start(t, src, st, rec.handle, fast)
	waitFor(t, "update after the panicking one", func() bool { return len(rec.ids()) == 1 && st.stored() == 5 })
}

func TestPollerOnlyPollsWhileHoldingTheLease(t *testing.T) {
	src, st, rec := &scriptedSource{}, &memStore{}, &recorder{}
	// Another instance holds the lease: this one must stand by and never poll.
	st.holder = &memLease{s: st}
	stop := start(t, src, st, rec.handle, fast)
	time.Sleep(60 * time.Millisecond)
	if src.calls() != 0 {
		t.Fatalf("polled %d times without the lease", src.calls())
	}
	// The other instance goes away: this one takes over and polls.
	_ = st.holder.Release(context.Background())
	src.push(1)
	waitFor(t, "takeover", func() bool { return len(rec.ids()) == 1 })
	stop()
}

func TestPollerStopsPollingWhenTheLeaseIsLost(t *testing.T) {
	src, st, rec := &scriptedSource{}, &memStore{}, &recorder{}
	start(t, src, st, rec.handle, fast)
	waitFor(t, "polling", func() bool { return src.calls() > 2 })
	st.stolen() // another instance took the lock: the next renewal reports it lost
	waitFor(t, "re-acquire", func() bool { st.mu.Lock(); defer st.mu.Unlock(); return st.acquires >= 2 })
	// It must also have renewed before losing it.
	if st.holder == nil || st.holder.lost {
		t.Fatal("after losing the lease the poller must acquire a fresh one")
	}
}

func TestPollerRetriesAcquireAndOffsetErrors(t *testing.T) {
	src, st, rec := &scriptedSource{}, &memStore{acquireEr: errors.New("redis down")}, &recorder{}
	src.push(1)
	start(t, src, st, rec.handle, fast)
	time.Sleep(30 * time.Millisecond)
	if src.calls() != 0 {
		t.Fatal("must not poll without the lease")
	}
	st.mu.Lock()
	st.acquireEr, st.offsetEr = nil, errors.New("redis blip")
	st.mu.Unlock()
	time.Sleep(30 * time.Millisecond)
	st.mu.Lock()
	st.offsetEr = nil
	st.mu.Unlock()
	waitFor(t, "recovery", func() bool { return len(rec.ids()) == 1 && st.stored() == 2 })
}

func TestPollerStopsPromptlyOnCancel(t *testing.T) {
	src := &scriptedSource{block: 10 * time.Second} // a long poll that would outlast the test
	st, rec := &memStore{}, &recorder{}
	stop := start(t, src, st, rec.handle, fast)
	waitFor(t, "polling", func() bool { return src.calls() > 0 })
	begin := time.Now()
	stop()
	if time.Since(begin) > 2*time.Second {
		t.Fatal("cancel must interrupt the long poll")
	}
	if st.holder != nil {
		t.Fatal("lease must be released")
	}
}

func TestPollerOptionDefaults(t *testing.T) {
	var o PollerOptions
	o.defaults()
	if o.Timeout != 25*time.Second || o.LockTTL != 60*time.Second || o.RetryMin != time.Second || o.RetryMax != 30*time.Second ||
		o.StandbyInterval != 10*time.Second || o.MaxAttempts != 5 {
		t.Fatalf("%+v", o)
	}
}
