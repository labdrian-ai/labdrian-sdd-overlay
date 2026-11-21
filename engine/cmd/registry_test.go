package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
)

// Tests of the lock the registry commands take. What 'registry refresh' says and writes is in
// registry_refresh_test.go.

func TestRegistryLockPathIsTheLockPropagateTakes(t *testing.T) {
	for registry, want := range map[string]string{
		"/p/.atl/skill-registry.md":      "/p/.atl/skill-registry.md.lock",
		"/p/x/../.atl/skill-registry.md": "/p/.atl/skill-registry.md.lock",
		"p//.atl/./skill-registry.md":    "p/.atl/skill-registry.md.lock",
	} {
		if got := registryLockPath(registry); got != want {
			t.Errorf("registryLockPath(%q) = %q, want %q", registry, got, want)
		}
		// propagate cleans the path it is given before it names the lock, so both name one file.
		if got := registryPathFromArgs([]string{"--registry", registry}) + ".lock"; got != want {
			t.Errorf("propagate's lock for %q = %q, want %q", registry, got, want)
		}
	}
}

// holdLock takes the lock of the registry the way a writer does and returns what lets it go.
func holdLock(t *testing.T, registry string) (release func()) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(registry), 0o755); err != nil {
		t.Fatal(err)
	}
	release, err := filelock.Acquire(registryLockPath(registry), filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return release
}

// Another writer of the registry and a propagation of it take turns: the lock one holds is the lock
// the other waits for.
func TestPropagateWaitsForTheLockAnotherWriterOfTheRegistryHolds(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the 2 s bound of the registry lock")
	}
	registry := filepath.Join(t.TempDir(), "proj", ".atl", "skill-registry.md")
	release := holdLock(t, registry)
	defer release()
	if err := os.WriteFile(registry, []byte("## Skills\n## Loading protocol\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := newCapturedProcess()

	runPropagate(p.process, []string{"--registry", registry, "--embedded-contract", embeddedAntiGenericDesign})

	if !strings.Contains(p.err.String(), "acquiring registry lock") || !equalInts(p.exits, []int{1}) {
		t.Errorf("propagate = stderr %q, exits %v, want it to find the lock taken and refuse", p.err.String(), p.exits)
	}
}
