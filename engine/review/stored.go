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
	return encodeFile("result", storedFile{
		Version: StoredVersion,
		Binding: bindingOf(c),
		Reply:   json.RawMessage(bytes.TrimSpace(reply)),
	})
}

// bindingOf is the binding of c as the stored files hold it.
func bindingOf(c Candidate) storedBinding {
	return storedBinding{Base: c.Base, Head: c.Head, DiffSHA256: c.DiffSHA256, Paths: c.Paths}
}

// encodeFile writes file as the stored files are written: indented with two spaces, ending in one newline, and
// with the characters of what it holds as they are (a '<' or an '&' is not escaped for HTML). what names the file
// in the error.
func encodeFile(what string, file any) ([]byte, error) {
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(file); err != nil {
		return nil, fmt.Errorf("review: encode the stored %s: %w", what, err)
	}
	return out.Bytes(), nil
}

// decodeFile reads a stored file into file as a stored file is read: one JSON object, no key written twice, and no
// member that file does not have, at any level. Any other file is refused with an error that wraps
// ErrInvalidStored. what names the file in the error.
func decodeFile(data []byte, what string, file any) error {
	err := jsonstrict.CheckNoDuplicateKeys(data)
	if err == nil {
		err = jsonstrict.DecodeStrict(data, what, file)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidStored, err)
	}
	return nil
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
	if err := decodeFile(data, "stored result", &file); err != nil {
		return Stored{}, err
	}
	switch {
	case file.Version != StoredVersion:
		return Stored{}, fmt.Errorf("%w: version %d, want %d", ErrInvalidStored, file.Version, StoredVersion)
	case len(file.Reply) == 0 || string(file.Reply) == "null":
		return Stored{}, fmt.Errorf("%w: it holds no reply", ErrInvalidStored)
	}
	return Stored{Binding: file.Binding.binding(), Reply: file.Reply}, nil
}

// binding is the stored binding as reviewresult holds a reply to it.
func (b storedBinding) binding() reviewresult.Binding {
	return reviewresult.Binding{Base: b.Base, Head: b.Head, DiffSHA256: b.DiffSHA256, Paths: b.Paths}
}

// The stored file of an exemption, as EncodeExemption writes it:
//
//	{
//	  "version": 1,
//	  "binding": {"base": "...", "head": "...", "diff_sha256": "...", "paths": ["..."]},
//	  "risk": "none",
//	  "report": {...the report of the verifier, as it gave it...}
//	}
//
// The report is the verifier's, member for member and character for character (white space aside); the risk is the
// declaration the agent made; the binding is the candidate the command computed from git. Together they let any
// later reader hold the risk and the report to the rules they were made under again (CheckRisk, CheckReport) and
// compare the binding with the one it computes for the same commits (CheckBinding), without asking the verifier
// or the parent for anything.
type storedExemptionFile struct {
	Version int             `json:"version"`
	Binding storedBinding   `json:"binding"`
	Risk    string          `json:"risk"`
	Report  json.RawMessage `json:"report"`
}

// EncodeExemption is the content of the file that stores an exemption: the declared risk and the report, which
// must be one JSON value, with the binding of c. It is written like a result (EncodeResult): indented with two
// spaces, ending in one newline, with the characters of the report as they are. It does not judge the risk or the
// report; CheckRisk and CheckReport have done so by the time the command calls it.
func EncodeExemption(c Candidate, risk string, report []byte) ([]byte, error) {
	if !json.Valid(report) {
		return nil, fmt.Errorf("%w, so it cannot be stored in a JSON file", ErrReplyNotJSON)
	}
	return encodeFile("exemption", storedExemptionFile{
		Version: StoredVersion,
		Binding: bindingOf(c),
		Risk:    risk,
		Report:  json.RawMessage(bytes.TrimSpace(report)),
	})
}

// StoredExemption is a stored exemption as DecodeExemption reads it back: the binding the exemption was made on,
// the risk that was declared and the report as it was stored.
type StoredExemption struct {
	Binding reviewresult.Binding
	Risk    string
	Report  []byte
}

// DecodeExemption reads a file that EncodeExemption wrote. It reads the file as it is: it does not hold the risk
// or the report to their rules again (CheckRisk and CheckReport do), and the binding is not recomputed from git
// (CheckBinding compares it). A file that is not one EncodeExemption writes (not one JSON object, a key written
// twice, a member it does not have, another version, no risk, no report, the file of a result) is refused with an
// error that wraps ErrInvalidStored.
func DecodeExemption(data []byte) (StoredExemption, error) {
	var file storedExemptionFile
	if err := decodeFile(data, "stored exemption", &file); err != nil {
		return StoredExemption{}, err
	}
	switch {
	case file.Version != StoredVersion:
		return StoredExemption{}, fmt.Errorf("%w: version %d, want %d", ErrInvalidStored, file.Version, StoredVersion)
	case file.Risk == "":
		return StoredExemption{}, fmt.Errorf("%w: it holds no risk", ErrInvalidStored)
	case len(file.Report) == 0 || string(file.Report) == "null":
		return StoredExemption{}, fmt.Errorf("%w: it holds no report", ErrInvalidStored)
	}
	return StoredExemption{Binding: file.Binding.binding(), Risk: file.Risk, Report: file.Report}, nil
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
