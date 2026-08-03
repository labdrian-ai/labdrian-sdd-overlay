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
