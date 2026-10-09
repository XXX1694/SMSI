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

func TestPendingApprovalsAreCappedPerKey(t *testing.T) {
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
	// One noisy key does not block the owner's other keys.
	other := r.e.apiKeyClient(r.owner.createKey("quiet agent", "posts:read", "posts:write", "posts:schedule"))
	needApproval(t, other.do("POST", "/api/v1/posts", map[string]any{"content": "quiet one", "social_account_ids": []string{r.acc},
		"schedule": true, "scheduled_at": near}), "post.schedule_soon")
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

// An approval for a PATCH covers the post as it will be after the edit, not just the field the owner was shown.
func TestApprovedPatchCannotBeReplayedWithAnotherEdit(t *testing.T) {
	r := newAgentRig(t, "patch-replay@example.com")
	patch := "/api/v1/posts/" + r.postID
	r.owner.must("POST", "/api/v1/posts/"+r.postID+"/schedule", map[string]any{"scheduled_at": fmtTime(time.Now().Add(time.Hour))}, 200)
	soon := fmtTime(time.Now().Add(time.Minute))

	id := needApproval(t, r.agent.do("PATCH", patch, map[string]any{"scheduled_at": soon}), "post.schedule_soon")
	r.approve(t, id)
	// Same approval, same time, but a different text: refused.
	needApproval(t, r.agent.doWith("PATCH", patch, map[string]any{"scheduled_at": soon, "content": "swapped after approval"}, withApproval(id)), "post.schedule_soon")
	// Same approval, same time, another account list or media list: refused too.
	needApproval(t, r.agent.doWith("PATCH", patch, map[string]any{"scheduled_at": soon, "media_ids": []string{}, "title": "other title"}, withApproval(id)), "post.schedule_soon")
	if res := r.agent.doWith("PATCH", patch, map[string]any{"scheduled_at": soon}, withApproval(id)); res.status != http.StatusOK {
		t.Fatalf("the approved edit itself: %d %s", res.status, res.body)
	}
}

// A post that runs within the minimum lead cannot be edited by a key without approval, even if the time is untouched.
func TestEditingAPostThatRunsSoonNeedsApproval(t *testing.T) {
	r := newAgentRig(t, "edit-soon@example.com")
	patch := "/api/v1/posts/" + r.postID
	r.owner.must("POST", "/api/v1/posts/"+r.postID+"/schedule", map[string]any{"scheduled_at": fmtTime(time.Now().Add(90 * time.Second))}, 200)

	edit := map[string]any{"content": "edited at T-90s"}
	id := needApproval(t, r.agent.do("PATCH", patch, edit), "post.schedule_soon")
	if got := r.owner.must("GET", patch, nil, 200); got["content"] == "edited at T-90s" {
		t.Fatal("the edit was applied without approval")
	}
	summary := r.owner.must("GET", "/api/v1/approvals/"+id, nil, 200)["summary"].(map[string]any)
	if summary["content"] != "edited at T-90s" {
		t.Fatalf("the owner must see the text they approve (the post after the edit): %v", summary)
	}
	r.approve(t, id)
	needApproval(t, r.agent.doWith("PATCH", patch, map[string]any{"content": "another text"}, withApproval(id)), "post.schedule_soon")
	if res := r.agent.doWith("PATCH", patch, edit, withApproval(id)); res.status != http.StatusOK {
		t.Fatalf("approved edit: %d %s", res.status, res.body)
	}

	// A post that runs an hour from now is a plain edit.
	far := r.draft("far away")
	r.owner.must("POST", "/api/v1/posts/"+far+"/schedule", map[string]any{"scheduled_at": fmtTime(time.Now().Add(time.Hour))}, 200)
	if res := r.agent.do("PATCH", "/api/v1/posts/"+far, map[string]any{"content": "fine"}); res.status != http.StatusOK {
		t.Fatalf("edit of a post an hour out: %d %s", res.status, res.body)
	}
	// And so is any edit of a draft.
	if res := r.agent.do("PATCH", "/api/v1/posts/"+r.draft("a draft"), map[string]any{"content": "fine"}); res.status != http.StatusOK {
		t.Fatalf("edit of a draft: %d %s", res.status, res.body)
	}
}
