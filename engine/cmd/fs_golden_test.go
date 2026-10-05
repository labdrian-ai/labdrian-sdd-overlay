package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"testing"
)

// The golden files under testdata/fs-golden record what the verbs of 'skills' do to the file
// system and what they say when it will not let them: the tree a verb reads (the walk of
// 'validate', the source tree 'install' copies), the manifest and the lock it opens, the files
// it writes (their modes and digests, never a temporary file left behind), and the words of the
// refusals that come from the operating system. The registry goldens (testdata/registry-golden)
// pin what the verbs say of a registry; these pin what they do to the disk, and were recorded
// from the program as it was before Phase 9 unit H17 (docs/architecture/hexagonal-target.md)
// moved every os call of engine/skills into engine/skills/skillsfs. They are the contract that
// move had to keep. A change to a byte of any of them fails here. Rewrite them deliberately with
//
//	go test ./cmd -run TestFSGolden -update-fs-golden
//
// and read the diff before committing it.
//
// A case runs the built program in a throwaway world, as the registry goldens do, and records
// every invocation (command line, exit code, both streams) and the state of the places it names:
// for each path under it, a directory, a link and where it points, or a regular file with its
// permission bits, its size and the start of its SHA-256. A case that needs a directory the
// program may not read or write is skipped for a user who may read and write everything (root),
// and recording refuses a run with a skipped case.
var updateFSGolden = flag.Bool("update-fs-golden", false, "rewrite the golden files of the file system verbs")

// stateDigestLength is how much of a SHA-256 a state line shows: enough to tell one content from another.
const stateDigestLength = 12

// state records, for each path under rel, what is there: a directory (name/), a link (name ->
// target), a regular file (name mode size digest) or something else (name (kind)). withoutDigest
// leaves out the digest, for a file whose content holds the time it was written.
func (w *registryWorld) state(rel string, withoutDigest ...bool) {
	w.t.Helper()
	omit := len(withoutDigest) > 0 && withoutDigest[0]
	root := w.path(rel)
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name, _ := filepath.Rel(root, p)
		name = filepath.ToSlash(name)
		if name == "." {
			return nil
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		switch mode := info.Mode(); {
		case mode&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			lines = append(lines, fmt.Sprintf("%s -> %s", name, target))
		case mode.IsDir():
			lines = append(lines, name+"/")
		case mode.IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				lines = append(lines, fmt.Sprintf("%s %04o %d (%v)", name, mode.Perm(), info.Size(), errors.Unwrap(err)))
				return nil
			}
			digest := "-"
			if !omit {
				sum := sha256.Sum256(data)
				digest = hex.EncodeToString(sum[:])[:stateDigestLength]
			}
			shown := fmt.Sprintf("%04o", mode.Perm())
			if strings.HasSuffix(name, ".lock") {
				// A lock file is made by the lock adapter with the mask of the process.
				shown = "mask"
			}
			lines = append(lines, fmt.Sprintf("%s %s %d %s", name, shown, len(data), digest))
		default:
			lines = append(lines, fmt.Sprintf("%s (%s)", name, kindOf(mode)))
		}
		return nil
	})
	if err != nil {
		w.write("--- state of %s ---\n(%v)\n\n", rel, err)
		return
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		lines = []string{"(empty)"}
	}
	w.write("--- state of %s ---\n%s\n\n", rel, strings.Join(lines, "\n"))
}

func kindOf(mode fs.FileMode) string {
	switch {
	case mode&fs.ModeNamedPipe != 0:
		return "named pipe"
	case mode&fs.ModeSocket != 0:
		return "socket"
	case mode&fs.ModeDevice != 0:
		return "device"
	}
	return "special file"
}

// symlink makes rel a link to target, which is written as it is given (a relative target stays
// relative to the directory of the link).
func (w *registryWorld) symlink(target, rel string) {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.path(rel)), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.Symlink(target, w.path(rel)); err != nil {
		w.t.Fatal(err)
	}
}

