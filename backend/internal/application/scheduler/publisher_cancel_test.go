package scheduler_test

import (
	"context"
	"testing"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/application/scheduler/schedulertest"
	"github.com/socialos/backend/internal/domain/post"
)

// cancelDuringPublish cancels the target and post (as a committed API cancel would) while the provider call is in
// flight, then fails the call with err.
type cancelDuringPublish struct {
	*schedulertest.Provider
	w   *schedulertest.World
	err error
}

func (p *cancelDuringPublish) Publish(context.Context, provider.PublishRequest) (provider.PublishResult, error) {
	t := p.w.Store.Targets[p.w.TargetID]
	t.Status = post.TargetCancelled
	p.w.Store.Targets[p.w.TargetID] = t
	ps := p.w.Store.Posts[p.w.PostID]
	ps.Status = post.StatusCancelled
	p.w.Store.Posts[p.w.PostID] = ps
	return provider.PublishResult{}, p.err
}

// A retryable failure must not resurrect a target that was cancelled while the provider call was running (#38).
func TestRetryLaterDoesNotOverwriteConcurrentCancel(t *testing.T) {
	prov := &cancelDuringPublish{Provider: schedulertest.NewProvider("fake"), err: perr(provider.KindRetryable, "HTTP_503")}
	w := schedulertest.NewWorld(prov)
	prov.w = w
	wantNil(t, runJob(w, retryInfo()))
	if got := w.Target().Status; got != post.TargetCancelled {
		t.Fatalf("target = %s after retryable failure, want cancelled to stay", got)
	}
	wantPost(t, w, post.StatusCancelled)
}

func retryInfo() scheduler.RetryInfo { return scheduler.RetryInfo{Retried: 1, MaxRetry: 5} }
