package review_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// replyText is a reply of the reviewer with characters that a JSON encoder escapes for HTML: the file
// holds what the reviewer said, and a reader of the file sees the text and not an escape.
const replyText = `{"candidate":{"base":"` + baseSHA + `","head":"` + headSHA + `","diff_sha256":"` + diffSHA + `"},` +
	`"inspection":{"status":"completed","paths":["engine/guard.go","engine/guard_test.go"],"reason":""},` +
	`"findings":[],"evidence":["read a < b && c > d"]}`

// The stored file is the reply with the binding it was validated against and the lens rows its launch was sent:
// a later reader holds the reply to that binding again, compares the binding with the one it computes from git,
// and counts the rows.
func TestEncodeResultStoresTheBindingTheRowsAndTheReply(t *testing.T) {
	data, err := review.EncodeResult(candidate(t), sentRows(), []byte(replyText))
	if err != nil {
		t.Fatalf("EncodeResult() error = %v, want nil", err)
	}
	var got struct {
		Version int `json:"version"`
		Binding struct {
			Base       string   `json:"base"`
			Head       string   `json:"head"`
			DiffSHA256 string   `json:"diff_sha256"`
			Paths      []string `json:"paths"`
		} `json:"binding"`
		Rows  []string        `json:"rows"`
		Reply json.RawMessage `json:"reply"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("the stored file is not JSON: %v\n%s", err, data)
	}
	if got.Version != 1 || review.StoredVersion != 1 {
		t.Errorf("version = %d (StoredVersion %d), want 1", got.Version, review.StoredVersion)
	}
	if got.Binding.Base != baseSHA || got.Binding.Head != headSHA || got.Binding.DiffSHA256 != diffSHA {
		t.Errorf("binding = %+v, want base %s, head %s and diff_sha256 %s", got.Binding, baseSHA, headSHA, diffSHA)
	}
	if !reflect.DeepEqual(got.Binding.Paths, changedPaths()) {
		t.Errorf("binding paths = %q, want %q", got.Binding.Paths, changedPaths())
	}
	if !reflect.DeepEqual(got.Rows, sentRows()) {
		t.Errorf("rows = %q, want the rows that were given, in the order they were given: %q", got.Rows, sentRows())
	}
	var storedReply, givenReply any
	if err := json.Unmarshal(got.Reply, &storedReply); err != nil {
		t.Fatalf("the stored reply is not JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(replyText), &givenReply); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !reflect.DeepEqual(storedReply, givenReply) {
		t.Errorf("the stored reply is %s, want the reply that was given: %s", got.Reply, replyText)
	}
}

// The file is for people as well as programs: indented, ending in a newline, and with the characters
// of the reply as they were written.
func TestEncodeResultWritesAReadableFileThatKeepsTheCharactersOfTheReply(t *testing.T) {
	data, err := review.EncodeResult(candidate(t), sentRows(), []byte(replyText))
	if err != nil {
		t.Fatalf("EncodeResult() error = %v, want nil", err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "{\n  \"version\": 1,\n  \"binding\": {\n") {
		t.Errorf("the file begins %q, want an indented object that starts with the version and the binding", firstLines(text, 3))
	}
	if !strings.Contains(text, "  },\n  \"rows\": [\n    \"default\",\n    \"readability\"\n  ],\n  \"reply\": {\n") {
		t.Errorf("the file does not hold the rows, one to a line, between the binding and the reply:\n%s", text)
	}
	if !strings.HasSuffix(text, "}\n") {
		t.Errorf("the file ends %q, want a closing brace and one newline", text[max(0, len(text)-5):])
	}
	if !strings.Contains(text, `"read a < b && c > d"`) {
		t.Errorf("the file does not hold the text of the reply as written (< and & unescaped):\n%s", text)
	}
}

func TestEncodeResultWritesNoPathAsAnEmptyArray(t *testing.T) {
	c, err := review.NewCandidate(baseSHA, headSHA, nil, nil)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	data, err := review.EncodeResult(c, sentRows(), []byte(replyText))
	if err != nil {
		t.Fatalf("EncodeResult() error = %v, want nil", err)
	}
	if !strings.Contains(string(data), `"paths": []`) {
		t.Errorf("the file does not say \"paths\": []:\n%s", data)
	}
}

// What is not JSON cannot be embedded in a JSON file; the command never gets here with such a reply (it
// is refused first), and the function does not write a file that does not parse.
func TestEncodeResultRefusesAReplyThatIsNotJSON(t *testing.T) {
	for _, reply := range []string{"", "no", `{"a":`} {
		if data, err := review.EncodeResult(candidate(t), sentRows(), []byte(reply)); !errors.Is(err, review.ErrReplyNotJSON) {
			t.Errorf("EncodeResult(%q) = %q, %v, want an error wrapping ErrReplyNotJSON", reply, data, err)
		}
	}
}

// The function does not write a file that DecodeResult would refuse: rows that are not a list of the rows of the
// canonical file are refused, and nothing is returned. The command never gets here with such rows (ParseRows
// refused them first).
func TestEncodeResultRefusesRowsThatAreNotRows(t *testing.T) {
	for name, rows := range map[string][]string{
		"no rows": nil, "an empty list": {}, "a row nobody wrote": {"default", "nope"}, "a row twice": {"default", "default"},
	} {
		t.Run(name, func(t *testing.T) {
			if data, err := review.EncodeResult(candidate(t), rows, []byte(replyText)); !errors.Is(err, review.ErrInvalidRows) || data != nil {
				t.Errorf("EncodeResult(%q) = %q, %v, want no file and an error wrapping ErrInvalidRows", rows, data, err)
			}
		})
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitAfter(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "")
}
