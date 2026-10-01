package main

// Contract tests between the TUI's overlayCLI adapter and the REAL
// bin/labdrian-overlay: they pin what the TUI assumes about the backend's
// answers, so a backend change that breaks an assumption fails here instead
// of in front of an operator. Like the other *_backend_test.go files they
// spawn the actual script, but only ever against scratch directories.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hermeticBackendEnv is the environment for running the real backend without
// any path to the developer's real state. HOME, STATE_DIR and OVERLAY_DIR all
// point at scratch directories; STATE_DIR is set explicitly because the
// script honors it ahead of $HOME/.labdrian-overlay, so a value exported in
// the caller's shell would otherwise leak into the run. exec uses the last
// value of a duplicated key, so these override whatever the caller exported.
func hermeticBackendEnv(t *testing.T) (env []string, home, stateDir string) {
	t.Helper()
	home = t.TempDir()
	stateDir = filepath.Join(home, "state")
	return append(os.Environ(),
		"HOME="+home,
		"STATE_DIR="+stateDir,
		"OVERLAY_DIR="+t.TempDir(),
	), home, stateDir
}

// realOverlayCLI returns an adapter over this checkout's real backend script,
// running under hermeticBackendEnv.
func realOverlayCLI(t *testing.T) (cli overlayCLI, home, stateDir string) {
	t.Helper()
	env, home, stateDir := hermeticBackendEnv(t)
	root := filepath.Dir(filepath.Dir(realBackendBin(t)))
	return overlayCLI{root: root, env: env}, home, stateDir
}

// noMeta, as the metaContent of writeBackupFixture, writes no .meta file at all.
const noMeta = "\x00__no_meta__"

// writeBackupFixture creates a retained backup of target at the given
// timestamp under the backend's state directory, with metaContent as its
// .meta (tab-separated: version, digest, applied_at).
func writeBackupFixture(t *testing.T, stateDir, target, timestamp, metaContent string) {
	t.Helper()
	dir := filepath.Join(stateDir, "backups", target, timestamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir backup fixture: %v", err)
	}
	if metaContent != noMeta {
		if err := os.WriteFile(filepath.Join(dir, ".meta"), []byte(metaContent), 0o644); err != nil {
			t.Fatalf("write .meta fixture: %v", err)
		}
	}
}

// TestOverlayCLIContract_CatalogIsWhatAllExpandsTo is the guarantee T1 exists
// for: the targets the TUI is shown are exactly the targets the backend's
// `--target all` acts on. The expansion is read from the real script's own
// resolve_targets, so a list kept separately on either side fails here.
func TestOverlayCLIContract_CatalogIsWhatAllExpandsTo(t *testing.T) {
	cli, home, _ := realOverlayCLI(t)

	catalog, err := cli.Targets()
	if err != nil {
		t.Fatalf("Targets against the real backend: %v", err)
	}
	if len(catalog) == 0 {
		t.Fatal("the real backend returned an empty catalog")
	}

	out, code := runBackendFunc(t, t.TempDir(), home, "resolve_targets", "all")
	if code != 0 {
		t.Fatalf("resolve_targets all exit=%d\n%s", code, out)
	}
	var names []string
	for _, tgt := range catalog {
		names = append(names, tgt.Name)
	}
	if got, want := strings.Join(names, " "), strings.TrimSpace(out); got != want {
		t.Errorf("catalog the TUI reads = %q, but `--target all` expands to %q", got, want)
	}
}

// TestOverlayCLIContract_KindsMatchTheBackendsCopyRule: capture and restore
// are offered only for KindCopy targets, so the kind the TUI parses must be
// the one the backend enforces (is_copy_target) when it refuses the rest.
func TestOverlayCLIContract_KindsMatchTheBackendsCopyRule(t *testing.T) {
	cli, home, _ := realOverlayCLI(t)

	catalog, err := cli.Targets()
	if err != nil {
		t.Fatalf("Targets against the real backend: %v", err)
	}
	if len(catalog) == 0 {
		t.Fatal("the real backend returned an empty catalog; nothing to compare")
	}
	for _, tgt := range catalog {
		_, code := runBackendFunc(t, t.TempDir(), home, "is_copy_target", tgt.Name)
		backendSaysCopy := code == 0
		if backendSaysCopy != (tgt.Kind == KindCopy) {
			t.Errorf("target %s: the TUI parsed kind %q, but is_copy_target exits %d", tgt.Name, tgt.Kind, code)
		}
	}
}
