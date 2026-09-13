package runtime_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// piFixtureOverlay writes a minimal overlay tree (one pi-targeted skill,
// agents/GADU.md, a matching registry) and returns its root and registry
// path, for exercising PiAdapter's pipkg wiring (task 2.4).
func piFixtureOverlay(t *testing.T) (overlayRoot, registryPath string) {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nbody\n")
	mustWrite(t, filepath.Join(root, "agents", "GADU.md"), "---\nname: GADU\ndescription: test agent\ntools: '*'\n---\nbody\n")
	mustWrite(t, filepath.Join(root, "skills", "_shared", "minimalism-contract.md"),
		"---\napplies_to_phases: [sdd-tasks, sdd-apply]\nexcluded_phases: [sdd-verify]\ninjection_point: \"## Skills to load before work\"\n---\nbody\n")
	mustWrite(t, filepath.Join(root, "skills", "_shared", "anti-generic-design.md"),
		"---\napplies_to_phases: [sdd-tasks, sdd-apply]\nexcluded_phases: [sdd-verify]\ninjection_point: \"## Skills to load before work\"\n---\nbody\n")
	registry := `version: "1"
skills:
  - id: pi-skill
    path: pi-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - pi
    lifecycle:
      updateStrategy: overlay-only
`
	regPath := filepath.Join(root, "skills.registry.yaml")
	mustWrite(t, regPath, registry)
	return root, regPath
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeStubPiScript writes a fake `pi` APPENDING every argv it received
// (one arg per line, invocations separated by a blank line) to
// recorderPath, then exits 0 — Install now makes up to two `pi`
// invocations (package install, then the Subagents extension install), so
// a single-shot overwrite would lose the first one. Every test exercising
// Install/Uninstall MUST set LABDRIAN_PI_BIN to one via t.Setenv — a
// developer machine may have a real `pi` on PATH, and an unstubbed test
// would shell out to it and mutate the real ~/.pi/agent/settings.json.
func writeStubPiScript(t *testing.T, recorderPath string) string {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "pi-stub.sh")
	quoted := "'" + strings.ReplaceAll(recorderPath, "'", `'\''`) + "'"
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + quoted + "\nprintf '\\n' >> " + quoted + "\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("write stub pi script: %v", err)
	}
	return scriptPath
}

// TestPiAdapter_ApplyInstallSyncCheck_WiredToPipkg pins task 2.4: with real
// overlay/registry/dest paths, Apply/Install build the package via pipkg and
// SyncCheck reports it as drift-free right after.
func TestPiAdapter_ApplyInstallSyncCheck_WiredToPipkg(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // Install now also links ~/.pi/agent/agents/GADU.md
	overlayRoot, registryPath := piFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	recorder := filepath.Join(t.TempDir(), "argv.txt")
	t.Setenv("LABDRIAN_PI_BIN", writeStubPiScript(t, recorder))

	adapter := engineRuntime.NewPiAdapterWithPaths(overlayRoot, registryPath, destDir)

	applyResult := adapter.Apply()
	if applyResult.Status == engineRuntime.CapabilityUnsupported {
		t.Fatalf("Apply with real paths must not be unsupported, got: %s", applyResult)
	}
	if _, err := os.Stat(filepath.Join(destDir, "package.json")); err != nil {
		t.Errorf("Apply must build the package: %v", err)
	}

	installResult := adapter.Install()
	if installResult.Status == engineRuntime.CapabilityUnsupported {
		t.Fatalf("Install with real paths must not be unsupported, got: %s", installResult)
	}

	syncResult := adapter.SyncCheck()
	if syncResult.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("SyncCheck right after a build must report supported (no drift), got: %s", syncResult)
	}
}

// TestPiAdapter_ApplyWithoutOverlayRoot_StaysHonestlyUnsupported guards the
// zero-arg NewPiAdapter() path (used by NewFoundationAdapter(TargetPi) and
// exercised by TestExpandTarget_Pi): with OVERLAY_DIR unset, wiring the
// pipkg calls must not fabricate success.
func TestPiAdapter_ApplyWithoutOverlayRoot_StaysHonestlyUnsupported(t *testing.T) {
	adapter := engineRuntime.NewPiAdapterWithPaths("", "", t.TempDir())
	for _, result := range []engineRuntime.LifecycleResult{adapter.Apply(), adapter.Install(), adapter.SyncCheck()} {
		if result.Status != engineRuntime.CapabilityUnsupported {
			t.Fatalf("%s with empty overlayRoot must stay unsupported, got: %s", result.Action, result)
		}
	}
}

// ---------------------------------------------------------------------------
// pi-contract-gate (slice 3, R-004/R-007): node-driven tests against the
// real engine/pipkg/labdrian-gate.ts source, copied byte-identical to a
// scratch .mjs fixture (design A5) so the SAME bytes run under Node here and
// under Pi's jiti loader in production. Skipped when node is unavailable
// (precedent: opencode_test.go:472).
// ---------------------------------------------------------------------------

const gateBasePrompt = "line 1\nline 2"
const gateHeader = "## Skills to load before work"

const validMinimalismFixture = "---\napplies_to_phases: [sdd-tasks, sdd-apply]\n" +
	"excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]\n" +
	"injection_point: \"## Skills to load before work\"\n---\n# Minimalism\nbody\n"

