package posts

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/post"
)

// DefaultMinAgentLead is how far ahead an API key must schedule without the owner's approval.
const DefaultMinAgentLead = 5 * time.Minute

const summaryContentRunes = 280

// postRequest binds an approval to one post at its current version: editing the post afterwards (updated_at moves)
// voids the approval. extra parts narrow it further (e.g. the requested time).
func postRequest(action approval.Action, p *post.Post, extra ...string) approval.Request {
	parts := append([]string{p.ID.String(), strconv.FormatInt(p.UpdatedAt.UnixMicro(), 10)}, extra...)
	return approval.Request{Action: action, ResourceType: "post", ResourceID: p.ID.String(),
		Fingerprint: approval.Fingerprint(parts...), Summary: postSummary(p)}
}

// deleteRequest is bound to the post id only: deleting is the same decision whatever the draft says.
func deleteRequest(p *post.Post) approval.Request {
	r := postRequest(approval.ActionPostDelete, p)
	r.Fingerprint = approval.Fingerprint(p.ID.String())
	return r
}

func postSummary(p *post.Post) map[string]any {
	platforms := make([]string, 0, len(p.Targets))
	for _, t := range p.Targets {
		platforms = append(platforms, t.Platform)
	}
	return map[string]any{"title": p.Title, "content": truncateRunes(p.Content, summaryContentRunes),
		"platforms": platforms, "status": string(p.Status)}
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// scheduleGuard sends a key's schedule closer than the minimum lead through the approval gate (OD-2): such a
// schedule is publish-now with extra steps.
func (s *Service) scheduleGuard(ctx context.Context, a actor.Actor, at time.Time, req approval.Request) error {
	if !a.NeedsApproval() || !at.Before(s.clock.Now().Add(s.minAgentLead)) {
		return nil
	}
	req.Action = approval.ActionPostScheduleSoon
	return s.gate.Require(ctx, a, req)
}

// scheduleRequest binds the approval to the post version and the requested time (to the second).
func scheduleRequest(p *post.Post, at time.Time) approval.Request {
	r := postRequest(approval.ActionPostScheduleSoon, p, strconv.FormatInt(at.UTC().Unix(), 10))
	r.Summary["scheduled_at"] = at.UTC().Format(time.RFC3339)
	return r
}

// createScheduleRequest binds the approval to the whole payload of a create-and-schedule call.
func createScheduleRequest(title string, in CreateInput, accountIDs []uuid.UUID, overrides map[uuid.UUID]string) approval.Request {
	keys := make([]string, 0, len(overrides))
	for id, c := range overrides {
		keys = append(keys, id.String()+"="+c)
	}
	sort.Strings(keys)
	canon, _ := json.Marshal(map[string]any{"title": title, "content": in.Content, "accounts": accountIDs,
		"overrides": keys, "media": in.MediaIDs, "at": in.ScheduledAt.UTC().Unix()})
	return approval.Request{Action: approval.ActionPostScheduleSoon, ResourceType: "post_input",
		Fingerprint: approval.Fingerprint(string(canon)),
		Summary: map[string]any{"title": title, "content": truncateRunes(in.Content, summaryContentRunes),
			"accounts": len(accountIDs), "scheduled_at": in.ScheduledAt.UTC().Format(time.RFC3339)}}
}
