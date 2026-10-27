package reviewverdict

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
)

// The members of each object of the contract, as agents/verifier.md spells them. A reply holds every one of
// them in every verdict (or, in the report of an exemption, in every check) and none is null: an empty string
// says "nothing", and a missing or null member says nothing at all.
var (
	verdictMembers = []string{"id", "result", "command", "output", "base_result"}
	checkMembers   = []string{"command", "result", "output"}
)

// The wire form of a reply: what the verifier's JSON holds, member by member. It stays inside this file; the
// exported types carry no tag, so the domain does not own the format.
type wireReply struct {
	Verdicts []struct {
		ID         string `json:"id"`
		Result     string `json:"result"`
		Command    string `json:"command"`
		Output     string `json:"output"`
		BaseResult string `json:"base_result"`
	} `json:"verdicts"`
}

// wireReport is the wire form of the report of an exemption (report.go).
type wireReport struct {
	Checks []struct {
		Command string `json:"command"`
		Result  string `json:"result"`
		Output  string `json:"output"`
	} `json:"checks"`
}

// decode reads the reply as the value it states, or says why it is not the object of the contract.
func decode(data []byte) ([]Verdict, error) {
	if err := checkShape(data, "verdicts", verdictMembers); err != nil {
		return nil, err
	}
	var wire wireReply
	if err := jsonstrict.DecodeStrict(data, "verifier reply", &wire); err != nil {
		return nil, unparseable("%v", err)
	}
	verdicts := make([]Verdict, len(wire.Verdicts))
	for i, w := range wire.Verdicts {
		verdicts[i] = Verdict{ID: w.ID, Result: Reproduction(w.Result), Command: w.Command, Output: w.Output, Base: Reproduction(w.BaseResult)}
	}
	return verdicts, nil
}

// decodeReport reads the report of an exemption as the value it states, or says why it is not the object of the
// contract.
func decodeReport(data []byte) ([]Check, error) {
	if err := checkShape(data, "checks", checkMembers); err != nil {
		return nil, err
	}
	var wire wireReport
	if err := jsonstrict.DecodeStrict(data, "verifier report", &wire); err != nil {
		return nil, unparseable("%v", err)
	}
	checks := make([]Check, len(wire.Checks))
	for i, w := range wire.Checks {
		checks[i] = Check{Command: w.Command, Result: CheckResult(w.Result), Output: w.Output}
	}
	return checks, nil
}

// checkShape fails when the reply is not, in form, the object of the contract: valid UTF-8, one JSON value
// with no key written twice, and then, object by object in the order the reply holds them (the reply and each
// item of its list: a verdict, or a check), the members of the contract and no others. list is the one member
// of the reply, an array, and members are the members of each item. In each object a member the contract does
// not name is refused first, then a member it names that is missing or null. The types of the values are
// checked by the decoding that follows, once every object has been looked at.
func checkShape(data []byte, list string, members []string) error {
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return unparseable("%v", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return unparseable("%v", err)
	}
	reply, err := strictObject(data, "reply", []string{list})
	if err != nil {
		return err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(reply[list], &items); err != nil {
		return unparseable("%s is not an array", list)
	}
	for i, item := range items {
		if _, err := strictObject(item, fmt.Sprintf("%s[%d]", list, i), members); err != nil {
			return err
		}
	}
	return nil
}

// strictObject reads raw as the JSON object of one record of the contract: an object with no member the
// contract does not name and every member it does, none null.
func strictObject(raw json.RawMessage, record string, members []string) (map[string]json.RawMessage, error) {
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil || got == nil {
		return nil, unparseable("%s is not a JSON object", record)
	}
	// raw is a JSON object, so this check can fail only on a member the contract does not name; it matches
	// names exactly, where the decoding that follows would match them whatever their case.
	if err := jsonstrict.CheckKnownFields(raw, record, members); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnknownMember, err)
	}
	for _, member := range members {
		value, ok := got[member]
		if !ok {
			return nil, unparseable("%s has no %q member", record, member)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, unparseable("%s has a null %q member", record, member)
		}
	}
	return got, nil
}

// unparseable is ErrUnparseable with the detail of what is wrong with the reply.
func unparseable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnparseable, fmt.Sprintf(format, args...))
}
