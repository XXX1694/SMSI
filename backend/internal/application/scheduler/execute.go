package scheduler

import (
	"context"
	"io"
	"log/slog"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// failure is a classified outcome to persist.
type failure struct {
	code, message string
	accountAuth   bool // mark account expired
}

// execute performs the provider call outside of any DB transaction.
func (p *Publisher) execute(ctx context.Context, r *run) error {
	if !r.account.Publishable() {
		code := string(errs.SocialAccountExpired)
		return p.fail(ctx, r, failure{code: code, message: r.account.Provider + " account must be reconnected"}, nil)
	}
	prov, pub, err := p.registry.Publisher(r.account.Provider)
	if err != nil {
		return p.fail(ctx, r, failure{code: string(errs.ProviderNotAvailable), message: err.Error()}, nil)
	}
	req, err := p.buildRequest(ctx, r)
	if err != nil {
		return p.onProviderError(ctx, r, prov, err)
	}
	if cm := prov.Capabilities().ConnectMethod; cm == provider.ConnectOAuth || cm == provider.ConnectToken {
		token, err := p.freshToken(ctx, r.account, prov)
		if err != nil {
			return p.onProviderError(ctx, r, prov, err)
		}
		req.AccessToken = token
	}
	if r.lookup {
		if res, found, err := p.lookup(ctx, pub, req); err != nil {
			return p.onProviderError(ctx, r, prov, err)
		} else if found {
			return p.succeed(ctx, r, res, map[string]any{"resolved_by": "lookup"})
		}
	}
	pctx, cancel := context.WithTimeout(ctx, PublishTimeout)
	defer cancel()
	res, err := pub.Publish(pctx, req)
	if err != nil {
		return p.onProviderError(ctx, r, prov, err)
	}
	return p.succeed(ctx, r, res, res.Metadata)
}

func (p *Publisher) lookup(ctx context.Context, pub provider.Publisher, req provider.PublishRequest) (provider.PublishResult, bool, error) {
	lk, ok := pub.(provider.Lookuper)
	if !ok {
		return provider.PublishResult{}, false, nil
	}
	return lk.Lookup(ctx, req)
}

func (p *Publisher) buildRequest(ctx context.Context, r *run) (provider.PublishRequest, error) {
	req := provider.PublishRequest{
		IdempotencyKey: r.target.IdempotencyKey,
		Title:          r.post.Title,
		Text:           r.target.Content,
		Account: provider.AccountRef{ID: r.account.ID.String(), ProviderAccountID: r.account.ProviderAccountID,
			Username: r.account.Username, Metadata: r.account.Metadata},
	}
	if len(r.post.MediaIDs) == 0 {
		return req, nil
	}
	items, err := p.media.GetMany(ctx, r.post.UserID, r.post.MediaIDs)
	if err != nil {
		return req, &provider.Error{Kind: provider.KindRetryable, Provider: r.account.Provider, Code: "MEDIA_LOAD", Message: "could not load media", Err: err}
	}
	byID := map[string]int{}
	for i, m := range items {
		byID[m.ID.String()] = i
	}
	for _, id := range r.post.MediaIDs {
		i, ok := byID[id.String()]
		if !ok {
			return req, &provider.Error{Kind: provider.KindPermanent, Provider: r.account.Provider, Code: "MEDIA_MISSING", Message: "attached media no longer exists"}
		}
		m := items[i]
		key := m.StorageKey
		req.Media = append(req.Media, provider.MediaFile{
			Kind: string(m.Kind), MimeType: m.MimeType, Size: m.SizeBytes, Name: m.OriginalName,
			Open: func(ctx context.Context) (io.ReadCloser, error) { return p.media.Open(ctx, key) },
		})
	}
	return req, nil
}

// freshToken returns a valid access token, refreshing it when it expires soon.
func (p *Publisher) freshToken(ctx context.Context, acc *socialaccount.Account, prov provider.Provider) (string, error) {
	creds, err := p.vault.Load(ctx, acc.ID)
	if err != nil {
		return "", &provider.Error{Kind: provider.KindRetryable, Provider: acc.Provider, Code: "CREDENTIALS_LOAD", Message: "could not load credentials", Err: err}
	}
	now := p.clock.Now()
	if creds.AccessToken == "" {
		return "", &provider.Error{Kind: provider.KindAuth, Provider: acc.Provider, Code: "NO_CREDENTIALS", Message: "no stored credentials"}
	}
	if !creds.NeedsRefresh(now, RefreshWindow) {
		return creds.AccessToken, nil
	}
	oauth, ok := prov.(provider.OAuth)
	if !ok || !creds.CanRefresh(now) {
		if creds.ExpiresAt.After(now) {
			return creds.AccessToken, nil // still valid for a few minutes, cannot refresh
		}
		return "", &provider.Error{Kind: provider.KindAuth, Provider: acc.Provider, Code: "TOKEN_EXPIRED", Message: "authorization has expired"}
	}
	tok, err := oauth.Refresh(ctx, creds.RefreshToken)
	if err != nil {
		return "", err
	}
	if err := p.vault.Save(ctx, acc.ID, tok); err != nil {
		return "", &provider.Error{Kind: provider.KindRetryable, Provider: acc.Provider, Code: "CREDENTIALS_SAVE", Message: "could not store refreshed credentials", Err: err}
	}
	p.log.InfoContext(ctx, "refreshed provider token", slog.String("account_id", acc.ID.String()), slog.String("provider", acc.Provider))
	return tok.AccessToken, nil
}

// onProviderError classifies a failure and persists the matching outcome.
func (p *Publisher) onProviderError(ctx context.Context, r *run, prov provider.Provider, err error) error {
	msg := provider.SafeMessage(err)
	code := provider.CodeOf(err)
	p.log.WarnContext(ctx, "publish attempt failed", slog.String("target_id", r.target.ID.String()),
		slog.String("provider", r.account.Provider), slog.String("kind", provider.Classify(err).String()), slog.String("code", code))
	switch provider.Classify(err) {
	case provider.KindAuth:
		return p.fail(ctx, r, failure{code: string(errs.SocialAccountExpired), message: msg, accountAuth: true}, err)
	case provider.KindUnknown:
		if prov.Capabilities().SafeToRetryAfterUnknown || p.canLookup(prov.Name()) {
			if !r.retryInfo.Exhausted() {
				return p.retryLater(ctx, r, failure{code: "OUTCOME_UNKNOWN", message: msg}, true, err)
			}
		}
		return p.needsReview(ctx, r, &failure{code: "OUTCOME_UNKNOWN", message: msg})
	case provider.KindRetryable:
		if !r.retryInfo.Exhausted() {
			return p.retryLater(ctx, r, failure{code: code, message: msg}, false, err)
		}
		return p.fail(ctx, r, failure{code: code, message: msg + " (retries exhausted)"}, err)
	case provider.KindUnsupported:
		return p.fail(ctx, r, failure{code: string(errs.ProviderNotAvailable), message: msg}, err)
	default:
		return p.fail(ctx, r, failure{code: code, message: msg}, err)
	}
}
