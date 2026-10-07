package post

import (
	"testing"

	"github.com/socialos/backend/internal/domain/errs"
)

func TestTransitionTable(t *testing.T) {
	allowed := map[[2]Status]bool{
		{StatusDraft, StatusScheduled}:               true,
		{StatusDraft, StatusPublishing}:              true,
		{StatusDraft, StatusCancelled}:               true,
		{StatusScheduled, StatusDraft}:               true,
		{StatusScheduled, StatusPublishing}:          true,
		{StatusScheduled, StatusCancelled}:           true,
		{StatusPublishing, StatusPublished}:          true,
		{StatusPublishing, StatusPartiallyPublished}: true,
		{StatusPublishing, StatusFailed}:             true,
		{StatusFailed, StatusScheduled}:              true,
		{StatusFailed, StatusPublishing}:             true,
		{StatusPartiallyPublished, StatusScheduled}:  true,
		{StatusPartiallyPublished, StatusPublishing}: true,
	}
	for _, from := range AllStatuses {
		for _, to := range AllStatuses {
			want := allowed[[2]Status{from, to}]
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s,%s)=%v want %v", from, to, got, want)
			}
			err := Transition(from, to)
			if want && err != nil {
				t.Errorf("Transition(%s,%s) unexpected err %v", from, to, err)
			}
			if !want && !errs.Is(err, errs.InvalidStateTransition) {
				t.Errorf("Transition(%s,%s) want INVALID_STATE_TRANSITION got %v", from, to, err)
			}
		}
	}
}

func TestTerminalAndPredicates(t *testing.T) {
	if !StatusPublished.Terminal() || !StatusCancelled.Terminal() || StatusFailed.Terminal() {
		t.Fatal("terminal states wrong")
	}
	if !StatusDraft.Editable() || !StatusScheduled.Editable() || StatusPublishing.Editable() {
		t.Fatal("editable wrong")
	}
	if StatusPublishing.Deletable() || StatusPartiallyPublished.Deletable() || !StatusPublished.Deletable() {
		t.Fatal("deletable wrong")
	}
	if !StatusFailed.Retryable() || !StatusPartiallyPublished.Retryable() || StatusDraft.Retryable() {
		t.Fatal("retryable wrong")
	}
	if Status("bogus").Valid() || !StatusDraft.Valid() {
		t.Fatal("valid wrong")
	}
}

func TestDeriveStatus(t *testing.T) {
	cases := []struct {
		name string
		in   []TargetStatus
		want Status
		ok   bool
	}{
		{"all published", []TargetStatus{TargetPublished, TargetPublished}, StatusPublished, true},
		{"some published", []TargetStatus{TargetPublished, TargetFailed}, StatusPartiallyPublished, true},
		{"needs review counts as not published", []TargetStatus{TargetPublished, TargetNeedsReview}, StatusPartiallyPublished, true},
		{"none published", []TargetStatus{TargetFailed, TargetNeedsReview}, StatusFailed, true},
		{"in flight", []TargetStatus{TargetPublished, TargetPublishing}, "", false},
		{"pending", []TargetStatus{TargetPending}, "", false},
		{"cancelled ignored", []TargetStatus{TargetCancelled, TargetPublished}, StatusPublished, true},
		{"all cancelled", []TargetStatus{TargetCancelled}, StatusCancelled, true},
		{"empty", nil, "", false},
	}
	for _, c := range cases {
		got, ok := DeriveStatus(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got (%s,%v) want (%s,%v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestJobStatusActive(t *testing.T) {
	if !JobPending.Active() || !JobEnqueued.Active() || JobDone.Active() || JobCancelled.Active() {
		t.Fatal("job active wrong")
	}
}
