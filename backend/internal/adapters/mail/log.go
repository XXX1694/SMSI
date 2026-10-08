// Package mail contains the Mailer adapters (SMTP, log) and the message templates.
package mail

import (
	"context"
	"log/slog"
	"strings"

	"github.com/socialos/backend/internal/application/port"
)

// Log is a Mailer that writes to the log instead of sending. Bodies carry
// one-time links, so they are logged only in development.
type Log struct {
	log     *slog.Logger
	logBody bool
}

// NewLog builds the log mailer; env is APP_ENV.
func NewLog(log *slog.Logger, env string) *Log {
	return &Log{log: log, logBody: env == "development"}
}

// Send logs the message with a masked recipient.
func (l *Log) Send(ctx context.Context, m port.Message) error {
	attrs := []any{slog.String("to", MaskEmail(m.To)), slog.String("subject", m.Subject), slog.String("template", m.Template)}
	if l.logBody {
		attrs = append(attrs, slog.String("text", m.Text))
	}
	l.log.InfoContext(ctx, "mail not sent (MAIL_PROVIDER=log)", attrs...)
	return nil
}

// MaskEmail turns "alice@example.com" into "a***@example.com".
func MaskEmail(addr string) string {
	at := strings.LastIndex(addr, "@")
	if at < 1 {
		return "***"
	}
	return addr[:1] + "***" + addr[at:]
}
