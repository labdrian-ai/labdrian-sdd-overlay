package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// The golden files under testdata/retrieval-golden pin what the query surfaces and the index commands do
// where the query reaches outside the memory: the vault's retrieval script, the embedding index and the
// embedding backend, and the vault's index scripts. They were recorded from the program as it stood before
// the query package took those as ports it owns (Phase 9, L4), and that change must not alter one byte of
// them. The CLI cases run the command in process; the MCP cases run `longterm-mem mcp` in process over pipes
// standing for stdin and stdout, one session per case, so the wire format of the tools is pinned as it is
// sent, not as a Go value.
//
// Nothing here reads the owner's HOME, vault registry, Engram database or embedding server: the world of a
// case is its own temporary directories, an Engram database built from the schema fixture, vault scripts the
// test writes, and an embedding backend of the test binary's own. What the program writes from the wall clock
// is masked by where it is, as the promote goldens mask the sentinel: the instant an embedding index was built
// at and the digest that covers it.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run 'TestEmbeddingIndexGolden|TestMCPSessionGolden|TestVaultIndexGolden' -update-retrieval-golden
//
// and read the diff before committing it.
var updateRetrievalGolden = flag.Bool("update-retrieval-golden", false, "rewrite the golden files of the retrieval, embedding index and vault index cases")

// embeddingBackendMode is what the fake embedding backend answers.
type embeddingBackendMode int32

const (
	backendAnswers embeddingBackendMode = iota
	// backendFails answers every request with a 500.
	backendFails
	// backendLacksTheModel answers every request with a 404, which is how a model that is not pulled is told.
	backendLacksTheModel
)

// embeddingBackend is an embedding server of the test's own, on a loopback port. It embeds a text into three
// numbers by the word it contains, so that a query and the observation it should find point the same way.
type embeddingBackend struct {
	server   *httptest.Server
	mode     atomic.Int32
	requests atomic.Int64
}

func newEmbeddingBackend(t *testing.T) *embeddingBackend {
	t.Helper()
	b := &embeddingBackend{}
	b.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.requests.Add(1)
		switch embeddingBackendMode(b.mode.Load()) {
		case backendFails:
			http.Error(w, "backend broke", http.StatusInternalServerError)
			return
		case backendLacksTheModel:
			http.Error(w, "model not found", http.StatusNotFound)
			return
		}
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": embeddingOf(req.Prompt)})
	}))
	t.Cleanup(b.server.Close)
	return b
}

// embeddingOf points a text at the word it is about: dragonscale along the first axis, cache along the second,
// register along the third, and a little of each for any other text.
func embeddingOf(text string) []float64 {
	switch lower := strings.ToLower(text); {
	case strings.Contains(lower, "dragonscale"):
		return []float64{1, 0, 0}
	case strings.Contains(lower, "cache"):
		return []float64{0, 1, 0}
	case strings.Contains(lower, "register"):
		return []float64{0, 0, 1}
	default:
		return []float64{0.25, 0.25, 0.25}
	}
}

// retrievalWorld is the world one case runs in: its own HOME, vault, state directory and Engram database, and
// the commands wired to the embedding backend of the test.
type retrievalWorld struct {
	home      string
	vaultRoot string
	stateDir  string
	dbPath    string
	backend   *embeddingBackend
	commands  commands
	// goldenDir is where the golden files are, resolved before the case changes its working directory.
	goldenDir string
}

// newRetrievalWorld builds the world and enters it: the environment and the working directory are the case's
// own and are restored with it. The vault holds no script yet; a case writes the ones it needs.
func newRetrievalWorld(t *testing.T) *retrievalWorld {
	t.Helper()
	goldenDir, err := filepath.Abs(filepath.Join("testdata", "retrieval-golden"))
	if err != nil {
		t.Fatalf("resolve the golden directory: %v", err)
	}
	w := &retrievalWorld{
		goldenDir: goldenDir,
		home:      realTempDir(t),
		vaultRoot: realTempDir(t),
		stateDir:  realTempDir(t),
		dbPath:    queryGoldenDatabase(t),
		backend:   newEmbeddingBackend(t),
	}
	backend := w.backend
	w.commands = commands{
		newEmbedClient: func(cfg embed.Config) (*embed.Client, error) {
			if cfg.Endpoint == "" {
				cfg.Endpoint = backend.server.URL
			}
			return embed.NewClient(cfg)
		},
		clock: fixedClock{at: fixtureInstant},
	}
	t.Setenv("HOME", w.home)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", w.dbPath)
	t.Setenv("LONGTERM_MEM_VAULT", w.vaultRoot)
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
	t.Setenv("LONGTERM_MEM_STATE_DIR", w.stateDir)
	// A directory that is not a repository, so no warning about the working directory is part of the output.
	t.Chdir(t.TempDir())
	return w
}

func (w *retrievalWorld) run(t *testing.T, args []string) (code int, stdout, stderr string) {
	t.Helper()
	stdout, stderr = captureStreams(t, func() { code = w.commands.run(args) })
	return code, stdout, stderr
}

