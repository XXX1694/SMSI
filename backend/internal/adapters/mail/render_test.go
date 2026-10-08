package mail

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestRenderGolden(t *testing.T) {
	d := Data{Link: "https://app.example.com/verify-email#token=abc123", ExpiresIn: "48 hours"}
	for _, name := range []string{VerifyEmail, ResetPassword, PasswordChanged, AccountDeleted, ExportReady} {
		m, err := Render(name, "alice@example.com", d)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := "Subject: " + m.Subject + "\n\n--- text ---\n" + m.Text + "\n--- html ---\n" + m.HTML
		path := filepath.Join("testdata", name+".golden")
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run go test ./internal/adapters/mail -update)", name, err)
		}
		if string(want) != got {
			t.Errorf("%s differs from %s:\n%s", name, path, got)
		}
		if m.Template != name || m.To != "alice@example.com" {
			t.Errorf("%s: envelope fields wrong: %+v", name, m)
		}
	}
}

func TestRenderEscapesHTMLOnly(t *testing.T) {
	m, err := Render(VerifyEmail, "a@b.c", Data{Link: "https://x.test/?a=1&b=<script>", ExpiresIn: "1h"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.HTML, "<script>") {
		t.Fatalf("html must be escaped: %s", m.HTML)
	}
	if !strings.Contains(m.Text, "a=1&b=<script>") {
		t.Fatalf("text part must stay verbatim: %s", m.Text)
	}
}

func TestRenderUnknownTemplate(t *testing.T) {
	if _, err := Render("nope", "a@b.c", Data{}); err == nil {
		t.Fatal("expected error")
	}
}
