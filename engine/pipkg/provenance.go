package pipkg

import (
	"os/exec"
	"strings"
)

// resolveBuildRev resolves overlayRoot's checked-out HEAD commit SHA
// (R-005), returning "" when overlayRoot is not a git repo, HEAD cannot be
// resolved, or the source tree is dirty (R3-001: files Build is about to
// copy would not match a recorded HEAD). Local, no-fetch lookup.
func resolveBuildRev(overlayRoot string) string {
	if isSourceDirty(overlayRoot) {
		return ""
	}
	out, err := exec.Command("git", "-C", overlayRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// isSourceDirty reports whether skills/, agents/, or skills.registry.yaml
// under overlayRoot have any uncommitted change, tracked or untracked
// (R3-001). A non-git overlayRoot or any git error is reported as clean.
func isSourceDirty(overlayRoot string) bool {
	out, err := exec.Command("git", "-C", overlayRoot, "status", "--porcelain", "--untracked-files=all", "--", "skills", "agents", "skills.registry.yaml").Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// resolvePackageVersion resolves the newest reachable "v*"-tag reachable
// from rev (D5: the same resolved commit labdrian.builtFrom records,
// "single source"), stripped of its leading "v", falling back to
// "0.0.0-dev" when no tag is reachable, rev is empty, or overlayRoot is not
// a git repo (this is a local, no-fetch lookup — never touches the
// network).
func resolvePackageVersion(overlayRoot, rev string) string {
	args := []string{"-C", overlayRoot, "describe", "--tags", "--abbrev=0", "--match", "v*"}
	if rev != "" {
		args = append(args, rev)
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "0.0.0-dev"
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
}
