package skillsfs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// Tree is the skills.SkillTree of the real file system. It holds nothing: the zero value is ready.
type Tree struct{}

var _ skills.SkillTree = Tree{}

// ScanSkillFiles walks skillsDir and returns every regular file it contains as a
// slash-separated path relative to skillsDir, sorted.
//
// Entries whose name begins with '.' are skipped, files and directories alike.
// Nothing the overlay deploys is dot-prefixed, so this keeps editor scratch files
// and VCS metadata from being reported as unregistered content without weakening
// the guard for anything real.
func (Tree) ScanSkillFiles(skillsDir string) ([]string, error) {
	info, err := os.Stat(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("ondisk: stat skills dir %s: %w", skillsDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("ondisk: %s is not a directory", skillsDir)
	}

	var out []string
	err = filepath.WalkDir(skillsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == skillsDir {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(skillsDir, path)
		if relErr != nil {
			return relErr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ondisk: walk %s: %w", skillsDir, err)
	}

	sort.Strings(out)
	return out, nil
}

// ReadSkillSource reads the files install would install from one skill directory,
// sorted by path. A symlink is not followed and not installed, and neither is
// anything that is not a regular file; a directory is only the way to its files, so
// an empty one installs nothing. The approval record and a writer's temporary file
// are skipped by the rule install shares with the Pi package builder.
func (Tree) ReadSkillSource(dir string) ([]skills.SourceFile, error) {
	var files []skills.SourceFile
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel != "." && skills.SkipWhenCopying(rel, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}
		files = append(files, skills.SourceFile{Rel: filepath.ToSlash(rel), Data: data, Mode: info.Mode().Perm()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	return files, nil
}
