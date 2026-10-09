package accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/socialaccount"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

const (
	testSecret = "tok_live_0123456789abcdef" // gitleaks:allow (fake test value)
	testHost   = "https://social.example.com"
)

// credRepo remembers the ciphertext the vault hands to the repository.
type credRepo struct {
	memRepo
	cmu   sync.Mutex
	creds map[uuid.UUID]EncryptedCredentials
}

func (r *credRepo) SaveCredentials(_ context.Context, id uuid.UUID, c EncryptedCredentials) error {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	r.creds[id] = c
	return nil
}

func (r *credRepo) GetCredentials(_ context.Context, id uuid.UUID) (*EncryptedCredentials, error) {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	if c, ok := r.creds[id]; ok {
		return &c, nil
	}
	return nil, errs.NotFoundf("credentials")
}

// fakeToken is a TokenConnector with a two-field form.
type fakeToken struct {
	verifyErr error
	profile   provider.Profile
	secret    string
	got       map[string]string
	calls     int
	slow      bool
}

func (f *fakeToken) Name() string        { return "tokennet" }
func (f *fakeToken) DisplayName() string { return "TokenNet" }
func (f *fakeToken) Supported() bool     { return true }
func (f *fakeToken) Configured() bool    { return true }
func (f *fakeToken) Capabilities() provider.Capabilities {
	return provider.Capabilities{ConnectMethod: provider.ConnectToken, ConnectFields: []provider.ConnectField{
		{Name: "instance_url", Label: "Instance URL", Kind: provider.FieldURL, Required: true},
		{Name: "access_token", Label: "Access token", Kind: provider.FieldSecret, Required: true},
		{Name: "handle", Label: "Handle", Kind: provider.FieldText},
	}}
}
func (f *fakeToken) Verify(ctx context.Context, fields map[string]string) (provider.Profile, string, error) {
	f.calls++
	f.got = fields
	if f.slow {
		<-ctx.Done()
		return provider.Profile{}, "", ctx.Err()
	}
	return f.profile, f.secret, f.verifyErr
}

type tokenRig struct {
	svc   *Service
	prov  *fakeToken
	repo  *credRepo
	audit *auditRec
	enc   *crypto.Cipher
	gate  *recGate
}

func newTokenRig(t *testing.T) *tokenRig {
	t.Helper()
	enc, err := crypto.NewCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	prov := &fakeToken{secret: testSecret, profile: provider.Profile{ID: "acct-1", Username: "alice",
		Metadata: map[string]any{"host": "social.example.com"}}}
	repo := &credRepo{memRepo: memRepo{accs: map[uuid.UUID]*socialaccount.Account{}}, creds: map[uuid.UUID]EncryptedCredentials{}}
	rec := &auditRec{}
	gate := &recGate{}
	svc := NewService(Deps{Repo: repo, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Registry: provider.NewRegistry(prov),
		Tx: &memTx{audit: rec}, Audit: rec, Clock: &fixedClock{t: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}, Enc: enc, Approvals: gate})
	return &tokenRig{svc: svc, prov: prov, repo: repo, audit: rec, enc: enc, gate: gate}
}

func validFields() map[string]string {
	return map[string]string{"instance_url": testHost, "access_token": testSecret}
}

func apiKeyActor(scopes ...apikey.Scope) actor.Actor {
	return actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, ID: "k1", Label: "agent", Scopes: scopes, EmailVerified: true}
}

func (r *tokenRig) connect(a actor.Actor, f map[string]string) (*socialaccount.Account, error) {
	return r.svc.ConnectWithToken(context.Background(), a, "tokennet", f)
}

func wantCode(t *testing.T, err error, code errs.Code) *errs.Error {
	t.Helper()
	e, ok := errs.As(err)
	if !ok || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	return e
}

func TestConnectWithTokenNeedsTheCriticalScope(t *testing.T) {
	r := newTokenRig(t)
	for name, a := range map[string]actor.Actor{
		"no scopes":             apiKeyActor(),
		"every scope but this":  apiKeyActor(apikey.SocialRead, apikey.PostsPublish, apikey.SocialDisconnect),
		"the default scope set": apiKeyActor(apikey.DefaultScopes()...),
		"unauthenticated":       {},
	} {
		_, err := r.connect(a, validFields())
		if name == "unauthenticated" {
			_ = wantCode(t, err, errs.Unauthenticated)
		} else {
			_ = wantCode(t, err, errs.InsufficientScope)
		}
		if r.prov.calls != 0 {
			t.Fatalf("%s: the provider must not be called without the scope", name)
		}
	}
	if _, err := r.connect(apiKeyActor(apikey.SocialConnect), validFields()); err != nil {
		t.Fatalf("a key with social:connect: %v", err)
	}
	if _, err := r.connect(session(uuid.New()), validFields()); err != nil {
		t.Fatalf("a browser session: %v", err)
	}
}

