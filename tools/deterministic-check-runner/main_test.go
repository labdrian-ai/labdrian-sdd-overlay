package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// updateGolden is the standard Go golden-file flag: `go test -run
// TestGoldenRowBlock -update` regenerates testdata/golden-row-block.txt from
// the current renderer; a plain `go test` run only compares against it.
var updateGolden = flag.Bool("update", false, "update golden test fixtures")

var rowPattern = regexp.MustCompile(`^(.+) \| (\d+) \| (.+)$`)

var bannedLiterals = []string{"PASS", "PASSED", "SUCCESS", "N/A", "NA", "NONE", "TODO", "TBD", "PLACEHOLDER"}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

// TestModuleShape asserts parity with the entry-contract-validator sibling
// module's shape, and that this module has zero external dependencies.
func TestModuleShape(t *testing.T) {
	root := repoRoot(t)
	runnerDir := filepath.Join(root, "tools", "deterministic-check-runner")
	validatorDir := filepath.Join(root, "tools", "entry-contract-validator")

	for _, name := range []string{"go.mod", "main.go", "main_test.go"} {
		if _, err := os.Stat(filepath.Join(validatorDir, name)); err != nil {
			t.Fatalf("sibling entry-contract-validator/%s missing: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(runnerDir, name)); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
	}

	info, err := os.Stat(filepath.Join(runnerDir, "testdata"))
	if err != nil {
		t.Errorf("testdata/ missing: %v", err)
	} else if !info.IsDir() {
		t.Errorf("testdata is not a directory")
	}

	data, err := os.ReadFile(filepath.Join(runnerDir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	modContent := string(data)
	const wantModule = "module github.com/labdrian-ai/labdrian-sdd-overlay/tools/deterministic-check-runner"
	if !strings.Contains(modContent, wantModule) {
		t.Errorf("go.mod module path = %q, want it to contain %q", modContent, wantModule)
	}
	if strings.Contains(modContent, "require") {
		t.Errorf("go.mod declares a dependency, want zero external dependencies: %q", modContent)
	}
}

// TestModuleDiscovery covers discoverModules: sorted output, relative-root
// normalization against the caller cwd, and subdirectory safety.
func TestModuleDiscovery(t *testing.T) {
	writeModule := func(t *testing.T, dir string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create module dir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/mod\n\ngo 1.21\n"), 0o644); err != nil {
			t.Fatalf("write go.mod in %s: %v", dir, err)
		}
	}
	assertModules := func(t *testing.T, root string, want []string) {
		t.Helper()
		got, err := discoverModules(root)
		if err != nil {
			t.Fatalf("discoverModules(%q): %v", root, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("discoverModules(%q) = %v, want %v", root, got, want)
		}
	}

	t.Run("sorted output across nested modules", func(t *testing.T) {
		tempDir := t.TempDir()
		moduleA, moduleB := filepath.Join(tempDir, "moduleA"), filepath.Join(tempDir, "moduleB")
		nested := filepath.Join(tempDir, "moduleC", "nested")
		writeModule(t, moduleB)
		writeModule(t, moduleA)
		writeModule(t, nested)
		assertModules(t, tempDir, []string{moduleA, moduleB, nested})
	})

	t.Run("relative root normalizes against the caller cwd", func(t *testing.T) {
		tempDir := t.TempDir()
		only := filepath.Join(tempDir, "only")
		writeModule(t, only)

		previousWD, err := os.Getwd()
		if err != nil {
			t.Fatalf("get working directory: %v", err)
		}
		if err := os.Chdir(tempDir); err != nil {
			t.Fatalf("chdir into %s: %v", tempDir, err)
		}
		defer func() {
			if err := os.Chdir(previousWD); err != nil {
				t.Fatalf("restore working directory: %v", err)
			}
		}()
		assertModules(t, ".", []string{only})
	})

	t.Run("subdirectory root only discovers modules beneath it", func(t *testing.T) {
		tempDir := t.TempDir()
		outer := filepath.Join(tempDir, "outer")
		inner := filepath.Join(outer, "inner")
		writeModule(t, inner)
		writeModule(t, filepath.Join(tempDir, "sibling"))
		assertModules(t, outer, []string{inner})
	})
}

// TestCheckRegistry asserts the hardcoded v1 check set: exactly 4 checks,
// each declaring deterministic and blocking explicitly, each with a
// non-empty checkArgv.
func TestCheckRegistry(t *testing.T) {
	if len(registry) != 4 {
		t.Fatalf("len(registry) = %d, want 4", len(registry))
	}

	wantBlocking := map[string]bool{
		"gofmt":       true,
		"go vet":      true,
		"staticcheck": true,
		"deadcode":    false,
	}

	seen := map[string]bool{}
	for _, c := range registry {
		seen[c.name] = true
		if !c.deterministic {
			t.Errorf("check %q: deterministic = false, want true for all v1 checks", c.name)
		}
		want, ok := wantBlocking[c.name]
		if !ok {
			t.Errorf("check %q: unexpected name in registry", c.name)
			continue
		}
		if c.blocking != want {
			t.Errorf("check %q: blocking = %v, want %v", c.name, c.blocking, want)
		}
		if len(c.checkArgv) == 0 {
			t.Errorf("check %q: checkArgv is empty, want non-empty", c.name)
		}
	}
	for name := range wantBlocking {
		if !seen[name] {
			t.Errorf("registry missing expected check %q", name)
		}
	}
}

// TestClassify asserts classify is the sole enforcement point combining
// deterministic and blocking: a non-deterministic check can never classify
// as blocking, regardless of its blocking field (R-002).
func TestClassify(t *testing.T) {
	tests := []struct {
		name          string
		deterministic bool
		blocking      bool
		want          bool
	}{
		{"deterministic and blocking", true, true, true},
		{"deterministic, not blocking", true, false, false},
		{"not deterministic, blocking", false, true, false},
		{"neither deterministic nor blocking", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := check{name: "test", deterministic: tt.deterministic, blocking: tt.blocking, checkArgv: []string{"true"}}
			if got := classify(c); got != tt.want {
				t.Errorf("classify(%+v) = %v, want %v", c, got, tt.want)
			}
		})
	}
}

// TestRegistryInvariantNonDeterministicNeverBlocking guards the registry
// itself: no entry may declare blocking=true without deterministic=true.
// This must fail if a future edit adds a violating entry.
func TestRegistryInvariantNonDeterministicNeverBlocking(t *testing.T) {
	for _, c := range registry {
		if c.blocking && !c.deterministic {
			t.Errorf("registry invariant violated: check %q has blocking=true and deterministic=false", c.name)
		}
	}
}

// TestClassifyDeadcodeNonBlocking asserts deadcode classifies as
// non-blocking (WARNING severity) per the amended R-016 severity-
// proportional rule.
func TestClassifyDeadcodeNonBlocking(t *testing.T) {
	for _, c := range registry {
		if c.name != "deadcode" {
			continue
		}
		if classify(c) {
			t.Errorf("classify(deadcode) = true, want false (deadcode is WARNING-only)")
		}
		return
	}
	t.Fatal("deadcode not found in registry")
}

// findRegistryCheck looks up a check by name in the real registry so outcome
// tests exercise selectOutcome against actual v1 classify() semantics.
func findRegistryCheck(t *testing.T, name string) check {
	t.Helper()
	for _, c := range registry {
		if c.name == name {
			return c
		}
	}
	t.Fatalf("check %q not found in registry", name)
	return check{}
}

// TestSelectOutcomePrecedence covers the full amended-R-016 precedence
// matrix: unexecutable BLOCKING-set check or a runner error outrank a
// failed blocking check, which outranks passed; a WARNING-only tool
// (deadcode) being unavailable never alone forces procedural_tooling_failed
// and never suppresses a real blocking failure.
func TestSelectOutcomePrecedence(t *testing.T) {
	gofmt := findRegistryCheck(t, "gofmt")
	staticcheck := findRegistryCheck(t, "staticcheck")
	deadcode := findRegistryCheck(t, "deadcode")

	tests := []struct {
		name    string
		results []result
		want    int
	}{
		{"all checks pass", []result{{check: gofmt}, {check: staticcheck}, {check: deadcode}}, 0},
		{"blocking deterministic check failed", []result{{check: gofmt}, {check: staticcheck, exitCode: 1}, {check: deadcode}}, 1},
		{"blocking-set check unexecutable", []result{{check: gofmt}, {check: staticcheck, exitCode: 127, unavailable: true}, {check: deadcode}}, 3},
		{"runner-internal error", []result{{check: gofmt}, {runnerErr: true}}, 3},
		{"deadcode unavailable, everything else green", []result{{check: gofmt}, {check: staticcheck}, {check: deadcode, exitCode: 127, unavailable: true}}, 0},
		{"deadcode unavailable and a blocking check failed", []result{{check: gofmt, exitCode: 1}, {check: staticcheck}, {check: deadcode, exitCode: 127, unavailable: true}}, 1},
		{"blocking-set unexecutable and blocking failure both present", []result{{check: gofmt, exitCode: 1}, {check: staticcheck, exitCode: 127, unavailable: true}, {check: deadcode}}, 3},
		{"non-blocking check red does not affect outcome", []result{{check: gofmt}, {check: staticcheck}, {check: deadcode, exitCode: 1}}, 0},
		{"gofmt reports findings via count with exit 0 (D3 trap, obs #2735)", []result{{check: gofmt, exitCode: 0, count: 1}, {check: staticcheck}, {check: deadcode}}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectOutcome(tt.results); got != tt.want {
				t.Errorf("selectOutcome(%q) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

// fullLogPathPattern extracts the "full: <path>" clause's path from a
// rendered row summary, when present.
var fullLogPathPattern = regexp.MustCompile(`full: (\S+)$`)

// assertRow validates one row against the "tool | exit_code | summary"
// shape, the expected tool name, and the banned-literal guard; it returns
// the exit_code field for caller-specific checks. When the summary carries a
// "full: <path>" clause, it also os.Stat's that path: the row's claim is
// only true evidence (skills/sdd-verify/SKILL.md:70) if the named file
// genuinely exists, so a row must never advertise a path nothing wrote.
func assertRow(t *testing.T, i int, line, wantTool string) string {
	t.Helper()
	m := rowPattern.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("row %d = %q does not match 'tool | exit_code | summary'", i, line)
	}
	if m[1] != wantTool {
		t.Errorf("row %d tool = %q, want %q (registry order)", i, m[1], wantTool)
	}
	for _, word := range bannedLiterals {
		if strings.EqualFold(m[3], word) {
			t.Errorf("row %d summary %q is a banned literal", i, m[3])
		}
	}
	if lp := fullLogPathPattern.FindStringSubmatch(m[3]); lp != nil {
		if _, err := os.Stat(lp[1]); err != nil {
			t.Errorf("row %d names full log path %q, but it does not exist/is not readable: %v", i, lp[1], err)
		}
	}
	return m[2]
}

// TestRowEmission asserts emitRows format, ordering, count (R-006), and that
// summaries come from the real 3E renderer, not the retired Phase-2
// "exit=%d" placeholder.
func TestRowEmission(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "go-vet.log")
	mustWriteFile(t, logPath, "a\nb\n", 0o644)
	results := []result{
		{check: check{name: "gofmt"}, exitCode: 0},
		{check: check{name: "go vet"}, exitCode: 1, count: 2, top: []string{"a", "b"}, logPath: logPath},
		{check: check{name: "staticcheck"}, exitCode: 127, unavailable: true},
		{check: check{name: "deadcode"}, exitCode: 0},
	}
	var buf bytes.Buffer
	emitRows(&buf, results, defaultTopN)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(results) {
		t.Fatalf("emitRows wrote %d lines, want %d", len(lines), len(results))
	}
	for i, line := range lines {
		gotExit := assertRow(t, i, line, results[i].check.name)
		if wantExit := strconv.Itoa(results[i].exitCode); gotExit != wantExit {
			t.Errorf("line %d exit_code = %q, want %q", i, gotExit, wantExit)
		}
	}
	wantSummary := renderSummary(2, []string{"a", "b"}, defaultTopN, logPath)
	if got := rowPattern.FindStringSubmatch(lines[1])[3]; got != wantSummary {
		t.Errorf("row 1 summary = %q, want real renderer output %q (not the exit=%%d placeholder)", got, wantSummary)
	}
}

// TestEmitRowsCapsByDroppingExcerptsNotRows is 3H's RED test for obs #2719:
// capPayload's raw byte truncate could slice a row mid-line, split a UTF-8
// rune, and silently drop every row past the cut. D6 requires the cap to
// operate on structured rows before flattening — dropping excerpts while
// keeping every row's tool, exit code, and count. A payload well over the
// 4 MiB cap is synthesized in memory (never a committed fixture).
func TestEmitRowsCapsByDroppingExcerptsNotRows(t *testing.T) {
	excerpt := strings.Repeat("x", excerptCharCap) // at the char cap, so it survives capExcerpt unchanged
	top := make([]string, 25000)                   // >4 MiB once rendered, forcing the cap
	for i := range top {
		top[i] = excerpt
	}
	results := []result{
		{check: check{name: "gofmt"}, exitCode: 0},
		{check: check{name: "go vet"}, exitCode: 1, count: len(top), top: top, logPath: "/tmp/go-vet.log"},
		{check: check{name: "staticcheck"}, exitCode: 0},
		{check: check{name: "deadcode"}, exitCode: 0},
	}
	var buf bytes.Buffer
	emitRows(&buf, results, len(top))
	out := buf.Bytes()

	if !utf8.Valid(out) {
		t.Errorf("capped emitRows output is not valid UTF-8: %q", out[:64])
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != len(results) {
		t.Fatalf("capped emitRows wrote %d lines, want exactly one row per configured check (%d) — a row must never be dropped", len(lines), len(results))
	}
	if strings.Contains(string(out), excerpt) {
		t.Errorf("capped output still contains excerpt text; want excerpts dropped, counts/paths retained")
	}
	if !strings.Contains(lines[1], "count=25000") || !strings.Contains(lines[1], "/tmp/go-vet.log") {
		t.Errorf("row 1 = %q, want count and full log path retained after the cap degrades excerpts", lines[1])
	}
}

// TestRunCheckSeparatesStreamsAndPopulatesSummary is 3F's RED test:
// runCheck must capture stdout and stderr into distinct buffers (never
// merged) and call c.parse on stdout alone, populating
// result.count/top/logPath. A merged-stream regression would double-count
// the line this test writes to both streams (audit finding, obs #2712).
func TestRunCheckSeparatesStreamsAndPopulatesSummary(t *testing.T) {
	c := check{
		name:      "fake",
		checkArgv: []string{"sh", "-c", "printf 'stdout-line\\n'; printf 'stderr-line\\n' >&2"},
		parse: func(exit int, out []byte) (int, []string) {
			trimmed := strings.TrimSpace(string(out))
			if trimmed == "" {
				return 0, nil
			}
			return len(strings.Split(trimmed, "\n")), strings.Split(trimmed, "\n")
		},
	}
	outDir := t.TempDir()
	got := runCheck(c, []string{t.TempDir()}, outDir)

	if got.count != 1 {
		t.Fatalf("runCheck count = %d, want 1 (stderr must not be merged into parsed stdout)", got.count)
	}
	if len(got.top) != 1 || got.top[0] != "stdout-line" {
		t.Fatalf("runCheck top = %v, want [\"stdout-line\"]", got.top)
	}
	if want := logPathFor("fake", outDir); got.logPath != want {
		t.Errorf("runCheck logPath = %q, want %q", got.logPath, want)
	}
}

// pinnedInvocationPattern matches CI's "go run <module>@<version> ./..."
// step invocations verbatim.
var pinnedInvocationPattern = regexp.MustCompile(`go run (\S+)@(\S+) \./\.\.\.`)

// TestCheckArgvPinnedToCIInvocation parses .github/workflows/ci.yml for the
// pinned "go run <module>@<version> ./..." invocations CI uses for
// staticcheck and deadcode, and asserts the registry's checkArgv match them
// exactly. This is the audit-finding regression guard (obs #2714): the
// runner previously resolved staticcheck/deadcode as bare PATH binaries
// (exit 127 when absent) while CI always uses a pinned `go run`, so a future
// CI version bump not mirrored here must fail loud rather than silently
// drift.
func TestCheckArgvPinnedToCIInvocation(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}

	pinned := map[string]string{}
	for _, m := range pinnedInvocationPattern.FindAllStringSubmatch(string(data), -1) {
		module, invocation := m[1], m[0]
		var tool string
		switch {
		case strings.Contains(module, "staticcheck"):
			tool = "staticcheck"
		case strings.Contains(module, "deadcode"):
			tool = "deadcode"
		default:
			continue
		}
		if prev, ok := pinned[tool]; ok && prev != invocation {
			t.Fatalf("ci.yml pins %s inconsistently across jobs: %q vs %q", tool, prev, invocation)
		}
		pinned[tool] = invocation
	}

	for _, tool := range []string{"staticcheck", "deadcode"} {
		want, ok := pinned[tool]
		if !ok {
			t.Fatalf("ci.yml has no pinned 'go run <module>@<version> ./...' invocation for %s", tool)
		}
		c := findRegistryCheck(t, tool)
		got := strings.Join(c.checkArgv, " ")
		if got != want {
			t.Errorf("registry checkArgv for %s = %q, want %q (parity with ci.yml pinned invocation)", tool, got, want)
		}
	}
}

// TestRunEndToEnd exercises discoverModules x registry x exec x
// classify/selectOutcome x emitRows against this repo's own module set via
// the "check" subcommand. Per-tool exit codes are environment-dependent, so
// only row structure (count, order, shape, banned-literal guard) is
// asserted.
func TestRunEndToEnd(t *testing.T) {
	root := repoRoot(t)
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir into repo root: %v", err)
	}
	defer func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()

	var stdout, stderr bytes.Buffer
	run([]string{"check"}, &stdout, &stderr)

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != len(registry) {
		t.Fatalf("run() wrote %d rows, want %d: %q", len(lines), len(registry), stdout.String())
	}
	for i, line := range lines {
		assertRow(t, i, line, registry[i].name)
	}
}

// TestRunFailsClosedWhenNoModulesDiscovered is correction C1's RED test: an
// empty module set must never look like a clean run. Before the fix, run()
// entered runCheckMode with zero modules, every check's per-module loop ran
// zero times, exitCode/count stayed 0, and the emitted block was
// byte-identical to four checks that genuinely executed and found nothing —
// procedural_tooling_failed silently collapsed into outcomePassed. Asserted
// through run() itself (the real dispatch path), not discoverModules, so the
// guard's wiring is what is proven, not just discoverModules' return value
// (obs #2735's lesson: a helper-level test proves nothing here).
func TestRunFailsClosedWhenNoModulesDiscovered(t *testing.T) {
	emptyRoot := t.TempDir()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(emptyRoot); err != nil {
		t.Fatalf("chdir into %s: %v", emptyRoot, err)
	}
	defer func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()

	var stdout, stderr bytes.Buffer
	got := run([]string{"check"}, &stdout, &stderr)

	if got != outcomeProceduralToolingFailed {
		t.Fatalf(`run(["check"]) under a root with no go.mod = exit %d, want %d (outcomeProceduralToolingFailed)`, got, outcomeProceduralToolingFailed)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() emitted a row block for zero discovered modules, want no rows at all: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "no Go modules discovered") {
		t.Fatalf("run() stderr = %q, want a diagnostic naming the zero-module condition", stderr.String())
	}
}

// TestRunNormalizeReportsFixerFailure is correction C2's RED test: a fixer
// that ran but could not converge must never look like a clean normalize
// run. Before the fix, runNormalize's ExitError branch was a silent no-op
// (cmd.Stdout/cmd.Stderr were never set, so the fixer's own diagnostic was
// discarded) and the loop always fell through to an unconditional
// outcomePassed — so a module gofmt -w could not parse (gofmt writes a
// parse error to stderr and exits non-zero, rewriting nothing) was
// silently swallowed. Asserted through run() itself (the real dispatch
// path), not runNormalize's own package-level reachability, per obs
// #2735: the wiring is what must be proven, not just the helper.
func TestRunNormalizeReportsFixerFailure(t *testing.T) {
	fixture := t.TempDir()
	mustWriteFile(t, filepath.Join(fixture, "go.mod"), "module example.com/normalizefailure\n\ngo 1.21\n", 0o644)
	// Missing closing paren: gofmt cannot parse this file at all, so
	// gofmt -w exits non-zero and rewrites nothing (the concrete trigger
	// named in the correction: "one file gofmt cannot parse").
	mustWriteFile(t, filepath.Join(fixture, "broken.go"), "package main\n\nfunc broken( {\n", 0o644)
	chdir(t, fixture)

	var stdout, stderr bytes.Buffer
	got := run([]string{"normalize"}, &stdout, &stderr)

	if got == outcomePassed {
		t.Fatalf(`run(["normalize"]) against an unparseable module = outcomePassed, want a non-passed outcome (fixer did not converge)`)
	}
	if !strings.Contains(stderr.String(), "gofmt") || !strings.Contains(stderr.String(), fixture) {
		t.Fatalf("run([\"normalize\"]) stderr = %q, want a diagnostic naming the fixer (gofmt) and the failing module (%s)", stderr.String(), fixture)
	}
}

// TestRunNormalizeHappyPath confirms normalize still converges a cleanly
// formattable module: outcomePassed, and gofmt -w actually reformats the
// file. Guards against C2's fix regressing the one thing normalize exists
// to do while it starts capturing and reporting the fixer's output.
func TestRunNormalizeHappyPath(t *testing.T) {
	fixture := t.TempDir()
	mustWriteFile(t, filepath.Join(fixture, "go.mod"), "module example.com/normalizeclean\n\ngo 1.21\n", 0o644)
	const unformatted = "package main\n\nfunc main( ) {\n\tprintln(  \"hi\"  )\n}\n"
	mustWriteFile(t, filepath.Join(fixture, "main.go"), unformatted, 0o644)
	chdir(t, fixture)

	var stdout, stderr bytes.Buffer
	got := run([]string{"normalize"}, &stdout, &stderr)

	if got != outcomePassed {
		t.Fatalf("run([\"normalize\"]) against a formattable module = exit %d, want %d (outcomePassed); stderr=%q", got, outcomePassed, stderr.String())
	}
	after, err := os.ReadFile(filepath.Join(fixture, "main.go"))
	if err != nil {
		t.Fatalf("read reformatted file: %v", err)
	}
	if string(after) == unformatted {
		t.Fatalf("run([\"normalize\"]) left main.go byte-identical to its unformatted input, want gofmt -w to have converged it")
	}
}

// TestRunNormalizeTimeoutNamesCorruptionRiskAndRecoveryArtifacts is the RED
// test for the mutating half of correction D1 (reportNormalizeKill documents
// the gofmt -w write path this reproduces and the GOROOT evidence for it).
// TestRunCheckEnforcesTimeout cannot catch this class: its fake tool is `sh -c
// "sleep 5"`, silent and non-mutating, so it proves the deadline fires and
// nothing about what firing costs on the path that rewrites files in place.
//
// This fake fixer copies gofmt -w's shape — sibling "<name>.go.<digits>"
// backup, then an in-place rewrite of the source — and then hangs, so the
// deadline SIGKILLs it mid-write. It asserts the operator is told the three
// things the pre-fix diagnostic never said: which module may now hold
// corrupted bytes, that a mid-write kill is what put them there, and the exact
// path of every recovery artifact left on disk. None of that may depend on the
// module sitting in a VCS working tree — runNormalize walks from the caller's
// arbitrary cwd, so modules outside a repo, plus ignored and untracked files,
// have no other way back. Driven through run() (the real dispatch path) per
// obs #2735; the registry is swapped because no real check both mutates and
// hangs.
func TestRunNormalizeTimeoutNamesCorruptionRiskAndRecoveryArtifacts(t *testing.T) {
	originalTimeout, originalWaitDelay, originalRegistry := toolExecTimeout, toolWaitDelay, registry
	toolExecTimeout = 500 * time.Millisecond
	toolWaitDelay = 20 * time.Millisecond
	t.Cleanup(func() {
		toolExecTimeout, toolWaitDelay, registry = originalTimeout, originalWaitDelay, originalRegistry
	})

	fixture := t.TempDir()
	mustWriteFile(t, filepath.Join(fixture, "go.mod"), "module example.com/normalizetimeout\n\ngo 1.21\n", 0o644)
	mustWriteFile(t, filepath.Join(fixture, "main.go"), "package main\n\nfunc main() {}\n", 0o644)
	registry = []check{{
		name:          "fake-fixer",
		deterministic: true,
		blocking:      true,
		checkArgv:     []string{"true"},
		normalizeArgv: []string{"sh", "-c", "cp main.go main.go.8342159073 && printf 'package corrupt' > main.go && sleep 5"},
		parse:         func(exit int, out []byte) (int, []string) { return 0, nil },
		failed:        func(exit, count int) bool { return false },
	}}
	chdir(t, fixture)

	var stdout, stderr bytes.Buffer
	got := run([]string{"normalize"}, &stdout, &stderr)

	if got != outcomeProceduralToolingFailed {
		t.Fatalf(`run(["normalize"]) with a fixer killed mid-write = exit %d, want %d (outcomeProceduralToolingFailed); stderr=%q`, got, outcomeProceduralToolingFailed, stderr.String())
	}
	diagnostic := stderr.String()
	if !strings.Contains(diagnostic, fixture) {
		t.Errorf("normalize timeout diagnostic %q does not name the module %s whose files the killed fixer was rewriting", diagnostic, fixture)
	}
	for _, want := range []string{"in place", "corrupt"} {
		if !strings.Contains(diagnostic, want) {
			t.Errorf("normalize timeout diagnostic %q does not state the mid-write corruption risk (missing %q)", diagnostic, want)
		}
	}
	backup := filepath.Join(fixture, "main.go.8342159073")
	if !strings.Contains(diagnostic, backup) {
		t.Errorf("normalize timeout diagnostic %q does not enumerate the recovery artifact %s the killed fixer left on disk", diagnostic, backup)
	}
}

// TestRunNormalizeTimeoutWithholdsRestoreFromStaleArtifacts is the RED test for
// R4-01: the restore instruction must be issued only for artifacts THIS run's
// kill actually produced. scanNormalizeRecoveryArtifacts matches any
// "<name>.go.<digits>" file whose stripped-suffix sibling exists, so a backup
// left by an EARLIER interrupted run is indistinguishable by shape from one this
// kill just created. An operator who follows "restore it over <file>" for the
// earlier one silently reverts that file to its pre-fixer bytes — the tool's own
// recovery guidance destroying an already-completed repair.
//
// The fixture holds both shapes at once: stale.go.111111, backdated with
// os.Chtimes to a day before the run and sitting next to a stale.go the operator
// has since repaired by hand, and main.go.8342159073, written inside this run by
// the fake fixer before the deadline SIGKILLs it. Only the fresh one may carry
// the bare restore instruction; the stale one must be told to be inspected
// first. Driven through run() (the real dispatch path) per obs #2735.
func TestRunNormalizeTimeoutWithholdsRestoreFromStaleArtifacts(t *testing.T) {
	originalTimeout, originalWaitDelay, originalRegistry := toolExecTimeout, toolWaitDelay, registry
	toolExecTimeout = 500 * time.Millisecond
	toolWaitDelay = 20 * time.Millisecond
	t.Cleanup(func() {
		toolExecTimeout, toolWaitDelay, registry = originalTimeout, originalWaitDelay, originalRegistry
	})

	fixture := t.TempDir()
	mustWriteFile(t, filepath.Join(fixture, "go.mod"), "module example.com/normalizestale\n\ngo 1.21\n", 0o644)
	mustWriteFile(t, filepath.Join(fixture, "main.go"), "package main\n\nfunc main() {}\n", 0o644)
	stale := filepath.Join(fixture, "stale.go.111111")
	mustWriteFile(t, filepath.Join(fixture, "stale.go"), "package main\n\n// repaired by hand after the earlier kill\n", 0o644)
	mustWriteFile(t, stale, "package main\n\n// pre-fixer bytes of an earlier, unrelated run\n", 0o644)
	backdated := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(stale, backdated, backdated); err != nil {
		t.Fatalf("backdate %s: %v", stale, err)
	}
	registry = []check{{
		name:          "fake-fixer",
		deterministic: true,
		blocking:      true,
		checkArgv:     []string{"true"},
		normalizeArgv: []string{"sh", "-c", "cp main.go main.go.8342159073 && printf 'package corrupt' > main.go && sleep 5"},
		parse:         func(exit int, out []byte) (int, []string) { return 0, nil },
		failed:        func(exit, count int) bool { return false },
	}}
	chdir(t, fixture)

	var stdout, stderr bytes.Buffer
	got := run([]string{"normalize"}, &stdout, &stderr)

	if got != outcomeProceduralToolingFailed {
		t.Fatalf(`run(["normalize"]) with a fixer killed mid-write = exit %d, want %d (outcomeProceduralToolingFailed); stderr=%q`, got, outcomeProceduralToolingFailed, stderr.String())
	}
	diagnostic := stderr.String()
	fresh := filepath.Join(fixture, "main.go.8342159073")
	freshLine := diagnosticLineNaming(t, diagnostic, fresh)
	if want := "restore it over " + filepath.Join(fixture, "main.go"); !strings.Contains(freshLine, want) {
		t.Errorf("line for the artifact this kill produced = %q, want the restore instruction %q", freshLine, want)
	}
	staleLine := diagnosticLineNaming(t, diagnostic, stale)
	if strings.Contains(staleLine, "restore it over") {
		t.Errorf("line for an artifact predating this run = %q, want no bare restore instruction: following it would revert the hand repair already made to stale.go", staleLine)
	}
	for _, want := range []string{"earlier", "revert", "inspect"} {
		if !strings.Contains(staleLine, want) {
			t.Errorf("line for an artifact predating this run = %q, does not warn against restoring it blind (missing %q)", staleLine, want)
		}
	}
}

// diagnosticLineNaming returns the one stderr line naming path, failing when the
// diagnostic names it zero times or more than once — either would make the
// per-artifact wording assertions above meaningless.
func diagnosticLineNaming(t *testing.T, diagnostic, path string) string {
	t.Helper()
	var found []string
	for _, line := range strings.Split(diagnostic, "\n") {
		if strings.Contains(line, path) {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("stderr names %s on %d lines, want exactly 1; stderr=%q", path, len(found), diagnostic)
	}
	return found[0]
}

// TestSubcommandUsage is 4.1's RED test (R-009): no subcommand exits
// non-zero and usage names both normalize and check.
func TestSubcommandUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run(nil, &stdout, &stderr)
	if got == 0 {
		t.Fatalf("run(nil) exit code = %d, want non-zero", got)
	}
	usage := stdout.String() + stderr.String()
	if !strings.Contains(usage, "normalize") {
		t.Errorf("usage text %q does not name normalize", usage)
	}
	if !strings.Contains(usage, "check") {
		t.Errorf("usage text %q does not name check", usage)
	}
}

// TestCheckNeverMutates is 4.3's RED test (R-009/R-010/D5): no registry
// checkArgv may contain a known fixer flag (e.g. gofmt -w, staticcheck -fix).
func TestCheckNeverMutates(t *testing.T) {
	for _, c := range registry {
		if containsFixerFlag(c.checkArgv) {
			t.Errorf("check %q: checkArgv %v contains a fixer flag, want check mode incapable of mutating", c.name, c.checkArgv)
		}
	}
}

// TestSummaryRendering covers 3E's renderer: zero/one/N/N+1 findings, the
// 200-char excerpt cap, and determinism across repeated calls (R-007).
func TestSummaryRendering(t *testing.T) {
	const logPath = "/tmp/labdrian-deterministic-checks/gofmt.log"
	longExcerpt := strings.Repeat("x", excerptCharCap+50)

	tests := []struct {
		name  string
		count int
		top   []string
		topN  int
		want  string
	}{
		{"zero findings render the literal 0", 0, nil, defaultTopN, "0"},
		{"one finding", 1, []string{"file.go:1:1: finding"}, defaultTopN, "count=1; top: file.go:1:1: finding; full: " + logPath},
		{"more findings than top-n truncates, keeps full count", 7, []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7"}, 3, "count=7; top: f1; f2; f3; full: " + logPath},
		{"excerpt over 200 chars is capped", 1, []string{longExcerpt}, defaultTopN, "count=1; top: " + longExcerpt[:excerptCharCap] + "; full: " + logPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderSummary(tt.count, tt.top, tt.topN, logPath); got != tt.want {
				t.Errorf("renderSummary(%d, %v, %d, ...) = %q, want %q", tt.count, tt.top, tt.topN, got, tt.want)
			}
		})
	}

	top := []string{"f1", "f2"}
	if a, b := renderSummary(2, top, defaultTopN, logPath), renderSummary(2, top, defaultTopN, logPath); a != b {
		t.Errorf("renderSummary is not deterministic: %q != %q", a, b)
	}
}

// TestLogPathForStable covers D6's stable (never timestamped) out-dir path.
func TestLogPathForStable(t *testing.T) {
	outDir := t.TempDir()
	if a, b := logPathFor("go vet", outDir), logPathFor("go vet", outDir); a != b || strings.Contains(a, " ") {
		t.Errorf("logPathFor(go vet) = %q / %q, want stable and space-free", a, b)
	}
}

// TestParseCheckArgs covers the shared FlagSet's per-flag shapes (R-007),
// including the combination that exposed D2: --out-dir alongside --top-n
// from the same parse, proving the two flags can no longer diverge.
func TestParseCheckArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantTopN   int
		wantOutDir string
		wantErr    bool
	}{
		{"no flag uses the default", nil, defaultTopN, "", false},
		{"explicit top-n", []string{"--top-n", "3"}, 3, "", false},
		{"malformed top-n value fails loud", []string{"--top-n", "not-a-number"}, 0, "", true},
		{"out-dir combined with top-n", []string{"--out-dir", "/var/tmp/logs", "--top-n", "20"}, 20, "/var/tmp/logs", false},
		{"unrecognized flag fails loud", []string{"--top-n", "20", "--bogus-flag", "x"}, 0, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topN, outDir, err := parseCheckArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseCheckArgs(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if topN != tt.wantTopN || outDir != tt.wantOutDir {
				t.Errorf("parseCheckArgs(%v) = (%d, %q), want (%d, %q)", tt.args, topN, outDir, tt.wantTopN, tt.wantOutDir)
			}
		})
	}
}

// TestCheckHonorsTopNWithOutDir is D2's production-path proof: it drives
// run() (not parseCheckArgs in isolation) with --out-dir and --top-n
// together. misformatted > defaultTopN so a still-broken cap at 5 is
// observably distinct from the requested 20 honored in full.
func TestCheckHonorsTopNWithOutDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	fixture := t.TempDir()
	mustWriteFile(t, filepath.Join(fixture, "go.mod"), "module example.com/topnfixture\n\ngo 1.21\n", 0o644)
	const misformatted = 7
	for i := 0; i < misformatted; i++ {
		mustWriteFile(t, filepath.Join(fixture, fmt.Sprintf("f%d.go", i)), fmt.Sprintf("package main\nfunc F%d(){\nreturn\n}\n", i), 0o644)
	}
	chdir(t, fixture)

	var stdout, stderr bytes.Buffer
	got := run([]string{"check", "--out-dir", t.TempDir(), "--top-n", "20"}, &stdout, &stderr)
	if got != outcomeVerificationFailed {
		t.Fatalf("run(check --out-dir ... --top-n 20) = %d, want outcomeVerificationFailed (%d); stderr=%s", got, outcomeVerificationFailed, stderr.String())
	}

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	assertRow(t, 0, lines[0], "gofmt")
	gofmtSummary := rowPattern.FindStringSubmatch(lines[0])[3]
	topClause := regexp.MustCompile(`top: (.+); full:`).FindStringSubmatch(gofmtSummary)
	if topClause == nil {
		t.Fatalf("gofmt summary %q missing top/full clause", gofmtSummary)
	}
	if got := len(strings.Split(topClause[1], "; ")); got != misformatted {
		t.Errorf("gofmt summary shows %d excerpts, want all %d (--top-n 20 > %d, so nothing should be capped at defaultTopN): %q", got, misformatted, misformatted, gofmtSummary)
	}
}

// TestCheckUnknownFlagFailsLoud is D2's decision: an unrecognized flag now
// fails loud via outcomeUsage instead of silently degrading to defaultTopN.
func TestCheckUnknownFlagFailsLoud(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run([]string{"check", "--top-n", "20", "--bogus-flag", "x"}, &stdout, &stderr)
	if got != outcomeUsage {
		t.Fatalf("run(check --top-n 20 --bogus-flag x) = %d, want outcomeUsage (%d)", got, outcomeUsage)
	}
	if !strings.Contains(stderr.String(), "bogus-flag") {
		t.Errorf("stderr %q does not name the unrecognized flag", stderr.String())
	}
}

// TestBannedLiterals covers R-008, including the adversarial case where a
// finding's own text is a banned word: renderSummary must never collapse
// into a bare literal. Checking only the zero case would miss that.
func TestBannedLiterals(t *testing.T) {
	logPath := logPathFor("gofmt", t.TempDir())
	if s := renderSummary(0, nil, defaultTopN, logPath); s != "0" || isBannedSummaryLiteral(s) {
		t.Errorf("renderSummary(0, ...) = %q, want literal 0, not a banned literal", s)
	}
	for _, literal := range bannedLiterals {
		t.Run(literal, func(t *testing.T) {
			got := renderSummary(1, []string{literal}, defaultTopN, logPath)
			if strings.EqualFold(got, literal) || isBannedSummaryLiteral(got) {
				t.Errorf("renderSummary(1, [%q], ...) = %q, want it not to be a bare/banned literal", literal, got)
			}
		})
	}
}

// TestGuardAgainstBannedSummaryPanics is 3I.0's RED test: it wires
// isBannedSummaryLiteral into the render path via guardAgainstBannedSummary,
// which renderSummary now calls on every constructed summary. Before this,
// the guard was defined and unit-tested but never invoked from production
// code (deadcode flagged it as unreachable during 3H), so nothing prevented
// a banned literal from being emitted at runtime.
func TestGuardAgainstBannedSummaryPanics(t *testing.T) {
	for _, literal := range bannedLiterals {
		t.Run(literal, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatalf("guardAgainstBannedSummary(%q) did not panic, want it to fail loud on a bare banned literal", literal)
				}
			}()
			guardAgainstBannedSummary(literal)
		})
	}

	// A well-formed summary must never panic; if it did, this call itself
	// would fail the test.
	guardAgainstBannedSummary("count=1; top: finding; full: /tmp/x.log")
}

