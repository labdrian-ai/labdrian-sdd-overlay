package main

// Tests for the first record of a chain of a feature (labdrian-orchestrator-opencode S16 B5, T57): 'roles append'
// refuses the record with seq 1 of a goal that is a feature of the checkout, so the chain starts with
// 'workflow open', which writes its first two records.

import (
	"reflect"
	"strings"
	"testing"
)

const featureEntryRefusal = "error: roles append: odd/tasks/f.md is a feature document, so its role chain starts with " +
	"labdrian workflow open --feature f --task <T>, which writes its first two records; run it, then append the next hops with the --chain it prints\n"

func TestRolesAppendRefusesTheFirstRecordOfAFeatureBeforeWorkflowOpen(t *testing.T) {
	root := bgBareRepo(t)
	before := boundChainStore(t)
	got := boundIn(t, root, boundAppendArgs("proj-1", "f", "f"), boundFirstRecord("proj-1", "f", "f"))
	if got.code != 1 || got.stdout != "" || got.stderr != featureEntryRefusal {
		t.Errorf("code=%d stdout=%q stderr=%q, want exit 1, no stdout and stderr %q", got.code, got.stdout, got.stderr, featureEntryRefusal)
	}
	if after := boundChainStore(t); !reflect.DeepEqual(before, after) {
		t.Errorf("the chain store changed: %v -> %v", before, after)
	}
}

func TestRolesAppendStoresTheFirstRecordOfAGoalThatIsNoFeature(t *testing.T) {
	root := bgBareRepo(t)
	got := boundIn(t, root, boundAppendArgs("proj-1", "g", "g"), boundFirstRecord("proj-1", "g", "g"))
	if got.code != 0 || got.stderr != "" || !strings.Contains(got.stdout, "roles append: stored record at ") {
		t.Fatalf("code=%d stdout=%q stderr=%q, want a stored record", got.code, got.stdout, got.stderr)
	}
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
	bgRepo(t)
	if n := len(boundChainStore(t)); n != 2 {
		t.Errorf("the chain of the opened workflow holds %d files, want 2 (seq 1 and 2)", n)
	}
}