func TestConnectWithTokenStoresAnEncryptedCredential(t *testing.T) {
	r := newTokenRig(t)
	u := uuid.New()
	acc, err := r.connect(session(u), validFields())
	if err != nil {
		t.Fatal(err)
	}
	if acc.UserID != u || acc.Provider != "tokennet" || acc.ProviderAccountID != "acct-1" || acc.Status != socialaccount.StatusActive {
		t.Fatalf("account: %+v", acc)
	}
	c, ok := r.repo.creds[acc.ID]
	if !ok {
		t.Fatal("no credential stored")
	}
	if strings.Contains(c.AccessTokenEnc, testSecret) || c.AccessTokenEnc == "" || c.ExpiresAt != nil {
		t.Fatalf("credential must be ciphertext without an expiry: %+v", c)
	}
	back, err := r.svc.Vault().Load(context.Background(), acc.ID)
	if err != nil || back.AccessToken != testSecret || back.NeedsRefresh(time.Now(), time.Hour) {
		t.Fatalf("vault round trip: %+v %v", back, err)
	}
}

func TestConnectWithTokenLeavesNoSecretInTheAccountOrAudit(t *testing.T) {
	r := newTokenRig(t)
	acc, err := r.connect(session(uuid.New()), validFields())
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(acc)
	if strings.Contains(string(blob), testSecret) {
		t.Fatalf("account leaks the secret: %s", blob)
	}
	if len(r.audit.entries) != 1 || r.audit.entries[0].action != audit.ActionAccountConnected {
		t.Fatalf("audit: %+v", r.audit.entries)
	}
	blob, _ = json.Marshal(r.audit.entries[0].meta)
	if strings.Contains(string(blob), testSecret) {
		t.Fatalf("audit leaks the secret: %s", blob)
	}
}

func TestConnectWithTokenRefusesAProfileThatLeaksTheSecret(t *testing.T) {
	for name, mutate := range map[string]func(*fakeToken){
		"metadata holds the verify secret":  func(f *fakeToken) { f.profile.Metadata = map[string]any{"token": f.secret} },
		"username holds a submitted secret": func(f *fakeToken) { f.profile.Username = "u-" + testSecret },
		"nested metadata":                   func(f *fakeToken) { f.profile.Metadata = map[string]any{"a": map[string]any{"b": []string{f.secret}}} },
	} {
		r := newTokenRig(t)
		mutate(r.prov)
		_, err := r.connect(session(uuid.New()), validFields())
		_ = wantCode(t, err, errs.Internal)
		if len(r.repo.accs) != 0 || len(r.repo.creds) != 0 {
			t.Errorf("%s: nothing may be stored", name)
		}
	}
}

func TestConnectWithTokenValidatesTheForm(t *testing.T) {
	r := newTokenRig(t)
	a := session(uuid.New())
	long := strings.Repeat("a", MaxFieldBytes+1)
	for name, tc := range map[string]struct {
		fields map[string]string
		field  string
	}{
		"unknown field":     {map[string]string{"instance_url": testHost, "access_token": testSecret, "extra": "x"}, "fields"},
		"missing required":  {map[string]string{"instance_url": testHost}, "access_token"},
		"blank required":    {map[string]string{"instance_url": testHost, "access_token": "   "}, "access_token"},
		"nil map":           {nil, "instance_url"},
		"oversize":          {map[string]string{"instance_url": testHost, "access_token": long}, "access_token"},
		"oversize optional": {map[string]string{"instance_url": testHost, "access_token": testSecret, "handle": long}, "handle"},
		"http url":          {map[string]string{"instance_url": "http://social.example.com", "access_token": testSecret}, "instance_url"},
		"javascript url":    {map[string]string{"instance_url": "javascript:alert(1)", "access_token": testSecret}, "instance_url"},
		"url with userinfo": {map[string]string{"instance_url": "https://u:p@social.example.com", "access_token": testSecret}, "instance_url"},
		"url without host":  {map[string]string{"instance_url": "https://", "access_token": testSecret}, "instance_url"},
		"not a url":         {map[string]string{"instance_url": "social.example.com", "access_token": testSecret}, "instance_url"},
	} {
		_, err := r.connect(a, tc.fields)
		e := wantCode(t, err, errs.Validation)
		if e.Fields[tc.field] == "" {
			t.Errorf("%s: want a message on %q, got %+v", name, tc.field, e.Fields)
		}
		if strings.Contains(e.Message, testSecret) || strings.Contains(e.Message, "javascript") {
			t.Errorf("%s: message echoes input: %q", name, e.Message)
		}
	}
	if r.prov.calls != 0 {
		t.Fatalf("an invalid form must not reach the provider (%d calls)", r.prov.calls)
	}
}

