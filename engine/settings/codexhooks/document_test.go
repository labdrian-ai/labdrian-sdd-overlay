package codexhooks

import (
	"bytes"
	"encoding/json"
	"regexp"
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
		if err != nil || bytes.Contains(repaired, []byte("--old")) || bytes.Count(repaired, []byte("--event SessionStart")) != 1 || !bytes.Contains(repaired, []byte("/opt/engine other")) {
			t.Fatalf("repair: %s %v", repaired, err)
		}
		duplicate := bytes.Replace(repaired, []byte(`"SessionStart":[`), []byte(`"SessionStart":[{"hooks":[{"command":"/opt/engine projection hook --old"}]},`), 1)
		deduped, _, err := Merge(duplicate, "/opt/engine")
		if err != nil || bytes.Contains(deduped, []byte("--old")) || bytes.Count(deduped, []byte("--event SessionStart")) != 1 {
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
	for _, input := range []string{"", `{`, `null`, `[]`, `{"hooks":[]}`, `{"hooks":{"SessionStart":{}}}`, `{"hooks":{"PreToolUse":{}}}`, `{"hooks":{"PostToolUse":[{"hooks":{}}]}}`, `{"hooks":{},"hooks":{}}`} {
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

// eventEntries reads back the handlers of one event: the matcher and the command of every group.
func eventEntries(t *testing.T, data []byte, event string) []struct{ Matcher, Command string } {
	t.Helper()
	var doc struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	var out []struct{ Matcher, Command string }
	for _, g := range doc.Hooks[event] {
		for _, h := range g.Hooks {
			out = append(out, struct{ Matcher, Command string }{g.Matcher, h.Command})
		}
	}
	return out
}

// M12b: the memory gate and the read evidence are two more owned entries, whose commands carry
// --source codex; the SessionStart entry of M12 is unchanged and does not carry it.
func TestMergeWritesThreeOwnedEntriesAndOnlyTheToolOnesCarryTheSource(t *testing.T) {
	got, changed, err := Merge(nil, "/opt/engine")
	if err != nil || !changed {
		t.Fatalf("merge: %t %v", changed, err)
	}
	session := eventEntries(t, got, "SessionStart")
	if len(session) != 1 || session[0].Matcher != "startup|resume|clear|compact" || strings.Contains(session[0].Command, "--source") ||
		!strings.Contains(session[0].Command, "projection hook --event SessionStart") {
		t.Errorf("SessionStart entry: %+v", session)
	}
	for _, event := range []string{"PreToolUse", "PostToolUse"} {
		entries := eventEntries(t, got, event)
		if len(entries) != 1 || entries[0].Matcher != memoryMatcher ||
			!strings.Contains(entries[0].Command, "projection hook --event "+event+" --source codex") ||
			!strings.HasSuffix(entries[0].Command, " || true") || !strings.HasPrefix(entries[0].Command, "command -v /opt/engine ") {
			t.Errorf("%s entry: %+v", event, entries)
		}
	}
	if missing, err := Missing(got, "/opt/engine"); err != nil || len(missing) != 0 {
		t.Errorf("missing after merge: %v %v", missing, err)
	}
	if ok, err := Present(got, "/opt/engine"); err != nil || !ok {
		t.Errorf("present after merge: %t %v", ok, err)
	}
}

// An install of M12 has only the SessionStart entry: the merge adds the other two and leaves
// the bytes of the first alone, so the owner's trust of it (a hash of its configuration) holds.
func TestMergeCompletesAnInstallThatHasOnlySessionStart(t *testing.T) {
	const entry = `{"hooks":[{"command":"command -v /opt/engine >/dev/null 2>&1 && /opt/engine projection hook --event SessionStart || true","type":"command"}],"matcher":"startup|resume|clear|compact"}`
	old := []byte(`{"hooks":{"SessionStart":[` + entry + `]}}`)
	if missing, err := Missing(old, "/opt/engine"); err != nil || strings.Join(missing, ",") != "PreToolUse,PostToolUse" {
		t.Fatalf("missing in an M12 install: %v %v", missing, err)
	}
	if ok, _ := Present(old, "/opt/engine"); ok {
		t.Error("an install with one of three entries is not present")
	}
	got, changed, err := Merge(old, "/opt/engine")
	if err != nil || !changed {
		t.Fatalf("merge: %t %v", changed, err)
	}
	if !bytes.Contains(got, []byte(entry)) {
		t.Errorf("the SessionStart entry was rewritten: %s", got)
	}
	if len(eventEntries(t, got, "PreToolUse")) != 1 || len(eventEntries(t, got, "PostToolUse")) != 1 {
		t.Errorf("tool entries missing: %s", got)
	}
}

func TestMissingDetectsDriftAndMergeRepairsIt(t *testing.T) {
	installed, _, err := Merge(nil, "/opt/engine")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, old, replacement string }{
		{"matcher", memoryMatcher, "Bash"},
		{"source", "--event PreToolUse --source codex", "--event PreToolUse"},
		{"command", "--event PreToolUse --source codex", "--event PreToolUse --source codex --old"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			drifted := bytes.Replace(installed, []byte(tt.old), []byte(tt.replacement), 1)
			before := bytes.Clone(drifted)
			missing, err := Missing(drifted, "/opt/engine")
			if err != nil || strings.Join(missing, ",") != "PreToolUse" {
				t.Fatalf("missing: %v %v", missing, err)
			}
			if ok, err := Present(drifted, "/opt/engine"); err != nil || ok || !bytes.Equal(drifted, before) {
				t.Fatalf("drift reported present or input mutated: %t %v", ok, err)
			}
			repaired, changed, err := Merge(drifted, "/opt/engine")
			if err != nil || !changed || !bytes.Equal(repaired, installed) {
				t.Fatalf("repair: %s %t %v", repaired, changed, err)
			}
			twice, changed, err := Merge(repaired, "/opt/engine")
			if err != nil || changed || !bytes.Equal(twice, repaired) {
				t.Fatalf("second merge: %t %v", changed, err)
			}
		})
	}
}

// The matcher is the narrowest one the gate acts on after the adapter: the Engram reads (with an
// optional plugin prefix) and longterm-mem query and get, in the spellings Codex sends. It is a
// Rust regex in Codex and the same syntax here.
func TestMemoryMatcherMatchesTheNamesCodexSendsForTheGatesReadsAndNothingElse(t *testing.T) {
	re := regexp.MustCompile(memoryMatcher)
	for _, name := range []string{
		"mcp__engram__mem_search", "mcp__engram__mem_context", "mcp__engram__mem_get_observation", "mcp__engram__mem_timeline",
		"mcp__plugin_engram_engram__mem_search", "mcp__plugin_engram_engram__mem_get_observation",
		"mcp__longterm_mem__query", "mcp__longterm_mem__get",
	} {
		if !re.MatchString(name) {
			t.Errorf("%s must be gated", name)
		}
	}
	for _, name := range []string{
		"mcp__engram__mem_save", "mcp__engram__mem_update", "mcp__longterm_mem__promote", "mcp__longterm_mem__sync",
		"mcp__longterm-mem__query", "mcp__engramx__mem_search", "mcp__longterm_memx__query", "mcp__a__engram__mem_search",
		"mcp__context7__resolve", "Bash", "apply_patch", "spawn_agent", "engram__mem_search", "mcp__engram__mem_search_extra", "",
	} {
		if re.MatchString(name) {
			t.Errorf("%s must not be matched", name)
		}
	}
}

func TestRemoveTakesOutAllThreeOwnedEntriesAndNothingElse(t *testing.T) {
	const foreign = `{"hooks":[{"type":"command","command":"moshi notify"}]}`
	input := []byte(`{"hooks":{"PreToolUse":[` + foreign + `],"PostToolUse":[` + foreign + `],"SessionStart":[` + foreign + `]}}`)
	merged, _, err := Merge(input, "/opt/engine")
	if err != nil {
		t.Fatal(err)
	}
	removed, changed, err := Remove(merged, "/opt/engine")
	if err != nil || !changed {
		t.Fatalf("remove: %t %v", changed, err)
	}
	if !bytes.Equal(removed, input) {
		t.Errorf("remove did not return the foreign document: %s", removed)
	}
	if missing, _ := Missing(removed, "/opt/engine"); len(missing) != 3 {
		t.Errorf("missing after remove: %v", missing)
	}
}

// A foreign handler in a tool event that merely names the binary is not ours.
func TestAForeignToolHandlerThatNamesTheBinaryIsKept(t *testing.T) {
	input := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"/opt/engine audit"}]}]}}`)
	merged, _, err := Merge(input, "/opt/engine")
	if err != nil || !bytes.Contains(merged, []byte(`"/opt/engine audit"`)) {
		t.Fatalf("foreign handler lost: %s %v", merged, err)
	}
	if n := len(eventEntries(t, merged, "PreToolUse")); n != 2 {
		t.Errorf("PreToolUse handlers = %d, want the foreign one and ours", n)
	}
}
