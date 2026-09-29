package capability_test

import (
	"reflect"
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
func TestRuntimesOtherThanClaudeAreDeclaredOnly(t *testing.T) {
	for _, d := range capability.All() {
		if d.Target == capability.TargetClaude {
			continue
		}
		for _, c := range d.Claims {
			if c.Capability == capability.Installation {
				continue
			}
			if c.Status != capability.Unsupported {
				t.Errorf("%s/%s is %s, want unsupported: Phase 7 implements it for Claude Code only", d.Target, c.Capability, c.Status)
			}
		}
	}
}
