package pipkg

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// resolveSymlinkTarget walks linkTarget from base, re-resolving any real
// symlink a component names, so a chain like "b" -> "." can't fool a
// lexical Join+Clean into looking contained while physically escaping
// (R1/R3-chain-escape). A missing component is only join-checked after.
func resolveSymlinkTarget(realDest, base, linkTarget string) (string, error) {
	current, missing, hops := base, false, 0
	queue := strings.Split(filepath.ToSlash(linkTarget), "/")
	for len(queue) > 0 {
		part := queue[0]
		queue = queue[1:]
		switch part {
		case "", ".":
			continue
		case "..":
			current = filepath.Dir(current)
		default:
			current = filepath.Join(current, part)
		}
		if rel, err := filepath.Rel(realDest, current); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("resolved path escapes destination")
		}
		if missing {
			continue
		}
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			missing = true
			continue
		} else if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if hops++; hops > 40 {
			return "", fmt.Errorf("too many symlink hops resolving %s", linkTarget)
		}
		link, err := os.Readlink(current)
		if err != nil {
			return "", err
		}
		if filepath.IsAbs(link) || filepath.VolumeName(link) != "" {
			return "", fmt.Errorf("intermediate symlink has absolute target: %s", current)
		}
		current = filepath.Dir(current)
		queue = append(strings.Split(filepath.ToSlash(link), "/"), queue...)
	}
	return current, nil
}

// extractTar writes r's tar stream into dest, refusing any entry whose name
// would resolve outside dest -- git archive never produces such entries for
// a normal repository, but this is defense in depth against a corrupted or
// crafted archive stream. A symlink entry is recreated as a real symlink on
// disk when its link target resolves inside dest (a tracked symlink such as
// skills/<id> -> ../<dir>/<id> is legitimate git-archive output); a symlink
// whose target would resolve outside dest is refused, as
// is any hard-link entry -- there is no legitimate use case for one in a
// git archive of tracked content.
func extractTar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(hdr.Name))
		if rel, relErr := filepath.Rel(dest, target); relErr != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("refusing tar entry outside destination: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			linkTarget := filepath.FromSlash(hdr.Linkname)
			if filepath.IsAbs(linkTarget) || filepath.VolumeName(linkTarget) != "" {
				return fmt.Errorf("refusing symlink with absolute target: %s -> %s", hdr.Name, hdr.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			realParent, err := resolveSymlinkTarget(dest, dest, filepath.Dir(filepath.FromSlash(hdr.Name)))
			if err != nil {
				return fmt.Errorf("refusing symlink whose parent escaped destination: %s: %w", hdr.Name, err)
			}
			if _, err := resolveSymlinkTarget(dest, realParent, linkTarget); err != nil {
				return fmt.Errorf("refusing symlink outside destination: %s -> %s: %w", hdr.Name, hdr.Linkname, err)
			}
			if err := os.Symlink(linkTarget, target); err != nil {
				return err
			}
		case tar.TypeLink:
			return fmt.Errorf("refusing hard link in git archive: %s", hdr.Name)
		}
	}
}
