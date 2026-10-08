package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/emailtoken"
	"github.com/socialos/backend/internal/domain/errs"
)

func TestForgotPasswordOnlyValidatesAndEnqueues(t *testing.T) {
	r := newRig(t, true)
	for _, e := range []string{"Ann@Example.com", "nobody@example.com"} {
		if err := r.svc.ForgotPassword(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.svc.ForgotPassword(ctx, "not an email"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.forgotQ.emails, ","); got != "ann@example.com,nobody@example.com" {
		t.Fatalf("queued %q", got)
	}
	if len(r.tokens.rows) != 0 || len(r.mail.got) != 0 {
		t.Fatal("the request path must not look anything up or send anything")
	}
}

func TestResetKeepsKeysUnlessAskedAndSaysSo(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		r := newRig(t, true)
		_ = r.forgot(r.u.Email)
		raw := tokenFrom(t, r.mail.got[0])
		if err := r.svc.ResetPassword(ctx, raw, "brand new password", revoke, ClientInfo{}); err != nil {
			t.Fatal(err)
		}
		notice := r.mail.got[len(r.mail.got)-1]
		if (r.keys.revoked == 3) != revoke {
			t.Fatalf("revoke=%v but keys revoked=%d", revoke, r.keys.revoked)
		}
		if got := strings.Contains(notice.Text, "were revoked too"); got != revoke {
			t.Fatalf("revoke=%v notice: %q", revoke, notice.Text)
		}
		if !revoke && !strings.Contains(notice.Text, "were not revoked") || !strings.Contains(notice.Text, "https://app.example/developer") && !revoke {
			t.Fatalf("notice must tell the owner to review keys at /developer: %q", notice.Text)
		}
		meta := r.audit.got[len(r.audit.got)-1].meta
		if meta["revoke_keys"] != revoke {
			t.Fatalf("audit: %+v", meta)
		}
		if revoke && meta["keys_revoked"] != int64(3) {
			t.Fatalf("audit: %+v", meta)
		}
	}
}

func TestChangePasswordCanRevokeKeys(t *testing.T) {
	r := newRig(t, true)
	a := actor.Actor{UserID: r.u.ID, Type: actor.TypeUser, SessionID: r.addSession()}
	if err := r.svc.ChangePassword(ctx, a, "old password 1", "brand new password", false); err != nil || r.keys.revoked != 0 {
		t.Fatalf("keys must survive by default: %v %d", err, r.keys.revoked)
	}
	if err := r.svc.ChangePassword(ctx, a, "brand new password", "another new password", true); err != nil || r.keys.revoked != 3 {
		t.Fatalf("keys must be revoked on request: %v %d", err, r.keys.revoked)
	}
	if m := r.audit.got[len(r.audit.got)-1].meta; m["keys_revoked"] != int64(3) || m["revoke_keys"] != true {
		t.Fatalf("audit: %+v", m)
	}
}

func TestMailTokensAreCappedPerDay(t *testing.T) {
	r := newRig(t, true)
	for i := 0; i < emailtoken.MaxPerDay; i++ {
		_ = r.forgot(r.u.Email)
		r.clock.now = r.clock.now.Add(emailtoken.Cooldown + time.Second)
	}
	if len(r.mail.got) != emailtoken.MaxPerDay {
		t.Fatalf("mails before the cap: %d", len(r.mail.got))
	}
	if err := r.forgot(r.u.Email); err != nil || len(r.mail.got) != emailtoken.MaxPerDay {
		t.Fatalf("the 11th request must be dropped silently: %v, %d mails", err, len(r.mail.got))
	}
	// Verification mail obeys the same cap: resend answers nil without sending.
	sess := actor.Actor{UserID: r.u.ID, Type: actor.TypeUser, SessionID: r.addSession()}
	for i := 0; i < emailtoken.MaxPerDay; i++ {
		r.svc.IssueVerification(ctx, r.u)
		r.clock.now = r.clock.now.Add(emailtoken.Cooldown + time.Second)
	}
	before := len(r.mail.got)
	if err := r.svc.ResendVerification(ctx, sess); err != nil || len(r.mail.got) != before {
		t.Fatalf("resend over the cap: %v, %d new mails", err, len(r.mail.got)-before)
	}
	// A day later the window has rolled over.
	r.clock.now = r.clock.now.Add(emailtoken.Window + time.Hour)
	if err := r.forgot(r.u.Email); err != nil || len(r.mail.got) != before+1 {
		t.Fatalf("after the window: %v, %d new mails", err, len(r.mail.got)-before)
	}
}

func TestRetireMailTokenKillsTheLinkOfAnUndeliveredMail(t *testing.T) {
	r := newRig(t, true)
	r.svc.IssueVerification(ctx, r.u)
	m := r.mail.got[0]
	if m.TokenID == "" || strings.Contains(m.Text, m.TokenID) {
		t.Fatalf("the message must carry the token row id, never the raw token: %q", m.TokenID)
	}
	if err := r.svc.RetireMailToken(ctx, m.TokenID); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.VerifyEmail(ctx, tokenFrom(t, m), ClientInfo{}); !errs.Is(err, errs.Validation) {
		t.Fatalf("the link still works: %v", err)
	}
	if err := r.svc.RetireMailToken(ctx, "not-a-uuid"); err != nil {
		t.Fatalf("garbage ids are ignored: %v", err)
	}
}