const validAntiGenericFixture = "---\napplies_to_phases: [sdd-tasks, sdd-apply]\n" +
	"excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]\n" +
	"injection_point: \"## Skills to load before work\"\n---\n# Anti-Generic\nbody\n"

// malformedMinimalismFixture omits the required "[...]" bracket form for
// applies_to_phases -- parseStrictInlineList must reject it (R-004: malformed
// frontmatter yields no injection for that contract, never a crash).
const malformedMinimalismFixture = "---\napplies_to_phases: sdd-tasks, sdd-apply\n---\nbroken\n"

// writeGatePackage writes a scratch package root with extensions/labdrian-
// gate.mjs (the real embedded source, byte-identical) and the two managed
// contracts under skills/_shared/, and returns the root plus each contract's
// absolute path (the canonical entry lines the gate injects).
func writeGatePackage(t *testing.T, minimalismContent, antiGenericContent string) (root, minimalismPath, antiGenericPath string) {
	t.Helper()
	root = t.TempDir()
	mustWrite(t, filepath.Join(root, "extensions", "labdrian-gate.mjs"), pipkg.GateExtensionSource())
	minimalismPath = filepath.Join(root, "skills", "_shared", "minimalism-contract.md")
	antiGenericPath = filepath.Join(root, "skills", "_shared", "anti-generic-design.md")
	mustWrite(t, minimalismPath, minimalismContent)
	mustWrite(t, antiGenericPath, antiGenericContent)
	return root, minimalismPath, antiGenericPath
}

// runNodeScript requires node (skipping the test when absent) and runs
// script, returning stdout or failing the test on a nonzero exit.
func runNodeScript(t *testing.T, script string) string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable; skipping labdrian-gate.ts behavior test: %v", err)
	}
	scriptPath := filepath.Join(t.TempDir(), "gate-script.mjs")
	mustWrite(t, scriptPath, script)
	out, err := exec.Command("node", scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("node script failed: %v\n%s", err, out)
	}
	return string(out)
}

type gateEventResult struct {
	SystemPrompt *string `json:"systemPrompt"`
}

// TestLabdrianGatePathContainment_RejectsTraversal (task 3.1, F1): the
// exported resolveContractPath helper must reject any relative path that
// escapes the package root -- via ".." traversal or an absolute input --
// while still resolving a well-formed relative path inside root.
func TestLabdrianGatePathContainment_RejectsTraversal(t *testing.T) {
	root, minimalismPath, _ := writeGatePackage(t, validMinimalismFixture, validAntiGenericFixture)
	gatePath := filepath.Join(root, "extensions", "labdrian-gate.mjs")

	script := fmt.Sprintf(`
const mod = await import(%q);
const root = %q;
const results = {
  safe: mod.resolveContractPath(root, "skills/_shared/minimalism-contract.md"),
  traversal: mod.resolveContractPath(root, "../../../etc/passwd"),
  absolute: mod.resolveContractPath(root, "/etc/passwd"),
  dotSegment: mod.resolveContractPath(root, "skills/./_shared/minimalism-contract.md"),
};
console.log(JSON.stringify(results));
`, gatePath, root)

	out := runNodeScript(t, script)
	var results struct {
		Safe       *string `json:"safe"`
		Traversal  *string `json:"traversal"`
		Absolute   *string `json:"absolute"`
		DotSegment *string `json:"dotSegment"`
	}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("unmarshal node output %q: %v", out, err)
	}
	if results.Safe == nil || *results.Safe != minimalismPath {
		t.Errorf("resolveContractPath for a contained relative path = %v, want %q", results.Safe, minimalismPath)
	}
	if results.Traversal != nil {
		t.Errorf("resolveContractPath must reject a \"..\" traversal, got %v", *results.Traversal)
	}
	if results.Absolute != nil {
		t.Errorf("resolveContractPath must reject an absolute input, got %v", *results.Absolute)
	}
	if results.DotSegment != nil {
		t.Errorf("resolveContractPath must reject a \".\" segment, got %v", *results.DotSegment)
	}
}

