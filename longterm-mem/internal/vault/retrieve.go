package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// DefaultTopN is the result count Retrieve requests when the caller does
// not supply an explicit override (R-004).
const DefaultTopN = 5

// retrieveScript is the vault-relative path to the retrieval entrypoint,
// always invoked with the vault root as the subprocess's working directory.
const retrieveScript = "scripts/retrieve.py"

// retrieveInterpreter is the interpreter Retrieve runs retrieveScript
// under (D8): the entrypoint carries no shebang and no exec bit, so it
// must never be exec'd directly.
const retrieveInterpreter = "python3"

// retrieveTimeout bounds a single Retrieve call (D8).
const retrieveTimeout = 30 * time.Second

// Candidate is one parsed row of the vault's retrieval entrypoint output,
// keeping only the five fields R-004 requires; the script's stdout JSON
// carries more fields (chunk_id, chunk_index, rerank_source, ...) that
// longterm-mem does not consume.
type Candidate struct {
	PageAddress  string  `json:"page_address"`
	AbsolutePath string  `json:"absolute_path"`
	BM25Score    float64 `json:"bm25_score"`
	RerankScore  float64 `json:"rerank_score"`
	Snippet      string  `json:"snippet"`
}

// Result is the parsed outcome of a Retrieve call.
type Result struct {
	Project    string
	Query      string
	Status     string // StatusOK or StatusNotProvisioned
	Candidates []Candidate
}

// retrieveOutput mirrors only the field of retrieve.py's stdout JSON that
// Result needs.
type retrieveOutput struct {
	Candidates []Candidate `json:"candidates"`
}

// Retrieve invokes the vault's retrieval entrypoint (scripts/retrieve.py)
// inside runner's vault root for query, requesting up to top results
// (DefaultTopN when top <= 0), and parses the page address, absolute path,
// BM25 score, rerank score, and snippet for every returned row (R-004).
//
// If the entrypoint exits with the vault's not-provisioned sentinel exit
// code, Retrieve returns a Result with Status == StatusNotProvisioned
// instead of an error (R-024). Any other non-zero exit, or a Runner-level
// failure, is reported as an error.
func Retrieve(ctx context.Context, runner *Runner, project, query string, top int) (Result, error) {
	if top <= 0 {
		top = DefaultTopN
	}

	ctx, cancel := context.WithTimeout(ctx, retrieveTimeout)
	defer cancel()

	stdout, stderr, exitCode, err := runner.RunInterpreted(ctx, retrieveInterpreter, retrieveScript, query, "--top", strconv.Itoa(top))
	if err != nil {
		return Result{}, fmt.Errorf("vault: invoke retrieve entrypoint: %w", err)
	}

	status, mapped := statusForExitCode(exitCode)
	if !mapped {
		return Result{}, fmt.Errorf("vault: retrieve entrypoint exited %d: %s", exitCode, string(stderr))
	}
	if status == StatusNotProvisioned {
		return Result{Project: project, Query: query, Status: StatusNotProvisioned}, nil
	}

	var parsed retrieveOutput
	if err := json.Unmarshal(stdout, &parsed); err != nil {
		return Result{}, fmt.Errorf("vault: parse retrieve entrypoint output: %w", err)
	}

	return Result{Project: project, Query: query, Status: StatusOK, Candidates: parsed.Candidates}, nil
}
