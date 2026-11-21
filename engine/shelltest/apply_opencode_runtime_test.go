package shelltest

// `labdrian apply --target opencode` runs `engine runtime install --target opencode`, the step
// `apply --target codex` runs for codex (apply_codex_runtime_test.go): the OpenCode adapter writes the
// plugins, opencode.json and the AGENTS.md block, none of which a copy of the manifest rows reaches,
// and the longterm-mem step that follows reads the opencode.json the adapter wrote.
//
// The step is tested with a stand-in for the engine that records its call, then the whole command with
// the engine the tests build over a sandbox overlay repository and a temporary home.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// opencodeStepSnippet runs the step the way cmd_apply does, for an overlay checkout in "$1", the engine
// in "$2" and the target in "$3".
const opencodeStepSnippet = `OVERLAY_DIR="$1"; ENGINE_BINARY="$2"; cd "$OVERLAY_DIR"
rc=0; runtime_install_and_report "$3" || rc=$?; echo "exit: $rc"
`

func runRuntimeStep(t *testing.T, target string, withEngine, withSource bool, answer, exit string) (out, record string) {
	t.Helper()
	overlaytest.RequireBash(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	checkout := filepath.Join(dir, "checkout")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if withSource {
		source := map[string]string{"codex": "instructions/owner-rules.md", "opencode": "skills/_shared/minimalism-contract.md"}[target]
		if err := os.MkdirAll(filepath.Dir(filepath.Join(checkout, source)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(checkout, source), []byte("placeholder\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	engineBinary := filepath.Join(dir, "missing-engine")
	if withEngine {
		engineBinary = filepath.Join(dir, "engine")
		overlaytest.WriteTool(t, engineBinary, recordingEngine)
	}
	recordFile := filepath.Join(dir, "record")
	shell := overlaytest.Shell{
		Env: []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "RECORD=" + recordFile, "ANSWER=" + answer, "EXIT=" + exit},
		Dir: dir,
	}
	out, err := overlaytest.Source(t, overlaytest.Script(t), shell, opencodeStepSnippet, checkout, engineBinary, target)
	if err != nil {
		t.Fatalf("the step failed to run: %v\n%s", err, out)
	}
	data, _ := os.ReadFile(recordFile)
	return out, strings.ReplaceAll(strings.ReplaceAll(string(data), checkout, "CHECKOUT"), home, "HOME")
}

func TestApplyOpencodeRunsTheOpencodeRuntimeInstallAgainstTheCheckout(t *testing.T) {
	out, record := runRuntimeStep(t, "opencode", true, true, "opencode install supported", "0")
	if want := "runtime install --target opencode|LABDRIAN_OVERLAY_DIR=CHECKOUT|HOME=HOME|CWD=CHECKOUT\n"; record != want {
		t.Errorf("the engine was called %q, want exactly %q", record, want)
	}
	if want := "  opencode install supported\nexit: 0\n"; out != want {
		t.Errorf("want the engine's answer shown indented and the step to succeed, got:\n%q, want %q", out, want)
	}
}

func TestApplyOpencodeReportsAFailedOpencodeRuntimeInstall(t *testing.T) {
	out, record := runRuntimeStep(t, "opencode", true, true, "line one\nline two", "1")
	if !strings.Contains(record, "runtime install --target opencode") {
		t.Errorf("the engine was not called: %q", record)
	}
	if want := "  line one\n  line two\nexit: 1\n"; out != want {
		t.Errorf("a failed install must show every line of its answer indented and make the step fail, got:\n%q, want %q", out, want)
	}
}

func TestApplyOpencodeWithoutAnEngineBinaryNamesTheRecoveryAndDoesNotFail(t *testing.T) {
	out, record := runRuntimeStep(t, "opencode", false, true, "", "0")
	if record != "" {
		t.Errorf("an engine that is not there was called: %q", record)
	}
	for _, want := range []string{"  opencode: engine binary not found at ", "runtime not installed (run 'labdrian install-hooks', then 'labdrian apply --target opencode')", "exit: 0"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in the note, got:\n%s", want, out)
		}
	}
}

// The codex note keeps the words it has always had.
func TestApplyCodexNoteForAMissingEngineKeepsItsWordsInTheGeneralStep(t *testing.T) {
	out, _ := runRuntimeStep(t, "codex", false, true, "", "0")
	if !strings.Contains(out, "  codex: engine binary not found at ") || !strings.Contains(out, "; owner rules not installed (run 'labdrian install-hooks', then 'labdrian apply --target codex')\nexit: 0\n") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

// copyModule puts the module at root/rel of this checkout, less its tests and fixtures, in files.
func copyModule(t *testing.T, files map[string]string, rel string) {
	t.Helper()
	root := overlaytest.RepoRoot(t)
	err := filepath.WalkDir(filepath.Join(root, rel), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		r, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(r)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// opencodeOverlay is a sandbox overlay with one OpenCode agent row, the contracts and the output style the adapter
// reads (copies of this checkout's), and, when withLongtermMem, the longterm-mem module and the row that makes apply install it.
func opencodeOverlay(t *testing.T, home string, withLongtermMem bool) (overlay string, env []string) {
	t.Helper()
	files := map[string]string{
		"overlay.manifest":            "opencode/agents/foreman.md custom opencode-agent\n",
		"opencode/agents/foreman.md":  "---\ndescription: placeholder foreman\nmode: primary\n---\nplaceholder\n",
		"instructions/owner-rules.md": "## Rules\n\n- placeholder rule\n",
	}
	for _, rel := range []string{"skills/_shared/minimalism-contract.md", "skills/_shared/anti-generic-design.md", "skills/_shared/oo-quality-contract.md", "output-styles/labdrian.md"} {
		data, err := os.ReadFile(filepath.Join(overlaytest.RepoRoot(t), filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		files[rel] = string(data)
	}
	if withLongtermMem {
		files["overlay.manifest"] += "longterm-mem/go.mod custom mcp\n"
		for _, module := range []string{"longterm-mem", "archguard", "identity"} {
			copyModule(t, files, module)
		}
	}
	dir, env := overlaytest.NewOverlay(t, home, files)
	if withLongtermMem {
		env = append(env, overlaytest.ToolchainEnv(t)...)
	}
	return dir, env
}

func TestApplyTargetOpencodeInstallsTheRuntimeAndRegistersLongtermMem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	script := overlaytest.Script(t)
	home := t.TempDir()
	overlay, env := opencodeOverlay(t, home, true)
	config := filepath.Join(home, ".config", "opencode")
	env = append(env, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"))

	out, err := overlaytest.Combined(t, script, env, overlay, "apply", "--target", "opencode")
	if err != nil {
		t.Fatalf("apply --target opencode on an empty config root: %v\n%s", err, out)
	}
	for _, rel := range []string{"plugins", "AGENTS.md", "opencode.json", "agents/foreman.md"} {
		if _, err := os.Stat(filepath.Join(config, rel)); err != nil {
			t.Errorf("apply --target opencode left no %s: %v\n%s", rel, err, out)
		}
	}
	if agents, err := os.ReadFile(filepath.Join(config, "AGENTS.md")); err != nil || !strings.Contains(string(agents), "<!-- labdrian:opencode-instructions") {
		t.Errorf("AGENTS.md holds no OpenCode instructions block (%v):\n%s", err, agents)
	}
	data, err := os.ReadFile(filepath.Join(config, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"default_agent": "foreman"`, `"longterm-mem"`} {
		if !strings.Contains(strings.ReplaceAll(string(data), `":"`, `": "`), want) {
			t.Errorf("opencode.json does not hold %s:\n%s", want, data)
		}
	}
	if strings.Contains(out, "register --target opencode failed") {
		t.Errorf("the longterm-mem register failed:\n%s", out)
	}
}

// A runtime install that fails does not stop the deploy of the target: the agents are copied, the
// failure is shown, and apply exits non-zero.
func TestApplyTargetOpencodeWithAFailingRuntimeInstallDeploysThenExitsNonZero(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	script := overlaytest.Script(t)
	home := t.TempDir()
	overlay, env := opencodeOverlay(t, home, false)
	config := filepath.Join(home, ".config", "opencode")
	env = append(env, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"))
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	const broken = "{ not json\n"
	if err := os.WriteFile(filepath.Join(config, "opencode.json"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	res := overlaytest.Run(t, script, env, overlay, "apply", "--target", "opencode")
	if res.Exit == 0 {
		t.Errorf("a failed runtime install must make apply exit non-zero:\n%s\n%s", res.Stdout, res.Stderr)
	}
	if _, err := os.Stat(filepath.Join(config, "agents", "foreman.md")); err != nil {
		t.Errorf("the target was not deployed before the exit: %v", err)
	}
	assertIndentedInstallFailure(t, res.Stderr)
	if got, _ := os.ReadFile(filepath.Join(config, "opencode.json")); string(got) != broken {
		t.Errorf("a broken opencode.json was rewritten: %q", got)
	}
}

// A checkout that holds no minimalism contract (an older main) has nothing the OpenCode adapter can
// install from: the step prints nothing and does not start the engine, as it does for codex with no
// owner rules, so apply's output for such a checkout is what it was.
func TestApplyOpencodeWithACheckoutThatHasNoContractIsSilentAndDoesNotStartTheEngine(t *testing.T) {
	out, record := runRuntimeStep(t, "opencode", true, false, "opencode install supported", "0")
	if record != "" {
		t.Errorf("the engine was called for a checkout with no contract: %q", record)
	}
	if out != "exit: 0\n" {
		t.Errorf("want no output and a step that does not fail, got:\n%q", out)
	}
}

// assertIndentedInstallFailure checks the failure output of `engine runtime install --target opencode`
// in stderr: it opens with the exact line the adapter answers, two spaces in front, and no line of it
// is shown without those two spaces.
func assertIndentedInstallFailure(t *testing.T, stderr string) {
	t.Helper()
	const opening = "  [opencode] install: partial — "
	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, opening) {
			found = true
		} else if strings.Contains(line, "[opencode] install:") {
			t.Errorf("line %d of the install's output is not indented: %q", i+1, line)
		}
	}
	if !found {
		t.Fatalf("stderr has no line that opens the failed install's output with %q:\n%s", opening, stderr)
	}
}

func readState(t *testing.T, home string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".labdrian-overlay", "state.json"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

// A runtime install that failed means the target is not deployed: no state row is recorded for it
// (status keeps saying it was never deployed), longterm-mem is not registered into the opencode.json the
// install could not write (the failure is shown once), apply exits non-zero, and a re-run after the
// cause is fixed deploys, records the target and registers longterm-mem.
func TestApplyTargetOpencodeWithAFailingRuntimeInstallRecordsNoStateAndSkipsTheRegister(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	script := overlaytest.Script(t)
	home := t.TempDir()
	overlay, env := opencodeOverlay(t, home, true)
	config := filepath.Join(home, ".config", "opencode")
	env = append(env, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"))
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	const broken = "{ not json\n"
	if err := os.WriteFile(filepath.Join(config, "opencode.json"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	res := overlaytest.Run(t, script, env, overlay, "apply", "--target", "opencode")
	all := res.Stdout + res.Stderr
	if res.Exit == 0 {
		t.Errorf("a failed runtime install must make apply exit non-zero:\n%s", all)
	}
	if state := readState(t, home); strings.Contains(state, `"opencode"`) {
		t.Errorf("a target whose runtime install failed was recorded as deployed:\n%s", state)
	}
	for _, unwanted := range []string{"longterm-mem: register: opencode", "register --target opencode failed", "per-target state was already written"} {
		if strings.Contains(all, unwanted) {
			t.Errorf("the failure was reported a second time (%q):\n%s", unwanted, all)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(config, "opencode.json")); string(got) != broken {
		t.Errorf("a broken opencode.json was rewritten: %q", got)
	}

	if err := os.Remove(filepath.Join(config, "opencode.json")); err != nil {
		t.Fatal(err)
	}
	again := overlaytest.Run(t, script, env, overlay, "apply", "--target", "opencode")
	if again.Exit != 0 {
		t.Fatalf("the re-run after the cause was fixed failed:\n%s\n%s", again.Stdout, again.Stderr)
	}
	if state := readState(t, home); !strings.Contains(state, `"opencode"`) {
		t.Errorf("the re-run did not record the target:\n%s", state)
	}
	if data, _ := os.ReadFile(filepath.Join(config, "opencode.json")); !strings.Contains(string(data), `"longterm-mem"`) {
		t.Errorf("the re-run did not register longterm-mem:\n%s", data)
	}
}

// A failed runtime install after an earlier successful apply drops the row that apply recorded, so the
// target reads as never deployed instead of deployed at the old version.
func TestApplyTargetOpencodeWithAFailingRuntimeInstallForgetsAnEarlierStateRow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	script := overlaytest.Script(t)
	home := t.TempDir()
	overlay, env := opencodeOverlay(t, home, true)
	config := filepath.Join(home, ".config", "opencode")
	env = append(env, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"))

	first := overlaytest.Run(t, script, env, overlay, "apply", "--target", "opencode")
	if first.Exit != 0 {
		t.Fatalf("the first apply failed:\n%s\n%s", first.Stdout, first.Stderr)
	}
	if state := readState(t, home); !strings.Contains(state, `"opencode"`) {
		t.Fatalf("the first apply did not record the target:\n%s", state)
	}

	if err := os.WriteFile(filepath.Join(config, "opencode.json"), []byte("{ not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := overlaytest.Run(t, script, env, overlay, "apply", "--target", "opencode")
	if res.Exit == 0 {
		t.Errorf("a failed runtime install must make apply exit non-zero:\n%s\n%s", res.Stdout, res.Stderr)
	}
	if state := readState(t, home); strings.Contains(state, `"opencode"`) {
		t.Errorf("the row of the earlier apply still claims the target is deployed:\n%s", state)
	}
}

// Codex is as it was: a failed runtime install still leaves its state row (the install runs after it)
// and apply exits non-zero.
func TestApplyTargetCodexWithAFailingRuntimeInstallStillRecordsItsState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	script := overlaytest.Script(t)
	home := t.TempDir()
	overlay, env := opencodeOverlay(t, home, false)
	codexHome := filepath.Join(home, ".codex")
	env = append(env, "CODEX_HOME="+codexHome)
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	// An opening marker with no closing one is a broken block: the adapter leaves the file and reports partial.
	if err := os.WriteFile(filepath.Join(codexHome, "AGENTS.md"), []byte("<!-- labdrian:owner-rules -->\nbroken\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := overlaytest.Run(t, script, env, overlay, "apply", "--target", "codex")
	if res.Exit == 0 {
		t.Errorf("a failed runtime install must make apply exit non-zero:\n%s\n%s", res.Stdout, res.Stderr)
	}
	if state := readState(t, home); !strings.Contains(state, `"codex"`) {
		t.Errorf("codex's state row was not recorded:\n%s", state)
	}
}
