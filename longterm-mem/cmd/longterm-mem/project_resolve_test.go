package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// declaredRepo builds a throwaway git repository under t.TempDir() that
// declares the given project name, and returns its path. Tests here run
// git inside their own repositories only; nothing touches the repository
// this module lives in.
func declaredRepo(t *testing.T, project string) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", "-b", "main")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(root, projectid.DeclaredFileName), []byte(project+"\n"), 0o644); err != nil {
		t.Fatalf("write declared file: %v", err)
	}
	// Resolve canonicalizes the directory it is given, so compare against
	// the canonical form here too rather than the possibly-symlinked temp
	// path (macOS /tmp, container mounts).
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return real
}

// TestResolveProjectFlag_OmittedResolvesFromWorkingDirectory is the point
// of the whole change: --project stops being mandatory when the working
// directory already answers the question.
func TestResolveProjectFlag_OmittedResolvesFromWorkingDirectory(t *testing.T) {
	repo := declaredRepo(t, "acme-widgets")
	t.Chdir(repo)

	var project string
	var exit int
	stderr := captureStderr(t, func() { project, exit = resolveProjectFlag("query", "") })

	if exit != exitOK {
		t.Fatalf("resolveProjectFlag with no --project inside a repository exited %d, want %d; stderr=%q", exit, exitOK, stderr)
	}
	if project != "acme-widgets" {
		t.Fatalf("resolved project = %q, want %q", project, "acme-widgets")
	}
}

// TestResolveProjectFlag_UnresolvableKeepsTheRefusalAndNamesWhy: outside a
// repository the old behaviour stands -- refuse -- but the message now
// says what failed instead of only restating the flag.
func TestResolveProjectFlag_UnresolvableKeepsTheRefusalAndNamesWhy(t *testing.T) {
	t.Chdir(t.TempDir())

	var project string
	var exit int
	stderr := captureStderr(t, func() { project, exit = resolveProjectFlag("query", "") })

	if exit != exitUsage {
		t.Fatalf("resolveProjectFlag outside a repository exited %d, want %d (usage)", exit, exitUsage)
	}
	if project != "" {
		t.Fatalf("a failed resolution produced project %q; an empty or junk project name must never reach Engram", project)
	}
	if !strings.Contains(stderr, "--project is required") {
		t.Errorf("refusal %q no longer carries the existing --project is required shape", stderr)
	}
	if !strings.Contains(stderr, "not inside a git repository") {
		t.Errorf("refusal %q does not name why resolution failed", stderr)
	}
}

// TestResolveProjectFlag_CorrespondenceCheck: an explicit --project that
// disagrees with the directory's canonical identity is the moment a
// fragmenting call is detectable. It warns and proceeds -- see
// projectid.Correspondence.Warning for why warn and not refuse.
func TestResolveProjectFlag_CorrespondenceCheck(t *testing.T) {
	repo := declaredRepo(t, "acme-widgets")
	t.Chdir(repo)

	t.Run("mismatch warns and still proceeds", func(t *testing.T) {
		var project string
		var exit int
		stderr := captureStderr(t, func() { project, exit = resolveProjectFlag("query", "some-other-project") })

		if exit != exitOK {
			t.Fatalf("a mismatching --project exited %d, want %d: naming another project deliberately must stay possible", exit, exitOK)
		}
		if project != "some-other-project" {
			t.Fatalf("resolved project = %q, want the operator's own %q", project, "some-other-project")
		}
		if !strings.Contains(stderr, "WARN") {
			t.Errorf("mismatch produced no warning; stderr=%q", stderr)
		}
		if !strings.Contains(stderr, "some-other-project") || !strings.Contains(stderr, "acme-widgets") {
			t.Errorf("warning %q must name both the given project and the directory's canonical identity", stderr)
		}
	})

	t.Run("match stays silent", func(t *testing.T) {
		var project string
		var exit int
		stderr := captureStderr(t, func() { project, exit = resolveProjectFlag("query", "acme-widgets") })

		if exit != exitOK || project != "acme-widgets" {
			t.Fatalf("resolveProjectFlag = (%q, %d), want (%q, %d)", project, exit, "acme-widgets", exitOK)
		}
		if stderr != "" {
			t.Errorf("a corresponding --project still produced output %q; a warning that fires on correct calls teaches operators to ignore warnings", stderr)
		}
	})

	t.Run("outside a repository there is nothing to check", func(t *testing.T) {
		t.Chdir(t.TempDir())
		var project string
		var exit int
		stderr := captureStderr(t, func() { project, exit = resolveProjectFlag("query", "some-project") })

		if exit != exitOK || project != "some-project" {
			t.Fatalf("resolveProjectFlag = (%q, %d), want (%q, %d)", project, exit, "some-project", exitOK)
		}
		if stderr != "" {
			t.Errorf("outside a repository the check produced %q, want silence", stderr)
		}
	})
}

// TestCommands_ProjectFlagIsOptional walks every subcommand that takes
// --project and proves the flag is no longer mandatory: run from inside a
// repository with no --project, each one gets past its own usage check and
// fails later, on the vault registry, with the project it resolved.
func TestCommands_ProjectFlagIsOptional(t *testing.T) {
	repo := declaredRepo(t, "acme-widgets")

	// An existing but empty registry: every project is unconfigured, so a
	// command that got its project answers exit 3, never exit 2.
	registry := filepath.Join(t.TempDir(), "vaults.json")
	if err := os.WriteFile(registry, []byte(`{"version":1,"vaults":{}}`), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"query", []string{"query", "some text"}},
		{"status", []string{"status"}},
		{"doctor", []string{"doctor"}},
		{"index", []string{"index"}},
		{"sync", []string{"sync"}},
		{"promote", []string{"promote", "--id", "1"}},
		{"promote reconcile", []string{"promote", "reconcile", "some-address"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(repo)
			t.Setenv(vaultsFileEnvVar, registry)

			var exit int
			stderr := captureStderr(t, func() { exit = run(tc.args) })

			if exit == exitUsage {
				t.Fatalf("%s with no --project still exits %d (usage): stderr=%q", tc.name, exitUsage, stderr)
			}
			if !strings.Contains(stderr, "acme-widgets") {
				t.Errorf("%s did not report the project it resolved from the working directory; stderr=%q", tc.name, stderr)
			}
		})
	}
}
