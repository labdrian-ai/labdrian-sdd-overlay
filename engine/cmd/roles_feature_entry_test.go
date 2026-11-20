package main

// Tests for the hops of a feature's own chain (labdrian-orchestrator-opencode S16 B5, T57): 'roles append' refuses,
// before 'workflow open', a record of chain <goal> whose from_role is not the prototyper while odd/tasks/<goal>.md
// is at the worktree root, because 'workflow open' writes the shaper's and the estimator's records. The
// prototyper's hops, a goal with no document and every record under an open bound workflow are not its concern.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

const featureEntryRefusal = "error: roles append: odd/tasks/f.md is a feature document: from the shaper on, its role chain is the one " +
	"labdrian workflow open --feature f --task <T> writes, with the shaper's and the estimator's records; run it, then append the next hops " +
	"with the --chain it prints (chain f takes only the prototyper's hops)\n"

// featureRecord is a record of project proj-1, goal and chain, with the given seq, previous digest and roles.
func featureRecord(seq int, prev, from, to, goal, chain string) string {
	return strings.NewReplacer(
		`"project_id": "proj-1"`, `"project_id": "proj-1"`,
		`"goal_id": "goal-1"`, `"goal_id": "`+goal+`"`,
		`"chain_id": "chain-1"`, `"chain_id": "`+chain+`"`,
	).Replace(rolesHandoffJSON(seq, prev, from, to, "completed", ""))
}

func requireStored(t *testing.T, got rolesRun) {
	t.Helper()
	if got.code != 0 || got.stderr != "" || !strings.Contains(got.stdout, "roles append: stored record at ") {
		t.Fatalf("code=%d stdout=%q stderr=%q, want a stored record", got.code, got.stdout, got.stderr)
	}
}

func requireFeatureRefusal(t *testing.T, root string, before map[string]string, record string) {
	t.Helper()
	got := boundIn(t, root, boundAppendArgs("proj-1", "f", "f"), record)
	if got.code != 1 || got.stdout != "" || got.stderr != featureEntryRefusal {
		t.Errorf("code=%d stdout=%q stderr=%q, want exit 1, no stdout and stderr %q", got.code, got.stdout, got.stderr, featureEntryRefusal)
	}
	if after := boundChainStore(t); !reflect.DeepEqual(before, after) {
		t.Errorf("the chain store changed: %v -> %v", before, after)
	}
}

func TestRolesAppendRefusesTheShaperHopOfAFeatureBeforeWorkflowOpen(t *testing.T) {
	root := bgBareRepo(t)
	requireFeatureRefusal(t, root, boundChainStore(t), boundFirstRecord("proj-1", "f", "f"))
}

func TestRolesAppendStoresThePrototyperHopOfAFeatureWhateverTheDocumentHolds(t *testing.T) {
	record := featureRecord(1, "", "prototyper", "shaper", "f", "f")
	for name, doc := range map[string]string{
		"a document with no shaped task (the verifier's dead end)": "## Tasks\n\n- [ ] T1 — S1 — delegated — not designed — commit: -\n\n## Log\n\n- **L1** (2026-11-20, user): ask\n",
		"a shaped feature with an unshaped task to insert (bgDoc)": bgDoc,
	} {
		t.Run(name, func(t *testing.T) {
			root := bgBareRepo(t)
			if err := os.WriteFile(filepath.Join(root, "odd", "tasks", "f.md"), []byte(doc), 0o644); err != nil {
				t.Fatal(err)
			}
			requireStored(t, boundIn(t, root, boundAppendArgs("proj-1", "f", "f"), record))
			if n := len(boundChainStore(t)); n != 1 {
				t.Errorf("the chain holds %d files, want 1", n)
			}
		})
	}
}

func TestRolesAppendRefusesTheShaperHopAfterAStoredPrototyperHop(t *testing.T) {
	root := bgBareRepo(t)
	first := featureRecord(1, "", "prototyper", "shaper", "f", "f")
	requireStored(t, boundIn(t, root, boundAppendArgs("proj-1", "f", "f"), first))
	second := featureRecord(2, sha256hex.Sum([]byte(first)), "shaper", "estimator", "f", "f")
	requireFeatureRefusal(t, root, boundChainStore(t), second)
}

func TestRolesAppendLeavesARepeatedFirstRecordOfTheBoundChainToTheChainStore(t *testing.T) {
	root := bgRepo(t)
	before := boundChainStore(t)
	got := boundIn(t, root, boundAppendArgs("proj-1", "f", "f-t1"), boundFirstRecord("proj-1", "f", "f-t1"))
	if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, "error: roles append: ") || strings.Contains(got.stderr, "is a feature document") ||
		!strings.Contains(got.stderr, "role chain store: refusing to replace immutable record") {
		t.Errorf("code=%d stdout=%q stderr=%q, want exit 1 with the chain store's refusal and no feature-document sentence", got.code, got.stdout, got.stderr)
	}
	if after := boundChainStore(t); !reflect.DeepEqual(before, after) {
		t.Errorf("the chain store changed: %v -> %v", before, after)
	}
}

