package review

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// The stored file of a launch, as EncodeResult writes it:
//
//	{
//	  "version": 1,
//	  "binding": {"base": "...", "head": "...", "diff_sha256": "...", "paths": ["..."]},
//	  "reply": {...the reply of the reviewer, as it gave it...}
//	}
//
// The reply is the reviewer's, member for member and character for character (white space aside); the
// binding is the candidate the command computed from git and held the reply to. Together they let any
// later reader hold the reply to its binding again (reviewresult.Parse) and compare the binding with the
// one it computes for the same commits, without asking the reviewer or the parent for anything.
type storedFile struct {
	Version int             `json:"version"`
	Binding storedBinding   `json:"binding"`
	Reply   json.RawMessage `json:"reply"`
}

type storedBinding struct {
	Base       string   `json:"base"`
	Head       string   `json:"head"`
	DiffSHA256 string   `json:"diff_sha256"`
	Paths      []string `json:"paths"`
}

// EncodeResult is the content of the file that stores a reply: reply, which must be one JSON value, with the
// binding of c. It is indented with two spaces, ends in one newline, and writes the characters of the reply
// as they are (a '<' or an '&' is not escaped for HTML), so that the file reads as the reviewer wrote it.
// It does not judge the reply; reviewresult.Parse has done so by the time the command calls it.
func EncodeResult(c Candidate, reply []byte) ([]byte, error) {
	if !json.Valid(reply) {
		return nil, fmt.Errorf("review: the reply is not JSON, so it cannot be stored in a JSON file")
	}
	file := storedFile{
		Version: StoredVersion,
		Binding: storedBinding{Base: c.Base, Head: c.Head, DiffSHA256: c.DiffSHA256, Paths: c.Paths},
		Reply:   json.RawMessage(bytes.TrimSpace(reply)),
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(file); err != nil {
		return nil, fmt.Errorf("review: encode the stored result: %w", err)
	}
	return out.Bytes(), nil
}
