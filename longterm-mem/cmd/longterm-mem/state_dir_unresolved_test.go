package main

import (
	"errors"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
)

// withNoStateDirectory takes away everything that names the directory this module keeps its derived state in:
// the override and HOME.
func withNoStateDirectory(t *testing.T) *retrievalWorld {
	t.Helper()
	w := newRetrievalWorld(t)
	t.Setenv(stateDirEnvVar, "")
	t.Setenv("HOME", "")
	return w
}

// `status` reports a state directory nobody can name as the same failure `index --embeddings` does (exit 8,
// exitPathUnresolvable: nothing was attempted, and the fix is the caller's environment), where it used to look
// for the index in the working directory and report "never". It prints nothing on stdout.
func TestStatusRefusesAStateDirectoryNobodyCanName(t *testing.T) {
	w := withNoStateDirectory(t)

	code, stdout, stderr := w.run(t, []string{"status", "--project", queryGoldenProject})

	const want = "longterm-mem: status: could not resolve the state directory; set HOME or LONGTERM_MEM_STATE_DIR\n"
	if code != exitPathUnresolvable || stdout != "" || stderr != want {
		t.Errorf("status = exit %d, stdout %q, stderr %q; want exit %d, nothing on stdout and %q", code, stdout, stderr, exitPathUnresolvable, want)
	}
}

// The same with --json: the refusal is on stderr, and no JSON is printed.
func TestStatusAsJSONRefusesAStateDirectoryNobodyCanName(t *testing.T) {
	w := withNoStateDirectory(t)

	code, stdout, _ := w.run(t, []string{"status", "--project", queryGoldenProject, "--json"})

	if code != exitPathUnresolvable || stdout != "" {
		t.Errorf("status --json = exit %d, stdout %q; want exit %d and nothing on stdout", code, stdout, exitPathUnresolvable)
	}
}

// What `status` and `doctor` read the index through, when there is no state directory, says so by what its
// error is (ops.StateDirUnresolved), and does not look anywhere else.
func TestTheIndexReaderWithoutAStateDirectoryAnswersThatItCouldNotBeResolved(t *testing.T) {
	withNoStateDirectory(t)
	reader := embeddingIndexReader("")

	_, builtAtErr := reader.EmbeddingIndexBuiltAt("p")
	_, idsErr := reader.IndexedEngramIDs("p")

	for name, err := range map[string]error{"built at": builtAtErr, "ids": idsErr} {
		var unresolved ops.StateDirUnresolved
		if !errors.As(err, &unresolved) {
			t.Errorf("%s: error = %v, want one the diagnostics find as ops.StateDirUnresolved", name, err)
		}
	}
}
