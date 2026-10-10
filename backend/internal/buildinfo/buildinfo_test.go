package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	stamped := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.0.0-20261010120000-0123456789ab"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
			{Key: "vcs.time", Value: "2026-10-10T12:00:00Z"},
		},
	}
	devel := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}
	tests := []struct {
		name                string
		ldV, ldC, ldB       string
		bi                  *debug.BuildInfo
		wantV, wantC, wantB string
	}{
		{"link-time values win", "0.4.0", "abc1234", "2026-10-09T08:00:00Z", stamped, "0.4.0", "abc1234", "2026-10-09T08:00:00Z"},
		{"vcs stamp fills the gaps", "", "", "", stamped, "0.0.0-20261010120000-0123456789ab", "0123456", "2026-10-10T12:00:00Z"},
		{"partial link-time values", "0.4.0", "", "", stamped, "0.4.0", "0123456", "2026-10-10T12:00:00Z"},
		{"devel without vcs", "", "", "", devel, "dev", "unknown", "unknown"},
		{"no build info", "", "", "", nil, "dev", "unknown", "unknown"},
		{"short revision kept whole", "", "", "", &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}}}, "dev", "abc", "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolve(tc.ldV, tc.ldC, tc.ldB, tc.bi)
			want := Info{Version: tc.wantV, Commit: tc.wantC, BuiltAt: tc.wantB}
			if got != want {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestGetIsStable(t *testing.T) {
	a, b := Get(), Get()
	if a != b || a.Version == "" || a.Commit == "" || a.BuiltAt == "" {
		t.Fatalf("Get() = %+v then %+v; want equal and non-empty", a, b)
	}
}