// TestCapPayload covers R-013's last-resort backstop (obs #2719 correction):
// capPayload now cuts on a line boundary, never mid-row or mid-rune, rather
// than an exact byte offset. Synthesized in memory, never a multi-megabyte
// fixture.
func TestCapPayload(t *testing.T) {
	small := []byte("gofmt | 0 | 0\n")
	if got := capPayload(small); string(got) != string(small) {
		t.Errorf("capPayload(small) = %q, want unchanged", got)
	}

	line := "gofmt | 0 | 0\n"
	huge := []byte(strings.Repeat(line, (payloadByteCap/len(line))+1000))
	got := capPayload(huge)
	if len(got) > payloadByteCap {
		t.Errorf("capPayload(huge) is %d bytes, want at most %d", len(got), payloadByteCap)
	}
	if len(got) == 0 || got[len(got)-1] != '\n' {
		t.Errorf("capPayload(huge) does not end on a line boundary: %q", got)
	}
	if !bytes.HasPrefix(huge, got) {
		t.Errorf("capPayload(huge) is not a prefix of the original payload — it must not alter kept lines")
	}
	if !utf8.Valid(got) {
		t.Errorf("capPayload(huge) is not valid UTF-8")
	}
	if second := capPayload(huge); !bytes.Equal(got, second) {
		t.Errorf("capPayload is not deterministic")
	}
}

