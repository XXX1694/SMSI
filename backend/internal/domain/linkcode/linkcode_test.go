package linkcode

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var shapeRe = regexp.MustCompile(`^SOS-[` + Alphabet + `]{8}$`)

func TestGenerateShapeAndAlphabet(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		c, err := Generate()
		if err != nil {
			t.Fatal(err)
		}
		if !shapeRe.MatchString(c) {
			t.Fatalf("bad shape %q", c)
		}
		if strings.ContainsAny(c[len(Prefix):], "0O1IL") {
			t.Fatalf("ambiguous character in %q", c)
		}
		if seen[c] {
			t.Fatalf("duplicate code %q after %d draws", c, i)
		}
		seen[c] = true
		if n, err := Normalize(c); err != nil || n != c {
			t.Fatalf("a generated code must normalise to itself: %q -> %q %v", c, n, err)
		}
	}
}

func TestGenerateUsesAllAlphabetCharactersUniformly(t *testing.T) {
	counts := map[byte]int{}
	for i := 0; i < 4000; i++ {
		c, _ := Generate()
		for j := len(Prefix); j < len(c); j++ {
			counts[c[j]]++
		}
	}
	if len(counts) != len(Alphabet) {
		t.Fatalf("only %d of %d characters ever drawn", len(counts), len(Alphabet))
	}
	// 32000 draws over 31 symbols: expect ~1032 each; a modulo bias or a broken source would be far outside this band.
	for ch, n := range counts {
		if n < 800 || n > 1300 {
			t.Errorf("character %q drawn %d times, want about 1032", ch, n)
		}
	}
}

func TestGenerateRejectsBiasedBytesAndPropagatesReadErrors(t *testing.T) {
	// Bytes >= 248 (256 - 256%31) must be skipped, not wrapped; 0..7 map to 'A'..'H'.
	src := bytes.NewReader(append(bytes.Repeat([]byte{255, 250}, 16), bytes.Repeat([]byte{0, 1, 2, 3, 4, 5, 6, 7}, 4)...))
	c, err := generate(src)
	if err != nil || c != "SOS-ABCDEFGH" {
		t.Fatalf("got %q %v", c, err)
	}
	if _, err := generate(bytes.NewReader([]byte{1, 2, 3})); err == nil {
		t.Fatal("a short random source must be an error")
	}
}

func TestNormalize(t *testing.T) {
	ok := map[string]string{
		"SOS-7KQ2M9XA":         "SOS-7KQ2M9XA",
		"sos-7kq2m9xa":         "SOS-7KQ2M9XA",
		"  SOS-7KQ2M9XA  ":     "SOS-7KQ2M9XA",
		"\n\tSos-7kQ2m9Xa\r\n": "SOS-7KQ2M9XA",
		string(zeroWidthSpace) + "SOS-7KQ2M9XA" + string(byteOrderMark): "SOS-7KQ2M9XA",
	}
	for in, want := range ok {
		if got, err := Normalize(in); err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"", "   ", "hello", "SOS-", "SOS-7KQ2M9", "SOS-7KQ2M9XAB", "SOS7KQ2M9XA", "XOS-7KQ2M9XA",
		"SOS-7KQ2M90A", // 0 is not in the alphabet
		"SOS-7KQ2M1LA", // 1 and L are not either
		"my code is SOS-7KQ2M9XA", "SOS-7KQ2M9XA please", "SOS-7KQ2 M9XA", "SOS-7KQ2M9XA\nSOS-7KQ2M9XA",
		strings.Repeat("A", 65), strings.Repeat(" ", 100) + "SOS-7KQ2M9XA",
	} {
		if got, err := Normalize(in); !errors.Is(err, ErrMalformed) {
			t.Errorf("Normalize(%q) = %q, %v; want ErrMalformed", in, got, err)
		}
	}
}

func TestHashIsStableHexAndNotTheCode(t *testing.T) {
	h := Hash("SOS-7KQ2M9XA")
	if len(h) != 64 || h == "SOS-7KQ2M9XA" || h != Hash("SOS-7KQ2M9XA") || h == Hash("SOS-7KQ2M9XB") {
		t.Fatalf("hash %q", h)
	}
	// Different spellings of one code share a hash only through Normalize.
	n1, _ := Normalize(" sos-7kq2m9xa ")
	n2, _ := Normalize("SOS-7KQ2M9XA")
	if Hash(n1) != Hash(n2) {
		t.Fatal("normalised spellings must hash alike")
	}
}

func TestLifecycle(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	c := &Code{ID: uuid.New(), ExpiresAt: now.Add(TTL)}
	if !c.Usable(now) || c.StatusAt(now) != StatusPending {
		t.Fatal("a fresh code is pending and usable")
	}
	if !c.Usable(now.Add(TTL-time.Nanosecond)) || c.Usable(now.Add(TTL)) || c.StatusAt(now.Add(TTL)) != StatusExpired {
		t.Fatal("a code expires exactly at ExpiresAt")
	}
	used := now.Add(time.Minute)
	c.UsedAt = &used
	if c.Usable(now.Add(2*time.Minute)) || !c.Used() || c.StatusAt(now.Add(2*time.Minute)) != StatusConnected {
		t.Fatal("a used code is connected and cannot be reused")
	}
	// Used wins over expired: a connected link stays "connected" after its TTL.
	if c.StatusAt(now.Add(TTL+time.Hour)) != StatusConnected {
		t.Fatal("used codes stay connected")
	}
}
