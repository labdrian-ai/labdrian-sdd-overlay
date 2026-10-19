// Package jsonstrict provides field-agnostic strict JSON wire validation
// shared by record parsers. It checks UTF-8 validity, rejects duplicate
// object keys at every nesting depth, rejects trailing data after the
// top-level JSON value, and rejects unknown top-level fields. It does not
// validate field requiredness, field shapes, or any record-specific
// semantics.
package jsonstrict

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// CheckUTF8 returns an error if data is not valid UTF-8.
func CheckUTF8(data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("input is not valid UTF-8")
	}
	return nil
}

// CheckNoDuplicateKeys validates the JSON token structure while rejecting
// duplicate keys in every object, before decoding into a Go value can
// discard duplicate member values. It also rejects trailing data after the
// top-level JSON value.
func CheckNoDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := scanJSONValue(dec); err != nil {
		if err == io.EOF {
			return fmt.Errorf("empty JSON document")
		}
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing data after JSON value")
		}
		return fmt.Errorf("trailing data after JSON value: %w", err)
	}
	return nil
}

func scanJSONValue(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object member name is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("invalid JSON object termination")
		}
	case '[':
		for dec.More() {
			if err := scanJSONValue(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("invalid JSON array termination")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	return nil
}

// CheckKnownFields unmarshals data as a JSON object and rejects any
// top-level field name not present in allowedFields. recordName is used in
// the error message so callers can identify which record rejected the
// field. A non-object root (array, string, number, null, or malformed JSON)
// is rejected by the underlying unmarshal error.
//
// When several fields are unknown the error names the smallest of them by
// bytes, so the same document is refused with the same words every time,
// whatever order its members were written in.
func CheckKnownFields(data []byte, recordName string, allowedFields []string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("%s must be a JSON object", recordName)
	}
	allowed := make(map[string]struct{}, len(allowedFields))
	for _, name := range allowedFields {
		allowed[name] = struct{}{}
	}
	unknown, found := "", false
	for field := range fields {
		if _, ok := allowed[field]; ok {
			continue
		}
		if !found || field < unknown {
			unknown, found = field, true
		}
	}
	if found {
		return fmt.Errorf("unknown %s field %q", recordName, unknown)
	}
	return nil
}

// WireVersion is the version a versioned record's parser dispatches on: the
// top-level, exact-case version member of data when it is a JSON integer
// written in digits, and 0 otherwise (another spelling such as 3.0, 3e0 or
// "3", null, no version member, or data that is not a JSON object). JSON
// itself refuses a leading zero or a plus sign, and decoding into an int
// refuses a fraction, an exponent and a string, so the member is read as it
// is written. A parser leaves 0 and every version it does not know to its
// oldest path, which reports the value exactly as it did before the later
// versions existed.
func WireVersion(data []byte) int {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return 0
	}
	// An absent member is empty input, which json.Unmarshal refuses like any other non-integer.
	var version int
	if err := json.Unmarshal(fields["version"], &version); err != nil {
		return 0
	}
	return version
}

// DecodeStrict decodes data into v, rejecting unknown fields (fields not
// present in v's JSON tags) and trailing data after the top-level JSON
// value. recordName is used in the trailing-data error message.
func DecodeStrict(data []byte, recordName string, v interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return fmt.Errorf("trailing data after the %s value", recordName)
	}
	return nil
}
