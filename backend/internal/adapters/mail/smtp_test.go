package mail

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"net"
	stdmail "net/mail"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/application/port"
)

type captured struct {
	from, rcpt, data, auth string
	tlsActive              bool
}

// fakeSMTP is a minimal in-process SMTP server on loopback.
type fakeSMTP struct {
	ln       net.Listener
	cert     tls.Certificate
	pool     *x509.CertPool
	startTLS bool // advertise STARTTLS
	implicit bool // TLS from the first byte
	mu       sync.Mutex
	got      []captured
	commands []string
}

func newFakeSMTP(t *testing.T, startTLS, implicit bool) *fakeSMTP {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool: pool, startTLS: startTLS, implicit: implicit}
	t.Cleanup(func() { _ = ln.Close() })
	go f.serve()
	return f
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeSMTP) cfg(mode string) SMTPConfig {
	return SMTPConfig{Host: "127.0.0.1", Port: f.port(), TLS: mode, Username: "resend", Password: "key-123",
		From: "Steerpost <no-reply@example.com>", Timeout: 5 * time.Second, tlsConfig: &tls.Config{RootCAs: f.pool, MinVersion: tls.VersionTLS12}}
}

func (f *fakeSMTP) serve() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(c)
	}
}

func (f *fakeSMTP) handle(c net.Conn) {
	defer func() { _ = c.Close() }()
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{f.cert}, MinVersion: tls.VersionTLS12}
	secure := false
	if f.implicit {
		c = tls.Server(c, tlsCfg)
		secure = true
	}
	r, w := bufio.NewReader(c), func(s string) { _, _ = io.WriteString(c, s+"\r\n") }
	w("220 fake ESMTP")
	cur := captured{}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(line)
		f.mu.Lock()
		f.commands = append(f.commands, strings.SplitN(line, " ", 2)[0])
		f.mu.Unlock()
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			w("250-fake")
			if f.startTLS && !secure {
				w("250-STARTTLS")
			}
			w("250 AUTH PLAIN")
		case cmd == "STARTTLS":
			w("220 go ahead")
			c = tls.Server(c, tlsCfg)
			r, secure = bufio.NewReader(c), true
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			cur.auth = line
			w("235 ok")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			cur.from = line
			w("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			cur.rcpt = line
			w("250 ok")
		case cmd == "DATA":
			w("354 go")
			var sb strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				sb.WriteString(strings.TrimPrefix(l, "."))
			}
			cur.data, cur.tlsActive = sb.String(), secure
			f.mu.Lock()
			f.got = append(f.got, cur)
			f.mu.Unlock()
			w("250 queued")
		case cmd == "QUIT":
			w("221 bye")
			return
		default:
			w("250 ok")
		}
	}
}

func (f *fakeSMTP) messages() []captured {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]captured(nil), f.got...)
}

func (f *fakeSMTP) sawCommand(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.commands {
		if strings.EqualFold(c, name) {
			return true
		}
	}
	return false
}

func testMessage() port.Message {
	return port.Message{To: "Alice <alice@example.org>", Subject: "Verify your email — Steerpost", Template: VerifyEmail,
		Text: "Open https://app.example.com/verify-email#token=abc\n\n.dot line\n", HTML: "<p>Open <a href=\"https://app.example.com/x\">link</a></p>"}
}

