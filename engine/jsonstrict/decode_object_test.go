package jsonstrict

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDecodeObjectWordsATypeErrorAsGo121Does(t *testing.T) {
	for _, tc := range []struct{ in, kind string }{
		{`[1]`, "array"}, {`"x"`, "string"}, {`1`, "number"}, {`true`, "bool"},
	} {
		fields, err := DecodeObject([]byte(tc.in))
		want := "json: cannot unmarshal " + tc.kind + " into Go value of type map[string]json.RawMessage"
		if err == nil || err.Error() != want {
			t.Errorf("DecodeObject(%s) error = %v, want %q", tc.in, err, want)
		}
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			t.Errorf("DecodeObject(%s): errors.As to *json.UnmarshalTypeError is false for %v", tc.in, err)
		}
		if fields != nil {
			t.Errorf("DecodeObject(%s) map = %v, want nil", tc.in, fields)
		}
	}

	fields, err := DecodeObject([]byte(`null`))
	if err != nil || fields != nil {
		t.Errorf("DecodeObject(null) = %v, %v; want nil map, nil error", fields, err)
	}

	fields, err = DecodeObject([]byte(`{"a":1}`))
	if err != nil || string(fields["a"]) != "1" || len(fields) != 1 {
		t.Errorf("DecodeObject({\"a\":1}) = %v, %v", fields, err)
	}

	_, err = DecodeObject([]byte(`{`))
	if err == nil {
		t.Fatal("DecodeObject({) = nil error")
	}
	var syn *json.SyntaxError
	if !errors.As(err, &syn) || strings.Contains(err.Error(), "cannot unmarshal") {
		t.Errorf("DecodeObject({) error = %v, want the toolchain's own syntax error", err)
	}
}
