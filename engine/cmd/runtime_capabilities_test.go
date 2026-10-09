package main

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

type capabilitiesRun struct {
	code   int
	stdout string
	stderr string
}

// runCapabilitiesTest drives the real runtime entry point with the
// "capabilities" action. Like the other cores it injects a non-terminating
// exit, so the first exit code wins and a run that never calls exit reports
// 0.
func runCapabilitiesTest(args ...string) capabilitiesRun {
	var out, errBuf bytes.Buffer
	code := -1
	exited := false
	runRuntimeCore(noPi(), noGit(), append([]string{"capabilities"}, args...), &out, &errBuf, func(c int) {
		if !exited {
			code = c
			exited = true
		}
	})
	if !exited {
		code = 0
	}
	return capabilitiesRun{code: code, stdout: out.String(), stderr: errBuf.String()}
}

func decodeCapabilitiesReport(t *testing.T, r capabilitiesRun) capability.Report {
	t.Helper()
	if r.code != 0 || r.stderr != "" {
		t.Fatalf("code=%d stderr=%q, want exit 0 and no stderr", r.code, r.stderr)
	}
	var report capability.Report
	if err := json.Unmarshal([]byte(r.stdout), &report); err != nil {
		t.Fatalf("stdout is not a capability report: %v\n%s", err, r.stdout)
	}
	return report
}

func TestRuntimeCapabilitiesDefaultsToAllFourTargetsInOrder(t *testing.T) {
	report := decodeCapabilitiesReport(t, runCapabilitiesTest())
	if report.Version != capability.ReportVersion {
		t.Errorf("version = %d, want %d", report.Version, capability.ReportVersion)
	}
	var targets []string
	for _, d := range report.Declarations {
		targets = append(targets, d.Target)
	}
	if want := []string{"claude", "codex", "pi", "opencode"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %v, want %v", targets, want)
	}
}

func TestRuntimeCapabilitiesTargetAllIsTheDefault(t *testing.T) {
	implicit := runCapabilitiesTest()
	explicit := runCapabilitiesTest("--target", "all")
	if implicit.code != 0 || explicit.code != 0 {
		t.Fatalf("codes = %d and %d, want 0", implicit.code, explicit.code)
	}
	if implicit.stdout != explicit.stdout {
		t.Fatalf("--target all differs from the default:\n%s\nvs\n%s", explicit.stdout, implicit.stdout)
	}
}

func TestRuntimeCapabilitiesSingleTargetPrintsOneDeclaration(t *testing.T) {
	for _, target := range capability.Targets() {
		t.Run(target, func(t *testing.T) {
			report := decodeCapabilitiesReport(t, runCapabilitiesTest("--target", target))
			if len(report.Declarations) != 1 {
				t.Fatalf("got %d declarations, want 1", len(report.Declarations))
			}
			want, err := capability.Declare(target)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(report.Declarations[0], want) {
				t.Fatalf("printed declaration differs from capability.Declare(%q):\n got %+v\nwant %+v", target, report.Declarations[0], want)
			}
		})
	}
}

func TestRuntimeCapabilitiesPrintsExactlyTheReportOfTheDeclarations(t *testing.T) {
	r := runCapabilitiesTest()
	want, err := capability.Report{Version: capability.ReportVersion, Declarations: capability.All()}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if r.code != 0 || r.stdout != string(want) {
		t.Fatalf("code=%d; stdout is not the marshaled report of capability.All():\n%s", r.code, r.stdout)
	}
	if !strings.HasSuffix(r.stdout, "}\n") {
		t.Errorf("stdout must end with one newline, ends %q", r.stdout[len(r.stdout)-3:])
	}
}

