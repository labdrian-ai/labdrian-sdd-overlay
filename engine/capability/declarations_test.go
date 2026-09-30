package capability_test

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

func TestAllReturnsOneValidDeclarationPerTargetInOrder(t *testing.T) {
	all := capability.All()
	targets := capability.Targets()
	if len(all) != len(targets) {
		t.Fatalf("All() has %d declarations, want %d (one per target)", len(all), len(targets))
	}
	for i, d := range all {
		if d.Target != targets[i] {
			t.Errorf("All()[%d].Target = %q, want %q", i, d.Target, targets[i])
		}
		if err := capability.Validate(d); err != nil {
			t.Errorf("declaration for %s is invalid: %v", d.Target, err)
		}
	}
}

func TestDeclareReturnsTheDeclarationAllListsForThatTarget(t *testing.T) {
	all := capability.All()
	for i, target := range capability.Targets() {
		got, err := capability.Declare(target)
		if err != nil {
			t.Fatalf("Declare(%q): %v", target, err)
		}
		if !reflect.DeepEqual(got, all[i]) {
			t.Errorf("Declare(%q) differs from All()[%d]", target, i)
		}
	}
}

func TestDeclareRefusesUnknownTargets(t *testing.T) {
	for _, target := range []string{"", "cursor", "Claude", " claude", "all"} {
		t.Run("target "+target, func(t *testing.T) {
			d, err := capability.Declare(target)
			if err == nil || !strings.Contains(err.Error(), "unknown target") {
				t.Fatalf("Declare(%q) = %v, %v; want an unknown-target error", target, d, err)
			}
			if !reflect.DeepEqual(d, capability.Declaration{}) {
				t.Errorf("Declare(%q) returned a non-zero declaration alongside an error: %+v", target, d)
			}
		})
	}
}

