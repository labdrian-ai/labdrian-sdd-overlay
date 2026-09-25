package shaper

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"
)

// containedOpenHook is an unexported test seam. readContainedRegularFile
// calls it with stage "pre-open" immediately before the byte-acquiring open
// and with stage "post-open" after the descriptor is confirmed regular and
// before containment is proven, so tests can race the filesystem at exactly
// those points. Production code never replaces it.
var containedOpenHook = func(stage, joined string) {}

// cleanContainedRelPath applies the lexical half of the contained-read rules
// shared by BindGoal and LoadHandoff: worktreeRoot must be a non-empty
// absolute path and relPath a non-empty relative path that neither cleans to
// the root itself nor traverses above it. It returns the cleaned relPath.
// Errors carry no operation prefix; callers add their own.
func cleanContainedRelPath(worktreeRoot, argName, relPath string) (string, error) {
	if worktreeRoot == "" {
		return "", fmt.Errorf("worktreeRoot must not be empty")
	}
	if !filepath.IsAbs(worktreeRoot) {
		return "", fmt.Errorf("worktreeRoot must be absolute, got %q", worktreeRoot)
	}
	if relPath == "" {
		return "", fmt.Errorf("%s must not be empty", argName)
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("%s must be relative, got %q", argName, relPath)
	}
	cleaned := filepath.Clean(relPath)
	if cleaned == "." {
		return "", fmt.Errorf("%s must not resolve to the worktree root itself, got %q", argName, relPath)
	}
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return "", fmt.Errorf("%s must not traverse outside the worktree root, got %q", argName, relPath)
		}
	}
	return cleaned, nil
}

// readContainedRegularFile reads worktreeRoot/cleaned with no check-then-use
// gap between what is proven and what is read:
//
//  1. It opens the target once with O_NOFOLLOW|O_NONBLOCK, so a final
//     component that is (or was swapped for) a symlink is refused by the
//     kernel and a FIFO cannot block the open.
//  2. It fstats that SAME descriptor and requires a regular file, which
//     refuses directories, FIFOs and devices.
//  3. It asks the kernel for the path the descriptor actually names and
//     proves that path lies strictly inside the resolved worktreeRoot with
//     pathguard.WithinRoot. No containment check re-walks the path by name,
//     so swapping an ancestor directory before or after the open cannot make
//     outside bytes pass. A descriptor whose file was unlinked is refused.
//  4. It reads the bytes from that same descriptor.
//
// worktreeRoot is caller-designated and trusted, so it is resolved by path.
// A hard link to an outside inode placed inside the root is still accepted:
// its kernel path is inside the root. The initial Lstat only shapes the
// "not accessible", "symlink" and "regular file" messages; it is not a
// security check. On platforms without a supported descriptor-path query the
// read fails closed. label names the source in errors ("goal source").
func readContainedRegularFile(worktreeRoot, cleaned, label string) ([]byte, error) {
	joined := filepath.Join(worktreeRoot, cleaned)

	info, err := os.Lstat(joined)
	if err != nil {
		return nil, fmt.Errorf("%s %q is not accessible: %w", label, cleaned, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s %q must not be a symlink", label, cleaned)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %q must be a regular file", label, cleaned)
	}

	containedOpenHook("pre-open", joined)
	f, err := openNoFollow(joined)
	if err != nil {
		if isSymlinkRefusal(err) {
			return nil, fmt.Errorf("%s %q must not be a symlink", label, cleaned)
		}
		return nil, fmt.Errorf("open %s %q: %w", label, cleaned, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s %q: %w", label, cleaned, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %q must be a regular file", label, cleaned)
	}

	containedOpenHook("post-open", joined)
	opened, err := fdPath(f)
	if err != nil {
		return nil, fmt.Errorf("could not prove containment of %q: %w", cleaned, err)
	}
	resolvedRoot, err := pathguard.ResolvePathKeepingMissing(worktreeRoot)
	if err != nil {
		return nil, fmt.Errorf("could not prove containment of %q: %w", cleaned, err)
	}
	if resolvedRoot == "" || opened == "" {
		return nil, fmt.Errorf("could not prove containment of %q: empty resolved path", cleaned)
	}
	if !pathguard.WithinRoot(filepath.Clean(resolvedRoot), filepath.Clean(opened)) {
		return nil, fmt.Errorf("%s %q resolves outside the worktree root", label, cleaned)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s %q: %w", label, cleaned, err)
	}
	return data, nil
}

// sha256Hex returns the lowercase hex SHA-256 of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