// TestLabdrianGateInjectsPathLine_SddTasksSddApply (task 3.2, R-004): the
// default before_agent_start handler injects both managed contracts' bare
// path lines, exactly matching the Go-side oracle (runtime.CanonicalEntry /
// runtime.InjectPrompt for the same contracts and header -- design C5), only
// for sdd-tasks/sdd-apply; is idempotent on a second call; is a no-op for
// every other or unnamed agent; and a malformed contract's frontmatter
// silently drops only that contract, never the other, and never throws.
func TestLabdrianGateInjectsPathLine_SddTasksSddApply(t *testing.T) {
	root, minimalismPath, antiGenericPath := writeGatePackage(t, validMinimalismFixture, validAntiGenericFixture)
	gatePath := filepath.Join(root, "extensions", "labdrian-gate.mjs")

	malformedRoot, malformedMinimalismPath, malformedAntiGenericPath := writeGatePackage(t, malformedMinimalismFixture, validAntiGenericFixture)
	malformedGatePath := filepath.Join(malformedRoot, "extensions", "labdrian-gate.mjs")

	script := fmt.Sprintf(`
function fakePi() {
  const handlers = {};
  return { on(name, fn) { handlers[name] = fn; }, handlers };
}

async function fireAll(modulePath, base) {
  const mod = await import(modulePath);
  const pi = fakePi();
  mod.default(pi);
  const handler = pi.handlers["before_agent_start"];
  const results = {};
  results.tasks = await handler({ agentName: "sdd-tasks", systemPrompt: base });
  results.apply = await handler({ agentName: "sdd-apply", systemPrompt: base });
  results.tasksAgain = await handler({ agentName: "sdd-tasks", systemPrompt: results.tasks.systemPrompt });
  results.explore = await handler({ agentName: "sdd-explore", systemPrompt: base });
  results.unnamed = await handler({ systemPrompt: base });
  return results;
}

const base = %q;
const valid = await fireAll(%q, base);
const malformed = await fireAll(%q, base);
console.log(JSON.stringify({ valid, malformed }));
`, gateBasePrompt, gatePath, malformedGatePath)

	out := runNodeScript(t, script)
	var results struct {
		Valid     map[string]gateEventResult `json:"valid"`
		Malformed map[string]gateEventResult `json:"malformed"`
	}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("unmarshal node output %q: %v", out, err)
	}

	expected := gateBasePrompt
	expected = engineRuntime.InjectPrompt(expected, minimalismPath, gateHeader)
	expected = engineRuntime.InjectPrompt(expected, antiGenericPath, gateHeader)

	for _, key := range []string{"tasks", "apply", "tasksAgain"} {
		got := results.Valid[key]
		if got.SystemPrompt == nil || *got.SystemPrompt != expected {
			t.Errorf("valid[%s].systemPrompt = %v, want %q", key, got.SystemPrompt, expected)
		}
	}
	for _, key := range []string{"explore", "unnamed"} {
		if results.Valid[key].SystemPrompt != nil {
			t.Errorf("valid[%s] must be a no-op, got systemPrompt %v", key, *results.Valid[key].SystemPrompt)
		}
	}

	malformedEntry := engineRuntime.CanonicalEntry(malformedMinimalismPath)
	otherEntry := engineRuntime.CanonicalEntry(malformedAntiGenericPath)
	got := results.Malformed["tasks"]
	if got.SystemPrompt == nil {
		t.Fatal("malformed frontmatter must not abort the handler (fail-safe, never throw)")
	}
	if !engineRuntime.HasExactEntry(*got.SystemPrompt, malformedAntiGenericPath) {
		t.Errorf("valid contract %q must still be injected alongside a malformed sibling, got:\n%s", otherEntry, *got.SystemPrompt)
	}
	if engineRuntime.HasExactEntry(*got.SystemPrompt, malformedMinimalismPath) {
		t.Errorf("contract with malformed frontmatter %q must never be injected, got:\n%s", malformedEntry, *got.SystemPrompt)
	}
}

// TestLabdrianGateChainsAfterGentlePi (task 3.5, design A4): the gate reads
// event.systemPrompt (already carrying gentle-pi's own before_agent_start
// contribution) and composes on top of it -- neither handler's output is
// clobbered.
func TestLabdrianGateChainsAfterGentlePi(t *testing.T) {
	root, minimalismPath, antiGenericPath := writeGatePackage(t, validMinimalismFixture, validAntiGenericFixture)
	gatePath := filepath.Join(root, "extensions", "labdrian-gate.mjs")

	const gentlePiOwnPrompt = "line 1\n\ngentle-pi's own preflight prompt fragment\nline 2"

	script := fmt.Sprintf(`
const mod = await import(%q);
function fakePi() {
  const handlers = {};
  return { on(name, fn) { handlers[name] = fn; }, handlers };
}
const pi = fakePi();
mod.default(pi);
const handler = pi.handlers["before_agent_start"];
const result = await handler({ agentName: "sdd-apply", systemPrompt: %q });
console.log(JSON.stringify(result));
`, gatePath, gentlePiOwnPrompt)

	out := runNodeScript(t, script)
	var result gateEventResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal node output %q: %v", out, err)
	}
	if result.SystemPrompt == nil {
		t.Fatal("expected a systemPrompt in the composition result")
	}
	got := *result.SystemPrompt
	if !strings.Contains(got, "gentle-pi's own preflight prompt fragment") {
		t.Errorf("gate must preserve gentle-pi's own contribution, got:\n%s", got)
	}
	if !engineRuntime.HasExactEntry(got, minimalismPath) || !engineRuntime.HasExactEntry(got, antiGenericPath) {
		t.Errorf("gate must still inject both contract path lines on top of gentle-pi's prompt, got:\n%s", got)
	}
}

// buildPiPackage builds destDir via pipkg.Build, failing the test on error.
func buildPiPackage(t *testing.T, overlayRoot, registryPath, destDir string) {
	t.Helper()
	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("pipkg.Build: %v", err)
	}
}

// newBuiltPiAdapterWithStub builds a real package at a fresh destDir and
// returns an adapter for it plus the destDir and a writeStubPiScript
// recorder path already set as LABDRIAN_PI_BIN. It also isolates HOME to a
// fresh t.TempDir(): Install/Uninstall now touch
// ~/.pi/agent/agents/GADU.md (R-013/R-016), and sharing TestMain's one
// process-wide HOME across every caller of this helper would let one
// test's GADU link leak into the next test's assertions.
func newBuiltPiAdapterWithStub(t *testing.T) (adapter engineRuntime.PiAdapter, destDir, recorder string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	overlayRoot, registryPath := piFixtureOverlay(t)
	destDir = filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	recorder = filepath.Join(t.TempDir(), "argv.txt")
	t.Setenv("LABDRIAN_PI_BIN", writeStubPiScript(t, recorder))
	return engineRuntime.NewPiAdapterWithPaths(overlayRoot, registryPath, destDir), destDir, recorder
}

