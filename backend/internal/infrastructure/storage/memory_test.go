package storage

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMemoryRoundTrip(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if err := m.Put(ctx, "a/b.png", strings.NewReader("hello"), 5, "image/png"); err != nil {
		t.Fatal(err)
	}
	if !m.Has("a/b.png") || m.Has("a/c.png") {
		t.Fatal("Has")
	}
	r, err := m.Get(ctx, "a/b.png")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	_ = r.Close()
	if string(b) != "hello" {
		t.Fatalf("%q", b)
	}
	// Stored bytes are a copy: mutating a read buffer does not change the object.
	b[0] = 'X'
	r, _ = m.Get(ctx, "a/b.png")
	again, _ := io.ReadAll(r)
	if string(again) != "hello" {
		t.Fatalf("object mutated through a reader: %q", again)
	}
	if err := m.Delete(ctx, "a/b.png"); err != nil || m.Has("a/b.png") {
		t.Fatalf("delete: %v", err)
	}
	if err := m.Delete(ctx, "a/b.png"); err != nil {
		t.Fatalf("deleting a missing object must succeed: %v", err)
	}
	if _, err := m.Get(ctx, "a/b.png"); err == nil {
		t.Fatal("Get after delete must fail")
	}
	if err := m.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryPutChecksSize(t *testing.T) {
	m := NewMemory()
	for name, tc := range map[string]struct {
		data string
		size int64
	}{"short": {"abc", 5}, "long": {"abcdef", 3}} {
		if err := m.Put(context.Background(), "k-"+name, strings.NewReader(tc.data), tc.size, "x/y"); err == nil {
			t.Errorf("%s: size mismatch must be an error", name)
		}
		if m.Has("k-" + name) {
			t.Errorf("%s: a failed put must not store anything", name)
		}
	}
	if err := m.Put(context.Background(), "empty", bytes.NewReader(nil), 0, "x/y"); err != nil {
		t.Fatalf("empty object: %v", err)
	}
}

func TestMemoryPresign(t *testing.T) {
	u, err := NewMemory().PresignGet(context.Background(), "users/1/media/a b.png", 15*time.Minute)
	if err != nil || !strings.HasPrefix(u, "memory://users%2F1%2Fmedia%2Fa%20b.png?expires=") {
		t.Fatalf("%q %v", u, err)
	}
	exp, err := time.Parse(time.RFC3339, u[strings.Index(u, "expires=")+8:])
	if err != nil || exp.Location() != time.UTC || time.Until(exp) < 14*time.Minute || time.Until(exp) > 16*time.Minute {
		t.Fatalf("expiry %v %v", exp, err)
	}
}

// Size -1 means "unknown length": the store reads to the end (video uploads stream this way).
func TestMemoryPutUnknownSize(t *testing.T) {
	m := NewMemory()
	if err := m.Put(context.Background(), "k", strings.NewReader("streamed"), -1, "video/mp4"); err != nil || !m.Has("k") {
		t.Fatalf("put: %v", err)
	}
	if err := m.Put(context.Background(), "short", strings.NewReader("abc"), 5, "x"); err == nil || m.Has("short") {
		t.Fatalf("a wrong known size must still fail: %v", err)
	}
}
