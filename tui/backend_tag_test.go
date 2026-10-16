package main

// The backend tests of the tui (the `*_backend_test.go` files, which build the engine and run the overlay
// script) are behind the `backend` build tag, so that `go test ./...` is fast and needs no engine. The tag
// hides them from every command that does not pass it, so the two halves of the arrangement are held
// here, in a test that always runs: every backend test file carries the tag, and the TUI job of CI passes
// it to each step that reads test files. Either half alone would let the backend tests stop running in CI
// with nothing failing.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const backendTag = "//go:build backend"

// backendTestFiles is every *_backend_test.go file of the module, as paths relative to it.
func backendTestFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, "_backend_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the module: %v", err)
	}
	return files
}

func TestEveryBackendTestFileCarriesTheBackendTag(t *testing.T) {
	files := backendTestFiles(t)
	if len(files) == 0 {
		t.Fatal("no *_backend_test.go file found: the walk is not over the tui module")
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if first, _, _ := strings.Cut(string(data), "\n"); first != backendTag {
			t.Errorf("%s begins with %q, want %q: without the tag it runs in every `go test ./...`", path, first, backendTag)
		}
	}
}

// tuiJobSteps are the run lines of the steps of ci.yml whose working directory is tui.
func tuiJobSteps(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read the CI workflow: %v", err)
	}
	var runs []string
	inTui := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "- name:"):
			inTui = false
		case trimmed == "working-directory: tui":
			inTui = true
		case inTui && strings.HasPrefix(trimmed, "run:"):
			runs = append(runs, strings.TrimSpace(strings.TrimPrefix(trimmed, "run:")))
		}
	}
	return runs
}

// readsTestFiles matches the commands of the TUI job that build or analyse the test files.
var readsTestFiles = regexp.MustCompile(`^go (vet|test)\b|staticcheck@`)

func TestTheTuiJobOfCIPassesTheBackendTagToEveryStepThatReadsTestFiles(t *testing.T) {
	runs := tuiJobSteps(t)
	found := map[string]bool{}
	for _, run := range runs {
		m := readsTestFiles.FindStringSubmatch(run)
		if m == nil {
			continue
		}
		kind := m[1]
		if kind == "" {
			kind = "staticcheck"
		}
		found[kind] = true
		if !strings.Contains(run, " -tags backend ") {
			t.Errorf("the TUI job runs %q without -tags backend: the backend tests would not be %s in CI", run, map[string]string{"vet": "vetted", "test": "run", "staticcheck": "linted"}[kind])
		}
	}
	for _, kind := range []string{"vet", "staticcheck", "test"} {
		if !found[kind] {
			t.Errorf("the TUI job has no %s step (found %q): the check above held nothing for it", kind, runs)
		}
	}
}
