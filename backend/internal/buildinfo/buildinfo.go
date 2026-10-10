// Package buildinfo reports which build of the backend is running (GET /version).
//
// Release images set the values at link time from the git tag and commit (backend/Dockerfile, release.yml):
//
//	go build -ldflags "-X github.com/socialos/backend/internal/buildinfo.version=0.4.0 \
//	  -X github.com/socialos/backend/internal/buildinfo.commit=abc1234 \
//	  -X github.com/socialos/backend/internal/buildinfo.builtAt=2026-10-10T12:00:00Z"
//
// Without them (go run, go build from a checkout) the VCS stamp of the Go toolchain is used when present, else "dev" and
// "unknown". Nothing host- or toolchain-specific is reported: the endpoint is public.
package buildinfo

import (
	"runtime/debug"
	"strings"
	"sync"
)

// Set with -ldflags "-X …" at build time; empty in local builds.
var (
	version string
	commit  string
	builtAt string
)

const (
	devVersion   = "dev"
	unknownValue = "unknown"
	shortSHA     = 7
)

// Info is the public description of the running build.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
}

// Get returns the build of this binary. The result never changes, so it is computed once.
var Get = sync.OnceValue(func() Info {
	bi, _ := debug.ReadBuildInfo() // nil (not ok) in binaries built without module support
	return resolve(version, commit, builtAt, bi)
})

// resolve prefers the link-time values, then the toolchain's VCS stamp, then placeholders.
func resolve(ldVersion, ldCommit, ldBuiltAt string, bi *debug.BuildInfo) Info {
	info := Info{Version: ldVersion, Commit: ldCommit, BuiltAt: ldBuiltAt}
	var vcsRevision, vcsTime string
	if bi != nil {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				vcsRevision = s.Value
			case "vcs.time":
				vcsTime = s.Value
			}
		}
		if info.Version == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			info.Version = strings.TrimPrefix(bi.Main.Version, "v")
		}
	}
	if info.Commit == "" && vcsRevision != "" {
		info.Commit = vcsRevision[:min(shortSHA, len(vcsRevision))]
	}
	// vcs.time is the commit time, the closest honest stand-in for a build time the toolchain records.
	if info.BuiltAt == "" {
		info.BuiltAt = vcsTime
	}
	info.Version = orDefault(info.Version, devVersion)
	info.Commit = orDefault(info.Commit, unknownValue)
	info.BuiltAt = orDefault(info.BuiltAt, unknownValue)
	return info
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
