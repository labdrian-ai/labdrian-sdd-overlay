package domain

import (
	"strings"
	"testing"
)

// TestBuildArgSets verifies the pure buildArgSets helper:
//   - TargetAgnostic action → single arg set with no --target (R-008 Scenario 8.1)
//   - Non-TargetAgnostic action with one target → args include --target (R-008 Scenario 8.2)
func TestBuildArgSets(t *testing.T) {
	t.Run("TargetAgnostic omits --target", func(t *testing.T) {
		action := Action{TargetAgnostic: true, Command: "status-hooks"}
		sets := BuildArgSets(action, nil, false)
		if len(sets) != 1 {
			t.Fatalf("expected 1 arg set, got %d", len(sets))
		}
		for _, arg := range sets[0] {
			if arg == "--target" {
				t.Fatal("TargetAgnostic action must NOT include --target in arg set")
			}
		}
		if sets[0][0] != "status-hooks" {
			t.Fatalf("first arg must be command name, got %q", sets[0][0])
		}
	})

	t.Run("non-TargetAgnostic with one target includes --target", func(t *testing.T) {
		action := Action{TargetAgnostic: false, Command: "apply", SupportsAll: true}
		targets := []Target{{Name: "agent-x"}}
		sets := BuildArgSets(action, targets, false)
		if len(sets) != 1 {
			t.Fatalf("expected 1 arg set, got %d", len(sets))
		}
		args := sets[0]
		foundTarget := false
		for i, arg := range args {
			if arg == "--target" && i+1 < len(args) && args[i+1] == "agent-x" {
				foundTarget = true
			}
		}
		if !foundTarget {
			t.Fatalf("non-TargetAgnostic action must include --target agent-x, got %v", args)
		}
	})
}

// TestBuildArgSetsAppendsArgs verifies that action.Args are appended after
// action.Command for TargetAgnostic actions.
func TestBuildArgSetsAppendsArgs(t *testing.T) {
	action := Action{
		TargetAgnostic: true,
		Command:        "skills",
		Args:           []string{"validate"},
	}
	sets := BuildArgSets(action, nil, false)
	if len(sets) != 1 {
		t.Fatalf("expected 1 arg set, got %d", len(sets))
	}
	args := sets[0]
	if len(args) < 2 {
		t.Fatalf("expected at least 2 args (command + verb), got %v", args)
	}
	if args[0] != "skills" {
		t.Errorf("args[0] = %q, want skills", args[0])
	}
	if args[1] != "validate" {
		t.Errorf("args[1] = %q, want validate", args[1])
	}
}

// TestBuildArgSetsNoArgsUnchanged verifies a TargetAgnostic action with no Args
// produces the same single-element arg set (backwards-compat for hooks actions).
func TestBuildArgSetsNoArgsUnchanged(t *testing.T) {
	action := Action{TargetAgnostic: true, Command: "status-hooks"}
	sets := BuildArgSets(action, nil, false)
	if len(sets) != 1 || len(sets[0]) != 1 {
		t.Fatalf("expected [[status-hooks]], got %v", sets)
	}
	if sets[0][0] != "status-hooks" {
		t.Errorf("sets[0][0] = %q, want status-hooks", sets[0][0])
	}
}

// TestAllArgSetsFlattensPrimaryThenAlso verifies allArgSets returns the
// primary action's arg set(s) followed by each Also entry's arg set(s), in
// that order.
func TestAllArgSetsFlattensPrimaryThenAlso(t *testing.T) {
	action := Action{
		Command:     "status",
		SupportsAll: true,
		Also: []Action{
			{Command: "status-hooks", TargetAgnostic: true},
		},
	}
	targets := []Target{{Name: "claude"}}

	sets := AllArgSets(action, targets, false)
	if len(sets) != 2 {
		t.Fatalf("expected 2 arg sets, got %d: %v", len(sets), sets)
	}
	if sets[0][0] != "status" {
		t.Errorf("first arg set must start with %q, got %v", "status", sets[0])
	}
	want := []string{"status-hooks"}
	if len(sets[1]) != len(want) || sets[1][0] != want[0] {
		t.Errorf("second arg set = %v, want %v", sets[1], want)
	}
}

