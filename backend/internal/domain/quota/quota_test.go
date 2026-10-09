package quota

import (
	"strings"
	"testing"

	"github.com/socialos/backend/internal/domain/errs"
)

func TestCheck(t *testing.T) {
	for name, tc := range map[string]struct {
		used, delta, limit int64
		ok                 bool
	}{
		"below":              {3, 1, 5, true},
		"exactly the limit":  {4, 1, 5, true},
		"one over":           {5, 1, 5, false},
		"big delta":          {0, 11, 10, false},
		"unlimited":          {1 << 40, 1 << 40, Unlimited, true},
		"already over, zero": {7, 0, 5, false},
	} {
		err := Check(ConnectedAccounts, tc.used, tc.delta, tc.limit)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err=%v", name, err)
		}
		if err != nil && !errs.Is(err, errs.QuotaExceeded) {
			t.Errorf("%s: code %s", name, errs.CodeOf(err))
		}
	}
}

func TestCheckMessagesSayWhatToDo(t *testing.T) {
	for m, want := range map[Metric]string{ConnectedAccounts: "Disconnect", ScheduledPostsMonth: "next month", MediaBytes: "Delete media"} {
		err := Check(m, 10<<20, 1<<20, 5<<20)
		e, _ := errs.As(err)
		if e == nil || !strings.Contains(e.Message, want) || e.Fields["quota"] != string(m) {
			t.Errorf("%s: %+v", m, e)
		}
	}
}

func TestLimitsFor(t *testing.T) {
	l := Limits{Accounts: 1, PostsPerMonth: 2, MediaBytes: 3}
	if l.For(ConnectedAccounts) != 1 || l.For(ScheduledPostsMonth) != 2 || l.For(MediaBytes) != 3 || l.For("x") != Unlimited {
		t.Fatalf("%+v", l)
	}
}
