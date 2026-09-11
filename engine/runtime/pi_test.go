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
	mustWrite(t, filepath.Join(root, "agents", "GADU.md"), "---\nname: GADU\n---\nbody\n")
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

// TestPiAdapter_ApplyInstallSyncCheck_WiredToPipkg pins task 2.4: with real
// overlay/registry/dest paths, Apply/Install build the package via pipkg and
// SyncCheck reports it as drift-free right after.
func TestPiAdapter_ApplyInstallSyncCheck_WiredToPipkg(t *testing.T) {
	overlayRoot, registryPath := piFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

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
