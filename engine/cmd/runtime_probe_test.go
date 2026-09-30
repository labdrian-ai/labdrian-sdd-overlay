package main

// Tests for 'runtime probe'. Every file the probe is pointed at is a fixture
// under t.TempDir(); no test names the real home directory. The action is the
// presence prober behind a command: it stats, prints JSON, and opens nothing.

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

type probeRun struct {
	code   int
	stdout string
	stderr string
}

// runProbeTest drives the probe core with an injected home and PATH. Like the
// other cores it injects a non-terminating exit, so the first exit code wins.
func runProbeTest(home, path string, args ...string) probeRun {
	var out, errBuf bytes.Buffer
	code := -1
	exited := false
	runRuntimeProbe(args, home, path, &out, &errBuf, func(c int) {
		if !exited {
			code = c
			exited = true
		}
	})
	if !exited {
		code = 0
	}
	return probeRun{code: code, stdout: out.String(), stderr: errBuf.String()}
}

type probeReportJSON struct {
	Version      int                    `json:"version"`
	Observations []workflow.Observation `json:"observations"`
}

func decodeProbe(t *testing.T, r probeRun) probeReportJSON {
	t.Helper()
	if r.code != 0 || r.stderr != "" {
		t.Fatalf("code=%d stderr=%q, want exit 0 and no stderr", r.code, r.stderr)
	}
	var rep probeReportJSON
	dec := json.NewDecoder(strings.NewReader(r.stdout))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rep); err != nil {
		t.Fatalf("stdout is not a probe report: %v\n%s", err, r.stdout)
	}
	return rep
}

func names(obs []workflow.Observation) []string {
	out := make([]string, len(obs))
	for i, o := range obs {
		out[i] = o.Capability
	}
	return out
}

const probeSecret = "PROBE-SECRET-CONTENT"