// TestPayloadBoundaryContract is 5.7's test pinning the evidence-piping
// contract's two boundary invariants that SKILL.md's "Deterministic Check
// Evidence" section (5.8) documents: capture-evidence's schema requires
// minLength 1, and rejects nothing over payloadByteCap by truncating
// instead (R-013). Neither invariant is reimplemented here — the registry
// is hardcoded non-empty (2B) and the cap already degrades by dropping
// excerpts before falling back to capPayload (3H) — this test only proves
// the contract holds at the boundary, confirmed genuine via mutation
// testing (temporarily emptied the registry / bypassed capPayload,
// observed real failures, reverted; see apply-progress evidence).
func TestPayloadBoundaryContract(t *testing.T) {
	t.Run("a real run is never empty, so the payload always satisfies minLength 1", func(t *testing.T) {
		if len(registry) == 0 {
			t.Fatal("registry is empty: a real run would emit zero bytes, violating capture-evidence's minLength 1 (R-013)")
		}
		results := make([]result, len(registry))
		outDir := t.TempDir()
		for i, c := range registry {
			results[i] = buildResult(c, 0, nil, outDir) // clean run: every check exits 0, finds nothing
		}
		var buf bytes.Buffer
		emitRows(&buf, results, defaultTopN)
		if buf.Len() < 1 {
			t.Fatalf("emitRows wrote %d bytes for an all-clean run, want at least 1 (minLength 1, R-013)", buf.Len())
		}
	})

	t.Run("a payload over 4194304 bytes truncates, never rejected", func(t *testing.T) {
		if payloadByteCap != 4194304 {
			t.Fatalf("payloadByteCap = %d, want 4194304 (R-013's exact capture-evidence boundary)", payloadByteCap)
		}
		excerpt := strings.Repeat("y", excerptCharCap)
		top := make([]string, 30000) // renders well past the 4194304-byte boundary
		for i := range top {
			top[i] = excerpt
		}
		results := []result{
			{check: check{name: "gofmt"}, exitCode: 0},
			{check: check{name: "go vet"}, exitCode: 1, count: len(top), top: top, logPath: "/tmp/go-vet.log"},
			{check: check{name: "staticcheck"}, exitCode: 0},
			{check: check{name: "deadcode"}, exitCode: 0},
		}
		var buf bytes.Buffer
		emitRows(&buf, results, len(top))
		got := buf.Bytes()

		if len(got) > payloadByteCap {
			t.Fatalf("emitRows wrote %d bytes, want at most %d — an oversized payload must truncate, never exceed the boundary", len(got), payloadByteCap)
		}
		if len(got) < 1 {
			t.Fatal("emitRows wrote 0 bytes for an oversized run: truncation must never reject the payload down to nothing")
		}
	})
}

