package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// testEmbedBackend is the embedding backend the tests of this package talk to: a server of the test binary's
// own, on a loopback port it was given, that answers every embedding request with a fixed vector. Nothing in
// the test binary is wired to embed.DefaultEndpoint, the address of the embedding server of whoever runs
// the tests (a developer's Ollama): TestMain builds it before the first test and the commands the tests run
// are given it by testEmbedClient.
var testEmbedBackend *httptest.Server

// testEmbedRequests counts the requests testEmbedBackend received.
var testEmbedRequests atomic.Int64

// testHomePrefix names the home directory the tests of this package run in.
const testHomePrefix = "longterm-mem-test-home-"

func TestMain(m *testing.M) {
	// No test runs in the home of whoever runs the tests: a command that is not told where Engram's database is
	// looks for it under the home directory, and so does the vault registry, so a test that forgets to name them
	// would read the machine's own. The tests that need a home of their own set it again (t.Setenv).
	home, err := os.MkdirTemp("", testHomePrefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create the home of the tests: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("HOME", home); err != nil {
		fmt.Fprintf(os.Stderr, "set the home of the tests: %v\n", err)
		os.Exit(1)
	}
	testEmbedBackend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testEmbedRequests.Add(1)
		vector := make([]float64, vecindex.DefaultDimension)
		for i := range vector {
			vector[i] = 0.5
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": vector})
	}))
	code := m.Run()
	testEmbedBackend.Close()
	os.RemoveAll(home)
	os.Exit(code)
}

// The tests run in a home directory made for them, under the temporary directory, whatever home the process was
// started with: the two stores a command consults when it is told nothing (Engram's database and the vault
// registry) are looked for there, so no test reads the machine's own.
func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(home, os.TempDir()) || !strings.HasPrefix(filepath.Base(home), testHomePrefix) {
		t.Errorf("the tests run in %q, want a directory named %s* under %s", home, testHomePrefix, os.TempDir())
	}
	if _, err := os.Stat(filepath.Join(home, ".engram")); !os.IsNotExist(err) {
		t.Errorf("the home of the tests holds an Engram directory (%v): it is not one made for them", err)
	}
}

// testEmbedClient is the embedding client factory of the commands the tests run: embed.NewClient, except that
// a client whose configuration names no endpoint talks to testEmbedBackend instead of embed.DefaultEndpoint.
// One that names an endpoint (a test's own server, or a remote one the test expects to be refused) is built as
// it was asked.
func testEmbedClient(cfg embed.Config) (*embed.Client, error) {
	if cfg.Endpoint == "" {
		cfg.Endpoint = testEmbedBackend.URL
	}
	return embed.NewClient(cfg)
}

// run is what the tests call to run a subcommand: the commands, wired to testEmbedClient and to a clock
// fixed at fixtureInstant, so that no test depends on the day it runs. A test that needs another instant
// builds its commands with a fixedClock of its own (promoteGoldenClock, for the golden files); the program's
// own wiring, productionCommands, which holds the wall clock, is built by main and by nothing a test runs.
func run(args []string) int {
	return commands{newEmbedClient: testEmbedClient, clock: fixedClock{at: fixtureInstant}}.run(args)
}

// A doctor run by a test probes the embedding backend the test binary gave it, not the default endpoint:
// the probe is the one request `doctor` makes to the network, and it reached the backend of whoever ran the
// tests until the commands were handed their client.
func TestDoctorProbesTheTestEmbeddingBackend(t *testing.T) {
	vaultRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)
	before := testEmbedRequests.Load()

	runQuietly(t, []string{"doctor", "--project", "embed-wiring-project"})

	if got := testEmbedRequests.Load() - before; got != 1 {
		t.Errorf("the doctor probed the test embedding backend %d times, want once", got)
	}
}
