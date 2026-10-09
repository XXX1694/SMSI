package accounts

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/linkcode"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// Connecting a chat (Telegram channel or group) needs proof that the Steerpost
// user controls it, otherwise anyone who knows a channel's @username could
// attach it to their own account through the shared bot. The proof is a
// one-time code:
//
//  1. StartChatLink creates a code bound to the user (only its hash is stored).
//  2. The user adds the bot as admin and posts the code in the chat.
//  3. The platform delivers that message (webhook or polling) to HandleChatUpdate,
//     which redeems the code for its owner after re-checking the bot's rights.

// LinkStart is what the user needs to complete a link. Code is shown once.
type LinkStart struct {
	ID           uuid.UUID
	Code         string
	ExpiresAt    time.Time
	BotUsername  string
	Instructions string
}

// LinkStatus is the owner-facing state of a link attempt.
type LinkStatus struct {
	Status  linkcode.Status
	Account *socialaccount.Account
}

// chatLinker resolves a provider that supports proving chat ownership by code.
func (s *Service) chatLinker(providerName string) (provider.Provider, provider.ChatLinker, error) {
	p, err := s.registry.Get(providerName)
	if err != nil {
		return nil, nil, err
	}
	l, ok := p.(provider.ChatLinker)
	if !ok || !p.Supported() {
		return nil, nil, errs.Newf(errs.ProviderNotAvailable, "%s does not support chat connections", p.DisplayName())
	}
	if !p.Configured() {
		return nil, nil, errs.Newf(errs.ProviderNotAvailable, "%s is not configured on this server", p.DisplayName())
	}
	return p, l, nil
}

// StartChatLink creates a one-time link code for the session user. At most
// linkcode.MaxActive codes are active per user; starting another retires the oldest.
func (s *Service) StartChatLink(ctx context.Context, a actor.Actor, providerName string) (*LinkStart, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	if err := a.RequireVerified(); err != nil {
		return nil, err
	}
	_, linker, err := s.chatLinker(providerName)
	if err != nil {
		return nil, err
	}
	bot, err := linker.BotUsername(ctx)
	if err != nil {
		return nil, providerFailure(err)
	}
	code, err := linkcode.Generate()
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "cannot generate a link code", err)
	}
	now := s.clock.Now()
	rec := &linkcode.Code{UserID: a.UserID, Hash: linkcode.Hash(code), ExpiresAt: now.Add(linkcode.TTL)}
	if err := s.links.Create(ctx, rec, linkcode.MaxActive, now); err != nil {
		return nil, err
	}
	return &LinkStart{ID: rec.ID, Code: code, ExpiresAt: rec.ExpiresAt, BotUsername: bot,
		Instructions: linker.LinkInstructions(bot, code, linkcode.TTL)}, nil
}

// ChatLinkStatus reports the state of one of the caller's link attempts. A
// link id that belongs to another user is NOT_FOUND, like an unknown one.
func (s *Service) ChatLinkStatus(ctx context.Context, a actor.Actor, id uuid.UUID) (*LinkStatus, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	rec, err := s.links.Get(ctx, a.UserID, id)
	if err != nil {
		return nil, err
	}
	st := &LinkStatus{Status: rec.StatusAt(s.clock.Now())}
	if st.Status == linkcode.StatusConnected && rec.SocialAccountID != nil {
		acc, err := s.repo.Get(ctx, a.UserID, *rec.SocialAccountID)
		if err != nil && !errs.Is(err, errs.NotFound) {
			return nil, err
		}
		st.Account = acc
	}
	return st, nil
}

// HandleChatUpdate processes one raw inbound platform update (a webhook body or
// a polled item). It is the single entry point for both Telegram intake modes.
//
// Everything that is not a valid proof is ignored silently: the chat gets no
// reply (so nothing leaks about which codes exist) and the debug log never
// contains the code. A non-nil error means a transient failure (database or
// platform unavailable) that is worth retrying; the polling loop does so.
func (s *Service) HandleChatUpdate(ctx context.Context, providerName string, raw []byte) error {
	p, linker, err := s.chatLinker(providerName)
	if err != nil {
		return nil // not configured here: nothing to do
	}
	msg, err := linker.ParseUpdate(raw)
	if err != nil {
		s.log.Debug("ignoring malformed chat update", slog.String("provider", p.Name()), slog.Any("error", err))
		return nil
	}
	if msg == nil {
		return nil
	}
	return s.redeemLinkCode(ctx, p, linker, *msg)
}

