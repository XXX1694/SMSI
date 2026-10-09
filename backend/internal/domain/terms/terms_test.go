package terms

import (
	"os"
	"strings"
	"testing"
)

func TestVersionMatchesFrontend(t *testing.T) {
	src, err := os.ReadFile("../../../../frontend/src/lib/legal.ts")
	if err != nil {
		t.Fatalf("read frontend legal constants: %v", err)
	}
	want := "LEGAL_VERSION = '" + CurrentVersion + "'"
	if !strings.Contains(string(src), want) {
		t.Fatalf("frontend/src/lib/legal.ts must contain %s", want)
	}
}
