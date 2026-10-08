package queue

import (
	"testing"
	"time"
)

func TestProcessAtForRoundsUpToWholeSecond(t *testing.T) {
	whole := time.Date(2026, 10, 8, 21, 26, 10, 0, time.UTC)
	cases := []struct{ in, want time.Time }{
		{whole, whole},
		{whole.Add(958 * time.Millisecond), whole.Add(time.Second)},
		{whole.Add(time.Nanosecond), whole.Add(time.Second)},
		{whole.Add(999_999_999 * time.Nanosecond), whole.Add(time.Second)},
	}
	for _, c := range cases {
		got := ProcessAtFor(c.in)
		if !got.Equal(c.want) || got.Before(c.in) {
			t.Errorf("ProcessAtFor(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
