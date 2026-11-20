package main

// Tests for the bound-chain rule of 'roles append', 'roles resume' and 'roles next' (labdrian-orchestrator-opencode
// S16 B5, T56): while the checkout is bound to an open workflow with a role chain, the parent is held to that
// chain. Everything runs in process in a temporary repository and a state home of its own; the verbs read the
// working directory, which each test hands the verb as the program does.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

const (
	boundWant = "this checkout is bound to workflow f-t1"
	boundFlag = "--project proj-1 --goal f --chain f-t1"
)

// boundAppendRefusal is the exact line of S16 B5 for 'roles append', spelled out here and not built by the code under test.
const boundAppendRefusal = "error: roles append: " + boundWant + "; append its hops with " + boundFlag + "\n"

// boundIn runs `roles <args>` as the program does, with the working directory at dir.
func boundIn(t *testing.T, dir string, args []string, stdin string) rolesRun {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := -1
	exited := false
	runRolesIn(func() (string, error) { return dir, nil }, args, strings.NewReader(stdin), &out, &errBuf, func(c int) {
		if !exited {
			code, exited = c, true
		}
	}, reviewEvidenceCheck(deps{}))
	if !exited {
		code = 0
	}
	return rolesRun{code: code, stdout: out.String(), stderr: errBuf.String()}
}

// boundFirstRecord is the first record of a chain, shaper to estimator, of the given ids.
func boundFirstRecord(project, goal, chain string) string {
	return strings.NewReplacer(
		`"project_id": "proj-1"`, `"project_id": "`+project+`"`,
		`"goal_id": "goal-1"`, `"goal_id": "`+goal+`"`,
		`"chain_id": "chain-1"`, `"chain_id": "`+chain+`"`,
	).Replace(rolesHandoffJSON(1, "", "shaper", "estimator", "completed", ""))
}

