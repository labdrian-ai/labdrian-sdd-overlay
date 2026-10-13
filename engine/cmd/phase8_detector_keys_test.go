package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeDetectorSources writes the files of a stand-in for the detector's package in a directory of its own.
func writeDetectorSources(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// detectorHeader opens every stand-in source file; the declarations follow it, so that a file can hold both
// types by joining their declarations under one header.
const detectorHeader = "package p\n\n"

const (
	detectorLockDecl  = "type ProjectLock struct {\n\tVersion int `json:\"version\"`\n\tSkills []int `json:\"skills\"`\n}\n"
	detectorEntryDecl = "type ProjectLockEntry struct {\n\tID string `json:\"id\"`\n\tTargets []string `json:\"targets,omitempty\"`\n\tIgnored int `json:\"-\"`\n}\n"

	detectorLockSource  = detectorHeader + detectorLockDecl
	detectorEntrySource = detectorHeader + detectorEntryDecl
	detectorBothSource  = detectorHeader + detectorLockDecl + detectorEntryDecl
)

// The keys are read from every non-test file of the package, wherever the two types are declared, and from
// no test file.
func TestPhase8LockKeysIn_ReadsEveryNonTestFileOfThePackage(t *testing.T) {
	dir := writeDetectorSources(t, map[string]string{
		"lock.go":      detectorLockSource,
		"entry.go":     detectorEntrySource,
		"lock_test.go": "package p\n\ntype ProjectLock struct {\n\tOnlyInATest int `json:\"only_in_a_test\"`\n}\n",
		"notes.txt":    "not go",
		"unrelated.go": "package p\n\ntype Other struct {\n\tX int `json:\"x\"`\n}\n",
	})

	top, entry, err := phase8LockKeysIn(dir)
	if err != nil {
		t.Fatalf("phase8LockKeysIn: %v", err)
	}
	if want := map[string]bool{"version": true, "skills": true}; !reflect.DeepEqual(top, want) {
		t.Errorf("ProjectLock keys = %v, want %v", top, want)
	}
	if want := map[string]bool{"id": true, "targets": true}; !reflect.DeepEqual(entry, want) {
		t.Errorf("ProjectLockEntry keys = %v, want %v", entry, want)
	}
}

// A package with no source to parse is an error that says so, not keys that are missing.
func TestPhase8LockKeysIn_RefusesAPackageWithNoNonTestSource(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"an empty directory":  {},
		"only test files":     {"lock_test.go": detectorLockSource},
		"a directory missing": nil,
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeDetectorSources(t, files)
			if files == nil {
				dir = filepath.Join(dir, "absent")
			}
			_, _, err := phase8LockKeysIn(dir)
			if err == nil || !strings.Contains(err.Error(), "no non-test Go source") {
				t.Fatalf("phase8LockKeysIn = %v, want the error that there is no non-test Go source", err)
			}
		})
	}
}

// A type declared twice is an error: the later declaration would replace the earlier one without a word.
func TestPhase8LockKeysIn_RefusesATypeDeclaredTwice(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"ProjectLock":      {"a.go": detectorBothSource, "b.go": detectorLockSource},
		"ProjectLockEntry": {"a.go": detectorBothSource, "b.go": detectorEntrySource},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := phase8LockKeysIn(writeDetectorSources(t, files))
			if err == nil || !strings.Contains(err.Error(), name+" is declared more than once") {
				t.Fatalf("phase8LockKeysIn = %v, want the error that %s is declared more than once", err, name)
			}
		})
	}
}

// A type that is missing is the old error.
func TestPhase8LockKeysIn_RefusesAPackageWithoutTheTypes(t *testing.T) {
	_, _, err := phase8LockKeysIn(writeDetectorSources(t, map[string]string{"lock.go": detectorLockSource}))
	if err == nil || !strings.Contains(err.Error(), "found no json fields") {
		t.Fatalf("phase8LockKeysIn = %v, want the error that the entry type has no fields", err)
	}
}

// The real package is read.
func TestPhase8DetectorLockKeys_ReadsTheRealPackage(t *testing.T) {
	top, entry := phase8DetectorLockKeys(t)
	if !top["version"] || !top["skills"] || !entry["id"] || !entry["targets"] {
		t.Errorf("the real detector's keys are %v and %v", top, entry)
	}
}