func TestRuntimeCapabilitiesMarksOnlyOpenCodeUntested(t *testing.T) {
	r := runCapabilitiesTest()
	var generic struct {
		Declarations []map[string]any `json:"declarations"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &generic); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, r.stdout)
	}
	if len(generic.Declarations) != 4 {
		t.Fatalf("got %d declarations, want 4", len(generic.Declarations))
	}
	for _, d := range generic.Declarations {
		target, _ := d["target"].(string)
		untested, present := d["untested"]
		switch {
		case target == "opencode" && (!present || untested == ""):
			t.Errorf("opencode must carry a non-empty untested reason, got %v", untested)
		case target != "opencode" && present:
			t.Errorf("%s must not carry an untested key, got %v", target, untested)
		}
	}
	if strings.Contains(r.stdout, "null") {
		t.Errorf("stdout must never contain null:\n%s", r.stdout)
	}
}

func TestRuntimeCapabilitiesTargetIsTrimmedAndLastValueWins(t *testing.T) {
	trimmed := decodeCapabilitiesReport(t, runCapabilitiesTest("--target", " codex "))
	if len(trimmed.Declarations) != 1 || trimmed.Declarations[0].Target != "codex" {
		t.Errorf("a padded --target should select codex, got %+v", trimmed.Declarations)
	}
	last := decodeCapabilitiesReport(t, runCapabilitiesTest("--target", "claude", "--target", "pi"))
	if len(last.Declarations) != 1 || last.Declarations[0].Target != "pi" {
		t.Errorf("the last --target should win, got %+v", last.Declarations)
	}
}

func TestRuntimeCapabilitiesRefusals(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{"an unknown target is refused", []string{"--target", "cursor"}, 2, `unknown target "cursor" (expected claude, codex, pi, opencode, or all)`},
		{"targets are case sensitive", []string{"--target", "Claude"}, 2, `unknown target "Claude"`},
		{"an empty target is refused", []string{"--target", ""}, 2, `unknown target ""`},
		{"an unknown flag is refused", []string{"--bogus"}, 1, `unknown flag "--bogus"`},
		{"the equals form is an unknown flag", []string{"--target=claude"}, 1, `unknown flag "--target=claude"`},
		{"a flag that belongs to other actions is an unknown flag", []string{"--config-root", "/tmp/x"}, 1, `unknown flag "--config-root"`},
		{"a flag after a good target is still refused", []string{"--target", "pi", "--bogus"}, 1, `unknown flag "--bogus"`},
		{"--target without a value is a usage error", []string{"--target"}, 1, "--target requires a value"},
		{"a positional argument is a usage error", []string{"claude"}, 1, `unexpected argument "claude"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runCapabilitiesTest(tt.args...)
			if r.code != tt.wantCode {
				t.Errorf("code = %d, want %d\nstderr=%q", r.code, tt.wantCode, r.stderr)
			}
			if !strings.Contains(r.stderr, "runtime capabilities") || !strings.Contains(r.stderr, tt.wantStderr) {
				t.Errorf("stderr = %q, want it to name %q", r.stderr, tt.wantStderr)
			}
			if r.stdout != "" {
				t.Errorf("a refused run must print nothing on stdout, got %q", r.stdout)
			}
		})
	}
}

// TestRuntimeCapabilitiesReportsFailedStdoutWrite covers the branch where the
// report itself cannot be written (a closed pipe, a full disk). The action must
// name the failed write, surface its cause, and exit 1 exactly once: a missing
// return after the exit would let the run fall through to exit 0, which the
// first-code-wins helpers used elsewhere in this file would hide, so this test
// records every call. The failing writer is the one the memory plan tests use.
func TestRuntimeCapabilitiesReportsFailedStdoutWrite(t *testing.T) {
	var errBuf bytes.Buffer
	var codes []int
	runRuntimeCore(noPi(), noGit(), []string{"capabilities"}, failingMemoryWriter{}, &errBuf, func(c int) {
		codes = append(codes, c)
	})
	if !reflect.DeepEqual(codes, []int{1}) {
		t.Errorf("exit calls = %v, want exactly [1]", codes)
	}
	for _, want := range []string{"runtime capabilities", "writing report", "broken pipe"} {
		if !strings.Contains(errBuf.String(), want) {
			t.Errorf("stderr = %q, want it to contain %q", errBuf.String(), want)
		}
	}
}

