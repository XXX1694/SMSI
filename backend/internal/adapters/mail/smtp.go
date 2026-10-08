package mail

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/socialos/backend/internal/application/port"
)

// TLS modes of the SMTP connection.
const (
	TLSStartTLS = "starttls" // plain connect (port 587), then a mandatory STARTTLS upgrade
	TLSImplicit = "implicit" // TLS from the first byte (port 465)
)

// DefaultTimeout bounds dialing plus the whole SMTP conversation.
const DefaultTimeout = 30 * time.Second

// SMTPConfig configures the SMTP mailer.
type SMTPConfig struct {
	Host     string
	Port     int
	TLS      string
	Username string
	Password string
	From     string // "Name <addr>" or "addr"
	Timeout  time.Duration

	// tlsConfig is for tests (trust the fake server's certificate).
	tlsConfig *tls.Config
}

// SMTP sends mail through an SMTP relay. It never authenticates, or sends
// anything, over a connection that is not encrypted.
type SMTP struct {
	cfg  SMTPConfig
	from *mail.Address
}

// NewSMTP validates the configuration.
func NewSMTP(cfg SMTPConfig) (*SMTP, error) {
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("mail: invalid From: %w", err)
	}
	if cfg.Host == "" || cfg.Port < 1 {
		return nil, errors.New("mail: SMTP host and port are required")
	}
	if cfg.TLS == "" {
		cfg.TLS = TLSStartTLS
	}
	if cfg.TLS != TLSStartTLS && cfg.TLS != TLSImplicit {
		return nil, fmt.Errorf("mail: unknown TLS mode %q", cfg.TLS)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &SMTP{cfg: cfg, from: from}, nil
}

// Send delivers m. Recipient and subject must be single-line.
func (s *SMTP) Send(ctx context.Context, m port.Message) error {
	to, err := parseRecipient(m.To)
	if err != nil {
		return err
	}
	if strings.ContainsAny(m.Subject, "\r\n") {
		return errors.New("mail: subject contains a line break")
	}
	if m.Text == "" || m.HTML == "" {
		return errors.New("mail: both text and html bodies are required")
	}
	body, err := s.build(to, m)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	c, err := s.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("mail: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: end of DATA: %w", err)
	}
	_ = c.Quit()
	return nil
}

func parseRecipient(raw string) (*mail.Address, error) {
	if strings.ContainsAny(raw, "\r\n") {
		return nil, errors.New("mail: recipient contains a line break")
	}
	a, err := mail.ParseAddress(raw)
	if err != nil {
		return nil, fmt.Errorf("mail: invalid recipient: %w", err)
	}
	return a, nil
}

// connect dials, secures the connection and returns a client that is ready for AUTH.
func (s *SMTP) connect(ctx context.Context) (*smtp.Client, error) {
	d := net.Dialer{Timeout: s.cfg.Timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port)))
	if err != nil {
		return nil, fmt.Errorf("mail: dial: %w", err)
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	tlsCfg := s.cfg.tlsConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		tlsCfg = tlsCfg.Clone()
	}
	tlsCfg.ServerName = s.cfg.Host
	if s.cfg.TLS == TLSImplicit {
		conn = tls.Client(conn, tlsCfg)
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mail: greeting: %w", err)
	}
	if s.cfg.TLS == TLSStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			_ = c.Close()
			return nil, errors.New("mail: server does not offer STARTTLS; refusing to send unencrypted")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("mail: STARTTLS: %w", err)
		}
	}
	return c, nil
}

// build renders the RFC 5322 message with CRLF line endings.
func (s *SMTP) build(to *mail.Address, m port.Message) ([]byte, error) {
	var sb strings.Builder
	var buf = &sb
	mw := multipart.NewWriter(buf)
	h := func(k, v string) { sb.WriteString(k + ": " + v + "\r\n") }
	h("From", s.from.String())
	h("To", to.String())
	h("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	h("Date", time.Now().UTC().Format(time.RFC1123Z))
	id, err := messageID(s.from.Address)
	if err != nil {
		return nil, err
	}
	h("Message-ID", id)
	h("MIME-Version", "1.0")
	h("Content-Type", "multipart/alternative; boundary="+mw.Boundary())
	sb.WriteString("\r\n")
	for _, p := range []struct{ typ, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		pw, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {p.typ + "; charset=utf-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		qw := quotedprintable.NewWriter(pw)
		if _, err := qw.Write([]byte(strings.ReplaceAll(p.body, "\r\n", "\n"))); err != nil {
			return nil, err
		}
		if err := qw.Close(); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return []byte(sb.String()), nil
}

func messageID(from string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	domain := "localhost"
	if at := strings.LastIndex(from, "@"); at >= 0 {
		domain = from[at+1:]
	}
	return "<" + hex.EncodeToString(b) + "@" + domain + ">", nil
}
