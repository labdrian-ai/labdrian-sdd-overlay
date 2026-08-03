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
