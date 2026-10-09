package approvals

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
)

type rig struct {
	svc   *Service
	repo  *memRepo
	audit *auditLog
	clock *clockAt
	owner uuid.UUID
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{repo: newMemRepo(), audit: &auditLog{}, clock: &clockAt{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}, owner: uuid.New()}
	r.svc = NewService(Deps{Repo: r.repo, Tx: passTx{}, Audit: r.audit, Clock: r.clock,
		Config: Config{TTL: 10 * time.Minute, MaxPending: 3, WebBaseURL: "https://app.test"}})
	return r
}

func (r *rig) key(id string) actor.Actor {
	return actor.Actor{UserID: r.owner, Type: actor.TypeAPIKey, ID: id, Label: "agent " + id, Scopes: apikey.AllScopes(), EmailVerified: true}
}

func (r *rig) session() actor.Actor {
	return actor.Actor{UserID: r.owner, Type: actor.TypeUser, ID: r.owner.String(), SessionID: uuid.New(), EmailVerified: true}
}

func publishReq(post string) approval.Request {
	return approval.Request{Action: approval.ActionPostPublish, ResourceType: "post", ResourceID: post,
		Fingerprint: approval.Fingerprint(post, "v1"), Summary: map[string]any{"title": "Hello"}}
}

// ask runs Require and returns the pending approval id from the 428 fields.
func (r *rig) ask(t *testing.T, a actor.Actor, req approval.Request) uuid.UUID {
	t.Helper()
	e := wantCode(t, r.svc.Require(context.Background(), a, req), errs.ApprovalRequired)
	id, err := uuid.Parse(e.Fields["approval_id"])
	if err != nil {
		t.Fatalf("no approval_id in %v", e.Fields)
	}
	return id
}

func (r *rig) retry(a actor.Actor, id uuid.UUID, req approval.Request) error {
	return r.svc.Require(approval.WithID(context.Background(), id), a, req)
}

func wantCode(t *testing.T, err error, code errs.Code) *errs.Error {
	t.Helper()
	e, ok := errs.As(err)
	if !ok || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	return e
}

func TestSessionsSchedulersAndTrustedKeysNeverNeedApproval(t *testing.T) {
	r := newRig(t)
	trusted := r.key("k")
	trusted.DangerousPolicy = approval.PolicyTrusted
	for name, a := range map[string]actor.Actor{"session": r.session(), "scheduler": actor.Scheduler(r.owner), "trusted key": trusted} {
		if err := r.svc.Require(context.Background(), a, publishReq("p1")); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(r.repo.rows) != 0 {
		t.Fatal("no approval row may be created for them")
	}
}

func TestKeyGetsA428WithApprovalDetailsAndAPendingRow(t *testing.T) {
	r := newRig(t)
	e := wantCode(t, r.svc.Require(context.Background(), r.key("k1"), publishReq("p1")), errs.ApprovalRequired)
	id := uuid.MustParse(e.Fields["approval_id"])
	if e.Fields["approve_url"] != "https://app.test/approvals" || e.Fields["action"] != "post.publish" ||
		e.Fields["expires_at"] != "2026-10-09T12:10:00Z" {
		t.Fatalf("fields: %v", e.Fields)
	}
	row := r.repo.rows[id]
	if row.Status != approval.StatusPending || row.ActorID != "k1" || row.UserID != r.owner || row.Summary["title"] != "Hello" {
		t.Fatalf("row: %+v", row)
	}
	if got := r.audit.actions(); !reflect.DeepEqual(got, []string{audit.ActionApprovalRequested}) {
		t.Fatalf("audit: %v", got)
	}
}

func TestAskingAgainReusesThePendingApproval(t *testing.T) {
	r := newRig(t)
	first := r.ask(t, r.key("k1"), publishReq("p1"))
	if again := r.ask(t, r.key("k1"), publishReq("p1")); again != first || len(r.repo.rows) != 1 {
		t.Fatalf("a repeated request must not pile up approvals (%d rows)", len(r.repo.rows))
	}
}

func TestApprovedCallRunsOnceThenNeedsANewApproval(t *testing.T) {
	r := newRig(t)
	k := r.key("k1")
	id := r.ask(t, k, publishReq("p1"))
	if _, err := r.svc.Approve(context.Background(), r.session(), id); err != nil {
		t.Fatal(err)
	}
	if err := r.retry(k, id, publishReq("p1")); err != nil {
		t.Fatalf("approved retry: %v", err)
	}
	second := wantCode(t, r.retry(k, id, publishReq("p1")), errs.ApprovalRequired)
	if second.Fields["approval_id"] == id.String() {
		t.Fatal("a used approval must not be offered again")
	}
	want := []string{audit.ActionApprovalRequested, audit.ActionApprovalApproved, audit.ActionApprovalUsed, audit.ActionApprovalRequested}
	if got := r.audit.actions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("audit: %v want %v", got, want)
	}
}

