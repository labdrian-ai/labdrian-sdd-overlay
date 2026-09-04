package vault

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// retrieveOKFixture is a fake retrieve.py: a real Python entrypoint (no
// shebang, non-executable mode — D8 runs it via `python3`, never execs it
// directly). It records every argv element it received (one per line, in
// scripts/captured-args.txt relative to the vault root cwd) and prints
// canned JSON shaped like the real retrieve.py output (exploration #3121),
// carrying extra fields Result does not consume so parsing must select
// only the five R-004 names.
const retrieveOKFixture = `import json, sys
with open("captured-args.txt", "w") as f:
    f.write("\n".join(sys.argv[1:]))
    if sys.argv[1:]:
        f.write("\n")
print(json.dumps({"query": "placeholder", "strategy": "hybrid", "top_k": 2, "candidates": [
    {"chunk_id": "ch-1", "page_address": "c-000001", "page_path": "wiki/memory/c-000001.md", "absolute_path": "/vault/wiki/memory/c-000001.md", "chunk_index": 0, "bm25_score": 7.125, "rerank_score": 0.81, "rerank_source": "cross-encoder", "snippet": "first snippet"},
    {"chunk_id": "ch-2", "page_address": "c-000002", "page_path": "wiki/memory/c-000002.md", "absolute_path": "/vault/wiki/memory/c-000002.md", "chunk_index": 0, "bm25_score": 5.44, "rerank_score": 0.62, "rerank_source": "cross-encoder", "snippet": "second snippet"},
]}))
sys.exit(0)
`

// retrieveNotProvisionedFixture stands in for a never-indexed vault: exit
// 10 (the vault's not-provisioned sentinel), no output.
const retrieveNotProvisionedFixture = "import sys\nsys.exit(10)\n"

// wantCandidates is what retrieveOKFixture's JSON should parse to.
var wantCandidates = []Candidate{
	{PageAddress: "c-000001", AbsolutePath: "/vault/wiki/memory/c-000001.md", BM25Score: 7.125, RerankScore: 0.81, Snippet: "first snippet"},
	{PageAddress: "c-000002", AbsolutePath: "/vault/wiki/memory/c-000002.md", BM25Score: 5.44, RerankScore: 0.62, Snippet: "second snippet"},
}

// newFixtureVault creates a temp vault root with scripts/retrieve.py set to
// body, and returns the root and a Runner bound to it.
func newFixtureVault(t *testing.T, body string) (root string, runner *Runner) {
	t.Helper()
	root = t.TempDir()
	scriptsDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("create scripts dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "retrieve.py"), []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture retrieve.py: %v", err)
	}
	return root, &Runner{Root: root}
}

// capturedArgs reads back the argv the fixture script recorded.
func capturedArgs(t *testing.T, vaultRoot string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vaultRoot, "captured-args.txt"))
	if err != nil {
		t.Fatalf("read captured args: %v", err)
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func TestRetrieve_DefaultTopNAndFullFieldParse(t *testing.T) {
	vaultRoot, runner := newFixtureVault(t, retrieveOKFixture)

	result, err := Retrieve(context.Background(), runner, "labdrian-sdd-overlay", "what is dragonscale?", 0)
	if err != nil {
		t.Fatalf("Retrieve returned an unexpected error: %v", err)
	}

	if got, want := capturedArgs(t, vaultRoot), []string{"what is dragonscale?", "--top", "5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("expected argv %v (default top-N must be 5), got %v", want, got)
	}
	if result.Status != StatusOK {
		t.Fatalf("expected Status %q, got %q", StatusOK, result.Status)
	}
	if !reflect.DeepEqual(result.Candidates, wantCandidates) {
		t.Errorf("expected candidates %+v, got %+v", wantCandidates, result.Candidates)
	}
}

func TestRetrieve_ExplicitTopNOverride(t *testing.T) {
	vaultRoot, runner := newFixtureVault(t, retrieveOKFixture)

	if _, err := Retrieve(context.Background(), runner, "labdrian-sdd-overlay", "query text", 12); err != nil {
		t.Fatalf("Retrieve returned an unexpected error: %v", err)
	}

	if got, want := capturedArgs(t, vaultRoot), []string{"query text", "--top", "12"}; !reflect.DeepEqual(got, want) {
		t.Errorf("expected the explicit top-N to override the default: expected argv %v, got %v", want, got)
	}
}

func TestRetrieve_NotProvisionedExitTenMapsToStatus(t *testing.T) {
	_, runner := newFixtureVault(t, retrieveNotProvisionedFixture)

	result, err := Retrieve(context.Background(), runner, "labdrian-sdd-overlay", "anything", 0)
	if err != nil {
		t.Fatalf("expected exit 10 to map to a status, not an error, got: %v", err)
	}
	if result.Status != StatusNotProvisioned {
		t.Errorf("expected Status %q, got %q", StatusNotProvisioned, result.Status)
	}
	if len(result.Candidates) != 0 {
		t.Errorf("expected no candidates for a not-provisioned vault, got %d", len(result.Candidates))
	}
}
