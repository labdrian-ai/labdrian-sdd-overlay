package review

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
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
		return nil, fmt.Errorf("%w, so it cannot be stored in a JSON file", ErrReplyNotJSON)
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

// Stored is a stored result as DecodeResult reads it back: the binding the reply was held to and the reply
// as it was stored.
type Stored struct {
	Binding reviewresult.Binding
	Reply   []byte
}

// DecodeResult reads a file that EncodeResult wrote. It reads the file as it is: it does not hold the reply
// to the binding again (reviewresult.Parse does), and the binding is not recomputed from git. A file that is
// not one EncodeResult writes (not one JSON object, a key written twice, a member it does not have, another
// version, no reply) is refused with an error that wraps ErrInvalidStored.
func DecodeResult(data []byte) (Stored, error) {
	var file storedFile
	err := jsonstrict.CheckNoDuplicateKeys(data)
	if err == nil {
		err = jsonstrict.DecodeStrict(data, "stored result", &file)
	}
	switch {
	case err != nil:
		return Stored{}, fmt.Errorf("%w: %v", ErrInvalidStored, err)
	case file.Version != StoredVersion:
		return Stored{}, fmt.Errorf("%w: version %d, want %d", ErrInvalidStored, file.Version, StoredVersion)
	case len(file.Reply) == 0 || string(file.Reply) == "null":
		return Stored{}, fmt.Errorf("%w: it holds no reply", ErrInvalidStored)
	}
	b := file.Binding
	return Stored{Binding: reviewresult.Binding{Base: b.Base, Head: b.Head, DiffSHA256: b.DiffSHA256, Paths: b.Paths}, Reply: file.Reply}, nil
}

// EncodeVerdict is the content of the file that stores the reply of the verifier: the reply, which must be
// one JSON value, indented with two spaces, ending in one newline, with the characters of the reply as they
// are, so that the file reads as the verifier wrote it. It does not judge the reply; reviewverdict.Parse has
// done so by the time the command calls it.
func EncodeVerdict(reply []byte) ([]byte, error) {
	if !json.Valid(reply) {
		return nil, fmt.Errorf("%w, so it cannot be stored in a JSON file", ErrReplyNotJSON)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, bytes.TrimSpace(reply), "", "  "); err != nil {
		return nil, fmt.Errorf("review: indent the stored verdict: %w", err)
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
