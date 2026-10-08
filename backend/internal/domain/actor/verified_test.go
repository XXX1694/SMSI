package actor

import (
	"testing"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/errs"
)

func TestRequireVerified(t *testing.T) {
	uid := uuid.New()
	for name, tc := range map[string]struct {
		a    Actor
		want errs.Code
	}{
		"unverified session": {Actor{UserID: uid, Type: TypeUser, SessionID: uuid.New()}, errs.EmailNotVerified},
		"unverified api key": {Actor{UserID: uid, Type: TypeAPIKey}, errs.EmailNotVerified},
		"verified session":   {Actor{UserID: uid, Type: TypeUser, SessionID: uuid.New(), EmailVerified: true}, ""},
		"verified api key":   {Actor{UserID: uid, Type: TypeAPIKey, EmailVerified: true}, ""},
		"scheduler":          {Scheduler(uid), ""},
		"system":             {System(uid, "telegram"), ""},
		"anonymous":          {Actor{}, errs.Unauthenticated},
	} {
		err := tc.a.RequireVerified()
		if tc.want == "" && err != nil {
			t.Errorf("%s: unexpected %v", name, err)
		}
		if tc.want != "" && !errs.Is(err, tc.want) {
			t.Errorf("%s: want %s, got %v", name, tc.want, err)
		}
	}
}
