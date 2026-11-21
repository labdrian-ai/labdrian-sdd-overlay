package settings_test

import (
	"encoding/json"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// The top-level "outputStyle" key (T27, L57's ownership rules): the overlay owns it only while it
// holds exactly "labdrian". Any other value belongs to the owner or to another tool.

func outputStyleOf(doc settings.Document) (interface{}, bool) {
	v, ok := doc.Root()["outputStyle"]
	return v, ok
}

func TestMergeWritesTheOutputStyleKey(t *testing.T) {
	doc := settings.Empty()
	if !mustMerge(t, &doc) {
		t.Fatal("Merge reported no change on an empty document")
	}
	if v, ok := outputStyleOf(doc); !ok || v != "labdrian" {
		t.Errorf(`outputStyle = %v, %v; want "labdrian"`, v, ok)
	}
}

func TestMergeTwiceKeepsTheOutputStyleKeyAndChangesNothing(t *testing.T) {
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

func TestMergeKeepsForeignKeysNextToTheOutputStyleKey(t *testing.T) {
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

func TestMergeLeavesAForeignOutputStyleValueUntouched(t *testing.T) {
	for _, value := range []string{`"Gentleman"`, `""`, `null`, `7`, `{"name":"x"}`} {
		doc := mustParse(t, `{"outputStyle":`+value+`}`)
		mustMerge(t, &doc)
		got, _ := json.Marshal(doc.Root()["outputStyle"])
		if string(got) != value {
			t.Errorf("outputStyle = %s, want the foreign value %s kept", got, value)
		}
	}
}

func TestRemoveTakesOutTheOutputStyleKeyOnlyWhenOwned(t *testing.T) {
	owned := mustParse(t, `{"outputStyle":"labdrian","model":"opus"}`)
	if changed, err := owned.Remove(documentBinary); err != nil || !changed {
		t.Fatalf("Remove = %v, %v; want a change", changed, err)
	}
	if _, ok := outputStyleOf(owned); ok {
		t.Error("the owned outputStyle key is still there")
	}
	if owned.Root()["model"] != "opus" {
		t.Error("a foreign key was lost")
	}

	for _, value := range []string{`"Gentleman"`, `null`, `7`} {
		foreign := mustParse(t, `{"outputStyle":`+value+`}`)
		if changed, _ := foreign.Remove(documentBinary); changed {
			t.Errorf("Remove reported a change for the foreign outputStyle value %s", value)
		}
		got, _ := json.Marshal(foreign.Root()["outputStyle"])
		if string(got) != value {
			t.Errorf("outputStyle = %s, want the foreign value %s kept", got, value)
		}
	}
}

func TestOutputStyleState(t *testing.T) {
	for text, want := range map[string]settings.OutputStyleState{
		`{}`:                          settings.OutputStyleMissing,
		`{"outputStyle":"labdrian"}`:  settings.OutputStyleOwned,
		`{"outputStyle":"Gentleman"}`: settings.OutputStyleConflict,
		`{"outputStyle":null}`:        settings.OutputStyleConflict,
		`{"outputStyle":7}`:           settings.OutputStyleConflict,
	} {
		if got := settings.OutputStyle(mustParse(t, text).Root()); got != want {
			t.Errorf("OutputStyle(%s) = %v, want %v", text, got, want)
		}
	}
}

func TestOutputStyleValueJSONNamesTheValueAsItIsWritten(t *testing.T) {
	for text, want := range map[string]string{
		`{}`:                           ``,
		`{"outputStyle":"Gentleman"}`:  `"Gentleman"`,
		`{"outputStyle":"a<b>&c"}`:     `"a<b>&c"`,
		`{"outputStyle":{"name":"x"}}`: `{"name":"x"}`,
	} {
		if got := settings.OutputStyleValueJSON(mustParse(t, text).Root()); got != want {
			t.Errorf("OutputStyleValueJSON(%s) = %q, want %q", text, got, want)
		}
	}
}

func TestForeignOutputStyleNoticeSaysTheKeyWasLeftAlone(t *testing.T) {
	const want = `/h/.claude/settings.json holds "outputStyle": "Gentleman"; it was left as it is, so the labdrian output style was not enabled (set it to "labdrian" or remove the key, then run install-hooks again)`
	if got := settings.ForeignOutputStyleNotice("/h/.claude/settings.json", `"Gentleman"`); got != want {
		t.Errorf("notice = %q, want %q", got, want)
	}
}
