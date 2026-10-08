package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/emailtoken"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/user"
)

var ctx = context.Background()

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if !errs.Is(err, code) {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func TestIssueVerificationMailsFragmentLink(t *testing.T) {
	r := newRig(t, true)
	r.svc.IssueVerification(ctx, r.u)
	if len(r.mail.got) != 1 || r.mail.got[0].To != "ann@example.com" || r.mail.got[0].Template != "verify_email" {
		t.Fatalf("mail: %+v", r.mail.got)
	}
	m := r.mail.got[0]
	if !strings.Contains(m.Text, "https://app.example/verify-email#token=") || strings.Contains(m.Text, "?token=") {
		t.Fatalf("link must use the fragment: %q", m.Text)
	}
	if !strings.Contains(m.HTML, "https://app.example/verify-email#token=") {
		t.Fatalf("html link: %q", m.HTML)
	}
	// Only the hash is stored.
	if raw := tokenFrom(t, m); r.tokens.rows[0].Hash == raw || len(r.tokens.rows[0].Hash) != 64 {
		t.Fatalf("raw token stored: %+v", r.tokens.rows[0])
	}
}

func TestIssueVerificationNeverFailsRegistration(t *testing.T) {
	r := newRig(t, true)
	r.mail.fail = errors.New("redis down")
	r.svc.IssueVerification(ctx, r.u) // must not panic or return an error
	if len(r.mail.got) != 0 {
		t.Fatal("queued despite failure")
	}
}

func TestVerifyEmailIsSingleUse(t *testing.T) {
	r := newRig(t, true)
	r.svc.IssueVerification(ctx, r.u)
	raw := tokenFrom(t, r.mail.got[0])
	if err := r.svc.VerifyEmail(ctx, raw, ClientInfo{}); err != nil {
		t.Fatal(err)
	}
	if !r.u.EmailVerified() {
		t.Fatal("not verified")
	}
	wantCode(t, r.svc.VerifyEmail(ctx, raw, ClientInfo{}), errs.Validation)
	if r.audit.got[len(r.audit.got)-1].action != audit.ActionEmailVerified {
		t.Fatalf("audit: %+v", r.audit.got)
	}
}

func TestVerifyEmailExpires(t *testing.T) {
	r := newRig(t, true)
	r.svc.IssueVerification(ctx, r.u)
	raw := tokenFrom(t, r.mail.got[0])
	r.clock.now = r.clock.now.Add(emailtoken.VerifyTTL + time.Second)
	wantCode(t, r.svc.VerifyEmail(ctx, raw, ClientInfo{}), errs.Validation)
	if r.u.EmailVerified() {
		t.Fatal("expired token verified the email")
	}
}

func TestVerifyEmailRejectsGarbageAndWrongPurpose(t *testing.T) {
	r := newRig(t, true)
	wantCode(t, r.svc.VerifyEmail(ctx, "", ClientInfo{}), errs.Validation)
	wantCode(t, r.svc.VerifyEmail(ctx, strings.Repeat("a", 500), ClientInfo{}), errs.Validation)
	wantCode(t, r.svc.VerifyEmail(ctx, "nope", ClientInfo{}), errs.Validation)
	// A reset token cannot verify an email, and a verification token cannot reset a password.
	if err := r.svc.ForgotPassword(ctx, r.u.Email); err != nil {
		t.Fatal(err)
	}
	reset := tokenFrom(t, r.mail.got[0])
	wantCode(t, r.svc.VerifyEmail(ctx, reset, ClientInfo{}), errs.Validation)
	r.clock.now = r.clock.now.Add(time.Minute * 2)
	r.svc.IssueVerification(ctx, r.u)
	verify := tokenFrom(t, r.mail.got[1])
	wantCode(t, r.svc.ResetPassword(ctx, verify, "brand new password", ClientInfo{}), errs.Validation)
	if r.u.PasswordHash != "h:old password 1" {
		t.Fatal("password changed by a verification token")
	}
}

func TestNewTokenRetiresOlderOne(t *testing.T) {
	r := newRig(t, true)
	r.svc.IssueVerification(ctx, r.u)
	r.clock.now = r.clock.now.Add(time.Hour)
	r.svc.IssueVerification(ctx, r.u)
	first, second := tokenFrom(t, r.mail.got[0]), tokenFrom(t, r.mail.got[1])
	wantCode(t, r.svc.VerifyEmail(ctx, first, ClientInfo{}), errs.Validation)
	if err := r.svc.VerifyEmail(ctx, second, ClientInfo{}); err != nil {
		t.Fatal(err)
	}
}

func TestResendVerification(t *testing.T) {
	r := newRig(t, true)
	sess := actor.Actor{UserID: r.u.ID, Type: actor.TypeUser, SessionID: r.addSession()}
	if err := r.svc.ResendVerification(ctx, sess); err != nil {
		t.Fatal(err)
	}
	wantCode(t, r.svc.ResendVerification(ctx, sess), errs.RateLimited) // cooldown
	r.clock.now = r.clock.now.Add(emailtoken.Cooldown + time.Second)
	if err := r.svc.ResendVerification(ctx, sess); err != nil {
		t.Fatal(err)
	}
	wantCode(t, r.svc.ResendVerification(ctx, actor.Actor{UserID: r.u.ID, Type: actor.TypeAPIKey}), errs.Forbidden)
	now := r.clock.now
	r.u.EmailVerifiedAt = &now
	wantCode(t, r.svc.ResendVerification(ctx, sess), errs.Conflict)
}

func TestForgotPasswordRevealsNothing(t *testing.T) {
	r := newRig(t, true)
	for _, email := range []string{"nobody@example.com", "not an email", "", r.u.Email} {
		if err := r.svc.ForgotPassword(ctx, email); err != nil {
			t.Fatalf("%q: %v", email, err)
		}
	}
	if len(r.mail.got) != 1 || r.mail.got[0].To != r.u.Email || r.mail.got[0].Template != "reset_password" {
		t.Fatalf("mail: %+v", r.mail.got)
	}
	// Cooldown: a second request within a minute is silently dropped.
	if err := r.svc.ForgotPassword(ctx, r.u.Email); err != nil || len(r.mail.got) != 1 {
		t.Fatalf("cooldown: %v %d", err, len(r.mail.got))
	}
	// A failing queue is invisible to the caller too.
	r.clock.now = r.clock.now.Add(time.Hour)
	r.mail.fail = errors.New("redis down")
	if err := r.svc.ForgotPassword(ctx, r.u.Email); err != nil {
		t.Fatalf("enqueue failure leaked: %v", err)
	}
	r.u.Status = user.StatusDisabled
	r.mail.fail = nil
	r.clock.now = r.clock.now.Add(time.Hour)
	_ = r.svc.ForgotPassword(ctx, r.u.Email)
	if len(r.mail.got) != 1 {
		t.Fatal("mail sent to a disabled account")
	}
}

func TestResetPasswordRevokesEverySession(t *testing.T) {
	r := newRig(t, true)
	s1, s2 := r.addSession(), r.addSession()
	_ = r.svc.ForgotPassword(ctx, r.u.Email)
	raw := tokenFrom(t, r.mail.got[0])
	if err := r.svc.ResetPassword(ctx, raw, "brand new password", ClientInfo{}); err != nil {
		t.Fatal(err)
	}
	if r.u.PasswordHash != "h:brand new password" {
		t.Fatalf("hash: %s", r.u.PasswordHash)
	}
	if _, ok := r.sessions.ids[s1]; ok {
		t.Fatal("session 1 survived")
	}
	if _, ok := r.sessions.ids[s2]; ok {
		t.Fatal("session 2 survived")
	}
	if !r.u.EmailVerified() {
		t.Fatal("a redeemed mailed link proves the address")
	}
	if last := r.mail.got[len(r.mail.got)-1]; last.Template != "password_changed" || last.To != r.u.Email {
		t.Fatalf("no notice: %+v", last)
	}
	wantCode(t, r.svc.ResetPassword(ctx, raw, "another password", ClientInfo{}), errs.Validation) // single use
	if a := r.audit.got[len(r.audit.got)-1]; a.action != audit.ActionPasswordReset || a.meta["sessions_revoked"] != int64(2) {
		t.Fatalf("audit: %+v", a)
	}
}

func TestResetPasswordWeakPasswordKeepsTheLink(t *testing.T) {
	r := newRig(t, true)
	_ = r.svc.ForgotPassword(ctx, r.u.Email)
	raw := tokenFrom(t, r.mail.got[0])
	wantCode(t, r.svc.ResetPassword(ctx, raw, "short", ClientInfo{}), errs.Validation)
	if err := r.svc.ResetPassword(ctx, raw, "long enough password", ClientInfo{}); err != nil {
		t.Fatalf("link burned by a weak password: %v", err)
	}
}

func TestResetPasswordExpiresAndRefusesInactiveUsers(t *testing.T) {
	r := newRig(t, true)
	_ = r.svc.ForgotPassword(ctx, r.u.Email)
	raw := tokenFrom(t, r.mail.got[0])
	r.clock.now = r.clock.now.Add(emailtoken.ResetTTL + time.Second)
	wantCode(t, r.svc.ResetPassword(ctx, raw, "long enough password", ClientInfo{}), errs.Validation)

	r.clock.now = r.clock.now.Add(time.Hour)
	_ = r.svc.ForgotPassword(ctx, r.u.Email)
	raw = tokenFrom(t, r.mail.got[1])
	r.u.Status = user.StatusDeleted
	wantCode(t, r.svc.ResetPassword(ctx, raw, "long enough password", ClientInfo{}), errs.Validation)
}

func TestChangePasswordKeepsOnlyTheCurrentSession(t *testing.T) {
	r := newRig(t, true)
	cur, other := r.addSession(), r.addSession()
	a := actor.Actor{UserID: r.u.ID, Type: actor.TypeUser, SessionID: cur}
	if err := r.svc.ChangePassword(ctx, a, "old password 1", "brand new password"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.sessions.ids[cur]; !ok {
		t.Fatal("current session revoked")
	}
	if _, ok := r.sessions.ids[other]; ok {
		t.Fatal("other session survived")
	}
	if r.u.PasswordHash != "h:brand new password" {
		t.Fatal("password not changed")
	}
	if r.mail.got[len(r.mail.got)-1].Template != "password_changed" {
		t.Fatal("no notice mail")
	}
}

func TestChangePasswordChecksCurrentPasswordAndKind(t *testing.T) {
	r := newRig(t, true)
	a := actor.Actor{UserID: r.u.ID, Type: actor.TypeUser, SessionID: r.addSession()}
	wantCode(t, r.svc.ChangePassword(ctx, a, "wrong", "brand new password"), errs.Validation)
	wantCode(t, r.svc.ChangePassword(ctx, a, "old password 1", "short"), errs.Validation)
	wantCode(t, r.svc.ChangePassword(ctx, actor.Actor{UserID: r.u.ID, Type: actor.TypeAPIKey}, "old password 1", "brand new password"), errs.Forbidden)
	if r.u.PasswordHash != "h:old password 1" {
		t.Fatal("password changed on a failed attempt")
	}
}

func TestSecretsStayOutOfAuditAndLogs(t *testing.T) {
	r := newRig(t, true)
	r.svc.IssueVerification(ctx, r.u)
	_ = r.svc.ForgotPassword(ctx, r.u.Email)
	raws := []string{tokenFrom(t, r.mail.got[0]), tokenFrom(t, r.mail.got[1])}
	_ = r.svc.VerifyEmail(ctx, raws[0], ClientInfo{})
	_ = r.svc.ResetPassword(ctx, raws[1], "brand new password", ClientInfo{})
	dump := fmt.Sprintf("%+v", r.audit.got)
	for _, raw := range append(raws, "brand new password", "h:") {
		if strings.Contains(dump, raw) {
			t.Fatalf("audit leaks %q: %s", raw, dump)
		}
	}
}

func TestAuthenticationFlagsUnverifiedOwnersOnlyWhenEnforced(t *testing.T) {
	for _, enforce := range []bool{true, false} {
		r := newRig(t, enforce)
		if got := r.svc.verified(r.u); got != !enforce {
			t.Errorf("enforce=%v unverified owner: verified=%v", enforce, got)
		}
		now := r.clock.now
		r.u.EmailVerifiedAt = &now
		if !r.svc.verified(r.u) {
			t.Errorf("enforce=%v: verified owner flagged", enforce)
		}
	}
}
