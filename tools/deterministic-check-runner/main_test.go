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

// TestParseTopN covers the --top-n flag: default, explicit, malformed (R-007).
func TestParseTopN(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no flag uses the default", nil, defaultTopN},
		{"explicit value", []string{"--top-n", "3"}, 3},
		{"malformed value falls back to the default", []string{"--top-n", "not-a-number"}, defaultTopN},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseTopN(tt.args); got != tt.want {
				t.Errorf("parseTopN(%v) = %d, want %d", tt.args, got, tt.want)
			}
		})
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