// readRecordedInvocations reads a writeStubPiScript recorder file and
// returns one []string per `pi` invocation (argv elements in order).
func readRecordedInvocations(t *testing.T, recorderPath string) [][]string {
	t.Helper()
	data, err := os.ReadFile(recorderPath)
	if err != nil {
		return nil
	}
	var invocations [][]string
	for _, block := range strings.Split(string(data), "\n\n") {
		block = strings.TrimRight(block, "\n")
		if block == "" {
			continue
		}
		invocations = append(invocations, strings.Split(block, "\n"))
	}
	return invocations
}

// readRecordedArgv returns the FIRST `pi` invocation's argv (one element
// per line) -- Install may make a second invocation (the Subagents
// extension install) that most existing single-invocation assertions don't
// care about.
func readRecordedArgv(t *testing.T, recorderPath string) []string {
	t.Helper()
	invocations := readRecordedInvocations(t, recorderPath)
	if len(invocations) == 0 {
		return nil
	}
	return invocations[0]
}

// readAllRecordedTokens flattens every argv element across every `pi`
// invocation, for assertions about whether a particular call happened at
// all rather than about invocation order.
func readAllRecordedTokens(t *testing.T, recorderPath string) []string {
	t.Helper()
	var all []string
	for _, invocation := range readRecordedInvocations(t, recorderPath) {
		all = append(all, invocation...)
	}
	return all
}

// TestPiAdapter_InstallNoShellInjection (task 5.1): a destDir containing
// shell metacharacters must reach `pi install` byte-for-byte, never
// interpreted.
func TestPiAdapter_InstallNoShellInjection(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // Install now also links ~/.pi/agent/agents/GADU.md
	overlayRoot, registryPath := piFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), `labdrian-pi; $(touch injected); \`)
	buildPiPackage(t, overlayRoot, registryPath, destDir)

	recorder := filepath.Join(t.TempDir(), "argv.txt")
	t.Setenv("LABDRIAN_PI_BIN", writeStubPiScript(t, recorder))

	adapter := engineRuntime.NewPiAdapterWithPaths(overlayRoot, registryPath, destDir)
	result := adapter.Install()
	if result.Status == engineRuntime.CapabilityUnsupported {
		t.Fatalf("Install with a stub pi on PATH must not be unsupported, got: %s", result)
	}

	got := readRecordedArgv(t, recorder)
	if len(got) != 2 || got[0] != "install" || got[1] != destDir {
		t.Fatalf("recorded argv = %#v, want [\"install\", %q] (fixed argv, no shell interpretation)", got, destDir)
	}
	if _, err := os.Stat("injected"); err == nil {
		t.Fatal("destDir's shell metacharacters were interpreted -- a file named \"injected\" was created")
	}
}

// TestPiAdapter_StatusPartialOnUnprovenEntry (task 5.2): built but unlisted
// must report partial, naming the unproven entry.
func TestPiAdapter_StatusPartialOnUnprovenEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no real ~/.pi/agent/settings.json to read
	overlayRoot, registryPath := piFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)

	adapter := engineRuntime.NewPiAdapterWithPaths(overlayRoot, registryPath, destDir)
	result := adapter.Status()
	if result.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Status on a built-but-unlisted package = %s, want partial", result)
	}
	if !strings.Contains(result.Message, "listed in ~/.pi/agent/settings.json") {
		t.Fatalf("Status message should name the unproven listing entry, got %q", result.Message)
	}
}

// writePiSettingsListing writes a scratch ~/.pi/agent/settings.json that
// lists destDir as an installed package, so isPiPackageListed proves the
// "listed" entry (mirrors what a real `pi install <destDir>` would do).
func writePiSettingsListing(t *testing.T, destDir string) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Fatalf("os.UserHomeDir: %v", err)
	}
	settingsPath := filepath.Join(home, ".pi", "agent", "settings.json")
	raw, err := json.Marshal(struct {
		Packages []string `json:"packages"`
	}{Packages: []string{destDir}})
	if err != nil {
		t.Fatalf("marshal settings.json: %v", err)
	}
	mustWrite(t, settingsPath, string(raw))
}

// writeGaduLinkCurrent symlinks ~/.pi/agent/agents/GADU.md -> destDir's
// agents/GADU.md directly (bypassing Install/linkGaduAgent), for status
// tests that need the "current" link state without exercising Install.
func writeGaduLinkCurrent(t *testing.T, home, destDir string) {
	t.Helper()
	linkPath := filepath.Join(home, ".pi", "agent", "agents", "GADU.md")
	target := filepath.Join(destDir, "agents", "GADU.md")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(linkPath), err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("symlink %s -> %s: %v", linkPath, target, err)
	}
}

// TestPiAdapter_StatusAcceptsRelativePackageListing: a real `pi install
// <abs path>` records the package RELATIVE to ~/.pi/agent/ (observed on
// Pi 0.85.1: "../../.labdrian-overlay/pi/labdrian-pi"); the docs state
// relative entries resolve against the settings file. The listing probe
// must resolve entries the same way instead of comparing raw strings.
func TestPiAdapter_StatusAcceptsRelativePackageListing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlay(t)
	destDir := filepath.Join(home, ".labdrian-overlay", "pi", "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	writePiMcpRegistration(t, destDir, true)
	writeGaduLinkCurrent(t, home, destDir)
	settingsPath := filepath.Join(home, ".pi", "agent", "settings.json")
	adapter := engineRuntime.NewPiAdapterWithPaths(overlayRoot, registryPath, destDir)

	mustWrite(t, settingsPath, `{"packages":["../../.labdrian-overlay/pi/labdrian-pi","npm:pi-subagents-j0k3r"]}`)
	if result := adapter.Status(); result.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Status with a relative listing resolved against ~/.pi/agent = %s, want supported", result)
	}
	mustWrite(t, settingsPath, `{"packages":["../../elsewhere/labdrian-pi","npm:pi-subagents-j0k3r"]}`)
	if result := adapter.Status(); result.Status == engineRuntime.CapabilitySupported {
		t.Fatalf("Status must not accept a relative listing that resolves elsewhere, got %s", result)
	}
}

