// Package buildinfo exposes build-time metadata injected via -ldflags.
package buildinfo

import "strings"

// Version is set at link time by scripts/release.sh via
//
//	-ldflags "-X github.com/carroarmato0/nextui-bmo/internal/buildinfo.Version=<v>"
//
// where <v> is the exact git tag of the built commit, or the short commit SHA
// (suffixed "-dirty" when the working tree had uncommitted changes) when the
// commit is untagged. It is empty for plain `go build` (local dev) builds.
var Version = ""

// VersionString returns the build version, falling back to "dev" when no
// version was injected (e.g. local builds).
func VersionString() string {
	if v := strings.TrimSpace(Version); v != "" {
		return v
	}
	return "dev"
}
