package settings_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

const documentBinary = "/opt/labdrian/bin/gentle-ai-overlay"

func mustParse(t *testing.T, text string) settings.Document {
	t.Helper()
	doc, err := settings.Parse([]byte(text))
	if err != nil {
		t.Fatalf("Parse(%q) = %v", text, err)
	}
	return doc
}

func mustMerge(t *testing.T, doc *settings.Document) bool {
	t.Helper()
	changed, err := doc.Merge(documentBinary)
	if err != nil {
		t.Fatalf("Merge() = %v", err)
	}
	return changed
}

func mustBytes(t *testing.T, doc settings.Document) string {
	t.Helper()
	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("Bytes() = %v", err)
	}
	return string(data)
}

func TestParseRefusesWhatIsNotAnObject(t *testing.T) {
	for _, text := range []string{"", "  ", "not json", "[]", `"x"`, "7", `{"a":1} x`, `{"hooks":{`} {
		if _, err := settings.Parse([]byte(text)); err == nil {
			t.Errorf("Parse(%q) succeeded, want the JSON error", text)
		}
	}
}

func TestParseKeepsJSONNullApartFromAnEmptyObject(t *testing.T) {
	if !mustParse(t, "null").Null() {
		t.Error("null is not reported as null")
	}
	if mustParse(t, "{}").Null() || settings.Empty().Null() {
		t.Error("an object, even an empty one, was reported as null")
	}
}

func TestMergeIntoAnEmptyDocumentInstallsEveryFamilyOnce(t *testing.T) {
	doc := settings.Empty()

	if !mustMerge(t, &doc) {
		t.Fatal("Merge() reported no change for an empty document")
	}
	if !settings.HasSupportedClaudeLifecycleState(doc.Root(), documentBinary) {
		t.Errorf("the merged document does not hold every family:\n%s", mustBytes(t, doc))
	}
	before := mustBytes(t, doc)
	if mustMerge(t, &doc) {
		t.Error("a second Merge() reported a change")
	}
	if after := mustBytes(t, doc); after != before {
		t.Errorf("a second Merge() changed the bytes:\n%s\nbecame\n%s", before, after)
	}
}

func TestMergeKeepsWhatIsNotOurs(t *testing.T) {
	doc := mustParse(t, `{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo stop"}]}],
		"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"`+documentBinary+` other-verb"}]}]}}`)

	mustMerge(t, &doc)

	got := mustBytes(t, doc)
	for _, want := range []string{`"model": "opus"`, `echo stop`, documentBinary + ` other-verb`} {
		if !strings.Contains(got, want) {
			t.Errorf("Merge() lost %q:\n%s", want, got)
		}
	}
}

func TestRemoveUndoesMergeAndLeavesTheRest(t *testing.T) {
	doc := mustParse(t, `{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo stop"}]}]}}`)
	mustMerge(t, &doc)

	changed, err := doc.Remove(documentBinary)
	if err != nil || !changed {
		t.Fatalf("Remove() = %v, %v, want a change", changed, err)
	}
	want := mustBytes(t, mustParse(t, `{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo stop"}]}]}}`))
	if got := mustBytes(t, doc); got != want {
		t.Errorf("Remove() left\n%s\nwant\n%s", got, want)
	}
	if changed, err := doc.Remove(documentBinary); err != nil || changed {
		t.Errorf("a second Remove() = %v, %v, want no change", changed, err)
	}
}

func TestNoHookCommandChangesNothing(t *testing.T) {
	doc := mustParse(t, `{"hooks":{}}`)
	before := mustBytes(t, doc)

	if _, err := doc.Merge(""); !errors.Is(err, settings.ErrEmptyHookCommand) {
		t.Errorf("Merge(\"\") = %v, want ErrEmptyHookCommand", err)
	}
	if _, err := doc.Remove(""); !errors.Is(err, settings.ErrEmptyHookCommand) {
		t.Errorf("Remove(\"\") = %v, want ErrEmptyHookCommand", err)
	}
	if got := mustBytes(t, doc); got != before {
		t.Errorf("a refused call changed the document: %s", got)
	}
}

// Merging into a settings.json that holds null used to write to a nil map and crash the process.
func TestMergeIntoNullIsRefusedAndRemoveIsANoOp(t *testing.T) {
	doc := mustParse(t, "null")

	if _, err := doc.Merge(documentBinary); !errors.Is(err, settings.ErrNotAnObject) {
		t.Errorf("Merge() = %v, want ErrNotAnObject", err)
	}
	if changed, err := doc.Remove(documentBinary); err != nil || changed {
		t.Errorf("Remove() = %v, %v, want no change and no error", changed, err)
	}
	if got := mustBytes(t, doc); got != "null" {
		t.Errorf("Bytes() = %q, want null", got)
	}
}

func TestBytesAreSortedTwoSpaceIndentedAndEscapeHTML(t *testing.T) {
	doc := mustParse(t, `{"b":1,"a":{"y":"<x>&","x":[1,2.5]}}`)

	want := "{\n  \"a\": {\n    \"x\": [\n      1,\n      2.5\n    ],\n    \"y\": \"\\u003cx\\u003e\\u0026\"\n  },\n  \"b\": 1\n}"
	if got := mustBytes(t, doc); got != want {
		t.Errorf("Bytes() = %q, want %q", got, want)
	}
}