// writePiMcpRegistration writes destDir/mcp.json with (or without) the
// longterm-mem MCP entry a real `longterm-mem register --target pi` call
// would add.
func writePiMcpRegistration(t *testing.T, destDir string, registered bool) {
	t.Helper()
	content := `{"mcpServers": {}}`
	if registered {
		content = `{"mcpServers": {"longterm-mem": {"type": "stdio", "command": "/opt/labdrian-overlay/bin/longterm-mem", "args": ["mcp"]}}}`
	}
	mustWrite(t, filepath.Join(destDir, "mcp.json"), content)
}

// TestPiAdapter_StatusTriangulatesAllOwnedEntries (C-01 remediation,
// extended by R-015 with two more owned entries): Status names five owned
// entries -- built+in-sync, listed in settings.json, longterm-mem
// registered in mcp.json, the Subagents extension installed, and GADU.md
// linked. It must never report supported while any one of them is
// unproven, and must report supported only once all five are proven.
func TestPiAdapter_StatusTriangulatesAllOwnedEntries(t *testing.T) {
	cases := []struct {
		name               string
		listed             bool
		mcpRegistered      bool
		subagentsInstalled bool
		gaduLinked         bool
		wantStatus         engineRuntime.CapabilityStatus
		wantContains       string
	}{
		{
			name:          "listed but MCP unregistered stays partial and names the register command",
			listed:        true,
			mcpRegistered: false,
			wantStatus:    engineRuntime.CapabilityPartial,
			wantContains:  "longterm-mem register --target pi",
		},
		{
			name:          "unlisted and MCP unregistered stays partial",
			listed:        false,
			mcpRegistered: false,
			wantStatus:    engineRuntime.CapabilityPartial,
			wantContains:  "listed in ~/.pi/agent/settings.json",
		},
		{
			name:               "all five entries proven reports supported",
			listed:             true,
			mcpRegistered:      true,
			subagentsInstalled: true,
			gaduLinked:         true,
			wantStatus:         engineRuntime.CapabilitySupported,
			wantContains:       "longterm-mem is registered in its mcp.json",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			overlayRoot, registryPath := piFixtureOverlay(t)
			destDir := filepath.Join(t.TempDir(), "labdrian-pi")
			buildPiPackage(t, overlayRoot, registryPath, destDir)
			var packages []string
			if c.listed {
				packages = append(packages, destDir)
			}
			if c.subagentsInstalled {
				packages = append(packages, "npm:pi-subagents-j0k3r")
			}
			writePiSettingsPackages(t, home, packages)
			writePiMcpRegistration(t, destDir, c.mcpRegistered)
			if c.gaduLinked {
				writeGaduLinkCurrent(t, home, destDir)
			}

			adapter := engineRuntime.NewPiAdapterWithPaths(overlayRoot, registryPath, destDir)
			result := adapter.Status()
			if result.Status != c.wantStatus {
				t.Fatalf("Status = %s, want %s", result, c.wantStatus)
			}
			if !strings.Contains(result.Message, c.wantContains) {
				t.Fatalf("Status message = %q, want it to contain %q", result.Message, c.wantContains)
			}
		})
	}
}

// TestPiAdapter_StatusDisclosesNoExtensionsNoSkills (task 5.2): always
// discloses the --no-extensions/--no-skills bypass and their -ne/-ns aliases.
func TestPiAdapter_StatusDisclosesNoExtensionsNoSkills(t *testing.T) {
	adapter := engineRuntime.NewPiAdapterWithPaths("", "", filepath.Join(t.TempDir(), "labdrian-pi"))
	result := adapter.Status()
	if !strings.Contains(result.Message, "--no-extensions") || !strings.Contains(result.Message, "--no-skills") {
		t.Fatalf("Status must always disclose --no-extensions/--no-skills, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "-ns") || !strings.Contains(result.Message, "-ne") {
		t.Fatalf("disclosure must name the -ne/-ns aliases pi --help documents, got %q", result.Message)
	}
}

// TestPiAdapter_UninstallUsesRemoveNotUninstall (task 5.2): `pi uninstall`
// does not exist -- must run `pi remove <path>`.
func TestPiAdapter_UninstallUsesRemoveNotUninstall(t *testing.T) {
	adapter, destDir, recorder := newBuiltPiAdapterWithStub(t)
	result := adapter.Uninstall()
	if result.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Uninstall with a stub pi on PATH must report supported, got: %s", result)
	}

	got := readRecordedArgv(t, recorder)
	if len(got) != 2 || got[0] != "remove" || got[1] != destDir {
		t.Fatalf("recorded argv = %#v, want [\"remove\", %q]", got, destDir)
	}
	if _, err := os.Stat(destDir); !os.IsNotExist(err) {
		t.Fatalf("Uninstall should remove the built package directory, stat err = %v", err)
	}
}

