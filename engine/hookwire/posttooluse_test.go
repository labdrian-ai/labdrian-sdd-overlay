package hookwire_test

// Tests for the PostToolUse decoder and reply (C23 M7): the call is kept as the gate reads a
// PreToolUse one, and of tool_response only its SHA-256 and its length are kept.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

func TestPostToolUseKeepsTheCallAndADigestOfTheResponse(t *testing.T) {
	response := `{"result":"two observations","ids":[1,2]}`
	in := `{"hook_event_name":"PostToolUse","cwd":"/repo/./sub/","session_id":"s-1","tool_use_id":"toolu_1",` +
		`"tool_name":"mcp__engram__mem_search","tool_input":{"project":"p","goal":"g","all_projects":true},` +
		`"tool_response":` + response + `,"later_field":[1,2,3]}`
	got, err := hookwire.DecodePostToolUse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("DecodePostToolUse: %v", err)
	}
	sum := sha256.Sum256([]byte(response))
	if got.Event != "PostToolUse" || got.Cwd != "/repo/sub" || got.Tool != "mcp__engram__mem_search" ||
		got.SessionID != "s-1" || got.CallID != "toolu_1" {
		t.Errorf("call = %+v", got)
	}
	if got.ResponseSHA256 != hex.EncodeToString(sum[:]) || got.ResponseBytes != int64(len(response)) {
		t.Errorf("response digest = %q, %d bytes; want %q, %d", got.ResponseSHA256, got.ResponseBytes, hex.EncodeToString(sum[:]), len(response))
	}
	q := got.Query()
	if !q.Named || q.Project != "p" || q.Goal != "g" || !q.AllProjects {
		t.Errorf("Query() = %+v", q)
	}
}

func TestPostToolUseReadsTheSkillTheFileAndTheID(t *testing.T) {
	cases := []struct {
		input, skill, file, id string
	}{
		{`{"skill":"memory-plan"}`, "memory-plan", "", ""},
		{`{"file_path":"/x/SKILL.md"}`, "", "/x/SKILL.md", ""},
		{`{"id":4216}`, "", "", "4216"},
		{`{"observation_id":17}`, "", "", "17"},
		{`{"engram_id":"42"}`, "", "", "42"},
		{`{"id":true}`, "", "", ""},
		{`"not an object"`, "", "", ""},
	}
	for _, c := range cases {
		in := `{"tool_name":"t","tool_input":` + c.input + `,"tool_response":"ok"}`
		got, err := hookwire.DecodePostToolUse(strings.NewReader(in))
		if err != nil {
			t.Fatalf("%s: %v", c.input, err)
		}
		if got.Skill() != c.skill || got.FilePath() != c.file || got.RecordID() != c.id {
			t.Errorf("%s: skill %q file %q id %q; want %q %q %q", c.input, got.Skill(), got.FilePath(), got.RecordID(), c.skill, c.file, c.id)
		}
	}
}

func TestPostToolUseHashesAResponseOverTheEnvelopeBound(t *testing.T) {
	response := `"` + strings.Repeat("a", hookwire.MaxEnvelopeBytes+10) + `"`
	in := `{"tool_name":"mcp__longterm-mem__get","tool_input":{"engram_id":1},"tool_response":` + response + `}`
	got, err := hookwire.DecodePostToolUse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("DecodePostToolUse: %v", err)
	}
	sum := sha256.Sum256([]byte(response))
	if got.ResponseSHA256 != hex.EncodeToString(sum[:]) || got.ResponseBytes != int64(len(response)) {
		t.Errorf("digest of a large response = %q, %d bytes", got.ResponseSHA256, got.ResponseBytes)
	}
}

func TestPostToolUseWithNoResponseHasNoDigest(t *testing.T) {
	got, err := hookwire.DecodePostToolUse(strings.NewReader(`{"tool_name":"t"}`))
	if err != nil {
		t.Fatalf("DecodePostToolUse: %v", err)
	}
	if got.ResponseSHA256 != "" || got.ResponseBytes != 0 {
		t.Errorf("no response gave digest %q, %d bytes", got.ResponseSHA256, got.ResponseBytes)
	}
}

func TestPostToolUseRefusesWhatItCannotUse(t *testing.T) {
	cases := map[string]string{
		"not an object":       `[1]`,
		"empty":               ``,
		"tool_name not text":  `{"tool_name":1}`,
		"session_id not text": `{"session_id":{}}`,
		"two objects":         `{} {}`,
		"cut short":           `{"tool_name":"t","tool_response":"abc`,
	}
	for name, in := range cases {
		if _, err := hookwire.DecodePostToolUse(strings.NewReader(in)); err == nil {
			t.Errorf("%s: accepted %q", name, in)
		}
	}
}

func TestPostToolUseRefusesAnInputOverItsBound(t *testing.T) {
	in := `{"tool_name":"t","tool_response":"` + strings.Repeat("a", hookwire.MaxPostToolUseBytes) + `"}`
	_, err := hookwire.DecodePostToolUse(strings.NewReader(in))
	if !errors.Is(err, hookwire.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestPostToolUseReplyIsAContextLineOrNothing(t *testing.T) {
	out, err := hookwire.PostToolUseReply{}.Encode()
	if err != nil || out != nil {
		t.Fatalf("empty reply = %q, %v; want nothing", out, err)
	}
	out, err = hookwire.PostToolUseReply{Context: "labdrian: not recorded <x>"}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"labdrian: not recorded <x>"}}` + "\n"
	if string(out) != want {
		t.Errorf("reply = %s, want %s", out, want)
	}
}