// readTestdata loads a fixture captured from real tool output.
func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return data
}

// TestParseGofmt covers D3: gofmt -l exits 0 even when it lists
// unformatted files, so parseGofmt's count (not gofmt's exit code) must
// drive failedGofmt. Fixtures are real `gofmt -l` output.
func TestParseGofmt(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		exit       int
		wantCount  int
		wantFailed bool
	}{
		{"zero findings", "gofmt-clean.txt", 0, 0, false},
		{"one finding, exit 0", "gofmt-one-finding.txt", 0, 1, true},
		{"several findings, exit 0", "gofmt-several-findings.txt", 0, 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := readTestdata(t, tt.fixture)
			count, top := parseGofmt(tt.exit, out)
			if count != tt.wantCount {
				t.Errorf("parseGofmt count = %d, want %d", count, tt.wantCount)
			}
			if len(top) != tt.wantCount {
				t.Errorf("parseGofmt top has %d entries, want %d", len(top), tt.wantCount)
			}
			if got := failedGofmt(tt.exit, count); got != tt.wantFailed {
				t.Errorf("failedGofmt(%d, %d) = %v, want %v", tt.exit, count, got, tt.wantFailed)
			}
		})
	}
}

// TestParseGoVet covers the counterpart case: go vet's own exit code is
// authoritative, so failedGoVet follows exit directly. Fixtures are real
// `go vet ./...` stderr output.
func TestParseGoVet(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		exit       int
		wantCount  int
		wantFailed bool
	}{
		{"zero findings, exit 0", "govet-clean.txt", 0, 0, false},
		{"one finding, exit 1", "govet-one-finding.txt", 1, 1, true},
		{"several findings, exit 1", "govet-several-findings.txt", 1, 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := readTestdata(t, tt.fixture)
			count, top := parseGoVet(tt.exit, out)
			if count != tt.wantCount {
				t.Errorf("parseGoVet count = %d, want %d", count, tt.wantCount)
			}
			if len(top) != tt.wantCount {
				t.Errorf("parseGoVet top has %d entries, want %d", len(top), tt.wantCount)
			}
			if got := failedGoVet(tt.exit, count); got != tt.wantFailed {
				t.Errorf("failedGoVet(%d, %d) = %v, want %v", tt.exit, count, got, tt.wantFailed)
			}
		})
	}
}

