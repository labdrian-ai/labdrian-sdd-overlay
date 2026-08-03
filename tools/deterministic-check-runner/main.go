package main

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable entry point. Check execution is not wired yet — this
// is the scaffold stub only.
func run(args []string, stdout, stderr io.Writer) int {
	return 0
}

// check describes one verification tool. deterministic and blocking are
// declared independently; classify() is the sole place they are combined.
// normalizeArgv, parse, and failed are deferred to later work units and are
// intentionally absent here to avoid rework.
type check struct {
	name          string
	deterministic bool
	blocking      bool
	checkArgv     []string
}

// registry is the hardcoded v1 check set. It is not configurable: gofmt,
// go vet, and staticcheck are deterministic and blocking; deadcode is
// deterministic but WARNING-only (amended R-016).
var registry = []check{
	{name: "gofmt", deterministic: true, blocking: true, checkArgv: []string{"gofmt", "-l", "."}},
	{name: "go vet", deterministic: true, blocking: true, checkArgv: []string{"go", "vet", "./..."}},
	{name: "staticcheck", deterministic: true, blocking: true, checkArgv: []string{"staticcheck", "./..."}},
	{name: "deadcode", deterministic: true, blocking: false, checkArgv: []string{"deadcode", "./..."}},
}

// classify is the single enforcement point for effective blocking: a check
// only blocks the run when it is both declared blocking AND deterministic.
// No other code in this module may compute effective blocking.
func classify(c check) bool {
	return c.blocking && c.deterministic
}

// Process exit codes returned by selectOutcome (D4).
const (
	outcomePassed                  = 0
	outcomeVerificationFailed      = 1
	outcomeProceduralToolingFailed = 3
)

// result is the outcome of executing one check. unavailable (could not run)
// and a plain failing exitCode (ran, found problems) are distinct states;
// runnerErr marks a runner-internal fault. Rows keep the raw exit code.
// count, top, and logPath land with the Phase-3 renderer.
type result struct {
	check       check
	exitCode    int
	unavailable bool
	runnerErr   bool
}

// selectOutcome is the single place outcome precedence lives (amended
// R-016): (1) an unexecutable blocking-set check or a runner error →
// procedural_tooling_failed, outranking everything else; (2) otherwise a
// blocking-set check that ran and failed → verification_failed; (3)
// otherwise → passed. A WARNING-only check (e.g. deadcode) can never alone
// reach gate 1, and never suppresses a real blocking failure or
// unavailability elsewhere.
func selectOutcome(results []result) int {
	for _, r := range results {
		if r.runnerErr {
			return outcomeProceduralToolingFailed
		}
	}
	for _, r := range results {
		if r.unavailable && classify(r.check) {
			return outcomeProceduralToolingFailed
		}
	}
	for _, r := range results {
		if !r.unavailable && !r.runnerErr && classify(r.check) && r.exitCode != 0 {
			return outcomeVerificationFailed
		}
	}
	return outcomePassed
}

// discoverModules walks root for go.mod files and returns their containing
// directories as absolute, sorted paths. root is normalized to absolute
// first, so a relative root resolves against the caller's cwd.
func discoverModules(root string) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	var modules []string
	walkErr := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		modules = append(modules, filepath.Dir(path))
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Strings(modules)
	return modules, nil
}
