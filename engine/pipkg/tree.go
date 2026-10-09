package pipkg

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// copyTree recursively copies src into dst, refusing any symlink found
// anywhere in the tree (R-010) and setting 0755 on directories / 0644 on
// files. What is never copied is decided by skills.SkipWhenCopying, the rule
// `skills install` follows too: the skill's approval record (repository governance
// state, not skill content) and a writer's half-written temporary file.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink at %s", path)
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return fmt.Errorf("refusing non-regular file at %s", path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if skills.SkipWhenCopying(rel, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

// copyFile copies src to dst, refusing a symlink source (R-010).
func copyFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink at %s", src)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// swap atomically replaces destDir with tmpDir's content, staging any
// previous destDir aside in a freshly created sibling dir and removing only
// that fresh dir afterward — a pre-existing "<destDir>.stale" is never
// touched (R3-stale-directory-deletion). On failure it restores destDir.
func swap(tmpDir, destDir string) error {
	hadPrevious := false
	var staleDir string
	if _, err := os.Stat(destDir); err == nil {
		staleDir, err = os.MkdirTemp(filepath.Dir(destDir), ".labdrian-pi-stale-*")
		if err != nil {
			return fmt.Errorf("pipkg: creating stale staging dir: %w", err)
		}
		if err := syscall.Rename(destDir, staleDir); err != nil { // os.Rename refuses a dir newpath
			_ = os.RemoveAll(staleDir)
			return fmt.Errorf("pipkg: staging previous package aside: %w", err)
		}
		hadPrevious = true
	}

	if err := os.Rename(tmpDir, destDir); err != nil {
		if hadPrevious {
			_ = os.Rename(staleDir, destDir)
		}
		return fmt.Errorf("pipkg: swapping built package into place: %w", err)
	}
	if hadPrevious {
		_ = os.RemoveAll(staleDir)
	}
	return nil
}

// fileEntry is one file's content plus its permission bits, as recorded by
// listFiles. Check diffs both: a byte-identical file whose mode changed is
// still drift (R-001).
type fileEntry struct {
	data []byte
	perm fs.FileMode
}

// listFiles walks root and returns every regular file's content and mode,
// keyed by its slash-separated path relative to root.
func listFiles(root string) (map[string]fileEntry, error) {
	out := make(map[string]fileEntry)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink at %s", path)
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("refusing non-regular file at %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = fileEntry{data: data, perm: info.Mode().Perm()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