// TestParseStaticcheck covers count extraction from staticcheck's finding
// lines ("path:line:col: message (CODE)"). CODE is not uniformly two
// letters: U1000 is one letter + four digits, ST1005/SA1006 are two + four
// (audit finding, obs #2711 — a two-letter-only pattern silently undercounts
// U-series findings). Fixtures are real `staticcheck@v0.7.0` output.
func TestParseStaticcheck(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		exit       int
		wantCount  int
		wantFailed bool
	}{
		{"zero findings, exit 0", "staticcheck-clean.txt", 0, 0, false},
		{"one finding, exit 1", "staticcheck-one-finding.txt", 1, 1, true},
		{"several findings mixing one- and two-letter codes, exit 1", "staticcheck-several-findings.txt", 1, 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := readTestdata(t, tt.fixture)
			count, top := parseStaticcheck(tt.exit, out)
			if count != tt.wantCount {
				t.Errorf("parseStaticcheck count = %d, want %d", count, tt.wantCount)
			}
			if len(top) != tt.wantCount {
				t.Errorf("parseStaticcheck top has %d entries, want %d", len(top), tt.wantCount)
			}
			if got := failedStaticcheck(tt.exit, count); got != tt.wantFailed {
				t.Errorf("failedStaticcheck(%d, %d) = %v, want %v", tt.exit, count, got, tt.wantFailed)
			}
		})
	}
}

// TestParseStaticcheckToolchainMismatch covers the audit finding (obs
// #2711): staticcheck@v0.7.0 cannot analyze a module whose go directive
// exceeds its build toolchain (reproduced against tui/go.mod's go 1.26.1).
// That failure is not a verification finding — the tool could not analyze
// the code at all — so parseStaticcheck must not count it, and
// isStaticcheckToolchainMismatch must distinguish it from real findings even
// though both exit non-zero.
func TestParseStaticcheckToolchainMismatch(t *testing.T) {
	out := readTestdata(t, "staticcheck-toolchain-mismatch.txt")

	if !isStaticcheckToolchainMismatch(out) {
		t.Fatal("isStaticcheckToolchainMismatch(toolchain-mismatch fixture) = false, want true")
	}
	if isStaticcheckToolchainMismatch(readTestdata(t, "staticcheck-clean.txt")) {
		t.Error("isStaticcheckToolchainMismatch(clean fixture) = true, want false")
	}
	if isStaticcheckToolchainMismatch(readTestdata(t, "staticcheck-one-finding.txt")) {
		t.Error("isStaticcheckToolchainMismatch(one-finding fixture) = true, want false")
	}

	count, top := parseStaticcheck(1, out)
	if count != 0 {
		t.Errorf("parseStaticcheck count = %d, want 0 (toolchain mismatch is not a finding)", count)
	}
	if top != nil {
		t.Errorf("parseStaticcheck top = %v, want nil", top)
	}
}

// TestBuildResultStaticcheckToolchainMismatchRoutesToProceduralToolingFailed
// covers the highest-severity open defect found while verifying 3F (obs
// #2668/#2719-adjacent): isStaticcheckToolchainMismatch existed and was
// unit-tested, but nothing wired it to result.unavailable, so a real
// mismatch produced count=0, exitCode=1, unavailable=false and
// selectOutcome routed it to verification_failed instead of the
// R-016-mandated procedural_tooling_failed. buildResult is runCheck's own
// post-parse result construction (extracted so this defect is directly
// testable without shelling out to a real toolchain-mismatched module), so
// this exercises the real wiring, not a reimplementation of it. The fixture
// is real staticcheck@v0.7.0 stdout (obs #2711/#2668 stream analysis: the
// message reaches stdout, not stderr — see staticcheckToolchainMismatchPattern).
func TestBuildResultStaticcheckToolchainMismatchRoutesToProceduralToolingFailed(t *testing.T) {
	staticcheck := findRegistryCheck(t, "staticcheck")
	out := readTestdata(t, "staticcheck-toolchain-mismatch.txt")
	outDir := t.TempDir()

	r := buildResult(staticcheck, 1, out, outDir)
	if !r.unavailable {
		t.Fatal("buildResult(staticcheck, 1, mismatch fixture).unavailable = false, want true")
	}
	if r.exitCode != 1 {
		t.Errorf("buildResult(...).exitCode = %d, want 1 (rows keep the raw exit code)", r.exitCode)
	}

	got := selectOutcome([]result{r})
	if got != outcomeProceduralToolingFailed {
		t.Errorf("selectOutcome([]result{mismatch}) = %d, want %d (outcomeProceduralToolingFailed) — a mismatch must never route to outcomeVerificationFailed (%d)", got, outcomeProceduralToolingFailed, outcomeVerificationFailed)
	}

	clean := buildResult(staticcheck, 0, readTestdata(t, "staticcheck-clean.txt"), outDir)
	if clean.unavailable {
		t.Error("buildResult(staticcheck, 0, clean fixture).unavailable = true, want false")
	}
}

// TestParseDeadcode covers the audit finding (obs #2709/#2711/#2712):
// deadcode exits 0 even when it reports findings (measured in this
// repository: 21 findings, exit code 0), so count — never exit code — must
// drive failedDeadcode. The last case reproduces the counting trap already
// hit once during the audit: when stdout and stderr are merged, the Go
// toolchain-switch message ("switching to goX.Y.Z") lands in the stream and
// would inflate the count by one if mistaken for a finding line; the fixture
// includes that line to prove parseDeadcode does not count it. Fixtures are
// real `deadcode@v0.48.0` stdout.
func TestParseDeadcode(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		exit       int
		wantCount  int
		wantFailed bool
	}{
		{"zero findings, exit 0", "deadcode-clean.txt", 0, 0, false},
		{"one finding, exit 0", "deadcode-one-finding.txt", 0, 1, true},
		{"several findings, exit 0", "deadcode-several-findings.txt", 0, 3, true},
		{"non-finding line not counted, exit 0", "deadcode-with-toolchain-switch-line.txt", 0, 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := readTestdata(t, tt.fixture)
			count, top := parseDeadcode(tt.exit, out)
			if count != tt.wantCount {
				t.Errorf("parseDeadcode count = %d, want %d", count, tt.wantCount)
			}
			if len(top) != tt.wantCount {
				t.Errorf("parseDeadcode top has %d entries, want %d", len(top), tt.wantCount)
			}
			if got := failedDeadcode(tt.exit, count); got != tt.wantFailed {
				t.Errorf("failedDeadcode(%d, %d) = %v, want %v", tt.exit, count, got, tt.wantFailed)
			}
		})
	}
}

// goldenRowBlockResults builds the fixed, synthetic results the golden test
// renders. These are never a live run: a golden pinned to this repository's
// actual deadcode/staticcheck findings would be brittle and would fail on
// unrelated commits (their count and excerpts drift as the repo changes).
// This fixture instead proves the *renderer* (emitRows/renderRowBlock/
// renderSummary) is deterministic for a fixed input — one row per outcome
// shape: clean, findings truncated by top-N, and unavailable.
func goldenRowBlockResults() []result {
	return []result{
		{check: check{name: "gofmt"}, exitCode: 0},
		{check: check{name: "go vet"}, exitCode: 1, count: 6, top: []string{
			"main.go:10:2: unused variable x",
			"main.go:22:5: shadowed err",
			"main.go:40:1: missing return",
			"main.go:58:9: nil pointer dereference",
			"main.go:71:3: unreachable code",
			"main.go:88:14: composite literal uses unkeyed fields",
		}, logPath: logPathFor("go vet", os.TempDir())},
		{check: check{name: "staticcheck"}, exitCode: 127, unavailable: true, logPath: logPathFor("staticcheck", os.TempDir())},
		{check: check{name: "deadcode"}, exitCode: 0, count: 0},
	}
}

// TestGoldenRowBlock is 3I.1-3I.2's golden test (R-006, R-007): renders
// goldenRowBlockResults through the real emitRows and compares the full
// stdout row block against testdata/golden-row-block.txt. Run with
// `-update` to regenerate the fixture, then rerun without `-update` — the
// two runs must be byte-identical, because reproducibility is what makes
// the runner's output evidence rather than narration.
func TestGoldenRowBlock(t *testing.T) {
	const goldenPath = "testdata/golden-row-block.txt"

	var buf bytes.Buffer
	emitRows(&buf, goldenRowBlockResults(), defaultTopN)
	got := buf.Bytes()

	if *updateGolden {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to generate it): %v", goldenPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("emitRows(goldenRowBlockResults()) does not match %s (run with -update to inspect/regenerate)\ngot:\n%s\nwant:\n%s", goldenPath, got, want)
	}

	var rerun bytes.Buffer
	emitRows(&rerun, goldenRowBlockResults(), defaultTopN)
	if !bytes.Equal(got, rerun.Bytes()) {
		t.Error("emitRows(goldenRowBlockResults()) is not deterministic across repeated calls")
	}
}

// TestUnavailableRowDiffersFromCleanRow is D3's RED test for the
// readability-lens defect: a check that ran and found nothing (gofmt,
// deadcode in goldenRowBlockResults) and a check that could not run at all
// (staticcheck, unavailable via LookPath/toolchain-mismatch/deadline) both
// rendered the bare literal "0" — the row bytes a human or capture-evidence
// reads carried no trace of result.unavailable, even though it is what
// drives selectOutcome's procedural_tooling_failed routing (D4/R-016).
// Driven through emitRows (not renderSummary/renderRowSummary in
// isolation), reusing goldenRowBlockResults so this proves the real
// evidence bytes, not a helper return value, carry the distinction. A
// revert of the rendering fix must fail this test, not just move the
// golden fixture.
func TestUnavailableRowDiffersFromCleanRow(t *testing.T) {
	var buf bytes.Buffer
	emitRows(&buf, goldenRowBlockResults(), defaultTopN)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	assertRow(t, 0, lines[0], "gofmt")
	assertRow(t, 2, lines[2], "staticcheck")
	assertRow(t, 3, lines[3], "deadcode")
	gofmtText := rowPattern.FindStringSubmatch(lines[0])[3]
	staticcheckText := rowPattern.FindStringSubmatch(lines[2])[3]
	deadcodeText := rowPattern.FindStringSubmatch(lines[3])[3]

	if gofmtText != "0" {
		t.Fatalf("gofmt (ran, 0 findings) summary = %q, want literal %q (R-008)", gofmtText, "0")
	}
	if deadcodeText != "0" {
		t.Fatalf("deadcode (ran, 0 findings) summary = %q, want literal %q (R-008)", deadcodeText, "0")
	}
	if staticcheckText == "0" {
		t.Errorf("staticcheck (unavailable, could not run) summary = %q, indistinguishable from a clean 0-finding check", staticcheckText)
	}
	if staticcheckText == gofmtText {
		t.Errorf("staticcheck (unavailable) summary %q must not equal gofmt (ran clean) summary %q", staticcheckText, gofmtText)
	}
	for _, word := range bannedLiterals {
		if strings.EqualFold(strings.TrimSpace(staticcheckText), word) {
			t.Errorf("staticcheck unavailable summary %q is itself a banned literal %q", staticcheckText, word)
		}
	}
}

// TestOutDirGuard is 4.7's RED test: --out-dir inside root is a usage error.
func TestOutDirGuard(t *testing.T) {
	root := repoRoot(t)
	chdir(t, root)
	inside := filepath.Join(root, "tmp-out-dir-guard-test")
	var stdout, stderr bytes.Buffer
	got := run([]string{"check", "--out-dir", inside}, &stdout, &stderr)
	if got != outcomeUsage {
		t.Fatalf("run(check --out-dir %s) = %d, want outcomeUsage (%d)", inside, got, outcomeUsage)
	}
	if !strings.Contains(stderr.String(), "out-dir") {
		t.Errorf("stderr %q does not mention out-dir", stderr.String())
	}
}

