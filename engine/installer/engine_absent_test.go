package installer_test

// The script asks the engine where each row of the manifest goes (`manifest routes`), so a machine
// that has neither the engine binary nor Go to build it with cannot run the commands that read the
// manifest. These tests pin how they stop: exit 1, the words that name the binary and say Go is
// not installed, and nothing written (routing_pins_test.go has the world).
//
// "No Go" is a PATH that holds every tool of the machine's PATH but go. The world is the pins'
// world without the engine that newPinWorld puts in its home.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pathWithout is a directory of links to every executable on the PATH of the machine but the named
// tool, to be the whole PATH of a run that must not find it.
func pathWithout(t *testing.T, tool string) string {
	t.Helper()
	dir := t.TempDir()
	seen := map[string]bool{tool: true}
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if seen[name] {
				continue
			}
			full := filepath.Join(d, name)
			info, err := os.Stat(full)
			if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
				continue
			}
			seen[name] = true
			if err := os.Symlink(full, filepath.Join(dir, name)); err != nil {
				t.Fatalf("link %s: %v", full, err)
			}
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, tool)); err == nil {
		t.Fatalf("%s is still on the PATH of the run", tool)
	}
	return dir
}

// withoutEngineOrGo takes the engine out of the home of the world and Go off its PATH.
func (w *pinWorld) withoutEngineOrGo() {
	w.t.Helper()
	if err := os.Remove(filepath.Join(w.home, ".claude", "bin", "gentle-ai-overlay")); err != nil {
		w.t.Fatalf("take the engine out of the home: %v", err)
	}
	path := pathWithout(w.t, "go")
	w.changePath(func(string) string { return path })
}

const engineNotFound = "ERROR: engine binary not found at $HOME/.claude/bin/gentle-ai-overlay and go is not installed; cannot continue\n"

// headOf is the commit a branch is at.
func (w *pinWorld) headOf(ref string) string {
	w.t.Helper()
	return strings.TrimSpace(w.git("rev-parse", ref))
}

func TestWithNeitherTheEngineNorGoTheCommandsThatReadTheManifestStopAndWriteNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	for _, args := range [][]string{
		{"status", "--target", "claude"},
		{"sync-check", "--target", "claude"},
		{"apply", "--target", "claude"},
		{"capture", "--target", "claude"},
	} {
		t.Run(args[0], func(t *testing.T) {
			w := fullPinWorld(t)
			w.withoutEngineOrGo()
			// The target directory is there, as sync-check wants it; no file is.
			if err := os.MkdirAll(filepath.Join(w.home, ".claude", "skills"), 0o755); err != nil {
				t.Fatal(err)
			}
			main, upstream := w.headOf("main"), w.headOf("upstream")

			run := w.run(args...)

			if run.Exit != 1 || !strings.Contains(run.Stderr, engineNotFound) {
				t.Errorf("%s = exit %d, stderr %q, want exit 1 and %q", args[0], run.Exit, run.Stderr, engineNotFound)
			}
			if files := w.tree(w.home); files != pinText("") {
				t.Errorf("%s left files in the home:\n%s", args[0], files)
			}
			if w.headOf("main") != main || w.headOf("upstream") != upstream {
				t.Errorf("%s moved a branch", args[0])
			}
			if status := w.git("status", "--porcelain"); status != "" {
				t.Errorf("%s left the repository dirty:\n%s", args[0], status)
			}
			if branch := strings.TrimSpace(w.git("rev-parse", "--abbrev-ref", "HEAD")); branch != "main" {
				t.Errorf("%s left the repository on %q, want main", args[0], branch)
			}
		})
	}
}

// Bootstrap writes a repository and commits to it before it asks the engine anything, so it asks
// first: with no engine and no Go it stops before it makes a repository, a file or a branch.
func TestWithNeitherTheEngineNorGoBootstrapStopsBeforeItWritesAnything(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	w := &pinWorld{t: t, script: overlayScript(t), home: pinHome(t), overlay: t.TempDir()}
	w.env = pinEnv(w.home, w.overlay)
	w.write(w.overlay, "overlay.manifest", pinManifest)
	w.write(w.overlay, "bin/overlay", "#!/bin/sh\n")
	w.write(w.home, ".gentle-ai/backups/upgrade-20260615T175529Z/snapshot.tar.gz", "not read")
	path := pathWithout(t, "go")
	w.changePath(func(string) string { return path })

	run := w.run("bootstrap")

	if run.Exit != 1 || !strings.Contains(run.Stderr, engineNotFound) {
		t.Errorf("bootstrap = exit %d, stderr %q, want exit 1 and %q", run.Exit, run.Stderr, engineNotFound)
	}
	for _, rel := range []string{".git", "README.md", ".gitignore", "skills", "agents"} {
		if _, err := os.Stat(filepath.Join(w.overlay, rel)); err == nil {
			t.Errorf("bootstrap made %s before it stopped", rel)
		}
	}
	if run.Stdout != "" {
		t.Errorf("bootstrap said %q on stdout before it stopped, want nothing", run.Stdout)
	}
}