func TestRolesAppendCountsADocumentTheReadRefusesAsPresent(t *testing.T) {
	for name, replace := range map[string]func(t *testing.T, doc string){
		"a symbolic link": func(t *testing.T, doc string) {
			copyPath := doc + ".copy"
			if err := os.Rename(doc, copyPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(copyPath, doc); err != nil {
				t.Fatal(err)
			}
		},
		"a directory": func(t *testing.T, doc string) {
			if err := os.Remove(doc); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(doc, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"a file over the size limit": func(t *testing.T, doc string) {
			if err := os.Truncate(doc, oddtask.MaxDocumentBytes+1); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := bgBareRepo(t)
			replace(t, filepath.Join(root, "odd", "tasks", "f.md"))
			requireFeatureRefusal(t, root, boundChainStore(t), boundFirstRecord("proj-1", "f", "f"))
		})
	}
}

func TestRolesAppendReadsTheFeatureDocumentAtTheWorktreeRootOfTheWorkingDirectory(t *testing.T) {
	record := boundFirstRecord("proj-1", "f", "f")
	t.Run("outside any repository the record is stored", func(t *testing.T) {
		bgBareRepo(t)
		requireStored(t, boundIn(t, t.TempDir(), boundAppendArgs("proj-1", "f", "f"), record))
	})
	t.Run("a nested working directory still sees the root document", func(t *testing.T) {
		root := bgBareRepo(t)
		deep := filepath.Join(root, "a", "b")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatal(err)
		}
		got := boundIn(t, deep, boundAppendArgs("proj-1", "f", "f"), record)
		if got.code != 1 || got.stderr != featureEntryRefusal {
			t.Errorf("code=%d stderr=%q, want the refusal %q", got.code, got.stderr, featureEntryRefusal)
		}
	})
	t.Run("a document only under the working directory is not the root's", func(t *testing.T) {
		root := bgBareRepo(t)
		if err := os.Remove(filepath.Join(root, "odd", "tasks", "f.md")); err != nil {
			t.Fatal(err)
		}
		nested := filepath.Join(root, "a", "odd", "tasks")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nested, "f.md"), []byte(bgDoc), 0o644); err != nil {
			t.Fatal(err)
		}
		requireStored(t, boundIn(t, filepath.Join(root, "a"), boundAppendArgs("proj-1", "f", "f"), record))
	})
}

func TestRolesAppendStoresTheFirstRecordOfAGoalThatIsNoFeature(t *testing.T) {
	root := bgBareRepo(t)
	requireStored(t, boundIn(t, root, boundAppendArgs("proj-1", "g", "g"), boundFirstRecord("proj-1", "g", "g")))
	if n := len(boundChainStore(t)); n != 1 {
		t.Errorf("the chain holds %d files, want 1", n)
	}
}

func TestRolesAppendStoresALaterRecordOfAGoalThatHasADocument(t *testing.T) {
	root := bgRepo(t)
	bgWorkflowCmd(t, root, "close", "--project", "proj-1", "--workflow", "f-t1", "--outcome", "abandoned", "--reason", "test")
	got := boundIn(t, root, boundAppendArgs("proj-1", "f", "f-t1"), boundBuilderHop(t))
	if got.code != 0 || got.stderr != "" || !strings.Contains(got.stdout, "roles append: stored record at ") {
		t.Errorf("code=%d stdout=%q stderr=%q, want a stored record (seq 3, unbound)", got.code, got.stdout, got.stderr)
	}
}

func TestWorkflowOpenStillWritesTheFirstTwoRecordsOfAFeatureChain(t *testing.T) {
	root := bgRepo(t)
	store, err := newRoleChainStore()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadChain("proj-1", "f", "f-t1")
	if err != nil || len(records) != 2 {
		t.Fatalf("chain f-t1: %d records, err=%v, want 2", len(records), err)
	}
	for i, want := range []struct{ from, to string }{{"shaper", "estimator"}, {"estimator", "builder"}} {
		h := records[i].Handoff
		if h.Seq != i+1 || string(h.FromRole) != want.from || string(h.ToRole) != want.to {
			t.Errorf("record %d: seq %d %s -> %s, want seq %d %s -> %s", i, h.Seq, h.FromRole, h.ToRole, i+1, want.from, want.to)
		}
	}
	described, err := describeBinding(root)
	if err != nil || described.Binding.Classification != projection.ClassificationOwned || described.Binding.Binding.WorkflowID != "f-t1" {
		t.Errorf("binding: %+v, err=%v, want an owned binding of workflow f-t1", described.Binding, err)
	}
}