// TestOutDirAcceptedWritesLogsThere is correction A3's RED test for the
// accepted branch: TestOutDirGuard only proves an inside-root --out-dir is
// rejected, never that an outside-root value does anything. Before this fix,
// parseOutDir's return was discarded past the guard and logPathFor
// hardcoded os.TempDir(), so an accepted --out-dir was silently inert. This
// does not assert on any internal signature — it drives the real run()
// entry point and checks the observable filesystem effect: a log lands
// under the requested directory, and the default os.TempDir() log is left
// exactly as it was before this call.
func TestOutDirAcceptedWritesLogsThere(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	fixture := t.TempDir()
	mustWriteFile(t, filepath.Join(fixture, "go.mod"), "module example.com/outdirfixture\n\ngo 1.21\n", 0o644)
	mustWriteFile(t, filepath.Join(fixture, "main.go"), "package main\n\nfunc main() {}\n", 0o644)
	chdir(t, fixture)

	defaultLogPath := filepath.Join(os.TempDir(), defaultOutDirName, "gofmt.log")
	beforeContent, _ := os.ReadFile(defaultLogPath)

	outDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	run([]string{"check", "--out-dir", outDir}, &stdout, &stderr)

	wantLogPath := filepath.Join(outDir, defaultOutDirName, "gofmt.log")
	if _, err := os.Stat(wantLogPath); err != nil {
		t.Fatalf("stat %s: %v (want the log written under the requested --out-dir)", wantLogPath, err)
	}

	afterContent, _ := os.ReadFile(defaultLogPath)
	if !bytes.Equal(beforeContent, afterContent) {
		t.Errorf("default os.TempDir() log %s changed during a --out-dir run; want writes confined to the requested --out-dir", defaultLogPath)
	}
}

// TestOutDirGuardRejectsSymlinkResolvingInsideRoot is shape 1's RED test:
// the guard compared filepath.Abs results, and Abs only Cleans — it never
// resolves symlinks. A --out-dir that lives outside the root but IS (or
// traverses) a symlink whose target is inside it therefore passed the
// lexical guard, and the log write landed inside the frozen candidate.
// Driven through run() because the guard's whole purpose is the observable
// refusal, not a helper's return value; the root deliberately holds no
// go.mod, so the pre-fix path falls through to the zero-module guard
// (exit 3) instead of the usage rejection this asserts.
func TestOutDirGuardRejectsSymlinkResolvingInsideRoot(t *testing.T) {
	root := t.TempDir()
	insideTarget := filepath.Join(root, "logs")
	if err := os.MkdirAll(insideTarget, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", insideTarget, err)
	}
	link := filepath.Join(t.TempDir(), "out-dir-link")
	if err := os.Symlink(insideTarget, link); err != nil {
		t.Skipf("symlinks unavailable on this filesystem: %v", err)
	}
	chdir(t, root)

	var stdout, stderr bytes.Buffer
	got := run([]string{"check", "--out-dir", link}, &stdout, &stderr)

	if got != outcomeUsage {
		t.Fatalf("run(check --out-dir %s) = %d, want outcomeUsage (%d); the link resolves to %s, inside the root %s", link, got, outcomeUsage, insideTarget, root)
	}
	if !strings.Contains(stderr.String(), "out-dir") {
		t.Errorf("stderr %q does not mention out-dir", stderr.String())
	}
}

// TestPathContainedInIsCaseInsensitive is shape 2's RED test. The
// containment test is exercised directly rather than through the
// filesystem so the assertion runs on every host: a case-insensitive
// volume (macOS is a documented host for this repository) resolves a
// case-shifted path to the very directory the byte-exact strings.HasPrefix
// comparison just refused to match. The sibling case pins the deliberate
// limit of the fail-closed widening: only a genuine path-segment
// containment counts, never a shared name prefix.
func TestPathContainedInIsCaseInsensitive(t *testing.T) {
	root := filepath.FromSlash("/Repo/Root")

	for _, path := range []string{
		filepath.FromSlash("/repo/root"),
		filepath.FromSlash("/REPO/ROOT"),
		filepath.FromSlash("/repo/root/logs"),
	} {
		if !pathContainedIn(root, path) {
			t.Errorf("pathContainedIn(%q, %q) = false, want true (case-shifted path denotes the same directory on a case-insensitive volume)", root, path)
		}
	}
	for _, path := range []string{
		filepath.FromSlash("/Repo/RootSibling"),
		filepath.FromSlash("/repo/rootsibling"),
		filepath.FromSlash("/elsewhere/root"),
	} {
		if pathContainedIn(root, path) {
			t.Errorf("pathContainedIn(%q, %q) = true, want false (not a path-segment containment)", root, path)
		}
	}
}

// TestOutDirGuardRejectsCaseShiftedRootOnCaseInsensitiveFS is shape 2's
// end-to-end companion. It can only be exercised where a case-shifted name
// actually resolves to the same directory, so it skips cleanly elsewhere;
// TestPathContainedInIsCaseInsensitive carries the assertion everywhere.
func TestOutDirGuardRejectsCaseShiftedRootOnCaseInsensitiveFS(t *testing.T) {
	root := t.TempDir()
	probe := filepath.Join(root, "CaseProbe")
	if err := os.MkdirAll(probe, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", probe, err)
	}
	if _, err := os.Stat(filepath.Join(root, "caseprobe")); err != nil {
		t.Skip("filesystem is case-sensitive; case-shift bypass cannot be exercised here")
	}
	chdir(t, root)

	shifted := filepath.Join(strings.ToUpper(root), "logs")
	var stdout, stderr bytes.Buffer
	got := run([]string{"check", "--out-dir", shifted}, &stdout, &stderr)

	if got != outcomeUsage {
		t.Fatalf("run(check --out-dir %s) = %d, want outcomeUsage (%d); the case-shifted path denotes the root %s", shifted, got, outcomeUsage, root)
	}
}

// TestDefaultOutDirInsideRootFailsClosed is shape 3's RED test: the guard
// ran against the raw flag value, and os.TempDir() was substituted only
// afterwards — so the default was never checked at all. A TMPDIR pointing
// inside the repository wrote the whole log tree into the frozen candidate
// with no flag involved. Asserted through run() with a real module fixture,
// so the pre-fix path genuinely executes the checks and writes the logs
// (exit 0/1) instead of stopping at the zero-module guard, which returns
// the same exit 3 this test wants and would mask the defect. The rejection
// is procedural, not a usage error: nothing about the invocation was wrong.
func TestDefaultOutDirInsideRootFailsClosed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module example.com/tmpdirfixture\n\ngo 1.21\n", 0o644)
	mustWriteFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc main() {}\n", 0o644)
	inside := filepath.Join(root, "tmp")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", inside, err)
	}
	t.Setenv("TMPDIR", inside)
	if os.TempDir() != inside {
		t.Skipf("os.TempDir() = %q does not honour TMPDIR on this platform", os.TempDir())
	}
	chdir(t, root)

	var stdout, stderr bytes.Buffer
	got := run([]string{"check"}, &stdout, &stderr)

	if got != outcomeProceduralToolingFailed {
		t.Fatalf("run(check) with TMPDIR=%s inside the root = %d, want outcomeProceduralToolingFailed (%d)", inside, got, outcomeProceduralToolingFailed)
	}
	if !strings.Contains(stderr.String(), "TMPDIR") {
		t.Errorf("stderr %q does not name TMPDIR, so the operator cannot tell what to fix", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(inside, defaultOutDirName)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("log directory %s exists after a rejected run (stat err %v); check must write nothing inside the root", filepath.Join(inside, defaultOutDirName), err)
	}
}

// TestOutDirRejectsControlCharacters is shape 4's RED test. --out-dir is
// the only caller-controlled input that reaches the emitted row block
// unsplit: it flows verbatim through logPathFor into result.logPath, into
// renderSummary's "; full: %s" clause, and from there into the bytes
// skills/sdd-verify/SKILL.md hands to capture-evidence. A newline in it
// appends a line that satisfies the same "tool | exit_code | summary"
// contract the capture parser reads. The fixture is deliberately
// unformatted so gofmt reports a finding: renderSummary only emits the
// "full:" clause when count > 0, so a clean fixture would not carry the
// injected bytes at all.
func TestOutDirRejectsControlCharacters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module example.com/controlcharfixture\n\ngo 1.21\n", 0o644)
	mustWriteFile(t, filepath.Join(root, "main.go"), "package main\nfunc main()  {}\n", 0o644)
	chdir(t, root)

	forged := filepath.Join(t.TempDir(), "logs") + "\nforged-tool | 0 | 0"
	var stdout, stderr bytes.Buffer
	got := run([]string{"check", "--out-dir", forged}, &stdout, &stderr)

	if got != outcomeUsage {
		t.Fatalf("run(check --out-dir %q) = %d, want outcomeUsage (%d)", forged, got, outcomeUsage)
	}
	if !strings.Contains(stderr.String(), "control character") {
		t.Errorf("stderr %q does not name the offending byte", stderr.String())
	}
	if strings.Contains(stdout.String(), "forged-tool") {
		t.Fatalf("a forged row reached the emitted evidence block: %q", stdout.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("run() emitted %q for a rejected --out-dir, want no rows at all", stdout.String())
	}
}

// TestOutDirControlCharacterRejectionIsExhaustive pins the accepted byte
// range: every byte below 0x20 plus DEL is refused, and an ordinary
// printable path is not.
func TestOutDirControlCharacterRejectionIsExhaustive(t *testing.T) {
	for b := 0; b < 0x20; b++ {
		if _, bad := firstControlByte("/tmp/logs" + string(rune(b))); !bad {
			t.Errorf("firstControlByte did not reject byte %#x", b)
		}
	}
	if _, bad := firstControlByte("/tmp/logs\x7f"); !bad {
		t.Error("firstControlByte did not reject DEL (0x7f)")
	}
	if got, bad := firstControlByte("/tmp/labdrian logs/out-dir"); bad {
		t.Errorf("firstControlByte rejected an ordinary path on byte %q", got)
	}
}

// chdir enters dir and restores the previous cwd via t.Cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	previousWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir into %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
}

// gitOutput runs a git subcommand in dir, returning its stdout.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, stderr.String())
	}
	return stdout.String()
}

// fileState is a per-file (mode, size, sha256) fingerprint (D5).
type fileState struct {
	mode fs.FileMode
	size int64
	sha  string
}

// snapshotTree captures status plus per-file fileState, skipping .git (D5).
func snapshotTree(t *testing.T, dir string) (status string, files map[string]fileState) {
	t.Helper()
	status = gitOutput(t, dir, "status", "--porcelain")
	files = make(map[string]fileState)
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		info, statErr := d.Info()
		content, readErr := os.ReadFile(path)
		rel, relErr := filepath.Rel(dir, path)
		if statErr != nil || readErr != nil || relErr != nil {
			return errors.Join(statErr, readErr, relErr)
		}
		sum := sha256.Sum256(content)
		files[rel] = fileState{mode: info.Mode(), size: info.Size(), sha: fmt.Sprintf("%x", sum)}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", dir, walkErr)
	}
	return status, files
}

