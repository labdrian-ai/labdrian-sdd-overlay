package reviewresult

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
)

// The members of each object of the contract, as agents/reviewer.md spells them. A reply holds every one
// of them in every object and none is null: an empty array says "nothing", a missing or null member says
// nothing at all, and the contract never reads that as "nothing".
var (
	resultMembers     = []string{"candidate", "inspection", "findings", "evidence"}
	candidateMembers  = []string{"base", "head", "diff_sha256"}
	inspectionMembers = []string{"status", "paths", "reason"}
	findingMembers    = []string{"id", "location", "severity", "claim", "evidence_class", "causal_disposition", "proof_refs"}
)

// The wire form of a reply: what the reviewer's JSON holds, member by member. It stays inside this file;
// the exported types carry no tag, so the domain does not own the format.
type wireResult struct {
	Candidate struct {
		Base       string `json:"base"`
		Head       string `json:"head"`
		DiffSHA256 string `json:"diff_sha256"`
	} `json:"candidate"`
	Inspection struct {
		Status string   `json:"status"`
		Paths  []string `json:"paths"`
		Reason string   `json:"reason"`
	} `json:"inspection"`
	Findings []wireFinding `json:"findings"`
	Evidence []string      `json:"evidence"`
}

type wireFinding struct {
	ID            string   `json:"id"`
	Location      string   `json:"location"`
	Severity      string   `json:"severity"`
	Claim         string   `json:"claim"`
	EvidenceClass string   `json:"evidence_class"`
	Disposition   string   `json:"causal_disposition"`
	ProofRefs     []string `json:"proof_refs"`
}

// decode reads the reply as the value it states, or says why it is not the object of the contract.
func decode(data []byte) (Result, error) {
	if err := checkShape(data); err != nil {
		return Result{}, err
	}
	var wire wireResult
	if err := jsonstrict.DecodeStrict(data, "reviewer result", &wire); err != nil {
		return Result{}, unparseable("%v", err)
	}
	result := Result{
		Candidate: Candidate{
			Base:       wire.Candidate.Base,
			Head:       wire.Candidate.Head,
			DiffSHA256: wire.Candidate.DiffSHA256,
		},
		Inspection: Inspection{
			Status: InspectionStatus(wire.Inspection.Status),
			Paths:  wire.Inspection.Paths,
			Reason: wire.Inspection.Reason,
		},
		Findings: make([]Finding, len(wire.Findings)),
		Evidence: wire.Evidence,
	}
	for i, f := range wire.Findings {
		result.Findings[i] = Finding{
			ID:            f.ID,
			Location:      f.Location,
			Severity:      Severity(f.Severity),
			Claim:         f.Claim,
			EvidenceClass: EvidenceClass(f.EvidenceClass),
			Disposition:   Disposition(f.Disposition),
			ProofRefs:     f.ProofRefs,
		}
	}
	return result, nil
}

// checkShape fails when the reply is not, in form, the object of the contract: valid UTF-8, one JSON value
// with no key written twice, and in every object (the reply, its candidate, its inspection and each of its
// findings) all the members of the contract and no null among them. The types of the values are checked
// by the decoding that follows.
func checkShape(data []byte) error {
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return unparseable("%v", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return unparseable("%v", err)
	}
	reply, err := strictObject(data, "reply", resultMembers)
	if err != nil {
		return err
	}
	for _, nested := range []struct {
		member  string
		members []string
	}{
		{"candidate", candidateMembers},
		{"inspection", inspectionMembers},
	} {
		if _, err := strictObject(reply[nested.member], nested.member, nested.members); err != nil {
			return err
		}
	}
	var findings []json.RawMessage
	if err := json.Unmarshal(reply["findings"], &findings); err != nil {
		return unparseable("findings is not an array")
	}
	for i, finding := range findings {
		record := fmt.Sprintf("findings[%d]", i)
		members, err := object(finding, record, findingMembers)
		if err != nil {
			return err
		}
		// The id is what the rest of the contract follows a finding by, so a finding that has none is
		// refused as that, not as a member missing from the shape.
		if id, ok := members["id"]; !ok || isNull(id) {
			return fmt.Errorf("%w: %s has no id", ErrMissingID, record)
		}
		if err := require(members, record, findingMembers); err != nil {
			return err
		}
	}
	return nil
}

// strictObject reads raw as the JSON object of one record of the contract: an object with no member the
// contract does not name and every member it does, none null.
func strictObject(raw json.RawMessage, record string, members []string) (map[string]json.RawMessage, error) {
	got, err := object(raw, record, members)
	if err != nil {
		return nil, err
	}
	return got, require(got, record, members)
}

// object reads raw as a JSON object with no member but the given ones, and returns its members, or says
// why raw is not that.
func object(raw json.RawMessage, record string, members []string) (map[string]json.RawMessage, error) {
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil || got == nil {
		return nil, unparseable("%s is not a JSON object", record)
	}
	// raw is a JSON object, so this check can fail only on a member the contract does not name; it
	// matches names exactly, where the decoding that follows would match them whatever their case.
	if err := jsonstrict.CheckKnownFields(raw, record, members); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnknownMember, err)
	}
	return got, nil
}

// require fails unless every one of the members is in the object and none is null.
func require(got map[string]json.RawMessage, record string, members []string) error {
	for _, member := range members {
		value, ok := got[member]
		if !ok {
			return unparseable("%s has no %q member", record, member)
		}
		if isNull(value) {
			return unparseable("%s has a null %q member", record, member)
		}
	}
	return nil
}

func isNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

// unparseable is ErrUnparseable with the detail of what is wrong with the reply.
func unparseable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnparseable, fmt.Sprintf(format, args...))
}
