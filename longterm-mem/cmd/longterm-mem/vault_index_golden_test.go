package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func vaultIndexGoldenSteps() []retrievalStep {
	index := []string{"index", "--project", queryGoldenProject}
	touch := func(name string) func(t *testing.T, w *retrievalWorld) {
		return func(t *testing.T, w *retrievalWorld) { w.writeVaultFile(t, name, "") }
	}
	remove := func(name string) func(t *testing.T, w *retrievalWorld) {
		return func(t *testing.T, w *retrievalWorld) {
			if err := os.Remove(filepath.Join(w.vaultRoot, name)); err != nil {
				t.Fatalf("remove %s: %v", name, err)
			}
		}
	}
	return []retrievalStep{
		{name: "01-a-vault-never-indexed-is-provisioned-then-refreshed", args: index},
		{name: "02-a-provisioned-vault-is-only-refreshed", args: index},
		{name: "03-rebuild-provisions-again", args: append(append([]string{}, index...), "--rebuild")},
		{
			name: "04-an-interrupted-provision-is-provisioned-again",
			before: func(t *testing.T, w *retrievalWorld) {
				if err := os.Remove(filepath.Join(w.vaultRoot, ".vault-meta", ".longterm-mem-provisioned")); err != nil {
					t.Fatalf("remove the sentinel: %v", err)
				}
			},
			args: index,
		},
		{name: "05-a-provision-that-fails-is-not-a-success", before: touch("fail-setup"), args: append(append([]string{}, index...), "--rebuild")},
		{name: "06-a-refresh-that-fails-is-not-a-success", before: func(t *testing.T, w *retrievalWorld) {
			remove("fail-setup")(t, w)
			touch("fail-prefix")(t, w)
		}, args: index},
		{name: "07-the-second-refresh-that-fails-is-not-a-success", before: func(t *testing.T, w *retrievalWorld) {
			remove("fail-prefix")(t, w)
			touch("fail-bm25")(t, w)
		}, args: index},
		{name: "08-a-vault-without-its-refresh-script", before: func(t *testing.T, w *retrievalWorld) {
			remove("fail-bm25")(t, w)
			remove("scripts/bm25-index.py")(t, w)
		}, args: index},
		{name: "09-a-project-no-vault-is-named-for", before: nil, args: []string{"index", "--project", queryGoldenProject, "--vault", "/nonexistent/vault"}},
	}
}

// TestVaultIndexGolden walks one vault through `longterm-mem index`: provisioned and refreshed, refreshed alone,
// provisioned again on demand and after an interruption, and each of its steps failing. Each step's golden holds
// the command's output and what the vault's scripts did to the vault.
func TestVaultIndexGolden(t *testing.T) {
	requirePython3(t)
	w := newRetrievalWorld(t)
	writeIndexStubs(t, w)
	for _, step := range vaultIndexGoldenSteps() {
		if step.before != nil {
			step.before(t, w)
		}
		// The calls a step shows are its own.
		if err := os.Remove(filepath.Join(w.vaultRoot, ".calls")); err != nil && !os.IsNotExist(err) {
			t.Fatalf("clear the calls: %v", err)
		}
		code, stdout, stderr := w.run(t, step.args)
		got := w.normalise(fmt.Sprintf("args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s--- vault ---\n%s",
			strings.Join(step.args, " "), code, stdout, stderr, renderIndexState(t, w)))
		w.check(t, "index-"+step.name, got)
	}
}