// TestAllArgSetsSkillsActionComposition verifies allArgSets on the actual
// Skills action from Actions() yields the status/validate/list arg sets in
// order.
func TestAllArgSetsSkillsActionComposition(t *testing.T) {
	var skills Action
	for _, a := range Actions() {
		if a.Command == "skills" {
			skills = a
			break
		}
	}
	if skills.Command == "" {
		t.Fatal("Actions() must contain a top-level skills action")
	}

	sets := AllArgSets(skills, nil, false)
	want := [][]string{
		{"skills", "status"},
		{"skills", "validate"},
		{"skills", "list"},
	}
	if len(sets) != len(want) {
		t.Fatalf("expected %d arg sets, got %d: %v", len(want), len(sets), sets)
	}
	for i, w := range want {
		if len(sets[i]) != len(w) {
			t.Fatalf("arg set %d = %v, want %v", i, sets[i], w)
		}
		for j := range w {
			if sets[i][j] != w[j] {
				t.Errorf("arg set %d = %v, want %v", i, sets[i], w)
				break
			}
		}
	}
}

// TestAllArgSetsSelfUpdateActionComposition verifies allArgSets on the
// actual registered self-update action: the primary self-update invocation
// (no --target, TargetAgnostic) followed by the chained apply invocation for
// the selected targets — the exact sequence the TUI now runs from a single
// button press.
func TestAllArgSetsSelfUpdateActionComposition(t *testing.T) {
	var selfUpdate Action
	for _, a := range Actions() {
		if a.Command == "self-update" {
			selfUpdate = a
			break
		}
	}
	if selfUpdate.Command == "" {
		t.Fatal("Actions() must contain a top-level self-update action")
	}

	targets := []Target{{Name: "claude"}, {Name: "opencode"}}

	t.Run("all targets selected -> apply --target all", func(t *testing.T) {
		sets := AllArgSets(selfUpdate, targets, true)
		want := [][]string{
			{"self-update"},
			{"apply", "--target", "all"},
		}
		if len(sets) != len(want) {
			t.Fatalf("expected %d arg sets, got %d: %v", len(want), len(sets), sets)
		}
		for i, w := range want {
			if strings.Join(sets[i], " ") != strings.Join(w, " ") {
				t.Errorf("arg set %d = %v, want %v", i, sets[i], w)
			}
		}
	})

	t.Run("one target selected -> apply --target <name>", func(t *testing.T) {
		sets := AllArgSets(selfUpdate, targets[:1], false)
		want := [][]string{
			{"self-update"},
			{"apply", "--target", "claude"},
		}
		if len(sets) != len(want) {
			t.Fatalf("expected %d arg sets, got %d: %v", len(want), len(sets), sets)
		}
		for i, w := range want {
			if strings.Join(sets[i], " ") != strings.Join(w, " ") {
				t.Errorf("arg set %d = %v, want %v", i, sets[i], w)
			}
		}
	})
}

// TestAllArgSetsLongtermMemComposition pins the actual argv the TUI would run:
// the positional subcommand must survive target routing (cmd_longterm_mem
// validates it before parsing any flag), --target must be passed, and
// --target all must be used when every target is selected.
func TestAllArgSetsLongtermMemComposition(t *testing.T) {
	targets := []Target{{Name: "claude"}, {Name: "opencode"}}

	for _, verb := range []string{"status", "install", "uninstall"} {
		a := findActionByCommandArgs(t, "longterm-mem", verb)

		t.Run(verb+"/all targets", func(t *testing.T) {
			sets := AllArgSets(a, threeCopyTargets(), true)
			want := [][]string{{"longterm-mem", verb, "--target", "all"}}
			if len(sets) != len(want) || strings.Join(sets[0], " ") != strings.Join(want[0], " ") {
				t.Fatalf("allArgSets = %v, want %v", sets, want)
			}
		})

		t.Run(verb+"/subset", func(t *testing.T) {
			sets := AllArgSets(a, targets, false)
			want := [][]string{
				{"longterm-mem", verb, "--target", "claude"},
				{"longterm-mem", verb, "--target", "opencode"},
			}
			if len(sets) != len(want) {
				t.Fatalf("allArgSets = %v, want %v", sets, want)
			}
			for i := range want {
				if strings.Join(sets[i], " ") != strings.Join(want[i], " ") {
					t.Errorf("arg set %d = %v, want %v", i, sets[i], want[i])
				}
			}
		})
	}
}
