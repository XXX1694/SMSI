package user

import (
	"strings"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	got, err := NormalizeEmail("  Alice@Example.COM ")
	if err != nil || got != "alice@example.com" {
		t.Fatalf("got %q %v", got, err)
	}
	for _, bad := range []string{"", "nope", "a@", "Alice <a@b.c>"} {
		if _, err := NormalizeEmail(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if ValidatePassword("short") == nil {
		t.Fatal("short accepted")
	}
	if ValidatePassword(strings.Repeat("x", 129)) == nil {
		t.Fatal("long accepted")
	}
	if err := ValidatePassword("correct horse battery"); err != nil {
		t.Fatal(err)
	}
}
