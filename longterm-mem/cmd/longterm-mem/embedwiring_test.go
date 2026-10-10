package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestMain(m *testing.M) {
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
	os.Exit(code)
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

// run is what the tests call to run a subcommand: the commands, wired to testEmbedClient. The program's own
// wiring, productionCommands, is built by main and by nothing a test runs.
func run(args []string) int {
	return commands{newEmbedClient: testEmbedClient}.run(args)
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
