// Package version holds the build's version, injected at link time:
//
//	go build -ldflags "-X github.com/elionaz/agenterm/internal/version.Version=0.1.0 \
//	  -X github.com/elionaz/agenterm/internal/version.Commit=abc1234"
//
// The single source of truth is the release tag (vX.Y.Z); see
// scripts/release-local.sh.
package version

import "runtime/debug"

// Set with -ldflags -X. Version has no leading "v".
var (
	Version = "dev"
	Commit  = ""
)

// Short returns the commit, falling back to the VCS revision Go embeds in
// builds made inside a git checkout ("" if unknown).
func Short() string {
	if Commit != "" {
		return Commit
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev == "" {
		return ""
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	return rev + dirty
}

// String is "0.1.0 (abc1234)", or just the version when the commit is unknown.
func String() string {
	if c := Short(); c != "" {
		return Version + " (" + c + ")"
	}
	return Version
}