// fifo makes rel a named pipe.
func (w *registryWorld) fifo(rel string) {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.path(rel)), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := syscall.Mkfifo(w.path(rel), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// chmod sets the permission bits of rel for the rest of the case, and gives them back (as a
// directory the world can be removed from) when it ends.
func (w *registryWorld) chmod(rel string, mode fs.FileMode) {
	w.t.Helper()
	path := w.path(rel)
	info, err := os.Stat(path)
	if err != nil {
		w.t.Fatal(err)
	}
	original := info.Mode().Perm()
	if err := os.Chmod(path, mode); err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _ = os.Chmod(path, original|0o700) })
}

// restore gives every directory under rel (and rel) the permission bits to be listed and written
// to again, and every file the bits to be read, so that what a case took away can be removed. A
// link is left alone.
func (w *registryWorld) restore(rel string) {
	w.t.Helper()
	var walk func(path string)
	walk = func(path string) {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&fs.ModeSymlink != 0 {
			return
		}
		if !info.IsDir() {
			_ = os.Chmod(path, info.Mode().Perm()|0o600)
			return
		}
		_ = os.Chmod(path, info.Mode().Perm()|0o700)
		entries, err := os.ReadDir(path)
		if err != nil {
			return
		}
		for _, e := range entries {
			walk(filepath.Join(path, e.Name()))
		}
	}
	walk(w.path(rel))
}

// removeAll deletes rel and everything under it.
func (w *registryWorld) removeAll(rel string) {
	w.t.Helper()
	if err := os.RemoveAll(w.path(rel)); err != nil {
		w.t.Fatal(err)
	}
}

// putMode writes content to rel with the permission bits mode.
func (w *registryWorld) putMode(rel, content string, mode fs.FileMode) {
	w.t.Helper()
	w.put(rel, content)
	if err := os.Chmod(w.path(rel), mode); err != nil {
		w.t.Fatal(err)
	}
}

// needsAUserWhoCannotReadEverything skips the case when the user may read and write what the
// case takes away: permission bits mean nothing to root.
func (w *registryWorld) needsAUserWhoCannotReadEverything() {
	w.t.Helper()
	if os.Geteuid() == 0 {
		w.t.Skip("the case takes permissions away, which root keeps")
	}
}

// temporaryName is the name a verb gives the temporary file of a write: a fixed prefix and a
// number that is different every time.
var temporaryName = regexp.MustCompile(`\.tmp-skills-[0-9]+`)

// maskTemporaryNames writes the number of a temporary file's name as <N>.
func maskTemporaryNames(text string) string {
	return temporaryName.ReplaceAllString(text, ".tmp-skills-<N>")
}

// TestFSGolden runs every case and compares its transcript with its golden file.
func TestFSGolden(t *testing.T) {
	for _, tc := range fsGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			if *updateFSGolden {
				t.Cleanup(func() {
					if t.Skipped() {
						t.Errorf("%s was skipped: a recording is made by a run in which every case runs", tc.name)
					}
				})
			}
			w := newRegistryWorld(t)
			w.filter = maskTemporaryNames
			tc.run(w)
			checkGoldenIn(t, "fs-golden", tc.name, w.text(), updateFSGolden)
		})
	}
}

// TestFSGoldenCasesAreDistinctFiles guards the case list itself, as the registry goldens do.
func TestFSGoldenCasesAreDistinctFiles(t *testing.T) {
	seen := map[string]bool{}
	for _, tc := range fsGoldenCases() {
		if seen[tc.name] {
			t.Errorf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true
	}
	entries, err := os.ReadDir(filepath.Join("testdata", "fs-golden"))
	if err != nil {
		t.Skipf("no golden files yet: %v", err)
	}
	for _, e := range entries {
		if name := strings.TrimSuffix(e.Name(), ".golden"); !seen[name] {
			t.Errorf("testdata/fs-golden/%s belongs to no case", e.Name())
		}
	}
}

// fsGoldenCases is every case, in the order of the files that hold them.
func fsGoldenCases() []registryGoldenCase {
	var cases []registryGoldenCase
	cases = append(cases, fsReadCases()...)
	cases = append(cases, fsWriteCases()...)
	cases = append(cases, fsProjectCases()...)
	return cases
}
