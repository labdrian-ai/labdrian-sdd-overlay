package main

// The reading of the registry file by the program: the repository of the composition root reads
// the file through a bounded reader, so a file that is not a registry cannot be held whole in
// memory by asking for it.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
)

func TestTheRegistryFileIsReadUpToItsBoundAndNoFurther(t *testing.T) {
	dir := t.TempDir()
	atBound := filepath.Join(dir, "at-bound.yaml")
	overBound := filepath.Join(dir, "over-bound.yaml")
	for path, size := range map[string]int{atBound: registryyaml.MaxFileBytes, overBound: registryyaml.MaxFileBytes + 1} {
		if err := os.WriteFile(path, []byte(strings.Repeat("#", size)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := readRegistryFile(atBound)
	if err != nil || len(got) != registryyaml.MaxFileBytes {
		t.Fatalf("readRegistryFile(a file of exactly the bound) = %d bytes, %v, want it whole", len(got), err)
	}

	got, err = readRegistryFile(overBound)
	want := fmt.Sprintf("read %s: more than %d bytes, the most a registry may have", overBound, registryyaml.MaxFileBytes)
	if err == nil || err.Error() != want || len(got) != 0 {
		t.Fatalf("readRegistryFile(a file over the bound) = %d bytes, %v, want nothing and %q", len(got), err, want)
	}
}

// A file the process cannot read, or that is not a file, keeps the words of the system, as it did
// when the program read it with os.ReadFile.
func TestTheRegistryFileReaderKeepsTheWordsOfTheSystem(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nowhere.yaml")
	_, err := readRegistryFile(missing)
	_, wantErr := os.ReadFile(missing)
	if err == nil || err.Error() != wantErr.Error() {
		t.Errorf("a missing file: error = %v, want %v", err, wantErr)
	}
	dir := t.TempDir()
	_, err = readRegistryFile(dir)
	_, wantErr = os.ReadFile(dir)
	if err == nil || err.Error() != wantErr.Error() {
		t.Errorf("a directory: error = %v, want %v", err, wantErr)
	}
}

// What the bounded reader refuses reaches the verbs as a store that could not be read, which is
// what it is: nothing of a registry is known of it.
func TestARegistryFileOverTheBoundIsAStoreThatCouldNotBeRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.yaml")
	if err := os.WriteFile(path, []byte(strings.Repeat("#", registryyaml.MaxFileBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := skills.ReadRegistry(newRegistryRepository(), path)
	if !skills.IsUnreadableRegistry(err) {
		t.Fatalf("ReadRegistry(a file over the bound) = %v, want a store that could not be read", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("%v wraps ErrNotExist", err)
	}
}