// writeVaultFile writes a file of the world's vault, executable, creating its directory.
func (w *retrievalWorld) writeVaultFile(t *testing.T, rel, body string) {
	t.Helper()
	path := filepath.Join(w.vaultRoot, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

var (
	// builtAtInOutput is the instant an embedding index says it was built at, as the manifest and the query
	// result print it, plain or inside a JSON string; the literal "never" is not an instant and stays.
	builtAtInOutput = regexp.MustCompile(`(\\?"built_at\\?": ?\\?")\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`)
	// revisionInOutput is the digest of a manifest, which covers the instant it was built at.
	revisionInOutput = regexp.MustCompile(`("revision": ")[0-9a-f]{64}`)
)

// normalise replaces what differs from one run to the next: the temporary paths, the instant an embedding index
// was built at and the digest that covers it, and the instant of the vault's provisioned sentinel.
func (w *retrievalWorld) normalise(text string) string {
	text = strings.ReplaceAll(text, w.vaultRoot, "<vault>")
	text = strings.ReplaceAll(text, w.home, "<home>")
	text = strings.ReplaceAll(text, w.dbPath, "<engram-db>")
	text = strings.ReplaceAll(text, w.stateDir, "<state>")
	text = builtAtInOutput.ReplaceAllString(text, "${1}<instant>")
	text = revisionInOutput.ReplaceAllString(text, "${1}<digest>")
	return promoteGoldenSentinel.ReplaceAllString(text, "${1}<now>")
}

// retrievalStep is one command of a case, with what is done to the world before it runs.
type retrievalStep struct {
	name   string
	before func(t *testing.T, w *retrievalWorld)
	args   []string
}

// check compares got with the golden file of the case, or rewrites it when the update flag is given.
func (w *retrievalWorld) check(t *testing.T, name, got string) {
	t.Helper()
	dir := w.goldenDir
	path := filepath.Join(dir, name+".golden")
	if *updateRetrievalGolden {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create the golden directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (record it with -update-retrieval-golden)", err)
	}
	if string(want) != got {
		t.Errorf("the case %q changed from its golden file %s:\n--- want ---\n%s\n--- got ---\n%s", name, path, want, got)
	}
}

// requirePython3 skips a case that runs the vault's scripts when python3 is not on PATH.
func requirePython3(t *testing.T) {
	t.Helper()
	if !vault.PrerequisitePresent("python3") {
		t.Skip("python3 is not on PATH: the vault's scripts run under it")
	}
}

// writeIndexStubs writes the vault's index scripts as stubs that note every call in .calls and refuse when a
// file named fail-<step> is in the vault: setup creates what makes a vault provisioned, the other two only note.
func writeIndexStubs(t *testing.T, w *retrievalWorld) {
	t.Helper()
	w.writeVaultFile(t, "bin/setup-retrieve.sh", `#!/bin/sh
echo "setup $*" >> .calls
if [ -f fail-setup ]; then
  echo "setup refused" >&2
  exit 3
fi
mkdir -p .vault-meta/bm25 .vault-meta/chunks
echo '{}' > .vault-meta/bm25/index.json
echo page > .vault-meta/chunks/a
exit 0
`)
	for _, step := range []struct{ file, name, arg string }{
		{"scripts/contextual-prefix.py", "prefix", "fail-prefix"},
		{"scripts/bm25-index.py", "bm25", "fail-bm25"},
	} {
		w.writeVaultFile(t, step.file, strings.NewReplacer("NAME", step.name, "MARKER", step.arg).Replace(`import os, sys
open(".calls", "a").write("NAME " + " ".join(sys.argv[1:]) + "\n")
if os.path.exists("MARKER"):
    sys.stderr.write("NAME refused\n")
    sys.exit(4)
`))
	}
}

// renderIndexState says what the index scripts did to the vault: the calls they noted, and which files the
// vault holds of a provisioned one. The instant of the sentinel is not part of it.
func renderIndexState(t *testing.T, w *retrievalWorld) string {
	t.Helper()
	var b strings.Builder
	calls, err := os.ReadFile(filepath.Join(w.vaultRoot, ".calls"))
	switch {
	case err == nil:
		fmt.Fprintf(&b, "calls:\n%s", calls)
	case os.IsNotExist(err):
		b.WriteString("calls: none\n")
	default:
		t.Fatalf("read the calls: %v", err)
	}
	for _, rel := range []string{".vault-meta/.longterm-mem-provisioned", ".vault-meta/bm25/index.json", ".vault-meta/chunks/a", "wiki/memory/c-000901.md", "wiki/index.md", "wiki/log.md"} {
		if _, err := os.Stat(filepath.Join(w.vaultRoot, rel)); err == nil {
			fmt.Fprintf(&b, "present: %s\n", rel)
		} else {
			fmt.Fprintf(&b, "absent: %s\n", rel)
		}
	}
	return b.String()
}