// TestPiAdapter_UninstallNeverTouchesGentlePiFiles (task 5.2): settings.json
// and mcp.json stay byte-identical -- only `pi remove` may touch them.
func TestPiAdapter_UninstallNeverTouchesGentlePiFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	piAgentDir := filepath.Join(home, ".pi", "agent")
	settingsPath := filepath.Join(piAgentDir, "settings.json")
	mcpPath := filepath.Join(piAgentDir, "mcp.json")
	settingsContent := []byte(`{"packages":["npm:gentle-pi"]}`)
	mcpContent := []byte(`{"mcpServers":{"other":{}}}`)
	mustWrite(t, settingsPath, string(settingsContent))
	mustWrite(t, mcpPath, string(mcpContent))

	adapter, _, _ := newBuiltPiAdapterWithStub(t)
	if result := adapter.Uninstall(); result.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Uninstall = %s, want supported", result)
	}

	assertFileUnchanged(t, settingsPath, settingsContent)
	assertFileUnchanged(t, mcpPath, mcpContent)
}

// assertFileUnchanged fails the test if path's bytes differ from want.
func assertFileUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != string(want) {
		t.Errorf("%s changed: got %q, want %q", path, got, want)
	}
}

// ---------------------------------------------------------------------------
// gadu-pi-subagent (Phase 5, R-012..R-016): the Pi Subagents extension probe/
// install, the overlay-owned ~/.pi/agent/agents/GADU.md symlink, its status
// entries, and its selective uninstall.
// ---------------------------------------------------------------------------

// writePiSettingsPackages writes a scratch ~/.pi/agent/settings.json listing
// exactly the given packages entries.
func writePiSettingsPackages(t *testing.T, home string, packages []string) {
	t.Helper()
	raw, err := json.Marshal(struct {
		Packages []string `json:"packages"`
	}{Packages: packages})
	if err != nil {
		t.Fatalf("marshal settings.json: %v", err)
	}
	mustWrite(t, filepath.Join(home, ".pi", "agent", "settings.json"), string(raw))
}

// TestSubagentsExtension_InstallWhenAbsent (task 5.1): neither accepted
// package name is listed -- Install must run `pi install
// npm:pi-subagents-j0k3r` via the stub, with fixed argv, and disclose the
// external dependency in its returned message.
func TestSubagentsExtension_InstallWhenAbsent(t *testing.T) {
	adapter, _, recorder := newBuiltPiAdapterWithStub(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	writePiSettingsPackages(t, home, nil)

	result := adapter.Install()
	if result.Status == engineRuntime.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}
	if !strings.Contains(result.Message, "third-party Pi extension pi-subagents-j0k3r (npm)") {
		t.Fatalf("Install message must disclose the external Subagents extension dependency, got %q", result.Message)
	}

	invocations := readRecordedInvocations(t, recorder)
	if len(invocations) != 2 {
		t.Fatalf("expected exactly 2 pi invocations (package install + extension install), got %#v", invocations)
	}
	got := invocations[1]
	if len(got) != 2 || got[0] != "install" || got[1] != "npm:pi-subagents-j0k3r" {
		t.Fatalf("second invocation argv = %#v, want [\"install\", \"npm:pi-subagents-j0k3r\"]", got)
	}
}

// TestSubagentsExtension_Noop (task 5.1): either accepted package name,
// with or without a version suffix, is treated as already satisfied --
// Install must never run a second, redundant `pi install` for it.
func TestSubagentsExtension_Noop(t *testing.T) {
	cases := []struct {
		name    string
		listing string
	}{
		{"canonical package, no version", "npm:pi-subagents-j0k3r"},
		{"canonical package with version", "npm:pi-subagents-j0k3r@1.5.15"},
		{"alternate package name", "npm:pi-subagents"},
		{"alternate package name with version", "npm:pi-subagents@2.0.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			adapter, _, recorder := newBuiltPiAdapterWithStub(t)
			home, err := os.UserHomeDir()
			if err != nil {
				t.Fatalf("UserHomeDir: %v", err)
			}
			writePiSettingsPackages(t, home, []string{c.listing})

			result := adapter.Install()
			if result.Status == engineRuntime.CapabilityUnsupported {
				t.Fatalf("Install must not be unsupported, got: %s", result)
			}

			for _, argv := range readAllRecordedTokens(t, recorder) {
				if strings.Contains(argv, "pi-subagents") {
					t.Fatalf("Install must not reinstall an already-satisfied Subagents extension, recorded argv contained %q", argv)
				}
			}
		})
	}
}