func (s *Service) redeemLinkCode(ctx context.Context, p provider.Provider, linker provider.ChatLinker, msg provider.ChatMessage) error {
	code, err := linkcode.Normalize(msg.Text)
	if err != nil {
		return nil // ordinary chat traffic
	}
	rec, err := s.links.FindByHash(ctx, linkcode.Hash(code))
	if errs.Is(err, errs.NotFound) {
		s.log.Debug("chat message did not match a link code", slog.String("provider", p.Name()))
		return nil
	}
	if err != nil {
		return err
	}
	now := s.clock.Now()
	if !rec.Usable(now) {
		s.log.Debug("link code is used or expired", slog.String("provider", p.Name()), slog.String("link_id", rec.ID.String()))
		return nil
	}
	// In groups anyone can post; only an administrator's message proves control.
	if !msg.SenderTrusted {
		admin, err := linker.IsChatAdmin(ctx, msg.ChatID, msg.SenderID)
		if err != nil {
			return s.ignoreOrRetry(err, p, rec.ID, "sender check failed")
		}
		if !admin {
			s.log.Debug("link code posted by a non-admin", slog.String("provider", p.Name()), slog.String("link_id", rec.ID.String()))
			return nil
		}
	}
	prof, err := linker.VerifyChat(ctx, msg.ChatID)
	if err != nil {
		return s.ignoreOrRetry(err, p, rec.ID, "bot rights check failed")
	}

	owner := actor.System(rec.UserID, p.Name())
	acc := accountFromProfile(owner, p.Name(), prof, []string{"post_messages"}, now)
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.connectAccount(ctx, owner, acc, nil); err != nil {
			return err
		}
		return s.links.MarkUsed(ctx, rec.ID, now, msg.ChatID, acc.ID)
	})
	if errs.Is(err, errs.NotFound) {
		s.log.Debug("link code was redeemed concurrently", slog.String("provider", p.Name()), slog.String("link_id", rec.ID.String()))
		return nil
	}
	if errs.Is(err, errs.QuotaExceeded) {
		// Not transient: retrying cannot help, and returning it would make the shared poller stall every user's linking.
		// The code stays unused (the transaction rolled back) and the dashboard keeps showing the link as pending.
		s.log.Warn("chat link refused: the owner is at the connected accounts limit", slog.String("provider", p.Name()), slog.String("link_id", rec.ID.String()))
		return nil
	}
	if err != nil {
		return err
	}
	s.log.Info("chat linked", slog.String("provider", p.Name()), slog.String("link_id", rec.ID.String()), slog.String("account_id", acc.ID.String()))

	// Best effort: the code is already spent, so removing it is only tidiness.
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := linker.DeleteMessage(dctx, msg.ChatID, msg.MessageID); err != nil {
		s.log.Debug("cannot delete the code message", slog.String("provider", p.Name()), slog.Any("error", err))
	}
	return nil
}

// ignoreOrRetry decides what a failed platform call means for an inbound code:
// transient failures are returned so the caller retries; anything else (missing
// rights, unknown chat, bot kicked) is a silent non-match.
func (s *Service) ignoreOrRetry(err error, p provider.Provider, linkID uuid.UUID, what string) error {
	if transientProviderError(err) {
		return err
	}
	attrs := []any{slog.String("provider", p.Name()), slog.String("link_id", linkID.String()), slog.String("reason", provider.CodeOf(err))}
	var pe *provider.Error
	if errors.As(err, &pe) && (pe.Code == "BOT_UNAUTHORIZED" || pe.Code == "BOT_NOT_CONFIGURED") {
		s.log.Error(what+": the bot credentials were rejected", attrs...)
	} else {
		s.log.Debug(what, attrs...)
	}
	return nil
}

// transientProviderError reports whether retrying the same platform call later can succeed.
func transientProviderError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	switch provider.Classify(err) {
	case provider.KindRetryable, provider.KindUnknown:
		return true
	}
	return false
}
