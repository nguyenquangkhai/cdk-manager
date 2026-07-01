package main

import "runtime/debug"

// version is overridden at build time via -ldflags -X main.version=...
// (GoReleaser sets it). A plain `go install` does NOT set it, so it stays
// "dev" and we fall back to the module version from the build info.
var version = "dev"

// resolvedVersion is what the CLI reports. Read this everywhere, not `version`.
var resolvedVersion = resolveVersion()

// resolveVersion prefers the ldflags-injected version; otherwise it uses the
// module version embedded by `go install module@vX.Y.Z` ("(devel)"/empty for
// local builds, which stay "dev").
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

func main() { Execute() }