// TestSubagentsExtension_SkipEnv (task 5.1): LABDRIAN_PI_SKIP_SUBAGENTS=1
// skips the extension probe/install entirely, even when absent.
func TestSubagentsExtension_SkipEnv(t *testing.T) {
	t.Setenv("LABDRIAN_PI_SKIP_SUBAGENTS", "1")
	adapter, _, recorder := newBuiltPiAdapterWithStub(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	writePiSettingsPackages(t, home, nil)

	result := adapter.Install()
	if result.Status == engineRuntime.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}
	if !strings.Contains(result.Message, "LABDRIAN_PI_SKIP_SUBAGENTS=1") {
		t.Fatalf("Install must disclose the skip, got %q", result.Message)
	}
	for _, argv := range readAllRecordedTokens(t, recorder) {
		if strings.Contains(argv, "pi-subagents") {
			t.Fatalf("skip env must prevent any pi-subagents install call, recorded argv contained %q", argv)
		}
	}
}

// gaduLinkFixture builds a real package (with agents/GADU.md) at a fresh
// destDir under a scratch HOME, returning the paths gaduLinkState needs.
func gaduLinkFixture(t *testing.T) (home, destDir, linkPath, targetPath string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlay(t)
	destDir = filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	linkPath = filepath.Join(home, ".pi", "agent", "agents", "GADU.md")
	targetPath = filepath.Join(destDir, "agents", "GADU.md")
	return home, destDir, linkPath, targetPath
}

// TestGaduLinkState_Matrix (task 5.2): missing/current/stale/conflict, both
// for a plain conflicting file and a symlink pointing elsewhere.
func TestGaduLinkState_Matrix(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		_, _, linkPath, targetPath := gaduLinkFixture(t)
		if got := engineRuntime.GaduLinkStateForTest(linkPath, targetPath); got != "missing" {
			t.Fatalf("gaduLinkState = %q, want missing", got)
		}
	})
	t.Run("current", func(t *testing.T) {
		_, _, linkPath, targetPath := gaduLinkFixture(t)
		if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(targetPath, linkPath); err != nil {
			t.Fatal(err)
		}
		if got := engineRuntime.GaduLinkStateForTest(linkPath, targetPath); got != "current" {
			t.Fatalf("gaduLinkState = %q, want current", got)
		}
	})
	t.Run("stale (broken target)", func(t *testing.T) {
		_, _, linkPath, targetPath := gaduLinkFixture(t)
		if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(targetPath, linkPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(targetPath); err != nil {
			t.Fatal(err)
		}
		if got := engineRuntime.GaduLinkStateForTest(linkPath, targetPath); got != "stale" {
			t.Fatalf("gaduLinkState = %q, want stale", got)
		}
	})
	t.Run("conflict (regular file)", func(t *testing.T) {
		_, _, linkPath, targetPath := gaduLinkFixture(t)
		mustWrite(t, linkPath, "gentle-pi's own managed GADU.md\n")
		if got := engineRuntime.GaduLinkStateForTest(linkPath, targetPath); got != "conflict" {
			t.Fatalf("gaduLinkState = %q, want conflict", got)
		}
	})
	t.Run("conflict (symlink elsewhere)", func(t *testing.T) {
		_, _, linkPath, targetPath := gaduLinkFixture(t)
		elsewhere := filepath.Join(t.TempDir(), "elsewhere.md")
		mustWrite(t, elsewhere, "not ours\n")
		if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(elsewhere, linkPath); err != nil {
			t.Fatal(err)
		}
		if got := engineRuntime.GaduLinkStateForTest(linkPath, targetPath); got != "conflict" {
			t.Fatalf("gaduLinkState = %q, want conflict", got)
		}
	})
}

// TestGaduLink_SurvivesOverwrite (task 5.3): a gentle-pi-style overwrite of
// its OWN managed files in the same directory must never touch our symlink
// -- ownership is per-file (Readlink equality), not directory-scoped.
func TestGaduLink_SurvivesOverwrite(t *testing.T) {
	adapter, destDir, _ := newBuiltPiAdapterWithStub(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if result := adapter.Install(); result.Status == engineRuntime.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}
	linkPath := filepath.Join(home, ".pi", "agent", "agents", "GADU.md")
	targetPath := filepath.Join(destDir, "agents", "GADU.md")
	before, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("expected a symlink at %s after Install: %v", linkPath, err)
	}

	// gentle-pi rewrites its OWN managed agent file in the same directory.
	otherAgent := filepath.Join(home, ".pi", "agent", "agents", "some-managed-agent.md")
	mustWrite(t, otherAgent, "first version\n")
	mustWrite(t, otherAgent, "gentle-pi rebuilt this\n")

	after, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("GADU.md link must still exist after a sibling rebuild: %v", err)
	}
	if before != after || after != targetPath {
		t.Fatalf("GADU.md link changed: before=%q after=%q want=%q", before, after, targetPath)
	}
}