func TestConnectWithTokenPassesTrimmedFieldsToVerify(t *testing.T) {
	r := newTokenRig(t)
	_, err := r.connect(session(uuid.New()), map[string]string{"instance_url": " " + testHost + " ", "access_token": " " + testSecret + "\n"})
	if err != nil {
		t.Fatal(err)
	}
	if r.prov.got["instance_url"] != testHost || r.prov.got["access_token"] != testSecret {
		t.Fatalf("fields: %v", r.prov.got)
	}
}

func TestConnectWithTokenMapsVerifyFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		code errs.Code
	}{
		"auth failure": {&provider.Error{Kind: provider.KindAuth, Provider: "tokennet", HTTPStatus: 401, Message: "bad token " + testSecret}, errs.Validation},
		"permanent":    {&provider.Error{Kind: provider.KindPermanent, Provider: "tokennet", HTTPStatus: 400, Message: "nope"}, errs.Validation},
		"retryable":    {provider.FromHTTPStatus("tokennet", 503, "down", 0), errs.ProviderError},
		"network":      {errors.New("dial failed"), errs.Validation},
		"timeout":      {context.DeadlineExceeded, errs.ProviderError},
		"unsupported":  {provider.ErrUnsupported, errs.ProviderNotAvailable},
	} {
		r := newTokenRig(t)
		r.prov.verifyErr = tc.err
		_, err := r.connect(session(uuid.New()), validFields())
		e := wantCode(t, err, tc.code)
		if tc.code == errs.Validation && !strings.Contains(e.Message, "TokenNet rejected these credentials") && name != "network" {
			t.Errorf("%s: message %q", name, e.Message)
		}
		if strings.Contains(e.Message, testSecret) {
			t.Errorf("%s: client message leaks the credential: %q", name, e.Message)
		}
		if len(r.repo.accs) != 0 || len(r.repo.creds) != 0 || len(r.audit.entries) != 0 {
			t.Errorf("%s: a failed verify must store nothing", name)
		}
	}
}

func TestConnectWithTokenVerifyHasATimeout(t *testing.T) {
	r := newTokenRig(t)
	r.prov.slow = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	go func() { time.Sleep(50 * time.Millisecond); cancel() }() // stands in for the 10 s deadline
	_, err := r.svc.ConnectWithToken(ctx, session(uuid.New()), "tokennet", validFields())
	_ = wantCode(t, err, errs.ProviderError)
	if time.Since(start) > 2*time.Second {
		t.Fatal("verify did not honour the context")
	}
}

func TestConnectWithTokenRejectsOtherProviders(t *testing.T) {
	r := newTokenRig(t)
	a := session(uuid.New())
	for _, name := range []string{"nope", "", "telegram"} {
		_, err := r.svc.ConnectWithToken(context.Background(), a, name, validFields())
		_ = wantCode(t, err, errs.ProviderNotAvailable)
	}
}

func TestReconnectingWithTokenUpdatesTheSameAccount(t *testing.T) {
	r := newTokenRig(t)
	a := session(uuid.New())
	first, err := r.connect(a, validFields())
	if err != nil {
		t.Fatal(err)
	}
	r.repo.accs[first.ID].Status = socialaccount.StatusExpired
	r.prov.secret = "tok_live_rotated_0123456789" // gitleaks:allow (fake test value)
	second, err := r.connect(a, validFields())
	if err != nil || second.ID != first.ID || len(r.repo.accs) != 1 {
		t.Fatalf("want one account, got %d (%v)", len(r.repo.accs), err)
	}
	if r.repo.accs[first.ID].Status != socialaccount.StatusActive {
		t.Fatal("reconnect must reactivate an expired account")
	}
	back, _ := r.svc.Vault().Load(context.Background(), first.ID)
	if back.AccessToken != r.prov.secret {
		t.Fatal("reconnect must replace the stored credential")
	}
}

func TestConnectWithTokenIsGatedForUnverifiedOwners(t *testing.T) {
	r := newTokenRig(t)
	unverified := session(uuid.New())
	unverified.EmailVerified = false
	_, err := r.connect(unverified, validFields())
	_ = wantCode(t, err, errs.EmailNotVerified)
	if r.prov.calls != 0 || len(r.repo.accs) != 0 {
		t.Fatal("an unverified owner must not reach the provider or store an account")
	}
	// The owner gate in connectAccount covers a verification that lapsed after the check.
	r.svc.gate = closedGate{}
	_, err = r.connect(session(uuid.New()), validFields())
	_ = wantCode(t, err, errs.EmailNotVerified)
	if len(r.repo.accs) != 0 || len(r.repo.creds) != 0 {
		t.Fatal("nothing may be stored when the gate is closed")
	}
	r.svc.gate = nil
	verified := session(uuid.New())
	verified.EmailVerified = true
	if _, err := r.connect(verified, validFields()); err != nil {
		t.Fatalf("a verified owner: %v", err)
	}
}