// TestCheckByteNeutral is 4.5's RED test (D5): check must leave a dirty
// tree, including a 0755 file, byte- and mode-identical.
func TestCheckByteNeutral(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	fixture := t.TempDir()
	gitOutput(t, fixture, "init", "-q")
	mustWriteFile(t, filepath.Join(fixture, "go.mod"), "module example.com/fixture\n\ngo 1.21\n", 0o644)
	mustWriteFile(t, filepath.Join(fixture, "main.go"), "package main\n\nfunc main() {}\n", 0o644)
	mustWriteFile(t, filepath.Join(fixture, "script.sh"), "#!/bin/sh\necho hi\n", 0o755)
	gitOutput(t, fixture, "add", "-A")
	gitOutput(t, fixture, "-c", "user.email=fixture@example.com", "-c", "user.name=Fixture", "commit", "-q", "-m", "initial")

	// dirty tree: an uncommitted edit plus an untracked file
	mustWriteFile(t, filepath.Join(fixture, "main.go"), "package main\n\nfunc main() {}\n\n// dirty\n", 0o644)
	mustWriteFile(t, filepath.Join(fixture, "untracked.txt"), "scratch\n", 0o644)

	beforeStatus, beforeFiles := snapshotTree(t, fixture)
	chdir(t, fixture)
	var stdout, stderr bytes.Buffer
	run([]string{"check"}, &stdout, &stderr)
	afterStatus, afterFiles := snapshotTree(t, fixture)
	if beforeStatus != afterStatus {
		t.Errorf("git status --porcelain changed:\nbefore:\n%s\nafter:\n%s", beforeStatus, afterStatus)
	}
	if !reflect.DeepEqual(beforeFiles, afterFiles) {
		t.Errorf("per-file (mode, size, sha256) changed:\nbefore: %+v\nafter: %+v", beforeFiles, afterFiles)
	}
}

// mustWriteFile writes content to path and chmods it to mode.
func mustWriteFile(t *testing.T, path, content string, mode fs.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

// writeExecutable writes an executable shell script named name in dir.
func writeExecutable(t *testing.T, dir, name, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("write executable %s/%s: %v", filepath.Join(dir, name), name, err)
	}
}

// TestMissingToolStubbedPATH is 4.9's RED test (amended R-016, obs #2700):
// unavailability is severity-proportional. Scenario A confirms an
// unavailable BLOCKING-set tool still forces procedural_tooling_failed.
// Scenario B is load-bearing: deadcode's checkArgv[0] is "go", shared with
// go vet/staticcheck, so it cannot be isolated by hiding "go" from PATH
// without also breaking those two. It is isolated instead by making the
// stubbed "go" itself exit 127 — the same convention exec.LookPath failure
// already synthesizes — proving the real-process detection 4.10 adds
// composes with the existing LookPath sites rather than duplicating them.
func TestMissingToolStubbedPATH(t *testing.T) {
	t.Run("blocking-set tool unavailable forces procedural_tooling_failed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir()) // no gofmt anywhere on PATH
		gofmt := findRegistryCheck(t, "gofmt")

		got := runCheck(gofmt, []string{t.TempDir()}, t.TempDir())
		if !got.unavailable {
			t.Fatalf("runCheck(gofmt, stubbed PATH).unavailable = false, want true")
		}
		if outcome := selectOutcome([]result{got}); outcome != outcomeProceduralToolingFailed {
			t.Errorf("selectOutcome([]result{gofmt unavailable}) = %d, want %d", outcome, outcomeProceduralToolingFailed)
		}
	})

	t.Run("deadcode alone unavailable never forces procedural_tooling_failed", func(t *testing.T) {
		stubDir := t.TempDir()
		writeExecutable(t, stubDir, "go", "#!/bin/sh\nexit 127\n")
		t.Setenv("PATH", stubDir)
		deadcode := findRegistryCheck(t, "deadcode")

		got := runCheck(deadcode, []string{t.TempDir()}, t.TempDir())
		if !got.unavailable {
			t.Fatalf("runCheck(deadcode, exit-127 stub).unavailable = false, want true (WARNING row, not silently clean)")
		}
		if classify(got.check) {
			t.Fatalf("classify(deadcode) = true, want false (WARNING-only, never blocking)")
		}

		gofmt, goVet, staticcheck := findRegistryCheck(t, "gofmt"), findRegistryCheck(t, "go vet"), findRegistryCheck(t, "staticcheck")
		outcome := selectOutcome([]result{{check: gofmt}, {check: goVet}, {check: staticcheck}, got})
		if outcome != outcomePassed {
			t.Errorf("selectOutcome(deadcode unavailable, rest green) = %d, want %d (outcomePassed) — an absent WARNING-only tool must not invalidate the run", outcome, outcomePassed)
		}
	})
}

// TestRunCheckEnforcesTimeout is correction D1's RED test: no subprocess had
// a deadline, so a stalled tool (e.g. a cold-cache "go run <module>@version"
// against a stalled proxy) could hang check indefinitely with zero partial
// evidence, since emitRows only writes after every check completes. This
// drives the assertion through runCheck itself (the real production entry
// point every registry check goes through), not a disconnected helper, per
// this change's own recurring "implemented, unit-tested, never consumed"
// lesson. toolExecTimeout and toolWaitDelay are both shrunk to 20ms for the
// duration of this test so the suite never waits out a real deadline; the
// fake tool sleeps 5s, far longer than either shrunk value, so the assertion
// only passes if the deadline actually kills the process. WaitDelay must be
// shrunk too: "sh -c sleep 5" runs sleep as a grandchild that inherits the
// stdout/stderr pipes, so killing sh alone leaves those pipes open and Wait
// blocked until the orphaned sleep exits on its own — proven while writing
// this test, where killing sh without a shrunk WaitDelay still took the
// full 5s wall time despite ctx firing at 20ms. Genuine RED was established
// by running this test against the pre-fix runCheck (plain exec.Command, no
// context): it hung for the full 5s sleep and then failed both assertions
// (unavailable stayed false, outcome stayed outcomePassed) — proving the
// assertion is void without real enforcement, not merely a compile check.
func TestRunCheckEnforcesTimeout(t *testing.T) {
	originalTimeout, originalWaitDelay := toolExecTimeout, toolWaitDelay
	toolExecTimeout = 20 * time.Millisecond
	toolWaitDelay = 20 * time.Millisecond
	t.Cleanup(func() {
		toolExecTimeout = originalTimeout
		toolWaitDelay = originalWaitDelay
	})

	c := check{
		name:          "fake-slow",
		deterministic: true,
		blocking:      true,
		checkArgv:     []string{"sh", "-c", "sleep 5"},
		parse:         func(exit int, out []byte) (int, []string) { return 0, nil },
		failed:        func(exit, count int) bool { return false },
	}
	got := runCheck(c, []string{t.TempDir()}, t.TempDir())

	if !got.unavailable {
		t.Fatalf("runCheck(sleep 5, %s deadline).unavailable = false, want true — a process killed by the deadline never analyzed the code, same class as exit 127", toolExecTimeout)
	}
	if outcome := selectOutcome([]result{got}); outcome != outcomeProceduralToolingFailed {
		t.Errorf("selectOutcome([]result{timed-out check}) = %d, want %d (procedural_tooling_failed) — a timeout must never read as verification_failed or passed", outcome, outcomeProceduralToolingFailed)
	}
}

// TestRunCheckGoVetFindingsReachRow is the row-level RED test for the
// stderr-drop defect: go vet writes its diagnostics to stderr, and runCheck
// used to pass only stdout to buildResult, so a real go vet failure rendered
// as the row "go vet | 1 | 0" — the exact byte sequence capture-evidence
// would have received. The fixture lives in t.TempDir(), never testdata/,
// because discoverModules only skips .git and would otherwise pick up a
// committed go.mod as a real module.
func TestRunCheckGoVetFindingsReachRow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	fixture := t.TempDir()
	mustWriteFile(t, filepath.Join(fixture, "go.mod"), "module example.com/vetfixture\n\ngo 1.21\n", 0o644)
	mustWriteFile(t, filepath.Join(fixture, "main.go"), "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Printf(\"%d\\n\", \"not-a-number\")\n}\n", 0o644)

	got := runCheck(findRegistryCheck(t, "go vet"), []string{fixture}, t.TempDir())

	var stdout bytes.Buffer
	emitRows(&stdout, []result{got}, 5)
	row := strings.TrimRight(stdout.String(), "\n")
	fields := strings.SplitN(row, " | ", 3)
	if len(fields) != 3 {
		t.Fatalf("emitted row %q does not have 3 fields", row)
	}
	if fields[0] != "go vet" || fields[1] != "1" {
		t.Fatalf("emitted row = %q, want prefix \"go vet | 1 | ...\"", row)
	}
	if fields[2] == "0" {
		t.Fatalf("go vet row summary = %q, want real finding evidence, not the zero-findings literal (stderr never reached the parser)", fields[2])
	}
	if !strings.HasPrefix(fields[2], "count=1; top:") {
		t.Errorf("go vet row summary = %q, want prefix %q", fields[2], "count=1; top:")
	}
}

// TestRunCheckWritesFindingsToLogFile is the RED test for the "full: <path>"
// defect: logPathFor (D6) only composed a string, and nothing ever wrote it,
// so a row's full: clause pointed at a file that never existed — worst
// exactly when emitRows drops excerpts (payloadByteCap), leaving the
// dangling path as the row's only pointer to the findings. renderSummary and
// logPathFor were already unit-tested and green while this was broken —
// proving nothing, since main_test.go is package main and each test's own
// direct call supplied the reachability production code lacked. This
// asserts at the level the claim is actually made: the emitted row's full:
// path must exist and its content must be the real captured findings
// stream, verified through assertRow (which os.Stat's it) and a direct read.
func TestRunCheckWritesFindingsToLogFile(t *testing.T) {
	outDir := t.TempDir()

	c := check{
		name:      "fake",
		checkArgv: []string{"sh", "-c", "printf 'finding-line\\n'; exit 1"},
		parse:     func(exit int, out []byte) (int, []string) { return 1, []string{"finding-line"} },
	}
	got := runCheck(c, []string{t.TempDir()}, outDir)

	if got.logPath == "" {
		t.Fatal("runCheck logPath = \"\", want the check's stable full-log path (write must succeed here)")
	}
	content, err := os.ReadFile(got.logPath)
	if err != nil {
		t.Fatalf("read %s: %v (the row's full: clause names a file that must exist and be readable)", got.logPath, err)
	}
	if string(content) != "finding-line\n" {
		t.Fatalf("log file content = %q, want the real captured findings stream %q", content, "finding-line\n")
	}

	var stdout bytes.Buffer
	emitRows(&stdout, []result{got}, defaultTopN)
	assertRow(t, 0, strings.TrimRight(stdout.String(), "\n"), "fake")

	// B2 full-coverage guard: this fixture's single module is fully usable
	// (modulesAttempted == modulesUsable == 1), so the row must stay
	// byte-identical to the pre-B2 shape — no "modules=" coverage note when
	// there is nothing partial to report.
	if strings.Contains(stdout.String(), "modules=") {
		t.Errorf("row %q gained a modules= coverage note despite full module coverage (B2 must not add noise here)", stdout.String())
	}
}