// boundBuilderHop is the third record of chain f-t1 of bgRepo: builder to reviewer, chained to the stored second.
func boundBuilderHop(t *testing.T) string {
	t.Helper()
	store, err := newRoleChainStore()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadChain("proj-1", "f", "f-t1")
	if err != nil || len(records) != 2 {
		t.Fatalf("chain f-t1: %d records, err=%v, want 2", len(records), err)
	}
	last := records[1]
	raw, err := json.Marshal(roles.RoleHandoff{
		Version: roles.HandoffVersion, ProjectID: "proj-1", GoalID: "f", ChainID: "f-t1", Seq: 3,
		FromRole: roles.RoleBuilder, ToRole: roles.RoleReviewer, PrevSHA256: sha256hex.Sum(last.Raw),
		PayloadKind: "progress", PayloadSHA256: last.Handoff.PayloadSHA256, Evidence: []roles.Evidence{},
		Context: roles.Context{Summary: "built", Decisions: []string{}, OpenQuestions: []string{}}, Status: roles.StatusCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// boundChainStore is the sha256 of every file under the role-chain store of the state home, by path.
func boundChainStore(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join(os.Getenv("XDG_STATE_HOME"), "labdrian", "role-chains")
	files := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = sha256hex.Sum(data)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return files
}

func boundAppendArgs(project, goal, chain string) []string {
	return []string{"append", "--project", project, "--goal", goal, "--chain", chain, "--stdin"}
}

func TestRolesAppendStoresAHopOfTheBoundWorkflowsOwnChain(t *testing.T) {
	root := bgRepo(t)
	got := boundIn(t, root, boundAppendArgs("proj-1", "f", "f-t1"), boundBuilderHop(t))
	if got.code != 0 || got.stderr != "" || !strings.Contains(got.stdout, "roles append: stored record at ") {
		t.Fatalf("code=%d stdout=%q stderr=%q, want a stored record", got.code, got.stdout, got.stderr)
	}
	if n := len(boundChainStore(t)); n != 3 {
		t.Errorf("the chain holds %d files, want 3", n)
	}
}

func TestRolesAppendRefusesARecordOfAnotherChainOfTheBoundWorkflow(t *testing.T) {
	root := bgRepo(t)
	for name, c := range map[string]struct{ project, goal, chain string }{
		"the feature named as the chain (the shape of L130)": {"proj-1", "f", "f"},
		"another goal":    {"proj-1", "g", "f-t1"},
		"another project": {"other", "f", "f-t1"},
	} {
		before := boundChainStore(t)
		got := boundIn(t, root, boundAppendArgs(c.project, c.goal, c.chain), boundFirstRecord(c.project, c.goal, c.chain))
		if got.code != 1 || got.stdout != "" || got.stderr != boundAppendRefusal {
			t.Errorf("%s: code=%d stdout=%q stderr=%q, want exit 1, no stdout and stderr %q", name, got.code, got.stdout, got.stderr, boundAppendRefusal)
		}
		if after := boundChainStore(t); !reflect.DeepEqual(before, after) {
			t.Errorf("%s: the chain store changed: %v -> %v", name, before, after)
		}
	}
}

func TestRolesAppendKeepsItsEarlierRefusalsBeforeTheBoundChainOne(t *testing.T) {
	root := bgRepo(t)
	got := boundIn(t, root, boundAppendArgs("proj-1", "f", "f"), boundFirstRecord("proj-1", "f", "f-t1"))
	want := "error: roles append: --chain \"f\" does not match the record's chain_id \"f-t1\"\n"
	if got.code != 1 || got.stderr != want {
		t.Errorf("code=%d stderr=%q, want exit 1 and %q", got.code, got.stderr, want)
	}
}

func TestRolesResumeAndNextRefuseTheBoundWorkflowsGoalUnderAnotherChain(t *testing.T) {
	root := bgRepo(t)
	for _, verb := range []string{"resume", "next"} {
		want := "error: roles " + verb + ": " + boundWant + ", whose role chain for goal f is f-t1: use --chain f-t1\n"
		got := boundIn(t, root, []string{verb, "--project", "proj-1", "--goal", "f", "--chain", "f"}, "")
		if got.code != 1 || got.stdout != "" || got.stderr != want {
			t.Errorf("%s: code=%d stdout=%q stderr=%q, want exit 1, no stdout and stderr %q", verb, got.code, got.stdout, got.stderr, want)
		}
		own := boundIn(t, root, []string{verb, "--project", "proj-1", "--goal", "f", "--chain", "f-t1"}, "")
		if own.code != 0 || own.stderr != "" || own.stdout == "" {
			t.Errorf("%s of the workflow's own chain: code=%d stdout=%q stderr=%q, want exit 0", verb, own.code, own.stdout, own.stderr)
		}
		other := boundIn(t, root, []string{verb, "--project", "proj-1", "--goal", "g", "--chain", "g"}, "")
		wantOther := "error: roles " + verb + ": chain has no records\n"
		if other.code != 1 || other.stderr != wantOther {
			t.Errorf("%s of another goal: code=%d stderr=%q, want exit 1 and %q (as before)", verb, other.code, other.stderr, wantOther)
		}
	}
}

func TestRolesVerbsAnswerAsBeforeWhereNothingIsBoundToOpenChain(t *testing.T) {
	stored := func(t *testing.T, dir string) {
		t.Helper()
		got := boundIn(t, dir, boundAppendArgs("proj-1", "f", "f"), boundFirstRecord("proj-1", "f", "f"))
		if got.code != 0 || got.stderr != "" || !strings.Contains(got.stdout, "stored record at ") {
			t.Errorf("code=%d stdout=%q stderr=%q, want a stored record", got.code, got.stdout, got.stderr)
		}
		for _, verb := range []string{"resume", "next"} {
			r := boundIn(t, dir, []string{verb, "--project", "proj-1", "--goal", "f", "--chain", "f"}, "")
			if r.code != 0 || r.stderr != "" {
				t.Errorf("%s: code=%d stderr=%q, want exit 0", verb, r.code, r.stderr)
			}
		}
	}
	t.Run("a repository bound to nothing", func(t *testing.T) { stored(t, bgBareRepo(t)) })
	t.Run("a closed workflow", func(t *testing.T) {
		root := bgRepo(t)
		bgWorkflowCmd(t, root, "close", "--project", "proj-1", "--workflow", "f-t1", "--outcome", "abandoned", "--reason", "test")
		stored(t, root)
	})
	t.Run("a workflow with no role chain", func(t *testing.T) {
		root := bgBareRepo(t)
		goal := filepath.Join(root, "odd", "goals", "f.json")
		bgWorkflowCmd(t, root, "create", "--project", "proj-1", "--workflow", "plain", "--goal", goal, "--profile", "odd")
		bgWorkflowCmd(t, root, "start", "--project", "proj-1", "--workflow", "plain")
		bgWorkflowCmd(t, root, "bind", "--project", "proj-1", "--workflow", "plain")
		stored(t, root)
	})
	t.Run("a binding that is not ours", func(t *testing.T) {
		root := bgBareRepo(t)
		dir := filepath.Join(os.Getenv("XDG_STATE_HOME"), "labdrian", "bindings")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, mustRepoKey(t, root)+".json"), []byte("not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		stored(t, root)
	})
	t.Run("a directory outside any repository", func(t *testing.T) {
		bgBareRepo(t)
		stored(t, t.TempDir())
	})
}

func TestRolesAppendRefusesWhenTheBindingCannotBeRead(t *testing.T) {
	root := bgBareRepo(t)
	// A state home that is not absolute: the binding store cannot be built, so the binding cannot be read.
	t.Setenv("XDG_STATE_HOME", "relative-state")
	got := boundIn(t, root, boundAppendArgs("proj-1", "f", "f"), boundFirstRecord("proj-1", "f", "f"))
	want := "error: roles append: the binding of this checkout could not be read (projection store: XDG_STATE_HOME \"relative-state\" is not absolute)\n"
	if got.code != 1 || got.stdout != "" || got.stderr != want {
		t.Errorf("code=%d stdout=%q stderr=%q, want exit 1, no stdout and %q", got.code, got.stdout, got.stderr, want)
	}
	// resume and next leave the binding alone when it cannot be read: they say what they said before.
	for _, verb := range []string{"resume", "next"} {
		args := []string{verb, "--project", "proj-1", "--goal", "f", "--chain", "f"}
		before := runRolesTest(args, "")
		after := boundIn(t, root, args, "")
		if before.code != 1 || !strings.HasPrefix(before.stderr, "error: roles "+verb+": ") || after != before {
			t.Errorf("%s: with the working directory %+v, without it %+v, want the same exit 1 answer", verb, after, before)
		}
	}
}
