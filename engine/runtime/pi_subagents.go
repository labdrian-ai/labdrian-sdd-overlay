package runtime

import (
	"strconv"
	"strings"
)

// The Pi Subagents runner: which package makes GADU dispatchable as a Pi subagent, how the
// adapter tells gentle-pi's native subagent tools (>= 2.6.0) from the legacy extension, and what
// it installs, or refuses to, so the two never end up side by side.

// subagentsExtensionPackage is the fixed argv target R-012 installs when
// neither accepted package is already present.
const subagentsExtensionPackage = "npm:pi-subagents-j0k3r"

// subagentsSkipSetting is how the person turned off the extension probe/install (R-012) -- for
// environments that manage the Subagents extension separately -- quoted back in the note the
// adapter returns. The composition root reads that setting once (PiOptions.SkipSubagents); the
// adapter reads no environment.
const subagentsSkipSetting = "LABDRIAN_PI_SKIP_SUBAGENTS=1"

// subagentsAcceptedPackagePrefixes are the two package names gentle-pi's
// Subagents extension ships under (D11); either satisfies R-012.
var subagentsAcceptedPackagePrefixes = []string{"npm:pi-subagents-j0k3r", "npm:pi-subagents"}

// hasAcceptedSubagentsPrefix reports whether entry is one of the accepted
// package names, with or without an "@version" suffix.
func hasAcceptedSubagentsPrefix(entry string) bool {
	for _, prefix := range subagentsAcceptedPackagePrefixes {
		if entry == prefix || strings.HasPrefix(entry, prefix+"@") {
			return true
		}
	}
	return false
}

// ensureSubagentsExtension installs pi-subagents-j0k3r via a FIXED argv
// (verb, package) when neither accepted package name is already listed
// (R-012). Always returns a human-readable disclosure/status note; never
// fails Install as a whole on an install error -- the caller folds the
// note into its own message instead.
func (a PiAdapter) ensureSubagentsExtension(bin string, settings piSettings) string {
	if settings.listsSubagentsExtension() {
		return "Pi Subagents extension already installed."
	}
	disclosure := "installing third-party Pi extension pi-subagents-j0k3r (npm) required for GADU dispatch."
	if err := a.runPi(bin, "install", subagentsExtensionPackage); err != nil {
		return disclosure + " `pi install " + subagentsExtensionPackage + "` failed: " + err.Error() + "."
	}
	return disclosure + " Ran `pi install " + subagentsExtensionPackage + "`."
}

// gentlePiPackagePrefix is the fixed package name gentle-pi's own npm
// entry is listed under in ~/.pi/agent/settings.json.
const gentlePiPackagePrefix = "npm:gentle-pi"

// minNativeSubagentsVersion is the first gentle-pi release whose README
// documents the native subagent_* tools replacing the third-party
// pi-subagents-j0k3r/pi-subagents extension (verified live 2026-09-13
// against gentle-pi 2.6.0, README.md line 782).
var minNativeSubagentsVersion = piVersion{major: 2, minor: 6, patch: 0}

// piVersion is a minimal major.minor.patch comparator -- enough to compare
// gentle-pi's semver-ish version strings without pulling in a full semver
// dependency. Any unparseable suffix past the numeric patch (a prerelease
// tag, say) is ignored.
type piVersion struct {
	major, minor, patch int
}

// lessThan reports whether v is strictly older than o.
func (v piVersion) lessThan(o piVersion) bool {
	if v.major != o.major {
		return v.major < o.major
	}
	if v.minor != o.minor {
		return v.minor < o.minor
	}
	return v.patch < o.patch
}

// parsePiVersion parses a "major.minor.patch[...]" string, ignoring any
// non-numeric suffix on the patch component (e.g. "2.6.0-rc.1" parses as
// 2.6.0). Returns ok=false when major/minor/patch cannot be parsed as
// integers.
func parsePiVersion(s string) (piVersion, bool) {
	parts := strings.SplitN(strings.TrimSpace(s), ".", 3)
	if len(parts) < 3 {
		return piVersion{}, false
	}
	patchStr := parts[2]
	for i, r := range patchStr {
		if r < '0' || r > '9' {
			patchStr = patchStr[:i]
			break
		}
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	patch, errPatch := strconv.Atoi(patchStr)
	if errMajor != nil || errMinor != nil || errPatch != nil {
		return piVersion{}, false
	}
	return piVersion{major: major, minor: minor, patch: patch}, true
}

// subagentRunnerState is subagentRunnerState's result: exactly one of
// native (gentle-pi's own subagent_* tools, gentle-pi >= 2.6.0), legacy
// (the third-party pi-subagents-j0k3r/pi-subagents extension, no native
// support detected), conflict (BOTH installed -- the exact state that
// silently broke dispatch on a live machine 2026-09-13, since installing
// the obsolete extension leaves gentle-pi's native tools unregistered), or
// absent (neither installed).
type subagentRunner string

const (
	subagentRunnerNative   subagentRunner = "native"
	subagentRunnerLegacy   subagentRunner = "legacy"
	subagentRunnerConflict subagentRunner = "conflict"
	subagentRunnerAbsent   subagentRunner = "absent"
)

// ensureSubagentRunner decides GADU's Pi dispatch runner at install time
// (R-012 revised): when gentle-pi's native subagent_* tools are available
// (>= 2.6.0), the obsolete pi-subagents-j0k3r/pi-subagents extension is
// NEVER installed -- installing it would leave gentle-pi's native tools
// unregistered (verified live 2026-09-13, gentle-pi README.md line 782).
// If that obsolete extension is already listed alongside native support,
// the conflict is disclosed with the exact removal command, but the
// overlay never removes a package it does not own. Only when native
// support is unavailable does this fall back to the legacy extension
// install path unchanged.
func (a PiAdapter) ensureSubagentRunner(bin, home string) string {
	if a.options.SkipSubagents {
		return "Pi Subagents extension check skipped (" + subagentsSkipSetting + ")."
	}
	settings := readPiSettings(home)
	if settings.nativeSubagents() {
		if settings.listsSubagentsExtension() {
			return "gentle-pi native subagents (>= 2.6.0) detected; the installed pi-subagents-j0k3r/pi-subagents extension conflicts with gentle-pi's native subagent tools and must be removed (it is not overlay-owned, so run this yourself): pi remove npm:pi-subagents-j0k3r."
		}
		return "gentle-pi native subagents (>= 2.6.0) detected; skipping the legacy Pi Subagents extension install."
	}
	return a.ensureSubagentsExtension(bin, settings)
}
