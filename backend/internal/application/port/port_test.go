package port

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/errs"
)

func TestCursorRoundTrip(t *testing.T) {
	c := Cursor{At: time.Date(2026, 1, 2, 3, 4, 5, 6000, time.UTC), ID: uuid.New()}
	got, err := DecodeCursor(c.Encode())
	if err != nil || !got.At.Equal(c.At) || got.ID != c.ID {
		t.Fatalf("got %+v %v", got, err)
	}
	for _, bad := range []string{"!!", "Zm9v", "MjAyNnx4"} {
		if _, err := DecodeCursor(bad); !errs.Is(err, errs.Validation) {
			t.Fatalf("expected validation error for %q", bad)
		}
	}
}

func TestNewPageAndPaginate(t *testing.T) {
	p, _ := NewPage(0, "")
	if p.Limit != DefaultLimit {
		t.Fatal("default limit")
	}
	p, _ = NewPage(1000, "")
	if p.Limit != MaxLimit {
		t.Fatal("max limit")
	}
	items := []int{1, 2, 3}
	r := Paginate(items, 2, func(i int) Cursor { return Cursor{At: time.Unix(int64(i), 0), ID: uuid.Nil} })
	if len(r.Items) != 2 || r.NextCursor == "" {
		t.Fatal("expected next page")
	}
	r = Paginate(items, 3, func(i int) Cursor { return Cursor{} })
	if r.NextCursor != "" {
		t.Fatal("no next page expected")
	}
}
