package settings_test

import (
	"encoding/json"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

func agentOf(t *testing.T, doc settings.Document) (interface{}, bool) {
	t.Helper()
	v, ok := doc.Root()["agent"]
	return v, ok
}

func TestMergeWritesTheMainAgentKey(t *testing.T) {
	doc := settings.Empty()
	if !mustMerge(t, &doc) {
		t.Fatal("Merge reported no change on an empty document")
	}
	if v, ok := agentOf(t, doc); !ok || v != "foreman" {
		t.Errorf(`agent = %v, %v; want "foreman"`, v, ok)
	}
}

func TestMergeTwiceKeepsTheMainAgentKeyAndChangesNothing(t *testing.T) {
	doc := settings.Empty()
	mustMerge(t, &doc)
	first := mustBytes(t, doc)
	if mustMerge(t, &doc) {
		t.Error("the second Merge reported a change")
	}
	if got := mustBytes(t, doc); got != first {
		t.Errorf("the second Merge changed the bytes:\n%s\nwant\n%s", got, first)
	}
}

func TestMergeKeepsForeignKeysNextToTheMainAgentKey(t *testing.T) {
	doc := mustParse(t, `{"model":"opus","env":{"A":"1"},"theme":"dark"}`)
	mustMerge(t, &doc)
	root := doc.Root()
	for key, want := range map[string]string{"model": `"opus"`, "env": `{"A":"1"}`, "theme": `"dark"`} {
		got, _ := json.Marshal(root[key])
		if string(got) != want {
			t.Errorf("%s = %s, want %s", key, got, want)
		}
	}
}

func TestMergeLeavesAForeignAgentValueUntouched(t *testing.T) {
	doc := mustParse(t, `{"agent":"someone-else"}`)
	mustMerge(t, &doc)
	if v, _ := agentOf(t, doc); v != "someone-else" {
		t.Errorf("agent = %v, want the foreign value kept", v)
	}
}

func TestRemoveTakesOutTheMainAgentKeyOnlyWhenOwned(t *testing.T) {
	owned := mustParse(t, `{"agent":"foreman","model":"opus"}`)
	if changed, err := owned.Remove(documentBinary); err != nil || !changed {
		t.Fatalf("Remove = %v, %v; want a change", changed, err)
	}
	if _, ok := agentOf(t, owned); ok {
		t.Error("the owned agent key is still there")
	}
	if owned.Root()["model"] != "opus" {
		t.Error("a foreign key was lost")
	}

	foreign := mustParse(t, `{"agent":"someone-else"}`)
	if changed, _ := foreign.Remove(documentBinary); changed {
		t.Error("Remove reported a change for a foreign agent value")
	}
	if v, _ := agentOf(t, foreign); v != "someone-else" {
		t.Errorf("agent = %v, want the foreign value kept", v)
	}
}

func TestMainAgentState(t *testing.T) {
	for text, want := range map[string]settings.MainAgentState{
		`{}`:                  settings.MainAgentMissing,
		`{"agent":"foreman"}`: settings.MainAgentOwned,
		`{"agent":"other"}`:   settings.MainAgentConflict,
		`{"agent":7}`:         settings.MainAgentConflict,
	} {
		if got := settings.MainAgent(mustParse(t, text).Root()); got != want {
			t.Errorf("MainAgent(%s) = %v, want %v", text, got, want)
		}
	}
}