// captureUsage returns what usage() prints. usage writes straight to
// os.Stderr, so the test swaps it for a pipe and drains that pipe
// concurrently: the help text is several kilobytes, more than some platforms'
// pipe buffers hold if it were only read afterwards.
func captureUsage(t *testing.T) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() = %v", err)
	}
	defer r.Close()
	drained := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		drained <- string(data)
	}()
	saved := os.Stderr
	os.Stderr = w
	func() {
		defer func() { os.Stderr = saved }()
		usage()
	}()
	w.Close()
	return <-drained
}

// TestUsageDocumentsCapabilitiesExitCodes pins the help line for the exit
// codes of 'runtime capabilities' to what TestRuntimeCapabilitiesRefusals pins
// the action to do: an unknown flag is a usage error (exit 1), like it is for
// 'runtime status' and the other runtime actions, and only an unknown --target
// value exits 2.
func TestUsageDocumentsCapabilitiesExitCodes(t *testing.T) {
	const want = "exit 0 success, 2 unknown --target value, 1 usage error including an unknown flag"
	var line string
	for _, l := range strings.Split(captureUsage(t), "\n") {
		if strings.Contains(l, "reads no configuration, HOME, or file") {
			line = l
		}
	}
	if line == "" {
		t.Fatal("usage has no line saying what 'runtime capabilities' reads; the test is not looking at the right text")
	}
	if !strings.Contains(line, want) {
		t.Errorf("usage line = %q\nwant it to document the exit codes as %q", line, want)
	}
}

// TestRuntimeCapabilitiesCreatesNothingEvenWhenEveryRootIsSet runs the action
// with every configuration root the lifecycle actions read pointed at one
// empty directory. It is strictly read-only and declarative, so nothing may
// appear there.
func TestRuntimeCapabilitiesCreatesNothingEvenWhenEveryRootIsSet(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "CODEX_HOME", "STATE_DIR", "OVERLAY_DIR", "LABDRIAN_OVERLAY_DIR"} {
		t.Setenv(name, home)
	}
	if r := runCapabilitiesTest(); r.code != 0 {
		t.Fatalf("code=%d stderr=%q", r.code, r.stderr)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("the action created %d entries under the roots it must not touch, first: %s", len(entries), entries[0].Name())
	}
}

// TestRuntimeCapabilitiesSourceImportsNoFilesystemOrAdapters pins the
// read-only contract statically: the file imports only fmt, io, strings, and
// engine/capability, so it cannot open a file, read the environment, start a
// process, or construct a runtime adapter. Widen the list only after
// reviewer approval.
func TestRuntimeCapabilitiesSourceImportsNoFilesystemOrAdapters(t *testing.T) {
	allowed := map[string]bool{
		"fmt":     true,
		"io":      true,
		"strings": true,
		"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability": true,
	}
	file, err := parser.ParseFile(token.NewFileSet(), "runtime_capabilities.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse runtime_capabilities.go: %v", err)
	}
	if len(file.Imports) == 0 {
		t.Fatal("runtime_capabilities.go declares no imports; the check is not looking at the right file")
	}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if !allowed[path] {
			t.Errorf("runtime_capabilities.go imports %q; the capabilities action must stay read-only and adapter-free", path)
		}
	}
}

// TestRuntimeActionErrorNamesCapabilities pins that the action list in the
// missing-action error includes the new action.
func TestRuntimeActionErrorNamesCapabilities(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := -1
	runRuntimeCore(noPi(), noGit(), []string{"--target", "claude"}, &out, &errBuf, func(c int) {
		if code == -1 {
			code = c
		}
	})
	if code != 1 || !strings.Contains(errBuf.String(), "capabilities") {
		t.Fatalf("code=%d stderr=%q, want exit 1 with an action list that names capabilities", code, errBuf.String())
	}
}
