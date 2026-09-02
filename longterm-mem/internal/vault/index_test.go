package vault

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupRetrieveOKFixture is a fixture bin/setup-retrieve.sh: a real shell
// script (shebang + exec bit — run directly via Runner.Run, never
// RunInterpreted) that records its invocation and materializes the two
// on-disk markers Provisioned checks (D12, exploration #3121).
const setupRetrieveOKFixture = `#!/bin/sh
printf 'setup %s\n' "$*" >> call-log.txt
mkdir -p .vault-meta/bm25 .vault-meta/chunks
printf '{}' > .vault-meta/bm25/index.json
: > .vault-meta/chunks/chunk-0.json
`

// contextualPrefixOKFixture and bm25IndexOKFixture are non-executable
// Python entrypoints (0644, no shebang — must run under python3 via
// Runner.RunInterpreted, never relying on an exec bit, matching
// retrieve.py's convention) that record their invocation and exit 0.
const contextualPrefixOKFixture = `import sys
with open("call-log.txt", "a") as f:
    f.write("contextual-prefix " + " ".join(sys.argv[1:]) + "\n")
sys.exit(0)
`

const bm25IndexOKFixture = `import sys
with open("call-log.txt", "a") as f:
    f.write("bm25-index " + " ".join(sys.argv[1:]) + "\n")
sys.exit(0)
`

// bm25IndexFailFixture stands in for a forced rebuild-step failure: it
// still records its invocation (proving it ran) before writing to stderr
// and exiting non-zero instead of succeeding.
const bm25IndexFailFixture = `import sys
with open("call-log.txt", "a") as f:
    f.write("bm25-index " + " ".join(sys.argv[1:]) + "\n")
sys.stderr.write("bm25-index: forced failure\n")
sys.exit(7)
`

// newIndexFixtureVault creates a temp vault root with all three rebuild
// scripts installed (setup/contextual-prefix fixed at their documented
// success behavior; bm25Body lets each test choose success or forced
// failure for the last step). provisioned pre-materializes Provisioned()'s
// two markers so the fixture starts out already indexed.
func newIndexFixtureVault(t *testing.T, bm25Body string, provisioned bool) (root string, runner *Runner) {
	t.Helper()
	root = t.TempDir()

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "setup-retrieve.sh"), []byte(setupRetrieveOKFixture), 0o755); err != nil {
		t.Fatalf("write fixture setup-retrieve.sh: %v", err)
	}

	scriptsDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("create scripts dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "contextual-prefix.py"), []byte(contextualPrefixOKFixture), 0o644); err != nil {
		t.Fatalf("write fixture contextual-prefix.py: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "bm25-index.py"), []byte(bm25Body), 0o644); err != nil {
		t.Fatalf("write fixture bm25-index.py: %v", err)
	}

	if provisioned {
		provisionMarkersOnly(t, root)
		sentinel := filepath.Join(root, provisionedSentinel)
		if err := os.WriteFile(sentinel, []byte("2024-01-01T00:00:00Z"), 0o644); err != nil {
			t.Fatalf("pre-provision completion sentinel: %v", err)
		}
	}

	return root, &Runner{Root: root}
}

