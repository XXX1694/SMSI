package errs

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestConstructorsAndAs(t *testing.T) {
	e := Newf(NotFound, "post %d not found", 7)
	if e.Code != NotFound || e.Message != "post 7 not found" || e.Error() != "NOT_FOUND: post 7 not found" {
		t.Fatalf("%+v / %s", e, e)
	}
	if v := Validationf("bad %s", "x"); v.Code != Validation || v.Message != "bad x" {
		t.Fatalf("%+v", v)
	}
	if nf := NotFoundf("media"); nf.Code != NotFound || nf.Message != "media not found" {
		t.Fatalf("%+v", nf)
	}
	wrapped := fmt.Errorf("layer: %w", e)
	got, ok := As(wrapped)
	if !ok || got != e {
		t.Fatal("As must find the error through fmt.Errorf wrapping")
	}
	if _, ok := As(errors.New("plain")); ok {
		t.Fatal("plain errors are not coded errors")
	}
	if _, ok := As(nil); ok {
		t.Fatal("nil is not a coded error")
	}
}

func TestCodeOfAndIs(t *testing.T) {
	if CodeOf(errors.New("boom")) != Internal || CodeOf(nil) != Internal {
		t.Fatal("unknown errors are INTERNAL")
	}
	err := fmt.Errorf("ctx: %w", New(Conflict, "dup"))
	if CodeOf(err) != Conflict || !Is(err, Conflict) || Is(err, NotFound) {
		t.Fatal("Is/CodeOf through wrapping")
	}
	if Is(nil, Internal) {
		t.Fatal("Is(nil) must be false")
	}
}

func TestWrapKeepsCauseHidden(t *testing.T) {
	cause := errors.New("connection refused to 10.0.0.1")
	e := Wrap(Internal, "storage failed", cause)
	if e.Message != "storage failed" || strings.Contains(e.Message, "10.0.0.1") {
		t.Fatalf("message must not include the cause: %q", e.Message)
	}
	if !errors.Is(e, cause) || !strings.Contains(e.Error(), "10.0.0.1") {
		t.Fatal("cause stays available for logs")
	}
}

func TestWithField(t *testing.T) {
	e := Validationf("invalid").WithField("email", "required").WithField("password", "too short")
	if len(e.Fields) != 2 || e.Fields["email"] != "required" || e.Fields["password"] != "too short" {
		t.Fatalf("%v", e.Fields)
	}
	if plain := New(NotFound, "x"); plain.Fields != nil {
		t.Fatal("fields must be nil unless set (omitted from JSON)")
	}
}
