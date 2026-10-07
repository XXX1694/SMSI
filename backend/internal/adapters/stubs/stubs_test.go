package stubs

import (
	"strings"
	"testing"

	"github.com/socialos/backend/internal/adapters/provider"
)

func TestStubsAreHonestlyUnsupported(t *testing.T) {
	all := All()
	want := []string{"instagram", "facebook", "tiktok", "youtube", "x", "threads", "pinterest"}
	if len(all) != len(want) {
		t.Fatalf("%d stubs", len(all))
	}
	seen := map[string]bool{}
	for i, p := range all {
		if p.Name() != want[i] || seen[p.Name()] {
			t.Errorf("stub %d is %q, want %q (names must be unique)", i, p.Name(), want[i])
		}
		seen[p.Name()] = true
		if p.DisplayName() == "" || p.Supported() || p.Configured() {
			t.Errorf("%s: display=%q supported=%v configured=%v", p.Name(), p.DisplayName(), p.Supported(), p.Configured())
		}
		c := p.Capabilities()
		if c.CanPublishText || c.CanPublishImage || c.CanPublishVideo || c.CanSchedule || c.CanDelete || c.CanAnalytics || c.MaxTextLength != 0 || c.MaxMediaCount != 0 {
			t.Errorf("%s claims capabilities it does not have: %+v", p.Name(), c)
		}
		if !c.RequiresApproval || c.ConnectMethod != provider.ConnectNone || !strings.HasPrefix(c.Notes, "UNSUPPORTED") || len(c.Notes) < 30 {
			t.Errorf("%s capabilities must be labelled as unsupported with the reason: %+v", p.Name(), c)
		}
		// A stub cannot even be asked to publish or connect: the registry refuses, and there is no code path that fakes success.
		if _, ok := p.(provider.Publisher); ok {
			t.Errorf("%s must not implement Publisher", p.Name())
		}
		if _, ok := p.(provider.OAuth); ok {
			t.Errorf("%s must not implement OAuth", p.Name())
		}
		if _, ok := p.(provider.ChatVerifier); ok {
			t.Errorf("%s must not implement ChatVerifier", p.Name())
		}
	}
	reg := provider.NewRegistry(all...)
	for _, name := range want {
		if _, _, err := reg.Publisher(name); err == nil {
			t.Errorf("registry returned a publisher for %s", name)
		}
		if _, _, err := reg.OAuth(name); err == nil {
			t.Errorf("registry returned an OAuth provider for %s", name)
		}
	}
}