// TestUninstall_OwnedLinkOnly (task 5.4): uninstall removes only our
// symlink, then runs `pi remove`; a foreign entry at the same path, and the
// Subagents extension package, are never touched.
func TestUninstall_OwnedLinkOnly(t *testing.T) {
	adapter, destDir, recorder := newBuiltPiAdapterWithStub(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if result := adapter.Install(); result.Status == engineRuntime.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}
	linkPath := filepath.Join(home, ".pi", "agent", "agents", "GADU.md")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("expected GADU.md link to exist after Install: %v", err)
	}

	result := adapter.Uninstall()
	if result.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Uninstall = %s, want supported", result)
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("GADU.md link should be removed after Uninstall, stat err = %v", err)
	}

	// Install already made two `pi` invocations; Uninstall's `pi remove`
	// is the LAST one recorded.
	invocations := readRecordedInvocations(t, recorder)
	got := invocations[len(invocations)-1]
	if len(got) != 2 || got[0] != "remove" || got[1] != destDir {
		t.Fatalf("last recorded invocation = %#v, want [\"remove\", %q]", got, destDir)
	}
}

// TestUninstall_LeavesForeignGaduFileUntouched (task 5.4): a pre-existing
// non-owned entry at the link path must never be removed by Uninstall.
func TestUninstall_LeavesForeignGaduFileUntouched(t *testing.T) {
	adapter, _, _ := newBuiltPiAdapterWithStub(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	linkPath := filepath.Join(home, ".pi", "agent", "agents", "GADU.md")
	foreignContent := []byte("gentle-pi's own GADU.md\n")
	mustWrite(t, linkPath, string(foreignContent))

	if result := adapter.Uninstall(); result.Status == engineRuntime.CapabilityUnsupported {
		t.Fatalf("Uninstall must not be unsupported, got: %s", result)
	}
	assertFileUnchanged(t, linkPath, foreignContent)
}

// TestInstall_WiresSubagentsExtensionAndGaduLink (task 5.9): Install wires
// BOTH the extension probe/install and the GADU link creation, and Status
// (task 5.8) reports both as newly-proven entries afterward.
func TestInstall_WiresSubagentsExtensionAndGaduLink(t *testing.T) {
	adapter, destDir, recorder := newBuiltPiAdapterWithStub(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	writePiMcpRegistration(t, destDir, true)

	if result := adapter.Install(); result.Status == engineRuntime.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}

	var sawInstall, sawSubagents bool
	for _, argv := range readAllRecordedTokens(t, recorder) {
		if argv == "install" {
			sawInstall = true
		}
		if strings.Contains(argv, "pi-subagents") {
			sawSubagents = true
		}
	}
	if !sawInstall || !sawSubagents {
		t.Fatalf("Install must run both the package install and the Subagents extension install, got argv lines: %v", readRecordedInvocations(t, recorder))
	}

	linkPath := filepath.Join(home, ".pi", "agent", "agents", "GADU.md")
	if target, err := os.Readlink(linkPath); err != nil || target != filepath.Join(destDir, "agents", "GADU.md") {
		t.Fatalf("Install must leave GADU.md linked to the package path, readlink=%q err=%v", target, err)
	}

	// task 5.8: Status must fold both new entries into the honesty model.
	// The stub `pi` only records argv -- it never writes a real
	// settings.json -- so this test writes it manually with BOTH the
	// package and the extension listed, exactly as a real `pi install`
	// would have left it.
	writePiSettingsPackages(t, home, []string{destDir, "npm:pi-subagents-j0k3r"})
	status := adapter.Status()
	if status.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Status with every entry proven = %s, want supported", status)
	}
}

// TestStatus_ReportsUnprovenSubagentsAndGaduLinkEntries (task 5.8): each new
// owned entry is independently observable and forces `partial` while
// unproven, without collapsing into a single boolean.
func TestStatus_ReportsUnprovenSubagentsAndGaduLinkEntries(t *testing.T) {
	overlayRoot, registryPath := piFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	home := t.TempDir()
	t.Setenv("HOME", home)
	writePiSettingsListing(t, destDir)
	writePiMcpRegistration(t, destDir, true)
	// Neither the Subagents extension nor the GADU link exist yet.

	adapter := engineRuntime.NewPiAdapterWithPaths(overlayRoot, registryPath, destDir)
	result := adapter.Status()
	if result.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Status = %s, want partial", result)
	}
	if !strings.Contains(result.Message, "Subagents extension") {
		t.Errorf("Status message must name the unproven Subagents extension entry, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "GADU.md") {
		t.Errorf("Status message must name the unproven GADU link entry, got %q", result.Message)
	}
}

// TestInstall_RejectsAmbiguousGaduFrontmatter (task 5.5/R-014): a package
// agents/GADU.md whose frontmatter mixes an inline `tools` scalar with a
// YAML list is refused before linking, with a named error, and no link is
// created.
func TestInstall_RejectsAmbiguousGaduFrontmatter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	overlayRoot, registryPath := piFixtureOverlay(t)
	mustWrite(t, filepath.Join(overlayRoot, "agents", "GADU.md"),
		"---\nname: GADU\ndescription: test\ntools: '*'\ntools:\n  - Read\n---\nbody\n")
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	recorder := filepath.Join(t.TempDir(), "argv.txt")
	t.Setenv("LABDRIAN_PI_BIN", writeStubPiScript(t, recorder))
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	adapter := engineRuntime.NewPiAdapterWithPaths(overlayRoot, registryPath, destDir)
	result := adapter.Install()
	if !strings.Contains(result.Message, "frontmatter") {
		t.Fatalf("Install message must name the frontmatter error, got %q", result.Message)
	}

	linkPath := filepath.Join(home, ".pi", "agent", "agents", "GADU.md")
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("GADU.md must not be linked when its frontmatter is ambiguous, stat err = %v", err)
	}
}
