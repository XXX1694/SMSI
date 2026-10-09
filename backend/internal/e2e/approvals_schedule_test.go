package e2e

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAgentSchedulingUnderTheMinimumLeadNeedsApproval(t *testing.T) {
	r := newAgentRig(t, "lead@example.com")
	sched := "/api/v1/posts/" + r.postID + "/schedule"
	at := func(d time.Duration) map[string]any {
		return map[string]any{"scheduled_at": fmtTime(time.Now().Add(d))}
	}

	needApproval(t, r.agent.do("POST", sched, at(4*time.Minute-30*time.Second)), "post.schedule_soon")
	// Six minutes ahead needs nothing.
	if res := r.agent.do("POST", sched, at(6*time.Minute)); res.status != http.StatusOK {
		t.Fatalf("schedule at +6m: %d %s", res.status, res.body)
	}
	// Moving it closer through PATCH is the same decision.
	patch := "/api/v1/posts/" + r.postID
	needApproval(t, r.agent.do("PATCH", patch, at(time.Minute)), "post.schedule_soon")
	if st := r.owner.must("GET", patch, nil, 200); st["scheduled_at"] == nil {
		t.Fatalf("post lost its schedule: %v", st)
	}
	// A browser session is never asked.
	r.owner.must("POST", "/api/v1/posts/"+r.postID+"/unschedule", nil, 200)
	r.owner.must("POST", sched, at(time.Minute), 200)
}

func TestApprovedScheduleIsBoundToTheRequestedTime(t *testing.T) {
	r := newAgentRig(t, "lead-bound@example.com")
	sched := "/api/v1/posts/" + r.postID + "/schedule"
	when := time.Now().Add(time.Minute)
	id := needApproval(t, r.agent.do("POST", sched, map[string]any{"scheduled_at": fmtTime(when)}), "post.schedule_soon")
	r.approve(t, id)
	other := map[string]any{"scheduled_at": fmtTime(when.Add(30 * time.Second))}
	needApproval(t, r.agent.doWith("POST", sched, other, withApproval(id)), "post.schedule_soon")
	if res := r.agent.doWith("POST", sched, map[string]any{"scheduled_at": fmtTime(when)}, withApproval(id)); res.status != http.StatusOK {
		t.Fatalf("approved schedule: %d %s", res.status, res.body)
	}
}

func TestCreateWithNearScheduleIsBoundToTheWholePayload(t *testing.T) {
	r := newAgentRig(t, "create-soon@example.com")
	near := time.Now().Add(time.Minute).UTC().Truncate(time.Second)
	mk := func(content string) map[string]any {
		return map[string]any{"content": content, "social_account_ids": []string{r.acc}, "schedule": true, "scheduled_at": fmtTime(near)}
	}
	id := needApproval(t, r.agent.do("POST", "/api/v1/posts", mk("scheduled by agent")), "post.schedule_soon")
	if got := r.owner.must("GET", "/api/v1/approvals/"+id, nil, 200); got["resource_type"] != "post_input" || got["resource_id"] != "" {
		t.Fatalf("create approvals bind to the payload, not a post: %v", got)
	}
	r.approve(t, id)
	needApproval(t, r.agent.doWith("POST", "/api/v1/posts", mk("scheduled by agent, changed"), withApproval(id)), "post.schedule_soon")
	if res := r.agent.doWith("POST", "/api/v1/posts", mk("scheduled by agent"), withApproval(id)); res.status != http.StatusCreated {
		t.Fatalf("approved create: %d %s", res.status, res.body)
	}
}

func TestPendingApprovalsPerUserAreCapped(t *testing.T) {
	r := newAgentRig(t, "cap@example.com")
	near := fmtTime(time.Now().Add(time.Minute))
	for i := 0; i < 10; i++ {
		needApproval(t, r.agent.do("POST", "/api/v1/posts", map[string]any{"content": "n" + strconv.Itoa(i),
			"social_account_ids": []string{r.acc}, "schedule": true, "scheduled_at": near}), "post.schedule_soon")
	}
	res := r.agent.do("POST", "/api/v1/posts", map[string]any{"content": "one too many", "social_account_ids": []string{r.acc},
		"schedule": true, "scheduled_at": near})
	if res.status != http.StatusTooManyRequests || res.errCode(t) != "RATE_LIMITED" {
		t.Fatalf("the 11th pending approval: %d %s", res.status, res.body)
	}
}

func TestKeyConnectWithTokenNeedsApprovalBoundToTheCredential(t *testing.T) {
	r := newAgentRig(t, "connect-approval@example.com")
	id := needApproval(t, r.agent.do("POST", tokenPath, tokenBody(goodKey)), "social_account.connect_token")
	got := r.owner.must("GET", "/api/v1/approvals/"+id, nil, 200)
	if blob := string(mustMarshal(got)); strings.Contains(blob, goodKey) {
		t.Fatalf("the credential leaked into the approval: %s", blob)
	}
	r.approve(t, id)
	swapped := r.agent.doWith("POST", tokenPath, tokenBody("another_secret_0123456789"), withApproval(id)) // gitleaks:allow (fake test value)
	needApproval(t, swapped, "social_account.connect_token")
	res := r.agent.doWith("POST", tokenPath, tokenBody(goodKey), withApproval(id))
	if res.status != http.StatusCreated || strings.Contains(string(res.body), goodKey) {
		t.Fatalf("approved connect: %d %s", res.status, res.body)
	}
}
