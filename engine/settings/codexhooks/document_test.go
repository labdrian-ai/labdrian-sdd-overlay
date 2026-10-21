package codexhooks

import (
	"bytes"
	"strings"
	"testing"
)

func TestMergeRemovePreserveForeignBytes(t *testing.T) {
	const gentle = `{ "hooks": [ {"type":"command", "command":"gentle-ai skill-registry refresh", "timeout":7} ], "extra": 9007199254740993 }`
	const moshi = `{"matcher":"startup", "hooks":[{"type":"command","command":"moshi notify"}], "unknown":"\u0061"}`
	const other = `"Other" : [ { "hooks": [], "future":true } ]`
	original := []byte(`{ "unknown" : {"n":1e+09}, "hooks" : {` + other + `, "SessionStart" : [` + gentle + `,` + moshi + `]}}`)
	merged, changed, err := Merge(original, "/opt/engine")
	if err != nil || !changed {
		t.Fatalf("merge: %t %v", changed, err)
	}
	for _, raw := range []string{gentle, moshi, other, `"unknown" : {"n":1e+09}`} {
		if !bytes.Contains(merged, []byte(raw)) {
			t.Errorf("lost foreign bytes %s in %s", raw, merged)
		}
	}
	if present, err := Present(merged, "/opt/engine"); err != nil || !present {
		t.Fatalf("present: %t %v", present, err)
	}
	twice, changed, err := Merge(merged, "/opt/engine")
	if err != nil || changed || !bytes.Equal(merged, twice) {
		t.Fatalf("second merge: changed=%t err=%v", changed, err)
	}
	removed, changed, err := Remove(merged, "/opt/engine")
	if err != nil || !changed {
		t.Fatalf("remove: %t %v", changed, err)
	}
	for _, raw := range []string{gentle, moshi, other} {
		if !bytes.Contains(removed, []byte(raw)) {
			t.Errorf("remove lost %s", raw)
		}
	}
	again, changed, err := Remove(removed, "/opt/engine")
	if err != nil || changed || !bytes.Equal(again, removed) {
		t.Fatalf("second remove: %t %v", changed, err)
	}
}

func TestEmptyReplacementAndOwnership(t *testing.T) {
	for _, input := range [][]byte{nil, []byte(`{}`), []byte(`{"hooks":{}}`)} {
		got, changed, err := Merge(input, "/opt/engine")
		if err != nil || !changed || !bytes.Contains(got, []byte(`"matcher":"startup|resume|clear|compact"`)) {
			t.Fatalf("empty: %s %t %v", got, changed, err)
		}
		old := bytes.ReplaceAll(got, []byte("--event SessionStart"), []byte("--event SessionStart --old"))
		old = bytes.Replace(old, []byte(`"SessionStart":[`), []byte(`"SessionStart":[{"hooks":[{"command":"/opt/engine other"}]},`), 1)
		repaired, _, err := Merge(old, "/opt/engine")
		if err != nil || bytes.Contains(repaired, []byte("--old")) || bytes.Count(repaired, []byte("projection hook")) != 1 || !bytes.Contains(repaired, []byte("/opt/engine other")) {
			t.Fatalf("repair: %s %v", repaired, err)
		}
		duplicate := bytes.Replace(repaired, []byte(`"SessionStart":[`), []byte(`"SessionStart":[{"hooks":[{"command":"/opt/engine projection hook --old"}]},`), 1)
		deduped, _, err := Merge(duplicate, "/opt/engine")
		if err != nil || bytes.Count(deduped, []byte("projection hook")) != 1 {
			t.Fatalf("duplicate survived: %s %v", deduped, err)
		}
		removed, _, err := Remove(got, "/opt/engine")
		if err != nil || bytes.Contains(removed, []byte("SessionStart")) {
			t.Fatalf("empty event survived: %s %v", removed, err)
		}
	}
	// Ownership tokens must occur in the same handler. A mixed group must retain its foreign handler and unknown keys.
	input := []byte(`{"hooks":{"SessionStart":[{"future":1,"hooks":[{"command":"/opt/engine other"},{"command":"foreign projection hook"},{"command":"/opt/engine projection hook --old"}]}]}}`)
	removed, _, err := Remove(input, "/opt/engine")
	if err != nil || !bytes.Contains(removed, []byte(`"future":1`)) || !bytes.Contains(removed, []byte("foreign projection hook")) || bytes.Contains(removed, []byte("--old")) {
		t.Fatalf("mixed: %s %v", removed, err)
	}
}

func TestMalformedIsUntouched(t *testing.T) {
	for _, input := range []string{"", `{`, `null`, `[]`, `{"hooks":[]}`, `{"hooks":{"SessionStart":{}}}`, `{"hooks":{},"hooks":{}}`} {
		for _, op := range []func([]byte, string) ([]byte, bool, error){Merge, Remove} {
			data, changed, err := op([]byte(input), "/opt/engine")
			if err == nil || changed || string(data) != input {
				t.Errorf("%q: %s %t %v", input, data, changed, err)
			}
		}
	}
	for _, op := range []func([]byte, string) ([]byte, bool, error){Merge, Remove} {
		if _, _, err := op(nil, ""); err == nil {
			t.Error("accepted empty binary")
		}
	}
}

func TestQuotedBinaryIsOwned(t *testing.T) {
	binary := "/opt/a b'c/engine"
	got, _, err := Merge(nil, binary)
	if err != nil {
		t.Fatal(err)
	}
	if yes, err := Present(got, binary); err != nil || !yes {
		t.Fatalf("quoted presence: %t %v %s", yes, err, got)
	}
	removed, _, err := Remove(got, binary)
	if err != nil || strings.Contains(string(removed), "SessionStart") {
		t.Fatalf("quoted removal: %s %v", removed, err)
	}
}
