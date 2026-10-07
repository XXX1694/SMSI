package post

// TargetStatus is the per-platform publishing state.
type TargetStatus string

const (
	TargetPending     TargetStatus = "pending"
	TargetPublishing  TargetStatus = "publishing"
	TargetPublished   TargetStatus = "published"
	TargetFailed      TargetStatus = "failed"
	TargetCancelled   TargetStatus = "cancelled"
	TargetNeedsReview TargetStatus = "needs_review"
)

// Terminal reports whether the target reached a final outcome for this run.
func (t TargetStatus) Terminal() bool {
	switch t {
	case TargetPublished, TargetFailed, TargetCancelled, TargetNeedsReview:
		return true
	}
	return false
}

// DeriveStatus computes the post status from target statuses.
// ok is false while any target is still in flight (pending/publishing).
// Cancelled targets are ignored unless every target is cancelled.
func DeriveStatus(targets []TargetStatus) (status Status, ok bool) {
	var counted, published int
	for _, t := range targets {
		if !t.Terminal() {
			return "", false
		}
		if t == TargetCancelled {
			continue
		}
		counted++
		if t == TargetPublished {
			published++
		}
	}
	switch {
	case counted == 0 && len(targets) > 0:
		return StatusCancelled, true
	case counted == 0:
		return "", false
	case published == counted:
		return StatusPublished, true
	case published > 0:
		return StatusPartiallyPublished, true
	default:
		return StatusFailed, true
	}
}

// AttemptStatus is the state of one publication attempt.
type AttemptStatus string

const (
	AttemptStarted   AttemptStatus = "started"
	AttemptSucceeded AttemptStatus = "succeeded"
	AttemptFailed    AttemptStatus = "failed"
	AttemptUnknown   AttemptStatus = "unknown"
)

// JobStatus is the state of a scheduled_jobs row.
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobEnqueued  JobStatus = "enqueued"
	JobDone      JobStatus = "done"
	JobCancelled JobStatus = "cancelled"
)

// Active reports whether the job may still run.
func (j JobStatus) Active() bool { return j == JobPending || j == JobEnqueued }