// TestDeclarationsAreCopies pins that a caller can never edit the table
// through a returned value: the CLI and later consumers receive their own
// slices.
func TestDeclarationsAreCopies(t *testing.T) {
	mutate := func(d capability.Declaration) {
		d.Target = "tampered"
		d.Untested = "tampered"
		for i := range d.Claims {
			d.Claims[i].Status = capability.Unsupported
			d.Claims[i].Detail = "tampered"
			for j := range d.Claims[i].Tests {
				d.Claims[i].Tests[j] = "tampered"
			}
		}
		d.Claims = d.Claims[:0]
	}
	pristine := capability.All()

	for _, d := range capability.All() {
		mutate(d)
	}
	if got := capability.All(); !reflect.DeepEqual(got, pristine) {
		t.Fatal("mutating the result of All() changed the package's table")
	}

	d, err := capability.Declare(capability.TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	mutate(d)
	if got := capability.All(); !reflect.DeepEqual(got, pristine) {
		t.Fatal("mutating the result of Declare() changed the package's table")
	}
}

// TestDeclarationsNeverCarryNilTests pins the invariant that lets a decoded
// report compare equal to the declaration it came from: an unsupported claim
// holds an empty, non-nil Tests slice.
func TestDeclarationsNeverCarryNilTests(t *testing.T) {
	for _, d := range capability.All() {
		for _, c := range d.Claims {
			if c.Tests == nil {
				t.Errorf("%s/%s has nil Tests, want an empty slice", d.Target, c.Capability)
			}
		}
	}
}

func TestOnlyOpenCodeIsDeclaredUntested(t *testing.T) {
	for _, d := range capability.All() {
		switch {
		case d.Target == capability.TargetOpenCode && strings.TrimSpace(d.Untested) == "":
			t.Errorf("opencode cannot be exercised on this machine, but its declaration has no untested reason")
		case d.Target != capability.TargetOpenCode && d.Untested != "":
			t.Errorf("%s can be exercised on this machine, but its declaration says untested: %q", d.Target, d.Untested)
		}
	}
}

func TestEveryTargetDeclaresInstallationWithEvidence(t *testing.T) {
	for _, d := range capability.All() {
		c := d.Claims[0]
		if c.Capability != capability.Installation {
			t.Fatalf("%s: first claim is %s, want installation", d.Target, c.Capability)
		}
		if c.Status == capability.Unsupported || len(c.Tests) == 0 {
			t.Errorf("%s: installation must be supported or partial with tests, got %s with %d tests", d.Target, c.Status, len(c.Tests))
		}
	}
}

// TestRuntimesOtherThanClaudeAreDeclaredOnly pins the Phase 7 scope: Codex,
// Pi, and OpenCode receive an honest declaration of what is proven today
// (installation), and every other capability stays unsupported with its
// limit written, because Phase 7 implements projection for Claude Code only.
//
// Skills is the one capability this rule does not cover. Phase 8 adds it, and
// its claim is stated by each runtime from the tests that prove it, not from
// Phase 7 scope: the rule about what each runtime may say, and what tests a
// status needs, is TestSkillsClaimsAreBoundedByTheirEvidence and
// TestSkillsClaimsStateTheirScope. The rule here stays exact for the eight
// capabilities Phase 7 defined.
func TestRuntimesOtherThanClaudeAreDeclaredOnly(t *testing.T) {
	for _, d := range capability.All() {
		if d.Target == capability.TargetClaude {
			continue
		}
		for _, c := range d.Claims {
			if c.Capability == capability.Installation || c.Capability == capability.Skills {
				continue
			}
			if c.Status != capability.Unsupported {
				t.Errorf("%s/%s is %s, want unsupported: Phase 7 implements it for Claude Code only", d.Target, c.Capability, c.Status)
			}
			// A declaration states only what is true of its own runtime. A
			// sentence about Claude Code would go stale, or contradict
			// Claude Code's own claims, whenever those claims change.
			if strings.Contains(c.Detail, "Claude") {
				t.Errorf("%s/%s detail mentions another runtime: %q", d.Target, c.Capability, c.Detail)
			}
		}
	}
}

// TestClaudeCodeStatuses pins which Claude Code claims are proven today. A
// claim is upgraded in the commit that adds the tests proving it, and this pin
// changes in the same commit, so every upgrade shows up in review and none can
// slip in on the side. The pin does not replace the evidence guard: a claim
// that names a test that does not exist still fails
// TestDeclaredEvidenceExists.
func TestClaudeCodeStatuses(t *testing.T) {
	want := map[capability.Capability]capability.Status{
		capability.Installation:      capability.Supported,
		capability.Projection:        capability.Supported,
		capability.Dispatch:          capability.Partial,
		capability.Cancellation:      capability.Partial,
		capability.Persistence:       capability.Supported,
		capability.Restart:           capability.Supported,
		capability.Authentication:    capability.Partial,
		capability.MemoryEnforcement: capability.Partial,
		capability.Skills:            capability.Partial,
	}
	d, err := capability.Declare(capability.TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range d.Claims {
		if c.Status != want[c.Capability] {
			t.Errorf("claude %s is %s, want %s", c.Capability, c.Status, want[c.Capability])
		}
	}
}

// TestClaudeCodeSessionClaimsStateTheirScope pins what the claims that rest on
// the session binding and the projection hook say about their own reach, because
// a status alone would let the wording drift away from what the tests prove. The
// hook is installed by install-hooks and tested by feeding it hook JSON and by
// reading the settings file, but no test observes a real Claude Code session
// receiving its context. Projection and restart are supported and dispatch is
// partial; each has to say that the hooks load only after a restart and that no
// real session is observed, and none may still say the hook is not installed.
func TestClaudeCodeSessionClaimsStateTheirScope(t *testing.T) {
	d, err := capability.Declare(capability.TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	claims := make(map[capability.Capability]capability.Claim)
	for _, c := range d.Claims {
		claims[c.Capability] = c
	}
	const installs = "install-hooks installs"

	for name, want := range map[capability.Capability][]string{
		capability.Persistence: {"workflow log", "binding", "survive", "transcripts are not managed"},
		capability.Dispatch:    {"can be bound", "stored", "the hook projects that workflow", "explicit workflow bind", "restart", "no test observes a real session being steered"},
		capability.Projection:  {installs, "UserPromptSubmit hook", "builds the bound workflow's context", "reports partial until install-hooks is re-run", "restart", "If the binding or workflow cannot be followed, the hook projects nothing and warns", "Tests feed the hook JSON", "a real session receiving the context is not part of them"},
		capability.Restart:     {"reads the binding and the workflow log from disk on every prompt", "separate processes", installs, "restart_required", "a real session re-binding is not part of the tests"},
	} {
		detail := claims[name].Detail
		for _, phrase := range want {
			if !strings.Contains(detail, phrase) {
				t.Errorf("%s detail %q does not state %q", name, detail, phrase)
			}
		}
	}

	// Cancellation and memory enforcement are partial because the PreToolUse gate
	// exists and is tested, but the hooks are not installed and the gate is
	// narrow. Each says what the gate does, and each says where it stops, so a
	// status alone cannot let the wording drift.
	for name, want := range map[capability.Capability][]string{
		capability.Cancellation: {
			"paused", "PreToolUse gate", "Write, Edit, MultiEdit, and NotebookEdit", "next tool call",
			"tells the session", installs, "in-flight tool call cannot be interrupted", "Bash is never gated", "the gate denies nothing when the binding or the workflow cannot be followed", "restart", "no test observes a real session being denied",
		},
		capability.MemoryEnforcement: {
			"longterm-mem query", "project", "memory plan", "no project",
			"Not enforced, because the tool input cannot verify these: ", "a get call (it carries no project)", "Engram tools", "the mapping between the plan's sources and a query's sources", "Writes are never blocked", installs, "no narrower than the gate's tool-name pattern", "restart", "no test observes a real session being denied",
		},
	} {
		detail := claims[name].Detail
		for _, phrase := range want {
			if !strings.Contains(detail, phrase) {
				t.Errorf("%s detail %q does not state %q", name, detail, phrase)
			}
		}
	}
	// No Claude Code claim may still say the hooks are not installed: they are,
	// and a claim that says otherwise would understate what a user can enable.
	for _, c := range d.Claims {
		for _, stale := range staleInstallWording(c.Detail) {
			t.Errorf("claude %s detail %q still says %q", c.Capability, c.Detail, stale)
		}
	}

	// The old wording, which denied that any gate exists, must be gone.
	for name, stale := range map[capability.Capability][]string{
		capability.Cancellation:      {"no gate denies any tool", "Not implemented yet"},
		capability.MemoryEnforcement: {"nothing enforces it", "Not implemented yet", "as the tool input cannot verify it: get carries no project, Engram tools, and"},
	} {
		for _, phrase := range stale {
			if strings.Contains(claims[name].Detail, phrase) {
				t.Errorf("%s detail %q still says %q, which the gate contradicts", name, claims[name].Detail, phrase)
			}
		}
	}
}

func skillsClaim(t *testing.T, target string) capability.Claim {
	t.Helper()
	d, err := capability.Declare(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range d.Claims {
		if c.Capability == capability.Skills {
			return c
		}
	}
	t.Fatalf("%s has no skills claim", target)
	return capability.Claim{}
}

// TestSkillsClaimsAreBoundedByTheirEvidence pins the status of every runtime's
// skills claim, and the kind of test each may rest on, so that neither can move
// without showing up here. All four are partial and none is supported: the global
// tier depends on the overlay's apply script and is tested only on a fixture
// overlay in a sandbox home, no detector covers a global skill nobody uses, and no
// test observes a runtime loading a skill. A status moves in the commit that adds
// the tests proving it, and this pin moves with it.
//
// Each claim has to be backed by the tests of the tier it speaks about: Claude Code
// and Codex by the project-tier tests (engine/skills) and the global-tier tests
// (engine/installer), Pi by the package tests (engine/pipkg), its adapter
// (engine/runtime), and the register tests that print its trust note, OpenCode by
// the global-tier tests only. OpenCode's claim says no test connects the project tier
// to it, so it may cite none that does.
func TestSkillsClaimsAreBoundedByTheirEvidence(t *testing.T) {
	for _, tc := range []struct {
		target    string
		required  []string // a cited test must come from each of these directories
		forbidden []string // and none from these
	}{
		{capability.TargetClaude, []string{"installer", "skills"}, nil},
		{capability.TargetCodex, []string{"installer", "skills"}, nil},
		{capability.TargetPi, []string{"pipkg", "runtime", "skills"}, []string{"installer"}},
		{capability.TargetOpenCode, []string{"installer"}, []string{"skills", "pipkg", "runtime"}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			c := skillsClaim(t, tc.target)
			if c.Status != capability.Partial {
				t.Fatalf("%s skills = %s, want partial", tc.target, c.Status)
			}
			dirs := map[string]bool{}
			for _, ref := range c.Tests {
				dir, _, _ := strings.Cut(ref, ":")
				dirs[dir] = true
			}
			for _, dir := range tc.required {
				if !dirs[dir] {
					t.Errorf("%s skills cites no test from %s/: %v", tc.target, dir, c.Tests)
				}
			}
			for _, dir := range tc.forbidden {
				if dirs[dir] {
					t.Errorf("%s skills cites a test from %s/, which does not speak about it: %v", tc.target, dir, c.Tests)
				}
			}
		})
	}
}

// TestSkillsClaimsStateTheirScope pins what each skills claim says about its own
// reach, because a status alone would let the wording drift away from what the
// tests prove: which tier each test covers, that the global tier comes from
// labdrian apply and not from the engine, where the evidence is recorded rather than
// tested, and that no test observes a runtime loading a skill. A claim speaks only of
// its own runtime: one runtime's name in another's detail would go stale, or
// contradict, whenever the other changes.
func TestSkillsClaimsStateTheirScope(t *testing.T) {
	const fixtureOnly = "fixture overlay and a sandbox home"
	for target, phrases := range map[string][]string{
		capability.TargetClaude: {
			"Project tier", "skills install and adopt", ".claude/skills and .agents/skills", "they own by hash",
			"project-register, revise, and retire", "locks serialize writers",
			"Global tier: skills add needs an approval record", "labdrian apply deploys global skills, not the engine", fixtureOnly, "never the real skills",
			"report-only and covers the project tier only", "No test observes a session loading a skill",
		},
		capability.TargetCodex: {
			"write project skills to .agents/skills", "live check on 2026-09-28", "recorded evidence, not a test",
			"deployed to ~/.codex/skills by labdrian apply, not the engine", fixtureOnly, "never the real skills",
			"No test observes Codex loading a skill", "no unused-skill detector",
		},
		capability.TargetPi: {
			"labdrian-pi package", "Pi-targeted skills", "approval records and writers' temporary files",
			"adapter tests replace the pi CLI with a recording stub", "never write .pi/skills", "only after project trust", "No test observes Pi loading a skill",
		},
		capability.TargetOpenCode: {
			"labdrian apply copies global skills into OpenCode's skills directory", fixtureOnly, "never the real skills",
			"A live OpenCode 1.18.31 session on 2026-09-30 discovered project skills under .agents/skills and .claude/skills",
			"recorded, not tested", "the global tier was not observed live", "its names also existing in several global directories",
			"No test observes OpenCode loading a skill", "no test connects the project tier to OpenCode",
		},
	} {
		detail := skillsClaim(t, target).Detail
		for _, phrase := range phrases {
			if !strings.Contains(detail, phrase) {
				t.Errorf("%s skills detail %q does not state %q", target, detail, phrase)
			}
		}
	}

	// A claim names no runtime but its own. "Pi" is matched as a word, so that the
	// prefix of another word cannot trip it. Claude Code is matched by its name and by
	// its user-level directory ~/.claude, not by the project directory .claude/skills:
	// like .agents/skills, that one is shared. skills install writes it for every
	// project, and a live OpenCode session discovered skills placed in it (2026-09-30),
	// so OpenCode's claim may name it without naming Claude Code.
	names := map[string]*regexp.Regexp{
		capability.TargetClaude:   regexp.MustCompile(`Claude|~/\.claude`),
		capability.TargetCodex:    regexp.MustCompile(`Codex|\.codex`),
		capability.TargetPi:       regexp.MustCompile(`\bPi\b|\.pi/`),
		capability.TargetOpenCode: regexp.MustCompile(`OpenCode|opencode`),
	}
	for target := range names {
		detail := skillsClaim(t, target).Detail
		for other, re := range names {
			if other == target {
				continue
			}
			// ~/.codex/skills and .pi/skills are named by the claims that own them, and
			// the shared .agents/skills and .claude/skills are named by several, so only
			// runtime names and each runtime's own directory count as a mention.
			if loc := re.FindString(detail); loc != "" {
				t.Errorf("%s skills detail %q names %s (%q)", target, detail, other, loc)
			}
		}
	}
}

// TestAuthenticationClaimsStateWhatPresenceDoesNotProve pins the wording of the
// authentication claims, because a status alone would let it drift into claiming
// more than a stat can show. Claude Code's claim is partial: the presence prober
// says whether its credentials file exists, and never that a session is
// authenticated. Codex and Pi stay unsupported (declared only), and their details
// may say the same probe reports file presence while being no part of an
// implementation for them; OpenCode has no credentials check at all.
func TestAuthenticationClaimsStateWhatPresenceDoesNotProve(t *testing.T) {
	claim := func(target string) capability.Claim {
		d, err := capability.Declare(target)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range d.Claims {
			if c.Capability == capability.Authentication {
				return c
			}
		}
		t.Fatalf("%s has no authentication claim", target)
		return capability.Claim{}
	}

	claude := claim(capability.TargetClaude)
	if claude.Status != capability.Partial || len(claude.Tests) == 0 {
		t.Fatalf("claude authentication = %s with %d tests, want partial with named tests", claude.Status, len(claude.Tests))
	}
	for _, phrase := range []string{
		"presence prober", "Claude Code's credentials file exists", "by stat only", "never reads the file",
		"cannot prove the credentials are valid", "No lifecycle operation requires authentication",
	} {
		if !strings.Contains(claude.Detail, phrase) {
			t.Errorf("claude authentication detail %q does not state %q", claude.Detail, phrase)
		}
	}
	if strings.Contains(claude.Detail, "Not implemented") || strings.Contains(claude.Detail, "does not check whether") {
		t.Errorf("claude authentication detail %q still says nothing is checked", claude.Detail)
	}

	for target, name := range map[string]string{capability.TargetCodex: "Codex", capability.TargetPi: "Pi"} {
		c := claim(target)
		if c.Status != capability.Unsupported || len(c.Tests) != 0 {
			t.Errorf("%s authentication = %s with %d tests, want unsupported without tests (declared only)", target, c.Status, len(c.Tests))
		}
		for _, phrase := range []string{
			"reports whether " + name + "'s credentials file exists, by stat only",
			"not part of an implementation for " + name,
			"does not prove " + name + " is authenticated",
		} {
			if !strings.Contains(c.Detail, phrase) {
				t.Errorf("%s authentication detail %q does not state %q", target, c.Detail, phrase)
			}
		}
	}

	open := claim(capability.TargetOpenCode)
	if open.Status != capability.Unsupported || strings.Contains(open.Detail, "credentials file exists") {
		t.Errorf("opencode authentication = %s %q, want unsupported with no presence claim (it has no credentials check)", open.Status, open.Detail)
	}
}

// staleInstallPhrases are the phrasings the Claude Code details used while
// install-hooks did not yet install the projection hooks. They are matched as
// whole phrases, not as the bare word "yet", so ordinary prose (a limit that
// says something has not been observed yet) is never rejected.
var staleInstallPhrases = []string{
	"does not install", "not installed",
	"settings yet", "hooks yet", "hook yet", "gated yet", "steered yet", "re-binds yet",
}

// staleInstallWording returns every stale phrase that detail contains.
func staleInstallWording(detail string) []string {
	var found []string
	for _, phrase := range staleInstallPhrases {
		if strings.Contains(detail, phrase) {
			found = append(found, phrase)
		}
	}
	return found
}

// TestStaleInstallGuardRejectsTheOldWordingOnly pins the guard itself: the
// wording the details had before the hooks were installed is caught, and
// ordinary sentences that happen to contain "yet" are not.
func TestStaleInstallGuardRejectsTheOldWordingOnly(t *testing.T) {
	stale := []string{
		"Limit: install-hooks does not install the hook into Claude Code settings yet, and hook changes need a restart.",
		"install-hooks does not install the hooks yet and hook changes need a Claude Code restart, so no session is gated yet.",
		"so no real session re-binds yet.",
		"The hooks are not installed.",
		"so no session is steered yet",
	}
	for _, detail := range stale {
		if len(staleInstallWording(detail)) == 0 {
			t.Errorf("the guard accepts the stale wording %q", detail)
		}
	}
	ordinary := []string{
		"Nothing here has been observed in a live session yet.",
		"A value the caller has not set yet is left empty.",
		"the hooks are installed by install-hooks; a real session receiving the context is not part of the tests",
	}
	for _, detail := range ordinary {
		if found := staleInstallWording(detail); len(found) != 0 {
			t.Errorf("the guard rejects ordinary prose %q because of %q", detail, found)
		}
	}
}