func TestAnApprovalIsBoundToKeyActionTargetAndPayload(t *testing.T) {
	base := publishReq("p1")
	other := func(f func(*approval.Request)) approval.Request { c := base; f(&c); return c }
	for name, tc := range map[string]struct {
		key string
		req approval.Request
	}{
		"another key of the same owner": {"k2", base},
		"another action":                {"k1", other(func(q *approval.Request) { q.Action = approval.ActionPostDelete })},
		"another target":                {"k1", other(func(q *approval.Request) { q.ResourceID = "p2" })},
		"another resource type":         {"k1", other(func(q *approval.Request) { q.ResourceType = "social_account" })},
		"an edited payload":             {"k1", other(func(q *approval.Request) { q.Fingerprint = approval.Fingerprint("p1", "v2") })},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			id := r.ask(t, r.key("k1"), base)
			if _, err := r.svc.Approve(context.Background(), r.session(), id); err != nil {
				t.Fatal(err)
			}
			_ = wantCode(t, r.retry(r.key(tc.key), id, tc.req), errs.ApprovalRequired)
			if r.repo.rows[id].Status != approval.StatusApproved {
				t.Fatal("a mismatched attempt must leave the approval usable by the right call")
			}
		})
	}
}

func TestOtherTenantsApprovalIsUseless(t *testing.T) {
	r := newRig(t)
	id := r.ask(t, r.key("k1"), publishReq("p1"))
	if _, err := r.svc.Approve(context.Background(), r.session(), id); err != nil {
		t.Fatal(err)
	}
	mallory := actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, ID: "k1", Scopes: apikey.AllScopes()}
	e := wantCode(t, r.retry(mallory, id, publishReq("p1")), errs.ApprovalRequired)
	if e.Fields["approval_id"] == id.String() {
		t.Fatal("the other tenant was shown the owner's approval")
	}
	if r.repo.rows[id].Status != approval.StatusApproved {
		t.Fatal("the owner's approval was touched")
	}
}

func TestPendingAndExpiredApprovalsDoNotWork(t *testing.T) {
	r := newRig(t)
	k := r.key("k1")
	id := r.ask(t, k, publishReq("p1"))
	_ = wantCode(t, r.retry(k, id, publishReq("p1")), errs.ApprovalRequired) // still pending
	if _, err := r.svc.Approve(context.Background(), r.session(), id); err != nil {
		t.Fatal(err)
	}
	r.clock.t = r.clock.t.Add(10 * time.Minute) // the deadline itself is already expired
	_ = wantCode(t, r.retry(k, id, publishReq("p1")), errs.ApprovalRequired)
}

func TestDeniedApprovalTellsTheAgentToStop(t *testing.T) {
	r := newRig(t)
	k := r.key("k1")
	id := r.ask(t, k, publishReq("p1"))
	if _, err := r.svc.Deny(context.Background(), r.session(), id); err != nil {
		t.Fatal(err)
	}
	_ = wantCode(t, r.retry(k, id, publishReq("p1")), errs.Forbidden)
	// A different request with the denied id is just a new request: nothing is revealed about the old one.
	_ = wantCode(t, r.retry(k, id, publishReq("p2")), errs.ApprovalRequired)
}

func TestTooManyPendingApprovalsAreRateLimited(t *testing.T) {
	r := newRig(t)
	k := r.key("k1")
	for _, p := range []string{"a", "b", "c"} {
		r.ask(t, k, publishReq(p))
	}
	_ = wantCode(t, r.svc.Require(context.Background(), k, publishReq("d")), errs.RateLimited)
	r.clock.t = r.clock.t.Add(time.Hour) // the old ones expired and no longer count
	r.ask(t, k, publishReq("d"))
}

func TestOnlyBrowserSessionsDecideAndOnlyOnceInTime(t *testing.T) {
	r := newRig(t)
	k := r.key("k1")
	id := r.ask(t, k, publishReq("p1"))
	for _, call := range []func(context.Context, actor.Actor, uuid.UUID) (*approval.Approval, error){r.svc.Approve, r.svc.Deny, r.svc.Get} {
		_, err := call(context.Background(), k, id)
		_ = wantCode(t, err, errs.Forbidden)
	}
	if _, err := r.svc.List(context.Background(), k, false, port.Page{Limit: 5}); !errs.Is(err, errs.Forbidden) {
		t.Fatalf("a key listed approvals: %v", err)
	}
	other := actor.Actor{UserID: uuid.New(), Type: actor.TypeUser, SessionID: uuid.New()}
	_, err := r.svc.Approve(context.Background(), other, id)
	_ = wantCode(t, err, errs.NotFound)

	if _, err := r.svc.Approve(context.Background(), r.session(), id); err != nil {
		t.Fatal(err)
	}
	_, err = r.svc.Deny(context.Background(), r.session(), id)
	_ = wantCode(t, err, errs.Conflict) // already decided

	late := r.ask(t, k, publishReq("p9"))
	r.clock.t = r.clock.t.Add(time.Hour)
	_, err = r.svc.Approve(context.Background(), r.session(), late)
	_ = wantCode(t, err, errs.Conflict) // expired
}

func TestListShowsOpenApprovalsOfTheOwnerOnly(t *testing.T) {
	r := newRig(t)
	r.ask(t, r.key("k1"), publishReq("p1"))
	foreign := actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, ID: "kx", Scopes: apikey.AllScopes()}
	r.ask(t, foreign, publishReq("p1"))
	res, err := r.svc.List(context.Background(), r.session(), false, port.Page{Limit: 10})
	if err != nil || len(res.Items) != 1 || res.Items[0].UserID != r.owner {
		t.Fatalf("list: %+v %v", res.Items, err)
	}
}
