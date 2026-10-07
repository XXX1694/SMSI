// Package post holds the post aggregate, its state machine and target rules.
package post

import "github.com/socialos/backend/internal/domain/errs"

// Status is the lifecycle state of a post.
type Status string

const (
	StatusDraft              Status = "draft"
	StatusScheduled          Status = "scheduled"
	StatusPublishing         Status = "publishing"
	StatusPublished          Status = "published"
	StatusPartiallyPublished Status = "partially_published"
	StatusFailed             Status = "failed"
	StatusCancelled          Status = "cancelled"
)

// AllStatuses lists every valid post status.
var AllStatuses = []Status{
	StatusDraft, StatusScheduled, StatusPublishing, StatusPublished,
	StatusPartiallyPublished, StatusFailed, StatusCancelled,
}

// transitions is the single allowed-transition table (ARCHITECTURE.md §2).
var transitions = map[Status][]Status{
	StatusDraft:              {StatusScheduled, StatusPublishing, StatusCancelled},
	StatusScheduled:          {StatusDraft, StatusPublishing, StatusCancelled},
	StatusPublishing:         {StatusPublished, StatusPartiallyPublished, StatusFailed},
	StatusFailed:             {StatusScheduled, StatusPublishing},
	StatusPartiallyPublished: {StatusScheduled, StatusPublishing},
	StatusPublished:          {},
	StatusCancelled:          {},
}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	_, ok := transitions[s]
	return ok
}

// CanTransition reports whether from → to is allowed.
func CanTransition(from, to Status) bool {
	for _, t := range transitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Transition validates from → to and returns INVALID_STATE_TRANSITION otherwise.
func Transition(from, to Status) error {
	if !CanTransition(from, to) {
		return errs.Newf(errs.InvalidStateTransition, "cannot move post from %s to %s", from, to)
	}
	return nil
}

// Terminal reports whether no further transitions exist.
func (s Status) Terminal() bool { return len(transitions[s]) == 0 }

// Editable reports whether content/targets may be changed.
func (s Status) Editable() bool { return s == StatusDraft || s == StatusScheduled }

// Deletable reports whether the post may be soft-deleted.
func (s Status) Deletable() bool {
	switch s {
	case StatusDraft, StatusScheduled, StatusFailed, StatusCancelled, StatusPublished:
		return true
	}
	return false
}

// Retryable reports whether an explicit retry is allowed.
func (s Status) Retryable() bool { return s == StatusFailed || s == StatusPartiallyPublished }