// TestRunCheckOmitsFullClauseWhenLogWriteFails is the omission-branch RED
// test for the pre-decided failure policy: a failed log write must clear
// result.logPath so renderSummary omits the full: clause while keeping the
// row's count/excerpts intact — it must never become a runnerErr or
// otherwise change selectOutcome, because a full /tmp must not hold
// verification hostage. The write is forced to fail deterministically by
// pointing outDir at a directory where the expected log subdirectory name is
// already taken by a regular file, so os.MkdirAll fails.
func TestRunCheckOmitsFullClauseWhenLogWriteFails(t *testing.T) {
	outDir := t.TempDir()
	mustWriteFile(t, filepath.Join(outDir, defaultOutDirName), "blocking file, not a directory", 0o644)

	c := check{
		name:      "fake",
		checkArgv: []string{"sh", "-c", "printf 'finding-line\\n'; exit 1"},
		parse:     func(exit int, out []byte) (int, []string) { return 1, []string{"finding-line"} },
	}
	got := runCheck(c, []string{t.TempDir()}, outDir)

	if got.logPath != "" {
		t.Fatalf("runCheck logPath = %q, want \"\" (write must fail deterministically here)", got.logPath)
	}
	summary := renderSummary(got.count, got.top, defaultTopN, got.logPath)
	if strings.Contains(summary, "full:") {
		t.Errorf("summary %q contains a full: clause despite a failed write", summary)
	}
	if !strings.HasPrefix(summary, "count=1; top:") {
		t.Errorf("summary %q lost its count/excerpts when the log write failed", summary)
	}
}

// TestRunCheckSurvivesExit127FromEarlierModule is B1's cross-module RED test
// for the early-return symptom: runCheck used to return immediately on a 127
// exit from ANY module, discarding every module already processed and
// skipping every module still to come. Two real directories are passed in
// discoverModules' sort order (a-unavailable before z-violating); the fake
// tool exits 127 only in the first (a real, mundane way a single module can
// be inapplicable — e.g. an unresolved "go run <module>@version" — without
// the tool itself being globally missing) and prints a genuine, parseable
// finding in the second. The later module's finding must survive, and the
// check as a whole must not be marked unavailable just because one module
// could not run it (unavailable is per module, not global).
func TestRunCheckSurvivesExit127FromEarlierModule(t *testing.T) {
	parent := t.TempDir()
	unavailableModule := filepath.Join(parent, "a-unavailable")
	violatingModule := filepath.Join(parent, "z-violating")
	for _, dir := range []string{unavailableModule, violatingModule} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	c := check{
		name: "fake",
		checkArgv: []string{"sh", "-c",
			`if [ "$(basename "$PWD")" = "a-unavailable" ]; then exit 127; fi; printf 'finding-line\n'; exit 1`,
		},
		parse: func(exit int, out []byte) (int, []string) {
			trimmed := strings.TrimSpace(string(out))
			if trimmed == "" {
				return 0, nil
			}
			lines := strings.Split(trimmed, "\n")
			return len(lines), lines
		},
	}

	got := runCheck(c, []string{unavailableModule, violatingModule}, t.TempDir())

	if got.unavailable {
		t.Fatal("runCheck([a-unavailable(127), z-violating]).unavailable = true, want false — one module's 127 must not erase a check that ran successfully elsewhere")
	}
	if got.count != 1 {
		t.Fatalf("runCheck([a-unavailable(127), z-violating]).count = %d, want 1 — the later module's finding must survive an earlier module's 127 exit (no early return)", got.count)
	}
	if len(got.top) != 1 || got.top[0] != "finding-line" {
		t.Fatalf("runCheck(...).top = %v, want [\"finding-line\"]", got.top)
	}

	// B2: the emitted row bytes must themselves say one of the two modules
	// was excluded — the defect this correction fixes is that a row like
	// "fake | 1 | count=1; ..." here is indistinguishable from a row where
	// both modules ran cleanly. Driven through emitRows (not a helper), so
	// this proves the wiring reaches the actual evidence bytes
	// capture-evidence receives, not just runCheck's return value.
	var stdout bytes.Buffer
	emitRows(&stdout, []result{got}, defaultTopN)
	line := strings.TrimRight(stdout.String(), "\n")
	assertRow(t, 0, line, "fake")
	if !strings.Contains(line, "modules=1/2") {
		t.Errorf("row %q does not state that only 1 of 2 modules was usable — partial coverage is silently indistinguishable from full coverage", line)
	}
}

// TestRunCheckToolchainMismatchInOneModuleDoesNotEraseFindingsInAnother is
// B1's sharper RED test for the shared-buffer symptom: runCheck used to
// concatenate every module's output into one buffer and call
// c.unavailableIf/c.parse on that concatenation exactly once, so a single
// module's toolchain-mismatch-shaped output made the ENTIRE check
// unavailable and discarded genuine findings from every other module (via
// parseStaticcheck's own defensive isStaticcheckToolchainMismatch check,
// which fires against the whole concatenation). The two check-struct fields
// under test (unavailableIf, parse) are the real staticcheck wiring; only
// checkArgv is faked, for a hermetic, network-free reproduction of the
// documented tui/go.mod (go 1.26.1) vs engine/go.mod (go 1.21) mismatch.
func TestRunCheckToolchainMismatchInOneModuleDoesNotEraseFindingsInAnother(t *testing.T) {
	fixtures := t.TempDir()
	mismatchFile := filepath.Join(fixtures, "mismatch.out")
	findingFile := filepath.Join(fixtures, "finding.out")
	mustWriteFile(t, mismatchFile, "-: /path/tui/main.go:6:1: package requires newer Go version go1.26 (application built with go1.25) (compile)\nexit status 1\n", 0o644)
	mustWriteFile(t, findingFile, "cmd/main_test.go:49:7: const passThrough is unused (U1000)\n", 0o644)
	t.Setenv("FAKE_MISMATCH_FILE", mismatchFile)
	t.Setenv("FAKE_FINDING_FILE", findingFile)

	parent := t.TempDir()
	mismatchModule := filepath.Join(parent, "a-mismatch")
	findingModule := filepath.Join(parent, "z-finding")
	for _, dir := range []string{mismatchModule, findingModule} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	c := check{
		name:          "fake-staticcheck",
		blocking:      true,
		deterministic: true,
		checkArgv: []string{"sh", "-c",
			`if [ "$(basename "$PWD")" = "a-mismatch" ]; then cat "$FAKE_MISMATCH_FILE"; else cat "$FAKE_FINDING_FILE"; fi; exit 1`,
		},
		parse:         parseStaticcheck,
		failed:        failedStaticcheck,
		unavailableIf: isStaticcheckToolchainMismatch,
	}

	got := runCheck(c, []string{mismatchModule, findingModule}, t.TempDir())

	if got.unavailable {
		t.Fatal("runCheck([mismatch, finding]).unavailable = true, want false — a mismatch in one module must not erase a check that produced real findings in another")
	}
	if got.count != 1 {
		t.Fatalf("runCheck([mismatch, finding]).count = %d, want 1 — the finding module's result must survive the other module's toolchain mismatch", got.count)
	}
	if len(got.top) != 1 || !strings.Contains(got.top[0], "U1000") {
		t.Fatalf("runCheck(...).top = %v, want the finding module's line", got.top)
	}

	outcome := selectOutcome([]result{got})
	if outcome != outcomeVerificationFailed {
		t.Errorf("selectOutcome([]result{got}) = %d, want %d (outcomeVerificationFailed) — a real finding elsewhere must not be rewritten to outcomeProceduralToolingFailed (%d) by an unrelated module's mismatch", outcome, outcomeVerificationFailed, outcomeProceduralToolingFailed)
	}
}

// partialCoverageCheck builds a fake staticcheck whose first module emits the
// real toolchain-mismatch text (excluded by unavailableIf) and whose second
// module runs clean and finds nothing. unavailableIf/parse/failed are the real
// staticcheck wiring; only checkArgv is faked, for a hermetic, network-free
// reproduction of this repository's own tui/go.mod (go 1.26.1) vs
// staticcheck@v0.7.0 mismatch. blocking is a parameter so the same shape can
// prove both halves of the gate.
func partialCoverageCheck(t *testing.T, blocking bool) (check, []string) {
	t.Helper()
	mismatchFile := filepath.Join(t.TempDir(), "mismatch.out")
	mustWriteFile(t, mismatchFile, "-: /path/tui/main.go:6:1: package requires newer Go version go1.26 (application built with go1.25) (compile)\nexit status 1\n", 0o644)
	t.Setenv("FAKE_MISMATCH_FILE", mismatchFile)

	parent := t.TempDir()
	mismatchModule := filepath.Join(parent, "a-mismatch")
	cleanModule := filepath.Join(parent, "z-clean")
	for _, dir := range []string{mismatchModule, cleanModule} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	c := check{
		name:          "fake-staticcheck",
		blocking:      blocking,
		deterministic: true,
		checkArgv: []string{"sh", "-c",
			`if [ "$(basename "$PWD")" = "a-mismatch" ]; then cat "$FAKE_MISMATCH_FILE"; exit 1; fi; exit 0`,
		},
		parse:         parseStaticcheck,
		failed:        failedStaticcheck,
		unavailableIf: isStaticcheckToolchainMismatch,
	}
	return c, []string{mismatchModule, cleanModule}
}

// TestPartialModuleCoverageWithNoFindingsFailsClosed is the RED test for the
// regression correction B1 opened: B1 correctly narrowed result.unavailable to
// "every attempted module was excluded", but selectOutcome never read the
// per-module coverage B1 introduced, so a blocking check that could not
// analyze SOME modules and found nothing in the ones it could analyze fell
// through to outcomePassed. Live reproduction in this repository: tui/go.mod
// declares go 1.26.1, staticcheck@v0.7.0 is built older and is excluded from
// that module, the other three modules are clean, so the run exited 0 and
// skills/sdd-verify/SKILL.md mapped that to `passed` — tui was never
// statically analyzed and the gate still recorded a clean verification. Before
// B1 that exact condition exited 3; the correction converted a fail-closed
// into a pass. Driven through runCheck (the real per-module loop) into
// selectOutcome, so what is proven is the production routing, not a
// hand-built result literal.
func TestPartialModuleCoverageWithNoFindingsFailsClosed(t *testing.T) {
	t.Run("blocking check with partial coverage and no findings", func(t *testing.T) {
		c, modules := partialCoverageCheck(t, true)
		got := runCheck(c, modules, t.TempDir())

		if got.unavailable {
			t.Fatalf("runCheck([mismatch, clean]).unavailable = true, want false — one usable module means the check itself ran (B1's definition, unchanged here)")
		}
		if got.count != 0 {
			t.Fatalf("runCheck([mismatch, clean]).count = %d, want 0 — the usable module is clean, so nothing must be found", got.count)
		}
		if got.modulesAttempted != 2 || got.modulesUsable != 1 {
			t.Fatalf("runCheck([mismatch, clean]) coverage = %d/%d usable/attempted, want 1/2", got.modulesUsable, got.modulesAttempted)
		}

		outcome := selectOutcome([]result{got})
		if outcome != outcomeProceduralToolingFailed {
			t.Errorf("selectOutcome([]result{partial coverage, 0 findings}) = %d, want %d (outcomeProceduralToolingFailed) — exit %d (outcomePassed) claims every module was verified and clean, while one module was never analyzed at all", outcome, outcomeProceduralToolingFailed, outcomePassed)
		}
	})

	t.Run("non-blocking check with partial coverage stays passed", func(t *testing.T) {
		c, modules := partialCoverageCheck(t, false)
		got := runCheck(c, modules, t.TempDir())

		if classify(got.check) {
			t.Fatalf("classify(non-blocking check) = true, want false")
		}
		outcome := selectOutcome([]result{got})
		if outcome != outcomePassed {
			t.Errorf("selectOutcome([]result{non-blocking, partial coverage}) = %d, want %d (outcomePassed) — a WARNING-only tool's coverage gap must never invalidate the run", outcome, outcomePassed)
		}
	})
}
