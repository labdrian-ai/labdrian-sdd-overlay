package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectOutcome(tt.results); got != tt.want {
				t.Errorf("selectOutcome(%q) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

// assertRow validates one row against the "tool | exit_code | summary"
// shape, the expected tool name, and the banned-literal guard; it returns
// the exit_code field for caller-specific checks.
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
	return m[2]
}

// TestRowEmission asserts emitRows format, ordering, and count (R-006).
func TestRowEmission(t *testing.T) {
	results := []result{
		{check: check{name: "gofmt"}, exitCode: 0},
		{check: check{name: "go vet"}, exitCode: 1},
		{check: check{name: "staticcheck"}, exitCode: 127, unavailable: true},
		{check: check{name: "deadcode"}, exitCode: 0},
	}
	var buf bytes.Buffer
	emitRows(&buf, results)

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
// classify/selectOutcome x emitRows against this repo's own module set.
// Per-tool exit codes are environment-dependent, so only row structure
// (count, order, shape, banned-literal guard) is asserted.
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
	run(nil, &stdout, &stderr)

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != len(registry) {
		t.Fatalf("run() wrote %d rows, want %d: %q", len(lines), len(registry), stdout.String())
	}
	for i, line := range lines {
		assertRow(t, i, line, registry[i].name)
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
	if a, b := logPathFor("go vet"), logPathFor("go vet"); a != b || strings.Contains(a, " ") {
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
	logPath := logPathFor("gofmt")
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

// TestCapPayload covers R-013: within-cap passthrough and oversized-payload
// truncation, synthesized in memory rather than a multi-megabyte fixture.
func TestCapPayload(t *testing.T) {
	small := []byte("gofmt | 0 | 0\n")
	if got := capPayload(small); string(got) != string(small) {
		t.Errorf("capPayload(small) = %q, want unchanged", got)
	}

	huge := []byte(strings.Repeat("x", payloadByteCap+4096))
	got := capPayload(huge)
	if len(got) != payloadByteCap {
		t.Errorf("capPayload(huge) is %d bytes, want exactly %d", len(got), payloadByteCap)
	}
	if second := capPayload(huge); !bytes.Equal(got, second) {
		t.Errorf("capPayload is not deterministic")
	}
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
