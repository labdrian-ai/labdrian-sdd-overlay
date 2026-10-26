package skills

// What the tests of this package share. They reach no adapter: the adapters import this package,
// so a test file of package skills cannot import them, and the tests that need the YAML file of a
// registry are in the external test package (the adapter tests). The file system the
// executors write through is stood in for here, by a ProjectFS made of os calls.

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard/fsresolve"
)

// osProjectFS is the ProjectFS of a test: the files of a temporary directory, with the os calls a
// real project file system makes. It is a double of the port, as the in-memory ones of the other
// tests are. The adapter contract tests of the external package hold it to the real adapter on the
// same calls.
type osProjectFS struct{}

func (osProjectFS) Stat(name string) (fs.FileInfo, error)       { return os.Stat(name) }
func (osProjectFS) ReadDir(name string) ([]fs.DirEntry, error)  { return os.ReadDir(name) }
func (osProjectFS) MkdirAll(dir string, perm fs.FileMode) error { return os.MkdirAll(dir, perm) }
func (osProjectFS) Rename(oldPath, newPath string) error        { return os.Rename(oldPath, newPath) }
func (osProjectFS) Remove(name string) error                    { return os.Remove(name) }
func (osProjectFS) ResolvePath(name string) (string, error)     { return fsresolve.KeepingMissing(name) }

// WriteTemp writes data to a fresh temporary file in dir, at the mode perm whatever the mask of the
// process, and returns its path; the name begins with the prefix the copiers of a tree leave out.
func (osProjectFS) WriteTemp(dir string, data []byte, perm fs.FileMode) (string, error) {
	tmp, err := os.CreateTemp(dir, atomicTempPrefix+"*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := os.Chmod(name, perm); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

// testProjectFS is the project file system of a test.
func testProjectFS() ProjectFS { return osProjectFS{} }

// osExists gives a fake locker the answer of the file system to 'is this path there'.
type osExists struct{}

func (osExists) Exists(path string) error {
	_, err := os.Stat(path)
	return err
}

// scanSkillFiles lists the files of a skills tree on disk the way the tree adapter does: every
// regular file below dir as a slash-separated path relative to it, sorted, leaving out a name that
// begins with a dot (a directory with it) and anything that is not a regular file. The adapter
// contract tests of the external package hold it to the adapter on the same files.
func scanSkillFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			rel, _ := filepath.Rel(dir, path)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// validateFile compares a registry with the manifest at path, as 'skills validate' does.
func validateFile(reg Registry, path string) ([]Divergence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ValidateAgainstManifest(reg, data)
}

// loadManifestViewFile parses the manifest at path.
func loadManifestViewFile(path string) (ManifestView, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadManifestViewReader(bytes.NewReader(data))
}

// fileApprovals is the ApprovalRecordStore of a test: the files of a skill are listed from the
// directory the domain names (SkillDirPath) as the adapter lists them, and the bytes of each, and
// the record at the path the domain names (ApprovalRecordPath), are read through read, which is
// os.ReadFile or the in-memory reader of the test, so a reader that gates or counts reads sees
// these too. The adapter contract test of the external package holds the listing to the adapter.
func fileApprovals(read FileReader) ApprovalRecordStore { return readerApprovals{read} }

type readerApprovals struct{ read FileReader }

func (r readerApprovals) ReadSkillTree(sourceRoot, path string) ([]SkillEntry, error) {
	dir := SkillDirPath(sourceRoot, path)
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	var entries []SkillEntry
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		entry := SkillEntry{Rel: filepath.ToSlash(rel), Type: d.Type()}
		if entry.Type.IsRegular() && entry.Rel != ApprovalRecordName {
			var info fs.FileInfo
			if info, err = d.Info(); err != nil {
				return err
			}
			// The owner execute bit and no other: the adapter holds it to that.
			entry.Executable = info.Mode().Perm()&0o100 != 0
			if entry.Data, err = r.read(p); err != nil {
				return err
			}
		}
		entries = append(entries, entry)
		return nil
	})
	return entries, err
}

func (r readerApprovals) ReadRecord(sourceRoot, id string) ([]byte, error) {
	return r.read(ApprovalRecordPath(sourceRoot, id))
}

// ReadSkillTreeOfTheDouble is the listing of fileApprovals over the file system, for the contract
// test that holds it to the adapter.
func ReadSkillTreeOfTheDouble(sourceRoot, path string) ([]SkillEntry, error) {
	return fileApprovals(os.ReadFile).(readerApprovals).ReadSkillTree(sourceRoot, path)
}

// LintBaselineSkills is the list of ids the lint excuses for its legacy findings, for the test of
// the external package that holds it to the real registry.
var LintBaselineSkills = lintBaselineSkills

// skillMDOnly is the entries of a skill whose directory holds just this SKILL.md.
func skillMDOnly(content string) []SkillEntry {
	return []SkillEntry{{Rel: skillMDName, Data: []byte(content)}}
}

// writeTestFile writes content to path, making the directories above it.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

// The registry fixtures of the end-to-end tests, for the external test package that holds them to
// the encoder.
const (
	ProjectCLIRegistryFixture = projectCLIRegistry
	InstallRegistryFixture    = installFixtureRegistry
)

// The doubles of this package that stand in for an adapter, for the contract test that holds each
// to the adapter it imitates.
var ScanSkillFilesOfTheDouble = scanSkillFiles

func ProjectFSOfTheDouble() ProjectFS { return testProjectFS() }