// provisionMarkersOnly materializes the two on-disk markers Provisioned
// checks (bm25 index + a chunk) without the completion sentinel, modeling a
// provision step interrupted after those markers exist but before it
// finished (SIGKILL/OOM, disk-full, operator Ctrl-C).
func provisionMarkersOnly(t *testing.T, root string) {
	t.Helper()
	metaBM25 := filepath.Join(root, ".vault-meta", "bm25")
	metaChunks := filepath.Join(root, ".vault-meta", "chunks")
	if err := os.MkdirAll(metaBM25, 0o755); err != nil {
		t.Fatalf("pre-provision .vault-meta/bm25: %v", err)
	}
	if err := os.MkdirAll(metaChunks, 0o755); err != nil {
		t.Fatalf("pre-provision .vault-meta/chunks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaBM25, "index.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("pre-provision bm25 index.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaChunks, "existing-chunk.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("pre-provision an existing chunk: %v", err)
	}
}

// callLog reads back the ordered invocation log the fixture scripts wrote.
func callLog(t *testing.T, vaultRoot string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vaultRoot, "call-log.txt"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read call-log.txt: %v", err)
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func TestIndex_AlreadyProvisionedRefresh(t *testing.T) {
	vaultRoot, runner := newIndexFixtureVault(t, bm25IndexOKFixture, true)

	if err := Rebuild(context.Background(), runner, false); err != nil {
		t.Fatalf("Rebuild returned an unexpected error: %v", err)
	}

	log := callLog(t, vaultRoot)
	if len(log) != 2 {
		t.Fatalf("expected exactly the two refresh steps to run for an already-provisioned vault, got %v", log)
	}
	if !strings.HasPrefix(log[0], "contextual-prefix --all --no-llm") {
		t.Errorf("expected contextual-prefix.py to run first with --all --no-llm, got %q", log[0])
	}
	if !strings.HasPrefix(log[1], "bm25-index build") {
		t.Errorf("expected bm25-index.py to run second with build, got %q", log[1])
	}
}

func TestIndex_NeverIndexedIsProvisionedFirst(t *testing.T) {
	vaultRoot, runner := newIndexFixtureVault(t, bm25IndexOKFixture, false)

	if Provisioned(vaultRoot) {
		t.Fatal("fixture setup bug: vault must start out not provisioned")
	}

	if err := Rebuild(context.Background(), runner, false); err != nil {
		t.Fatalf("Rebuild returned an unexpected error: %v", err)
	}

	log := callLog(t, vaultRoot)
	if len(log) != 3 {
		t.Fatalf("expected provision-then-refresh (3 steps) for a never-indexed vault, got %v", log)
	}
	if !strings.HasPrefix(log[0], "setup --no-llm") {
		t.Errorf("expected setup-retrieve.sh to run first with --no-llm, got %q", log[0])
	}
	if !strings.HasPrefix(log[1], "contextual-prefix") || !strings.HasPrefix(log[2], "bm25-index") {
		t.Errorf("expected the refresh steps to follow provisioning, got %v", log)
	}

	if !Provisioned(vaultRoot) {
		t.Fatal("expected the vault to be queryable (provisioned) after Rebuild")
	}
}

func TestIndex_RebuildStepFailureReportsFailure(t *testing.T) {
	_, runner := newIndexFixtureVault(t, bm25IndexFailFixture, true)

	err := Rebuild(context.Background(), runner, false)

	if err == nil {
		t.Fatal("expected Rebuild to return an error when a rebuild step fails")
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "rebuilt") || strings.Contains(lower, "success") {
		t.Errorf("expected the failure error to never claim success, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "7") {
		t.Errorf("expected the failure error to name the forced exit code, got %q", err.Error())
	}
}

// TestIndex_InterruptedProvisionRetriggersSetup proves a provision step
// interrupted after its markers exist but before the completion sentinel is
// written (SIGKILL/OOM, disk-full, Ctrl-C) does not falsely read as
// provisioned: the next Rebuild must re-run setup-retrieve.sh rather than
// only refreshing a half-built vault.
func TestIndex_InterruptedProvisionRetriggersSetup(t *testing.T) {
	vaultRoot, runner := newIndexFixtureVault(t, bm25IndexOKFixture, false)
	provisionMarkersOnly(t, vaultRoot)

	if Provisioned(vaultRoot) {
		t.Fatal("fixture setup bug: markers without a completion sentinel must not read as provisioned")
	}

	if err := Rebuild(context.Background(), runner, false); err != nil {
		t.Fatalf("Rebuild returned an unexpected error: %v", err)
	}

	log := callLog(t, vaultRoot)
	if len(log) != 3 || !strings.HasPrefix(log[0], "setup --no-llm") {
		t.Fatalf("expected an interrupted provision to re-run setup-retrieve.sh first, got %v", log)
	}
}

// TestIndex_ForceRebuildReprovisionsAlreadyProvisionedVault proves force=true
// gives an operator a fix-forward path: it re-runs setup-retrieve.sh even on
// a fully (sentinel-marked) provisioned vault.
func TestIndex_ForceRebuildReprovisionsAlreadyProvisionedVault(t *testing.T) {
	vaultRoot, runner := newIndexFixtureVault(t, bm25IndexOKFixture, true)

	if err := Rebuild(context.Background(), runner, true); err != nil {
		t.Fatalf("Rebuild returned an unexpected error: %v", err)
	}

	log := callLog(t, vaultRoot)
	if len(log) != 3 || !strings.HasPrefix(log[0], "setup --no-llm") {
		t.Fatalf("expected force=true to re-run setup-retrieve.sh even on an already-provisioned vault, got %v", log)
	}
}
