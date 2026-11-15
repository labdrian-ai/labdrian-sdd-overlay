package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The session hook (engine/synctrigger) starts longterm-mem with the one argument "sync" in the
// session's working directory, and logs the exit code as an outcome: 0 is ok, 3 skip:no-vault and 4
// failure:store-unavailable (the engine's sync-trigger golden pins that mapping). These tests run that
// exact invocation -- no --project, no --source -- over a scratch memorycard store, so the codes the
// hook reads are the codes plain `sync` answers.

// enterHookPath makes the working directory one that names the project, as a session's would: the root of a
// repository that declares it in .longterm-mem-project.
func enterHookPath(t *testing.T) {
	t.Helper()
	dir := realTempDir(t)
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, ".longterm-mem-project"), []byte(promoteGoldenProject+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}

func runHookSync(t *testing.T) (int, string, string) {
	t.Helper()
	code, stdout, stderr := runPromoteGolden(t, []string{"sync"})
	if want := `"` + promoteGoldenProject + `" from the working directory`; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want it to name %s", stderr, want)
	}
	return code, stdout, stderr
}

func TestHookPathSync_PromotesFromMemorycardAndExitsZero(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	enterHookPath(t)
	code, stdout, stderr := runHookSync(t)
	if code != exitOK {
		t.Fatalf("exit %d, want %d (the hook logs ok)\nstdout: %s\nstderr: %s", code, exitOK, stdout, stderr)
	}
	if !strings.Contains(stdout, "sync --source memorycard promoted 3 memory(ies)") {
		t.Errorf("stdout = %q, want the memorycard sync line\nstderr: %s", stdout, stderr)
	}
	if names := pageNames(t, w); len(names) != 3 {
		t.Errorf("pages = %v, want the 3 promoted memories", names)
	}
}

func TestHookPathSync_AProjectWithNoVaultIsExit3(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	enterHookPath(t)
	registry := filepath.Join(realTempDir(t), "vaults.json")
	if err := os.WriteFile(registry, []byte(`{"schema": 1, "vaults": {}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LONGTERM_MEM_VAULT", "")
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", registry)
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)
	code, stdout, stderr := runHookSync(t)
	if code != exitVaultNotConfigured {
		t.Errorf("exit %d, want %d (the hook logs skip:no-vault)\nstdout: %s\nstderr: %s", code, exitVaultNotConfigured, stdout, stderr)
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("a project with no vault changed the vault")
	}
}

func TestHookPathSync_AnUnlistableStoreIsExit4(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	enterHookPath(t)
	t.Setenv("LONGTERM_MEM_MEMORYCARD_DB", unlistableMemorycardDB(t))
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)
	code, stdout, stderr := runHookSync(t)
	if code != exitStoreUnavailable {
		t.Errorf("exit %d, want %d (the hook logs failure:store-unavailable)\nstdout: %s\nstderr: %s", code, exitStoreUnavailable, stdout, stderr)
	}
	if !strings.Contains(stderr, "no such table: memories") {
		t.Errorf("stderr = %q, want the store's own error", stderr)
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("an unlistable store changed the vault")
	}
}

func TestHookPathSync_AMissingStoreIsExit4(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	enterHookPath(t)
	missing := filepath.Join(realTempDir(t), "memory.db")
	t.Setenv("LONGTERM_MEM_MEMORYCARD_DB", missing)
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)
	code, stdout, stderr := runHookSync(t)
	if code != exitStoreUnavailable {
		t.Errorf("exit %d, want %d (the hook logs failure:store-unavailable)\nstdout: %s\nstderr: %s", code, exitStoreUnavailable, stdout, stderr)
	}
	if !strings.Contains(stderr, missing) {
		t.Errorf("stderr = %q, want it to name the missing database %s", stderr, missing)
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("a missing store changed the vault")
	}
}
