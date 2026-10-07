package clock

import (
	"testing"
	"time"
)

func TestSystemIsUTC(t *testing.T) {
	now := System{}.Now()
	if now.Location() != time.UTC || time.Since(now) > time.Second || time.Until(now) > time.Second {
		t.Fatalf("System clock: %v", now)
	}
}

func TestFakeClock(t *testing.T) {
	zone := time.FixedZone("x", 3600)
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, zone)
	f := NewFake(start)
	if !f.Now().Equal(start) || f.Now().Location() != time.UTC {
		t.Fatalf("fake must store UTC: %v", f.Now())
	}
}

func TestFakeAdvance(t *testing.T) {
	f := NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	f.Advance(90 * time.Minute)
	if want := time.Date(2026, 1, 1, 1, 30, 0, 0, time.UTC); !f.Now().Equal(want) {
		t.Fatalf("%v", f.Now())
	}
	f.Advance(-time.Hour)
	if want := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC); !f.Now().Equal(want) {
		t.Fatalf("%v", f.Now())
	}
}
