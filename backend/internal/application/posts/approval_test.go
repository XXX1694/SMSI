package posts

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/post"
)

type fixedNow struct{ t time.Time }

func (c fixedNow) Now() time.Time { return c.t }

type spyGate struct{ reqs []approval.Request }

func (g *spyGate) Require(_ context.Context, a actor.Actor, req approval.Request) error {
	if a.NeedsApproval() {
		g.reqs = append(g.reqs, req)
	}
	return nil
}

var now0 = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func guardRig(lead time.Duration) (*Service, *spyGate) {
	g := &spyGate{}
	return &Service{clock: fixedNow{now0}, gate: g, minAgentLead: lead}, g
}

func key() actor.Actor { return actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, ID: "k"} }
func session() actor.Actor {
	return actor.Actor{UserID: uuid.New(), Type: actor.TypeUser, SessionID: uuid.New()}
}

func testPost() *post.Post {
	return &post.Post{ID: uuid.New(), Title: "T", Content: "hello", Status: post.StatusDraft, UpdatedAt: now0,
		Targets: []post.Target{{Platform: "mock"}}}
}

func TestScheduleGuardOnlyStopsKeysSchedulingTooSoon(t *testing.T) {
	p := testPost()
	for name, tc := range map[string]struct {
		a    actor.Actor
		lead time.Duration
		at   time.Time
		want int
	}{
		"key, 4 minutes ahead":        {key(), 5 * time.Minute, now0.Add(4 * time.Minute), 1},
		"key, exactly the lead":       {key(), 5 * time.Minute, now0.Add(5 * time.Minute), 0},
		"key, 6 minutes ahead":        {key(), 5 * time.Minute, now0.Add(6 * time.Minute), 0},
		"key, rule switched off":      {key(), -1, now0.Add(time.Second), 0},
		"session, 1 minute ahead":     {session(), 5 * time.Minute, now0.Add(time.Minute), 0},
		"scheduler, 1 minute ahead":   {actor.Scheduler(uuid.New()), 5 * time.Minute, now0.Add(time.Minute), 0},
		"trusted key, 1 minute ahead": {trusted(key()), 5 * time.Minute, now0.Add(time.Minute), 0},
	} {
		s, g := guardRig(tc.lead)
		if err := s.scheduleGuard(context.Background(), tc.a, tc.at, scheduleRequest(p, tc.at)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(g.reqs) != tc.want {
			t.Errorf("%s: gate asked %d times, want %d", name, len(g.reqs), tc.want)
		}
		for _, r := range g.reqs {
			if r.Action != approval.ActionPostScheduleSoon {
				t.Errorf("%s: action %s", name, r.Action)
			}
		}
	}
}

func trusted(a actor.Actor) actor.Actor { a.DangerousPolicy = approval.PolicyTrusted; return a }

func TestPostApprovalsAreBoundToTheVersionAndTime(t *testing.T) {
	p := testPost()
	base := postRequest(approval.ActionPostPublish, p).Fingerprint
	edited := *p
	edited.UpdatedAt = now0.Add(time.Microsecond)
	if postRequest(approval.ActionPostPublish, &edited).Fingerprint == base {
		t.Fatal("editing the post must void the approval")
	}
	at := now0.Add(time.Minute)
	if scheduleRequest(p, at).Fingerprint == scheduleRequest(p, at.Add(time.Second)).Fingerprint {
		t.Fatal("another time needs another approval")
	}
	if deleteRequest(p).Fingerprint != deleteRequest(&edited).Fingerprint {
		t.Fatal("deleting is bound to the post, not to its text")
	}
}

func TestCreateScheduleApprovalCoversTheWholePayload(t *testing.T) {
	acc := []uuid.UUID{uuid.New()}
	at := now0.Add(time.Minute)
	in := CreateInput{Content: "hi", ScheduledAt: &at}
	base := createScheduleRequest("T", in, acc, nil)
	if base.ResourceType != "post_input" || base.ResourceID != "" {
		t.Fatalf("unexpected binding: %+v", base)
	}
	changed := in
	changed.Content = "hi!"
	if createScheduleRequest("T", changed, acc, nil).Fingerprint == base.Fingerprint {
		t.Fatal("changed content must need a new approval")
	}
	if createScheduleRequest("T", in, []uuid.UUID{uuid.New()}, nil).Fingerprint == base.Fingerprint {
		t.Fatal("other accounts must need a new approval")
	}
	if createScheduleRequest("T", in, acc, map[uuid.UUID]string{acc[0]: "x"}).Fingerprint == base.Fingerprint {
		t.Fatal("a per-network override must need a new approval")
	}
}

func TestSummaryIsBoundedForTheOwnersScreen(t *testing.T) {
	p := testPost()
	p.Content = strings.Repeat("é", 1000)
	got := postSummary(p)["content"].(string)
	if n := utf8.RuneCountInString(got); n != summaryContentRunes+1 || !strings.HasSuffix(got, "…") {
		t.Fatalf("summary content has %d runes", n)
	}
}
