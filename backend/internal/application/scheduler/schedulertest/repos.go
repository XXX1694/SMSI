package schedulertest

import (
	"context"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// Targets fakes scheduler.Targets.
type Targets struct{ S *Store }

// LockTarget returns (nil,false,nil) when S.Locked holds the id (SKIP LOCKED), NotFound when absent.
func (f Targets) LockTarget(_ context.Context, id uuid.UUID) (*post.Target, bool, error) {
	if err := f.S.fail("Targets.LockTarget"); err != nil {
		return nil, false, err
	}
	t, ok := f.S.Targets[id]
	if !ok {
		return nil, false, notFound("post target")
	}
	if f.S.Locked[id] {
		return nil, false, nil
	}
	return &t, true, nil
}

// LockTargetWait succeeds for an existing target of the owner (locks never block in memory).
func (f Targets) LockTargetWait(_ context.Context, userID, id uuid.UUID) error {
	if err := f.S.fail("Targets.LockTargetWait"); err != nil {
		return err
	}
	if t, ok := f.S.Targets[id]; !ok || t.UserID != userID {
		return notFound("post target")
	}
	return nil
}

// UpdateTarget stores the row and stamps UpdatedAt from the clock.
func (f Targets) UpdateTarget(_ context.Context, t *post.Target) error {
	if err := f.S.fail("Targets.UpdateTarget"); err != nil {
		return err
	}
	if _, ok := f.S.Targets[t.ID]; !ok {
		return notFound("post target")
	}
	c := *t
	c.UpdatedAt = f.S.Clock.Now()
	f.S.Targets[t.ID] = c
	return nil
}

// ListTargets returns the targets of a post of the owner, ordered by id.
func (f Targets) ListTargets(_ context.Context, userID, postID uuid.UUID) ([]post.Target, error) {
	if err := f.S.fail("Targets.ListTargets"); err != nil {
		return nil, err
	}
	var out []post.Target
	for _, t := range f.S.Targets {
		if t.PostID == postID && t.UserID == userID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out, nil
}

// LatestAttempt returns the attempt with the highest number, or (nil,nil).
func (f Targets) LatestAttempt(_ context.Context, targetID uuid.UUID) (*post.Attempt, error) {
	if err := f.S.fail("Targets.LatestAttempt"); err != nil {
		return nil, err
	}
	as := f.S.AttemptsFor(targetID)
	if len(as) == 0 {
		return nil, nil
	}
	a := as[len(as)-1]
	return &a, nil
}

// InsertAttempt stores a new attempt, assigning an id; (target, attempt_no) must be unique.
func (f Targets) InsertAttempt(_ context.Context, a *post.Attempt) error {
	if err := f.S.fail("Targets.InsertAttempt"); err != nil {
		return err
	}
	for _, e := range f.S.Attempts {
		if e.PostTargetID == a.PostTargetID && e.AttemptNo == a.AttemptNo {
			return errs.Newf(errs.Conflict, "duplicate attempt number")
		}
	}
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	f.S.Attempts = append(f.S.Attempts, *a)
	return nil
}

// UpdateAttempt replaces the attempt with the same id.
func (f Targets) UpdateAttempt(_ context.Context, a *post.Attempt) error {
	if err := f.S.fail("Targets.UpdateAttempt"); err != nil {
		return err
	}
	for i := range f.S.Attempts {
		if f.S.Attempts[i].ID == a.ID {
			f.S.Attempts[i] = *a
			return nil
		}
	}
	return notFound("attempt")
}

// StuckPublishing returns publishing targets last updated before the cutoff.
func (f Targets) StuckPublishing(_ context.Context, before time.Time, limit int) ([]post.Target, error) {
	if err := f.S.fail("Targets.StuckPublishing"); err != nil {
		return nil, err
	}
	var out []post.Target
	for _, t := range f.S.Targets {
		if t.Status == post.TargetPublishing && t.UpdatedAt.Before(before) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Posts fakes scheduler.Posts.
type Posts struct{ S *Store }

// GetForUpdate returns the owner's post, NotFound otherwise.
func (f Posts) GetForUpdate(_ context.Context, userID, id uuid.UUID) (*post.Post, error) {
	if err := f.S.fail("Posts.GetForUpdate"); err != nil {
		return nil, err
	}
	p, ok := f.S.Posts[id]
	if !ok || p.UserID != userID {
		return nil, notFound("post")
	}
	return &p, nil
}

// Update stores the post.
func (f Posts) Update(_ context.Context, p *post.Post) error {
	if err := f.S.fail("Posts.Update"); err != nil {
		return err
	}
	if _, ok := f.S.Posts[p.ID]; !ok {
		return notFound("post")
	}
	f.S.Posts[p.ID] = *p
	return nil
}

// Jobs fakes scheduler.Jobs.
type Jobs struct{ S *Store }

// Get returns a job or NotFound.
func (f Jobs) Get(_ context.Context, id uuid.UUID) (*post.Job, error) {
	if err := f.S.fail("Jobs.Get"); err != nil {
		return nil, err
	}
	j, ok := f.S.Jobs[id]
	if !ok {
		return nil, notFound("job")
	}
	return &j, nil
}

// Create inserts a job.
func (f Jobs) Create(_ context.Context, j *post.Job) error {
	if err := f.S.fail("Jobs.Create"); err != nil {
		return err
	}
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	f.S.Jobs[j.ID] = *j
	return nil
}

// MarkDoneForTarget finishes every active job of the target.
func (f Jobs) MarkDoneForTarget(_ context.Context, targetID uuid.UUID) error {
	if err := f.S.fail("Jobs.MarkDoneForTarget"); err != nil {
		return err
	}
	for id, j := range f.S.Jobs {
		if j.PostTargetID == targetID && j.Status.Active() {
			j.Status = post.JobDone
			f.S.Jobs[id] = j
		}
	}
	return nil
}

// MarkEnqueued records the queue task id and moves the job to enqueued.
func (f Jobs) MarkEnqueued(_ context.Context, jobID uuid.UUID, taskID string) error {
	if err := f.S.fail("Jobs.MarkEnqueued"); err != nil {
		return err
	}
	j, ok := f.S.Jobs[jobID]
	if !ok {
		return notFound("job")
	}
	j.AsynqTaskID, j.Status = taskID, post.JobEnqueued
	f.S.Jobs[jobID] = j
	return nil
}

// ActiveForTarget returns the target's active job or (nil,nil).
func (f Jobs) ActiveForTarget(_ context.Context, targetID uuid.UUID) (*post.Job, error) {
	if err := f.S.fail("Jobs.ActiveForTarget"); err != nil {
		return nil, err
	}
	for _, j := range f.S.Jobs {
		if j.PostTargetID == targetID && j.Status.Active() {
			c := j
			return &c, nil
		}
	}
	return nil, nil
}

// NeedingReconcile returns active jobs overdue before the cutoff plus pending
// jobs that were never enqueued, oldest first.
func (f Jobs) NeedingReconcile(_ context.Context, before time.Time, limit int) ([]post.Job, error) {
	if err := f.S.fail("Jobs.NeedingReconcile"); err != nil {
		return nil, err
	}
	var out []post.Job
	for _, j := range f.S.Jobs {
		never := j.Status == post.JobPending && j.AsynqTaskID == ""
		if j.Status.Active() && (j.RunAt.Before(before) || never) {
			out = append(out, j)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].RunAt.Before(out[k].RunAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Accounts fakes scheduler.Accounts.
type Accounts struct{ S *Store }

// Get returns the owner's account or NotFound.
func (f Accounts) Get(_ context.Context, userID, id uuid.UUID) (*socialaccount.Account, error) {
	if err := f.S.fail("Accounts.Get"); err != nil {
		return nil, err
	}
	a, ok := f.S.Accounts[id]
	if !ok || a.UserID != userID {
		return nil, notFound("social account")
	}
	return &a, nil
}

// MarkExpired sets the stored account to expired.
func (f Accounts) MarkExpired(_ context.Context, _ actor.Actor, acc *socialaccount.Account, _ string) error {
	if err := f.S.fail("Accounts.MarkExpired"); err != nil {
		return err
	}
	a, ok := f.S.Accounts[acc.ID]
	if !ok {
		return notFound("social account")
	}
	a.Status = socialaccount.StatusExpired
	f.S.Accounts[acc.ID] = a
	return nil
}

// Vault fakes scheduler.Vault on top of Store.Creds.
type Vault struct{ S *Store }

// Load returns the stored credentials or NotFound.
func (f Vault) Load(_ context.Context, accountID uuid.UUID) (socialaccount.Credentials, error) {
	if err := f.S.fail("Vault.Load"); err != nil {
		return socialaccount.Credentials{}, err
	}
	c, ok := f.S.Creds[accountID]
	if !ok {
		return socialaccount.Credentials{}, notFound("credentials")
	}
	return c, nil
}

// Save records the token and replaces the stored credentials.
func (f Vault) Save(_ context.Context, accountID uuid.UUID, tok provider.Token) error {
	if err := f.S.fail("Vault.Save"); err != nil {
		return err
	}
	f.S.Saved = append(f.S.Saved, tok)
	f.S.Creds[accountID] = socialaccount.Credentials{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken,
		ExpiresAt: tok.ExpiresAt, RefreshExpiresAt: tok.RefreshExpiresAt}
	return nil
}

// MediaStore fakes scheduler.MediaStore; bytes are the Media.StorageKey string itself.
type MediaStore struct{ S *Store }

// GetMany returns the owner's media that exist (missing ids are skipped, like an IN query).
func (f MediaStore) GetMany(_ context.Context, userID uuid.UUID, ids []uuid.UUID) ([]media.Media, error) {
	if err := f.S.fail("Media.GetMany"); err != nil {
		return nil, err
	}
	var out []media.Media
	for _, id := range ids {
		if m, ok := f.S.Media[id]; ok && m.UserID == userID {
			out = append(out, m)
		}
	}
	return out, nil
}

// Open streams the storage key as content.
func (f MediaStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	if err := f.S.fail("Media.Open"); err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(key)), nil
}

// Metrics fakes scheduler.Metrics.
type Metrics struct{ S *Store }

// Increment records the counter call.
func (f Metrics) Increment(_ context.Context, _, _, _ uuid.UUID, metric string, value int64) error {
	if err := f.S.fail("Metrics.Increment"); err != nil {
		return err
	}
	f.S.Metrics = append(f.S.Metrics, MetricCall{metric, value})
	return nil
}
