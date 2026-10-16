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
	"slices"
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

// tuiCommands is every command line run by a step of the workflow whose working directory is tui. The
// workflow is read as text, not as YAML (the module takes no YAML dependency for one test), under the
// layout GitHub workflows have: a job is a key indented by two spaces, a step begins with a line whose
// first token is "-", and a step's keys may come in any order. A run given as a block scalar (`run: |`
// or `run: >`) contributes each of its lines.
func tuiCommands(workflow string) []string {
	var commands []string
	var step []string // the commands of the current step
	inTui := false
	flush := func() {
		if inTui {
			commands = append(commands, step...)
		}
		step, inTui = nil, false
	}
	lines := strings.Split(workflow, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 2 && trimmed != "" || strings.HasPrefix(trimmed, "- ") {
			flush()
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			indent = len(line) - len(strings.TrimLeft(strings.TrimPrefix(strings.TrimLeft(line, " "), "- "), " "))
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "working-directory":
			inTui = inTui || value == "tui"
		case "run":
			if !strings.HasPrefix(value, "|") && !strings.HasPrefix(value, ">") {
				step = append(step, value)
				continue
			}
			for i+1 < len(lines) {
				next := lines[i+1]
				if strings.TrimSpace(next) != "" && len(next)-len(strings.TrimLeft(next, " ")) <= indent {
					break
				}
				i++
				if c := strings.TrimSpace(next); c != "" {
					step = append(step, c)
				}
			}
		}
	}
	flush()
	return commands
}

// testFileReader is the kind of a command that builds or analyses test files: "vet", "test" or
// "staticcheck", and "" for any other command.
func testFileReader(command string) string {
	fields := strings.Fields(command)
	switch {
	case len(fields) >= 2 && fields[0] == "go" && (fields[1] == "vet" || fields[1] == "test"):
		return fields[1]
	case slices.ContainsFunc(fields, func(f string) bool { return strings.Contains(f, "staticcheck@") }):
		return "staticcheck"
	}
	return ""
}

// tagsFlag matches the build-tags flag in either form go and staticcheck accept: "-tags list" (the list
// is the next field) or "-tags=list", with one or two dashes.
var tagsFlag = regexp.MustCompile(`^--?tags(=(.*))?$`)

// passesBackendTag reports whether the command passes the backend build tag, in any form of the flag and
// at any place in its comma-separated list.
func passesBackendTag(command string) bool {
	fields := strings.Fields(command)
	for i, f := range fields {
		m := tagsFlag.FindStringSubmatch(f)
		if m == nil {
			continue
		}
		list := m[2]
		if m[1] == "" && i+1 < len(fields) {
			list = fields[i+1]
		}
		if slices.Contains(strings.Split(list, ","), "backend") {
			return true
		}
	}
	return false
}

func TestTheTuiJobOfCIPassesTheBackendTagToEveryStepThatReadsTestFiles(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read the CI workflow: %v", err)
	}
	commands := tuiCommands(string(data))
	found := map[string]bool{}
	for _, command := range commands {
		kind := testFileReader(command)
		if kind == "" {
			continue
		}
		found[kind] = true
		if !passesBackendTag(command) {
			t.Errorf("the TUI job runs %q without the backend tag: CI would skip the backend tests in this %s step", command, kind)
		}
	}
	for _, kind := range []string{"vet", "staticcheck", "test"} {
		if !found[kind] {
			t.Errorf("the TUI job has no %s step (its commands: %q): the check above held nothing for it", kind, commands)
		}
	}
}

func TestTuiCommandsReadsEveryLayoutOfAStep(t *testing.T) {
	workflow := strings.Join([]string{
		"jobs:",
		"  test-tui:",
		"    steps:",
		"      - name: Single line",
		"        working-directory: tui",
		"        run: go vet ./...",
		"      - name: Run before directory",
		"        run: go test ./...",
		"        working-directory: tui",
		"      - name: Block scalar",
		"        working-directory: tui",
		"        run: |",
		"          echo one",
		"",
		"          go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...",
		"        continue-on-error: true",
		"      - name: Other directory",
		"        working-directory: engine",
		"        run: go test ./...",
		"  other-job:",
		"    working-directory: tui",
		"    steps:",
		"      - run: go test ./other",
	}, "\n")
	got := tuiCommands(workflow)
	want := []string{"go vet ./...", "go test ./...", "echo one", "go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./..."}
	if !slices.Equal(got, want) {
		t.Errorf("tuiCommands = %q, want %q", got, want)
	}
}

func TestPassesBackendTagAcceptsEveryFormOfTheFlag(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    bool
	}{
		{"go test -tags backend ./...", true},
		{"go test ./... -tags backend", true},
		{"go test -tags=backend ./...", true},
		{"go test --tags=other,backend ./...", true},
		{"go test -tags backend,other", true},
		{"go test ./...", false},
		{"go test -tags other ./...", false},
		{"go test -tags=backends ./...", false},
		{"go test -tags", false},
	} {
		if got := passesBackendTag(tc.command); got != tc.want {
			t.Errorf("passesBackendTag(%q) = %v, want %v", tc.command, got, tc.want)
		}
	}
}

func TestTestFileReaderKnowsTheCommandsThatReadTestFiles(t *testing.T) {
	for command, want := range map[string]string{
		"go vet -tags backend ./...":                             "vet",
		"go test ./... -cover":                                   "test",
		"go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...": "staticcheck",
		"go run golang.org/x/tools/cmd/deadcode@v0.48.0 ./...":   "",
		`test -z "$(gofmt -l .)"`:                                "",
	} {
		if got := testFileReader(command); got != want {
			t.Errorf("testFileReader(%q) = %q, want %q", command, got, want)
		}
	}
}
