package hookwire_test

// Tests for the PostToolUse decoder and reply (C23 M7): the call is kept as the gate reads a
// PreToolUse one, and of tool_response only its SHA-256 and its length are kept.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"runtime"
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

// endlessResponse serves a PostToolUse input whose tool_response is a string of size bytes, made
// as it is read, so the test holds none of it.
type endlessResponse struct {
	head, tail string
	size, sent int
	stage      int
}

func (e *endlessResponse) Read(p []byte) (int, error) {
	switch e.stage {
	case 0:
		e.stage = 1
		return copy(p, e.head), nil
	case 1:
		n := min(len(p), e.size-e.sent)
		for i := range p[:n] {
			p[i] = 'a'
		}
		e.sent += n
		if e.sent == e.size {
			e.stage = 2
		}
		return n, nil
	case 2:
		e.stage = 3
		return copy(p, e.tail), nil
	}
	return 0, io.EOF
}

func TestPostToolUseHashesTheResponseWithoutHoldingIt(t *testing.T) {
	const size = 48 << 20
	in := &endlessResponse{head: `{"tool_name":"t","tool_response":"`, tail: `","cwd":"/r"}`, size: size}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	got, err := hookwire.DecodePostToolUse(in)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("DecodePostToolUse: %v", err)
	}
	if got.ResponseBytes != size+2 || got.Cwd != "/r" {
		t.Errorf("response of %d bytes, cwd %q; want %d, /r", got.ResponseBytes, got.Cwd, size+2)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8<<20 {
		t.Errorf("decoding a %d MiB response allocated %d MiB: the response was held, not streamed", size>>20, allocated>>20)
	}
}

func TestPostToolUseFindsTheResponseWhateverItHolds(t *testing.T) {
	// The escapes are joined from two literals so that no tool rewrites them on the way to this file.
	response := `{"a":"} ] \" {","b":[1,{"c":null}],"d":"\` + `u0022x"}`
	in := `{"tool_input":{"tool_response":"not this"},"tool\` + `u005fresponse":` + response + `,"tool_name":"t"}`
	got, err := hookwire.DecodePostToolUse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("DecodePostToolUse: %v", err)
	}
	sum := sha256.Sum256([]byte(response))
	if got.ResponseSHA256 != hex.EncodeToString(sum[:]) || got.Tool != "t" {
		t.Errorf("digest %q, tool %q; want the digest of %s", got.ResponseSHA256, got.Tool, response)
	}
}

func TestPostToolUseHashesTheResponseAsItWasSent(t *testing.T) {
	for _, response := range []string{
		`[ 1 , -0.5e+3 , 0 , 12E7 , true , false , null , "" ]`,
		"{\n\t\"a\" : { } ,\r\n \"b\" : [ ]\n}",
		`-12.25`,
		`"\/\b\f\n\r\t\\"`,
	} {
		in := `{"tool_name":"t",  "tool_response" :  ` + response + "  \n}\n"
		got, err := hookwire.DecodePostToolUse(strings.NewReader(in))
		if err != nil {
			t.Fatalf("%s: %v", response, err)
		}
		sum := sha256.Sum256([]byte(response))
		if got.ResponseSHA256 != hex.EncodeToString(sum[:]) || got.ResponseBytes != int64(len(response)) {
			t.Errorf("%s: digest %q, %d bytes; want the digest of the value as sent", response, got.ResponseSHA256, got.ResponseBytes)
		}
	}
}

func TestPostToolUseRefusesAResponseThatIsNotJSON(t *testing.T) {
	for _, response := range []string{
		`[1,]`, `{"a" 1}`, `{"a":1,}`, `01`, `1.`, `1e`, `-`, `tru`, `"\x"`, "\"a\tb\"", `{1:2}`, `[1 2]`,
		strings.Repeat("[", 10001) + strings.Repeat("]", 10001),
	} {
		in := `{"tool_name":"t","tool_response":` + response + `}`
		if _, err := hookwire.DecodePostToolUse(strings.NewReader(in)); err == nil {
			t.Errorf("accepted the response %.40s", response)
		}
	}
}

func TestPostToolUseKeepsTheEnvelopeBoundForTheOtherFields(t *testing.T) {
	in := `{"tool_name":"t","tool_input":"` + strings.Repeat("a", hookwire.MaxEnvelopeBytes) + `","tool_response":"ok"}`
	if _, err := hookwire.DecodePostToolUse(strings.NewReader(in)); !errors.Is(err, hookwire.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge for fields other than the response over the envelope bound", err)
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
