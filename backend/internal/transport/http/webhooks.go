package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/socialos/backend/internal/adapters/telegram"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/transport/httpx"
)

const (
	// maxWebhookBody bounds one Telegram update (a text message is a few KB).
	maxWebhookBody = 256 << 10
	// webhookTimeout bounds the processing of one update after the request was verified.
	webhookTimeout = 20 * time.Second
)

// telegramWebhook receives updates when TELEGRAM_UPDATES_MODE=webhook. The
// request must carry the secret_token given to setWebhook; anything else is 401.
// Once verified it always answers 200, so Telegram never retries or queues up an
// update we cannot do anything with: the outcome (linked, ignored, failed) is
// never visible to the sender and only goes to the log.
func (a *API) telegramWebhook(w http.ResponseWriter, r *http.Request) {
	if !telegram.SecretMatches(r.Header.Get(telegram.WebhookSecretHeader), a.opt.TelegramWebhookSecret) {
		httpx.ErrorCode(w, r, errs.Unauthenticated, "invalid webhook secret")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		a.opt.Logger.Warn("telegram webhook body unreadable", slog.Bool("too_large", errors.As(err, &tooBig)))
	} else {
		// Finish the work even if Telegram hangs up first.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), webhookTimeout)
		defer cancel()
		if err := a.svc.Accounts.HandleChatUpdate(ctx, "telegram", body); err != nil {
			a.opt.Logger.Warn("telegram webhook update not processed", slog.Any("error", err))
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}