// probeFixture returns a home holding every signal and a PATH holding gentle-ai.
func probeFixture(t *testing.T) (home, path string) {
	t.Helper()
	home = t.TempDir()
	for _, rel := range [][]string{
		{".engram", "engram.db"},
		{".labdrian-overlay", "longterm-mem-registration.json"},
		{".claude", ".credentials.json"},
		{".codex", "auth.json"},
		{".pi", "agent", "auth.json"},
	} {
		p := filepath.Join(append([]string{home}, rel...)...)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(probeSecret), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	name := "gentle-ai"
	if runtime.GOOS == "windows" {
		name = "gentle-ai.exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte(probeSecret), 0o755); err != nil {
		t.Fatal(err)
	}
	return home, bin
}

func TestRuntimeProbeDefaultsToAllTargetsAndReportsPresenceOnly(t *testing.T) {
	home, path := probeFixture(t)
	r := runProbeTest(home, path)
	rep := decodeProbe(t, r)
	if rep.Version != 1 {
		t.Errorf("version = %d, want 1", rep.Version)
	}
	want := []string{"credentials:claude-code", "credentials:codex", "credentials:pi", "memory:engram", "memory:longterm-mem", "memory:procedural-skills", "gentle-ai-review"}
	if got := names(rep.Observations); !reflect.DeepEqual(got, want) {
		t.Fatalf("capabilities = %v, want %v", got, want)
	}
	for _, o := range rep.Observations {
		wantStatus := workflow.ObservationAvailable
		if o.Capability == "memory:procedural-skills" {
			wantStatus = workflow.ObservationUnavailable
		}
		if o.Status != wantStatus || o.Detail == "" {
			t.Errorf("observation %+v, want %s with a detail", o, wantStatus)
		}
	}
	for _, leak := range []string{probeSecret, home, path} {
		if strings.Contains(r.stdout, leak) {
			t.Errorf("stdout leaks %q", leak)
		}
	}
	if explicit := runProbeTest(home, path, "--target", "all"); explicit.stdout != r.stdout {
		t.Errorf("--target all differs from the default:\n%s\nvs\n%s", explicit.stdout, r.stdout)
	}
}

func TestRuntimeProbeTargetSelectsTheCredentialsSignalAndKeepsTheOthers(t *testing.T) {
	home, path := probeFixture(t)
	for target, credentials := range map[string]string{"claude": "credentials:claude-code", "codex": "credentials:codex", "pi": "credentials:pi"} {
		rep := decodeProbe(t, runProbeTest(home, path, "--target", target))
		want := []string{credentials, "memory:engram", "memory:longterm-mem", "memory:procedural-skills", "gentle-ai-review"}
		if got := names(rep.Observations); !reflect.DeepEqual(got, want) {
			t.Errorf("--target %s: capabilities = %v, want %v", target, got, want)
		}
	}
}

func TestRuntimeProbeReportsAbsenceWithoutInventingPresence(t *testing.T) {
	rep := decodeProbe(t, runProbeTest(t.TempDir(), t.TempDir()))
	for _, o := range rep.Observations {
		if o.Status != workflow.ObservationUnavailable {
			t.Errorf("observation %+v, want unavailable for an empty home and PATH", o)
		}
	}
	// An unknown home and an empty PATH are the same: nothing is guessed.
	rep = decodeProbe(t, runProbeTest("", ""))
	for _, o := range rep.Observations {
		if o.Status != workflow.ObservationUnavailable {
			t.Errorf("observation %+v, want unavailable with no home and no PATH", o)
		}
	}
}

func TestRuntimeProbeRefusals(t *testing.T) {
	home, path := probeFixture(t)
	for name, tc := range map[string]struct {
		args []string
		code int
		want string
	}{
		"unknown target value":        {[]string{"--target", "bogus"}, 2, `unknown target "bogus"`},
		"opencode has no credentials": {[]string{"--target", "opencode"}, 2, `unknown target "opencode"`},
		"empty target value":          {[]string{"--target", ""}, 2, "unknown target"},
		"unknown flag":                {[]string{"--bogus"}, 1, `unknown flag "--bogus"`},
		"capabilities flag":           {[]string{"--config-root", "x"}, 1, "unknown flag"},
		"target without value":        {[]string{"--target"}, 1, "--target requires a value"},
		"positional argument":         {[]string{"claude"}, 1, `unexpected argument "claude"`},
	} {
		t.Run(name, func(t *testing.T) {
			r := runProbeTest(home, path, tc.args...)
			if r.code != tc.code || r.stdout != "" || !strings.Contains(r.stderr, tc.want) {
				t.Errorf("code=%d stdout=%q stderr=%q, want exit %d, no stdout, stderr containing %q", r.code, r.stdout, r.stderr, tc.code, tc.want)
			}
		})
	}
}

func TestRuntimeProbeTakesTheLastOfARepeatedTargetAndTrimsIt(t *testing.T) {
	home, path := probeFixture(t)
	rep := decodeProbe(t, runProbeTest(home, path, "--target", "codex", "--target", " pi "))
	if got := names(rep.Observations)[0]; got != "credentials:pi" {
		t.Errorf("first capability = %q, want credentials:pi (the last --target, trimmed)", got)
	}
}

func TestRuntimeProbeIsDeterministic(t *testing.T) {
	home, path := probeFixture(t)
	if a, b := runProbeTest(home, path), runProbeTest(home, path); a.stdout != b.stdout {
		t.Errorf("two runs differ:\n%s\nvs\n%s", a.stdout, b.stdout)
	}
}

func TestRuntimeProbeExitsOneWhenTheReportCannotBeWritten(t *testing.T) {
	home, path := probeFixture(t)
	var errBuf bytes.Buffer
	var codes []int
	runRuntimeProbe(nil, home, path, failingMemoryWriter{}, &errBuf, func(c int) { codes = append(codes, c) })
	if len(codes) == 0 || codes[0] != 1 || !strings.Contains(errBuf.String(), "writing report") {
		t.Errorf("exits %v, stderr %q, want exit 1 naming the failed write", codes, errBuf.String())
	}
}

// TestRuntimeProbeIsDispatchedFromTheRuntimeVerb: 'runtime probe' is reachable
// through the same entry point as the other runtime actions, before the
// lifecycle flags are parsed. The process HOME is the test's isolated one, so
// nothing there is present.
func TestRuntimeProbeIsDispatchedFromTheRuntimeVerb(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := -1
	runRuntimeCore([]string{"probe", "--target", "pi"}, &out, &errBuf, func(c int) {
		if code == -1 {
			code = c
		}
	})
	if code != -1 && code != 0 {
		t.Fatalf("code=%d stderr=%q, want exit 0", code, errBuf.String())
	}
	rep := decodeProbe(t, probeRun{stdout: out.String(), stderr: errBuf.String()})
	if got := names(rep.Observations)[0]; got != "credentials:pi" {
		t.Errorf("first capability = %q, want credentials:pi", got)
	}
	for _, o := range rep.Observations {
		if o.Capability == "credentials:pi" && o.Status != workflow.ObservationUnavailable {
			t.Errorf("credentials:pi = %+v, want unavailable in the isolated home", o)
		}
	}
}

func TestUsageDocumentsRuntimeProbe(t *testing.T) {
	usageText := captureUsage(t)
	for _, want := range []string{
		"engine runtime probe [--target claude|codex|pi|all]",
		"stat only",
		"never opens",
		"exit 0 success, 2 unknown --target value, 1 usage error including an unknown flag",
	} {
		if !strings.Contains(usageText, want) {
			t.Errorf("usage does not contain %q", want)
		}
	}
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"runtime probe [--target claude|codex|pi|all]", "never opens, reads, hashes, or prints the contents"} {
		if !strings.Contains(string(readme), want) {
			t.Errorf("README does not contain %q", want)
		}
	}
}

// TestRuntimeProbeSourceOpensNoFile: the probe verb takes its home and PATH
// from the environment and hands them to the presence prober. Its source may
// use only os.UserHomeDir and os.Getenv, imports no os/exec or net, and calls no
// function that opens or reads a file.
func TestRuntimeProbeSourceOpensNoFile(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "runtime_probe.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if path == "os/exec" || path == "io/ioutil" || path == "syscall" || path == "net" || strings.HasPrefix(path, "net/") {
			t.Errorf("runtime_probe.go imports %s", path)
		}
	}
	allowed := map[string]bool{"UserHomeDir": true, "Getenv": true}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" && !allowed[sel.Sel.Name] {
			t.Errorf("runtime_probe.go uses os.%s; only os.UserHomeDir and os.Getenv are allowed", sel.Sel.Name)
		}
		return true
	})
}

func TestRuntimeProbeDeadlineIsBounded(t *testing.T) {
	if probeTimeout <= 0 || probeTimeout > 30_000_000_000 {
		t.Errorf("probeTimeout = %v, want a positive bound of at most 30s", probeTimeout)
	}
	var _ context.Context = context.Background()
}
