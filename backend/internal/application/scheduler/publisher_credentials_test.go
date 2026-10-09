package scheduler_test

import (
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/application/scheduler/schedulertest"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

func tp(d time.Duration) *time.Time { t := schedulertest.Epoch.Add(d); return &t }

func TestOAuthTokenHandling(t *testing.T) {
	newTok := provider.Token{AccessToken: "new-access", RefreshToken: "new-refresh", ExpiresAt: tp(time.Hour)}
	tests := []struct {
		name        string
		creds       *socialaccount.Credentials
		refreshErr  error
		saveErr     error
		wantToken   string
		wantRefresh int
		wantTarget  post.TargetStatus
		wantCode    string
		wantRetry   bool
		wantExpired bool
	}{
		{name: "valid token is used as is", creds: &socialaccount.Credentials{AccessToken: "tok", ExpiresAt: tp(time.Hour)},
			wantToken: "tok", wantTarget: post.TargetPublished},
		{name: "token without expiry is used as is", creds: &socialaccount.Credentials{AccessToken: "tok"},
			wantToken: "tok", wantTarget: post.TargetPublished},
		{name: "expiring token is refreshed and saved", wantToken: "new-access", wantRefresh: 1, wantTarget: post.TargetPublished,
			creds: &socialaccount.Credentials{AccessToken: "old", RefreshToken: "r1", ExpiresAt: tp(time.Minute)}},
		{name: "expiring token that cannot refresh but still valid is used", wantToken: "old", wantTarget: post.TargetPublished,
			creds: &socialaccount.Credentials{AccessToken: "old", ExpiresAt: tp(time.Minute)}},
		{name: "expired token that cannot refresh expires the account", wantTarget: post.TargetFailed,
			wantCode: string(errs.SocialAccountExpired), wantExpired: true,
			creds: &socialaccount.Credentials{AccessToken: "old", ExpiresAt: tp(-time.Minute)}},
		{name: "refresh token expired expires the account", wantTarget: post.TargetFailed,
			wantCode: string(errs.SocialAccountExpired), wantExpired: true,
			creds: &socialaccount.Credentials{AccessToken: "old", RefreshToken: "r", ExpiresAt: tp(-time.Minute), RefreshExpiresAt: tp(-time.Second)}},
		{name: "no stored access token expires the account", wantTarget: post.TargetFailed,
			wantCode: string(errs.SocialAccountExpired), wantExpired: true, creds: &socialaccount.Credentials{}},
		{name: "missing credentials row is retryable", wantTarget: post.TargetPending, wantCode: "CREDENTIALS_LOAD", wantRetry: true},
		{name: "refresh rejected as auth expires the account", wantTarget: post.TargetFailed, wantExpired: true,
			wantCode: string(errs.SocialAccountExpired), refreshErr: perr(provider.KindAuth, "invalid_grant"), wantRefresh: 1,
			creds: &socialaccount.Credentials{AccessToken: "old", RefreshToken: "r1", ExpiresAt: tp(time.Minute)}},
		{name: "storing the refreshed token fails: retry", wantTarget: post.TargetPending, wantCode: "CREDENTIALS_SAVE",
			wantRetry: true, saveErr: errInfra, wantRefresh: 1,
			creds: &socialaccount.Credentials{AccessToken: "old", RefreshToken: "r1", ExpiresAt: tp(time.Minute)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op := schedulertest.NewOAuthProvider("fake")
			op.RefreshTok, op.RefreshErr = newTok, tc.refreshErr
			w := schedulertest.NewWorld(op)
			if tc.creds != nil {
				w.Store.Creds[w.AccountID] = *tc.creds
			}
			if tc.saveErr != nil {
				w.Store.Errors["Vault.Save"] = tc.saveErr
			}
			err := runJob(w, scheduler.RetryInfo{MaxRetry: 5})
			if tc.wantRetry {
				wantRetry(t, err)
			} else {
				wantNil(t, err)
			}
			wantTarget(t, w, tc.wantTarget, tc.wantCode)
			if len(op.RefreshedBy) != tc.wantRefresh {
				t.Fatalf("refresh calls = %v", op.RefreshedBy)
			}
			if tc.wantToken != "" && op.Requests[0].AccessToken != tc.wantToken {
				t.Fatalf("token sent = %q, want %q", op.Requests[0].AccessToken, tc.wantToken)
			}
			if tc.wantToken == "new-access" && (len(w.Store.Saved) != 1 || w.Store.Saved[0].AccessToken != "new-access") {
				t.Fatalf("saved = %+v", w.Store.Saved)
			}
			if exp := w.Store.Accounts[w.AccountID].Status == socialaccount.StatusExpired; exp != tc.wantExpired {
				t.Fatalf("account expired = %v, want %v", exp, tc.wantExpired)
			}
			if tc.wantTarget != post.TargetPublished && op.PublishCalls() != 0 {
				t.Fatal("provider must not be called without a usable token")
			}
		})
	}
}

func TestMediaAttachments(t *testing.T) {
	addMedia := func(w *schedulertest.World) uuid.UUID {
		id := uuid.New()
		w.Store.Media[id] = media.Media{ID: id, UserID: w.UserID, Kind: media.KindImage, MimeType: "image/png",
			SizeBytes: 3, StorageKey: "key-1", OriginalName: "a.png"}
		w.SetPost(func(p *post.Post) { p.MediaIDs = []uuid.UUID{id} })
		return id
	}
	t.Run("media is passed to the provider and streams from storage", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		addMedia(w)
		wantNil(t, runJob(w, scheduler.RetryInfo{}))
		m := w.Provider.(*schedulertest.Provider).Requests[0].Media
		if len(m) != 1 || m[0].Kind != "image" || m[0].Name != "a.png" {
			t.Fatalf("media = %+v", m)
		}
		rc, err := m[0].Open(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		if string(b) != "key-1" {
			t.Fatalf("content = %q", b)
		}
	})
	t.Run("deleted media fails permanently", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		id := addMedia(w)
		delete(w.Store.Media, id)
		wantNil(t, runJob(w, scheduler.RetryInfo{MaxRetry: 5}))
		wantTarget(t, w, post.TargetFailed, "MEDIA_MISSING")
	})
	t.Run("media store failure is retried", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		addMedia(w)
		w.Store.Errors["Media.GetMany"] = errInfra
		wantRetry(t, runJob(w, scheduler.RetryInfo{MaxRetry: 5}))
		wantTarget(t, w, post.TargetPending, "MEDIA_LOAD")
	})
}
