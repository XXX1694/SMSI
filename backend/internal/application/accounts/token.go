package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

const (
	// VerifyTimeout bounds the live whoami call made while connecting.
	VerifyTimeout = 10 * time.Second
	// MaxFieldBytes bounds every submitted connect field.
	MaxFieldBytes = 2048
	// minLeakCheckLen: shorter secrets are not searched for in profiles, as a
	// 3-character value would match innocent text.
	minLeakCheckLen = 8
)

// ConnectWithToken connects an account from credentials the user pasted into the
// form a provider declares in Capabilities.ConnectFields. It needs the critical
// social:connect scope (browser sessions hold every scope). The credential is
// verified live, then stored encrypted in the vault; it is never returned,
// logged or written to the audit trail.
func (s *Service) ConnectWithToken(ctx context.Context, a actor.Actor, providerName string, fields map[string]string) (*socialaccount.Account, error) {
	if err := a.Require(apikey.SocialConnect); err != nil {
		return nil, err
	}
	if err := a.RequireVerified(); err != nil {
		return nil, err
	}
	p, conn, err := s.registry.TokenConnector(providerName)
	if err != nil {
		return nil, err
	}
	clean, err := validateConnectFields(p.Capabilities().ConnectFields, fields)
	if err != nil {
		return nil, err
	}
	vctx, cancel := context.WithTimeout(ctx, VerifyTimeout)
	defer cancel()
	prof, secret, err := conn.Verify(vctx, clean)
	if err != nil {
		s.log.WarnContext(ctx, "token connect rejected", slog.String("provider", p.Name()),
			slog.String("kind", provider.Classify(err).String()), slog.String("code", provider.CodeOf(err)))
		return nil, connectFailure(p.DisplayName(), err)
	}
	if secret == "" || prof.ID == "" {
		return nil, errs.Wrap(errs.ProviderError, p.DisplayName()+" returned an incomplete profile", errIncompleteVerify)
	}
	if leaksSecret(prof, secret, secretValues(p.Capabilities().ConnectFields, clean)) {
		s.log.ErrorContext(ctx, "provider adapter put a secret into the public profile", slog.String("provider", p.Name()))
		return nil, errs.New(errs.Internal, "could not connect the account")
	}
	acc := accountFromProfile(a, p.Name(), prof, nil, s.clock.Now())
	if err := s.connectAccount(ctx, a, acc, &provider.Token{AccessToken: secret}); err != nil {
		return nil, err
	}
	return acc, nil
}

// connectFailure maps a failed Verify. Rejected credentials and permanent
// refusals are the caller's input problem (400); everything else is a provider
// or network problem handled like any other provider failure.
func connectFailure(display string, err error) error {
	switch provider.Classify(err) {
	case provider.KindAuth, provider.KindPermanent:
		return errs.Wrap(errs.Validation, display+" rejected these credentials", err)
	default:
		return providerFailure(err)
	}
}

// validateConnectFields checks the submitted values against the provider form
// and returns trimmed copies. Messages never echo a submitted value.
func validateConnectFields(spec []provider.ConnectField, in map[string]string) (map[string]string, error) {
	byName := make(map[string]provider.ConnectField, len(spec))
	for _, f := range spec {
		byName[f.Name] = f
	}
	out := make(map[string]string, len(in))
	for name, raw := range in {
		f, ok := byName[name]
		if !ok {
			return nil, errs.Validationf("unknown field").WithField("fields", "unknown field")
		}
		if len(raw) > MaxFieldBytes {
			return nil, errs.Validationf("%s is too long", f.Label).WithField(name, "too long")
		}
		v := strings.TrimSpace(raw)
		if v != "" && f.Kind == provider.FieldURL {
			if err := checkHTTPS(v); err != nil {
				return nil, errs.Validationf("%s must be an https URL", f.Label).WithField(name, "must be an https URL")
			}
		}
		out[name] = v
	}
	for _, f := range spec {
		if f.Required && out[f.Name] == "" {
			return nil, errs.Validationf("%s is required", f.Label).WithField(f.Name, "required")
		}
	}
	return out, nil
}

func checkHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return errNotHTTPS
	}
	return nil
}

func secretValues(spec []provider.ConnectField, fields map[string]string) []string {
	var out []string
	for _, f := range spec {
		if v := fields[f.Name]; f.Kind == provider.FieldSecret && len(v) >= minLeakCheckLen {
			out = append(out, v)
		}
	}
	return out
}

// leaksSecret reports whether a credential appears in the profile that becomes
// the public account row (and accountDTO). It is a guard against adapter bugs.
func leaksSecret(prof provider.Profile, secret string, fieldSecrets []string) bool {
	blob, err := json.Marshal(prof)
	if err != nil {
		return true
	}
	text := string(blob)
	for _, v := range append(fieldSecrets, secret) {
		if len(v) >= minLeakCheckLen && strings.Contains(text, v) {
			return true
		}
	}
	return false
}

var (
	errIncompleteVerify = errors.New("verify returned no profile id or credential")
	errNotHTTPS         = errors.New("not an https url")
)
