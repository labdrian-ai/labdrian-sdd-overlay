package skills

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
)

// decodeRecord decodes the bytes of a JSON record the domain reads (the approval record of a skill,
// the project lock) into v, strictly (Phase 9, H19, decision D4): a field v does not have, bytes
// after the value, input that is not UTF-8 and a key that appears twice in any object are all
// refused. The standard decoder takes the last value of a repeated key and replaces bad bytes with
// U+FFFD without a word, so a record could say one thing to this program and another to a person
// reading the file. name is what the record is called in the refusal ("record", "lock"). Syntax and
// shape are decoded first, so every input that was refused before is refused with the same words.
func decodeRecord(data []byte, name string, v any) error {
	if err := jsonstrict.DecodeStrict(data, name, v); err != nil {
		return err
	}
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return err
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return err
	}
	return nil
}

// refuseRecord words a refusal of the bytes of a record for what the verb says: "parse approval
// record: <why>".
func refuseRecord(what string, err error) error { return fmt.Errorf("parse %s: %w", what, err) }