func TestSMTPStartTLSDeliversMultipart(t *testing.T) {
	f := newFakeSMTP(t, true, false)
	s, err := NewSMTP(f.cfg(TLSStartTLS))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), testMessage()); err != nil {
		t.Fatal(err)
	}
	got := f.messages()
	if len(got) != 1 {
		t.Fatalf("messages: %d", len(got))
	}
	m := got[0]
	if !m.tlsActive || m.auth == "" {
		t.Fatalf("must be encrypted and authenticated: %+v", m)
	}
	if !strings.Contains(m.from, "<no-reply@example.com>") || !strings.Contains(m.rcpt, "<alice@example.org>") {
		t.Fatalf("envelope: %q %q", m.from, m.rcpt)
	}
	msg, err := stdmail.ReadMessage(strings.NewReader(m.data))
	if err != nil {
		t.Fatal(err)
	}
	if dec, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject")); dec != "Verify your email — Steerpost" {
		t.Fatalf("subject: %q", dec)
	}
	if !strings.Contains(msg.Header.Get("From"), "no-reply@example.com") || !strings.Contains(msg.Header.Get("To"), "alice@example.org") ||
		msg.Header.Get("MIME-Version") != "1.0" || msg.Header.Get("Date") == "" || !strings.HasSuffix(msg.Header.Get("Message-ID"), "@example.com>") {
		t.Fatalf("headers: %v", msg.Header)
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("content-type: %q %v", mt, err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var types, bodies []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p) // multipart.Reader decodes quoted-printable itself
		types = append(types, p.Header.Get("Content-Type"))
		bodies = append(bodies, strings.ReplaceAll(string(b), "\r\n", "\n"))
	}
	if len(types) != 2 || !strings.HasPrefix(types[0], "text/plain") || !strings.HasPrefix(types[1], "text/html") {
		t.Fatalf("parts: %v", types)
	}
	if bodies[0] != testMessage().Text || bodies[1] != testMessage().HTML {
		t.Fatalf("bodies: %q", bodies)
	}
}

func TestSMTPImplicitTLS(t *testing.T) {
	f := newFakeSMTP(t, false, true)
	s, err := NewSMTP(f.cfg(TLSImplicit))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), testMessage()); err != nil {
		t.Fatal(err)
	}
	if got := f.messages(); len(got) != 1 || !got[0].tlsActive || got[0].auth == "" {
		t.Fatalf("got %+v", got)
	}
}

func TestSMTPRefusesWithoutSTARTTLS(t *testing.T) {
	f := newFakeSMTP(t, false, false)
	s, _ := NewSMTP(f.cfg(TLSStartTLS))
	err := s.Send(context.Background(), testMessage())
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("want STARTTLS error, got %v", err)
	}
	if len(f.messages()) != 0 || f.sawCommand("AUTH") || f.sawCommand("MAIL") {
		t.Fatalf("nothing may be sent or authenticated in clear text: %v", f.commands)
	}
}

func TestSMTPRejectsHeaderInjection(t *testing.T) {
	f := newFakeSMTP(t, true, false)
	s, _ := NewSMTP(f.cfg(TLSStartTLS))
	for name, mut := range map[string]func(*port.Message){
		"to crlf":      func(m *port.Message) { m.To = "a@example.org\r\nBcc: victim@example.net" },
		"to lf":        func(m *port.Message) { m.To = "a@example.org\nBcc: victim@example.net" },
		"subject crlf": func(m *port.Message) { m.Subject = "Hi\r\nBcc: victim@example.net" },
		"subject lf":   func(m *port.Message) { m.Subject = "Hi\nBcc: victim@example.net" },
		"bad address":  func(m *port.Message) { m.To = "not an address" },
	} {
		m := testMessage()
		mut(&m)
		if err := s.Send(context.Background(), m); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if len(f.messages()) != 0 || len(f.commands) != 0 {
		t.Fatalf("the server must not even be contacted: %v", f.commands)
	}
}

func TestSMTPTimeoutOnSilentServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			defer func() { _ = c.Close() }()
			time.Sleep(2 * time.Second) // never greets
		}
	}()
	p, _ := strconv.Atoi(strings.Split(ln.Addr().String(), ":")[1])
	s, _ := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: p, TLS: TLSStartTLS, From: "a@example.com", Timeout: 150 * time.Millisecond})
	start := time.Now()
	if err := s.Send(context.Background(), testMessage()); err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("send did not honour the timeout: %v", time.Since(start))
	}
}

func TestNewSMTPValidation(t *testing.T) {
	ok := SMTPConfig{Host: "h", Port: 587, From: "a@example.com"}
	for name, mut := range map[string]func(*SMTPConfig){
		"bad from": func(c *SMTPConfig) { c.From = "nope" },
		"no host":  func(c *SMTPConfig) { c.Host = "" },
		"bad tls":  func(c *SMTPConfig) { c.TLS = "none" },
	} {
		c := ok
		mut(&c)
		if _, err := NewSMTP(c); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := NewSMTP(ok); err != nil {
		t.Fatal(err)
	}
}
