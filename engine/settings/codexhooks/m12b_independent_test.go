package codexhooks

import (
	"bytes"
	"testing"
)

func TestM12bPreservesSessionStartMalformedHandlerDiagnostic(t *testing.T) {
	for _, tt := range []struct {
		name string
		run  func([]byte, string) ([]byte, bool, error)
	}{
		{"Merge", Merge},
		{"Remove", Remove},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(`{"hooks":{"SessionStart":[{"hooks":{}}]}}`)
			original := bytes.Clone(data)
			out, changed, err := tt.run(data, "/opt/labdrian")
			if err == nil || err.Error() != "group hooks must be an array" {
				t.Errorf("error = %v, want group hooks must be an array", err)
			}
			if changed {
				t.Error("malformed input reported a change")
			}
			if !bytes.Equal(out, original) || !bytes.Equal(data, original) {
				t.Errorf("malformed input changed: output %q, input %q, want %q", out, data, original)
			}
		})
	}
}
