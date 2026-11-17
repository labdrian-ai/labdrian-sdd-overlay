package settingsfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

// InspectOutputStyle says what a settings file holds under the top-level "outputStyle" key, as
// InspectMainAgent does for "agent": a file that is not there has no key, a foreign value is a
// conflict carrying that value as JSON, and a file that is not JSON is the decoder's error.
func TestInspectOutputStyle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		file  *string
		state settings.OutputStyleState
		value string
		fails bool
	}{
		{"no file", nil, settings.OutputStyleMissing, "", false},
		{"no key", ptr(`{"model":"opus"}`), settings.OutputStyleMissing, "", false},
		{"ours", ptr(`{"outputStyle":"labdrian"}`), settings.OutputStyleOwned, `"labdrian"`, false},
		{"foreign", ptr(`{"outputStyle":"Gentleman"}`), settings.OutputStyleConflict, `"Gentleman"`, false},
		{"null", ptr(`{"outputStyle":null}`), settings.OutputStyleConflict, `null`, false},
		{"not JSON", ptr(`{"outputStyle":`), settings.OutputStyleMissing, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if tc.file != nil {
				if err := os.WriteFile(path, []byte(*tc.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			state, value, err := settingsfile.Installer{}.InspectOutputStyle(path)
			if (err != nil) != tc.fails {
				t.Fatalf("error = %v, want failure %v", err, tc.fails)
			}
			wantValue := tc.value
			if tc.state == settings.OutputStyleOwned {
				wantValue = `"labdrian"`
			}
			if state != tc.state || (!tc.fails && value != wantValue) {
				t.Errorf("InspectOutputStyle = %v, %q, want %v, %q", state, value, tc.state, wantValue)
			}
		})
	}
}

func ptr(s string) *string { return &s }
