package fsadapter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// ContainedSource is a shaper.ContainedSource.
var _ shaper.ContainedSource = ContainedSource{}

// ContainedSource reads a handoff or a Goal from the file system, strictly inside a
// worktree root, with no gap between what is proven about the file and what is read from
// it. The zero value is ready to use.
//
// A hard link to an outside inode placed inside the root is still accepted: its kernel path
// is inside the root. Nothing here is a signature or an authority; it keeps a path that
// leads out of the worktree from being read as if it led in.
type ContainedSource struct {
	// openHook is a test seam, nil in production. ReadContained calls it with stage
	// "pre-open" immediately before the byte-acquiring open and with stage "post-open"
	// after the descriptor is confirmed regular and before containment is proven, so a test
	// can race the file system at exactly those points.
	openHook func(stage, joined string)
}

// hook calls the test seam, when there is one.
func (s ContainedSource) hook(stage, joined string) {
	if s.openHook != nil {
		s.openHook(stage, joined)
	}
}

// ReadContained reads worktreeRoot/relPath with no check-then-use gap between what is
// proven and what is read:
//
//  1. It opens the target once with O_NOFOLLOW|O_NONBLOCK, so a final component that is
//     (or was swapped for) a symlink is refused by the kernel and a FIFO cannot block the
//     open.
//  2. It fstats that SAME descriptor and requires a regular file, which refuses
//     directories, FIFOs and devices.
//  3. It asks the kernel for the path the descriptor actually names and proves that path
//     lies strictly inside the resolved worktreeRoot with pathguard.WithinRoot. No
//     containment check re-walks the path by name, so swapping an ancestor directory
//     before or after the open cannot make outside bytes pass. A descriptor whose file was
//     unlinked is refused.
//  4. It reads the bytes from that same descriptor.
//
// worktreeRoot is caller-designated and trusted, so it is resolved by path. The initial
// Lstat only shapes the "not accessible", "symlink" and "regular file" messages; it is not
// a security check. On platforms without a supported descriptor-path query the read fails
// closed. label names the source in errors ("goal source"); relPath is already cleaned by
// the caller (shaper does that before it asks), and the proof in step 3 does not depend on
// it.
func (s ContainedSource) ReadContained(worktreeRoot, relPath, label string) ([]byte, error) {
	joined := filepath.Join(worktreeRoot, relPath)

	info, err := os.Lstat(joined)
	if err != nil {
		return nil, fmt.Errorf("%s %q is not accessible: %w", label, relPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s %q must not be a symlink", label, relPath)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %q must be a regular file", label, relPath)
	}

	s.hook("pre-open", joined)
	f, err := openNoFollow(joined)
	if err != nil {
		if isSymlinkRefusal(err) {
			return nil, fmt.Errorf("%s %q must not be a symlink", label, relPath)
		}
		return nil, fmt.Errorf("open %s %q: %w", label, relPath, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s %q: %w", label, relPath, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %q must be a regular file", label, relPath)
	}

	s.hook("post-open", joined)
	opened, err := fdPath(f)
	if err != nil {
		return nil, fmt.Errorf("could not prove containment of %q: %w", relPath, err)
	}
	resolvedRoot, err := pathguard.ResolvePathKeepingMissing(worktreeRoot)
	if err != nil {
		return nil, fmt.Errorf("could not prove containment of %q: %w", relPath, err)
	}
	if resolvedRoot == "" || opened == "" {
		return nil, fmt.Errorf("could not prove containment of %q: empty resolved path", relPath)
	}
	if !pathguard.WithinRoot(filepath.Clean(resolvedRoot), filepath.Clean(opened)) {
		return nil, fmt.Errorf("%s %q resolves outside the worktree root", label, relPath)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s %q: %w", label, relPath, err)
	}
	return data, nil
}
