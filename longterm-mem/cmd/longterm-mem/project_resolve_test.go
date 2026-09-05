package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/identityledger"
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

// remoteRepo builds a throwaway repository whose origin normalizes to
// remote, and returns its canonical path.
func remoteRepo(t *testing.T, remote string) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"remote", "add", "origin", remote},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return real
}

// writeVaults writes a registry file naming exactly the given projects.
func writeVaults(t *testing.T, projects ...string) string {
	t.Helper()
	rows := make([]string, 0, len(projects))
	for _, p := range projects {
		rows = append(rows, `"`+p+`": {"path": "~/somewhere"}`)
	}
	path := filepath.Join(t.TempDir(), "vaults.json")
	if err := os.WriteFile(path, []byte(`{"schema":1,"vaults":{`+strings.Join(rows, ",")+`}}`), 0o600); err != nil {
		t.Fatalf("write vaults.json: %v", err)
	}
	return path
}

// The defect this fixes, reproduced: a repository whose origin normalizes
// to "github.com/acme/widgets" also derives the plain "widgets". When the
// memory already lives under "widgets", minting the URL-shaped name is the
// resolver fragmenting the repository itself -- a second, empty identity
// beside the real one.
func TestResolveProjectFlag_AdoptsTheNameTheMemoryAlreadyLivesUnder(t *testing.T) {
	t.Setenv(vaultsFileEnvVar, writeVaults(t, "widgets"))
	t.Setenv(engramDBEnvVar, filepath.Join(t.TempDir(), "absent.db"))
	t.Chdir(remoteRepo(t, "https://github.com/acme/widgets.git"))

	var project string
	var exit int
	stderr := captureStderr(t, func() { project, exit = resolveProjectFlag("query", "") })

	if exit != exitOK {
		t.Fatalf("exit = %d, want %d; stderr=%q", exit, exitOK, stderr)
	}
	if project != "widgets" {
		t.Fatalf("an established derivable name must be adopted, not re-minted: got %q; stderr=%q", project, stderr)
	}
	if !strings.Contains(stderr, "adopted") {
		t.Fatalf("adoption changes which project the command acts on and must be said out loud: stderr=%q", stderr)
	}
}

// Two derivable names both holding memory is the fragmentation itself, and
// it is provable rather than guessed. longterm-mem may not merge Engram's
// store (R-002 keeps its connection read-only), so what it owes the
// operator is the finding and the remedy -- not silence.
func TestResolveProjectFlag_ReportsDerivableNamesOwedAnIntegration(t *testing.T) {
	t.Setenv(vaultsFileEnvVar, writeVaults(t, "acme-widgets", "widgets"))
	t.Setenv(engramDBEnvVar, filepath.Join(t.TempDir(), "absent.db"))
	repo := remoteRepo(t, "https://github.com/acme/widgets.git")
	if err := os.WriteFile(filepath.Join(repo, projectid.DeclaredFileName), []byte("acme-widgets\n"), 0o644); err != nil {
		t.Fatalf("write declared file: %v", err)
	}
	t.Chdir(repo)

	var project string
	stderr := captureStderr(t, func() { project, _ = resolveProjectFlag("query", "") })

	if project != "acme-widgets" {
		t.Fatalf("the highest-ranked established name is canonical: got %q", project)
	}
	if !strings.Contains(stderr, "widgets") || !strings.Contains(stderr, "integrat") {
		t.Fatalf("the fragment owed an integration must be named, with its remedy: stderr=%q", stderr)
	}
}

// The hole derivation alone cannot close, end to end: a repository with no
// declaration and no remote is known only by its path. Move it, and that
// name stops being derivable -- and the memory stored under it becomes
// unreachable, silently, forever. The ledger travels inside .git, so it
// moves WITH the repository and reunites the two.
func TestResolveProjectFlag_AMovedRepositoryIsReunitedWithItsMemory(t *testing.T) {
	t.Setenv(engramDBEnvVar, filepath.Join(t.TempDir(), "absent.db"))
	parent := t.TempDir()
	before := filepath.Join(parent, "before")
	plainRepo(t, before)

	// Live once at the old path, so the ledger records that name.
	t.Setenv(vaultsFileEnvVar, writeVaults(t))
	t.Chdir(before)
	var oldName string
	captureStderr(t, func() { oldName, _ = resolveProjectFlag("query", "") })
	if oldName == "" {
		t.Fatal("the first resolution produced no name")
	}

	// The repository moves. Nothing derives its old name any more.
	after := filepath.Join(parent, "after")
	if err := os.Rename(before, after); err != nil {
		t.Fatalf("move the repository: %v", err)
	}

	// The memory is still filed under the old name.
	t.Setenv(vaultsFileEnvVar, writeVaults(t, oldName))
	t.Chdir(after)

	var got string
	stderr := captureStderr(t, func() { got, _ = resolveProjectFlag("query", "") })
	if got != oldName {
		t.Fatalf("a moved repository lost its memory: resolved %q, want the remembered %q; stderr=%q", got, oldName, stderr)
	}
}

// The ledger records the repository, not one checkout of it, so it lives in
// the git COMMON directory. A per-worktree ledger would fragment the very
// record that exists to prevent fragmentation.
func TestIdentityLedger_IsSharedByEveryWorktree(t *testing.T) {
	t.Setenv(engramDBEnvVar, filepath.Join(t.TempDir(), "absent.db"))
	t.Setenv(vaultsFileEnvVar, writeVaults(t))
	parent := t.TempDir()
	main := filepath.Join(parent, "main")
	plainRepo(t, main)
	runGit(t, main, "commit", "-q", "--allow-empty", "-m", "init")

	// Resolve once from the main checkout: the ledger gets written.
	t.Chdir(main)
	captureStderr(t, func() { resolveProjectFlag("query", "") })

	worktree := filepath.Join(parent, "feature")
	runGit(t, main, "worktree", "add", "-q", "-b", "feature", worktree)

	commonFromWorktree, err := projectid.CommonDir(worktree)
	if err != nil {
		t.Fatalf("CommonDir(worktree): %v", err)
	}
	names, err := identityledger.Names(commonFromWorktree)
	if err != nil {
		t.Fatalf("Names from the worktree: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("the worktree cannot see what the main checkout recorded: the ledger is not shared")
	}

	// And nothing was written into the worktree's own git directory.
	if _, err := os.Stat(filepath.Join(main, ".git", "worktrees", "feature", "longterm-mem")); !os.IsNotExist(err) {
		t.Fatalf("a per-worktree ledger was created; it must live in the common dir only (%v)", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// plainRepo initializes a repository with no declaration and no remote, so
// its only identity is its path.
func plainRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	runGit(t, path, "init", "-q", "-b", "main")
}
