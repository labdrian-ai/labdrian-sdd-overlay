package settings_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

func TestParseRefusesAByteOrderMarkInTheWordsOfGo121(t *testing.T) {
	_, err := settings.Parse([]byte("\xEF\xBB\xBF{}"))
	want := "invalid character 'ï' looking for beginning of value"
	if err == nil || err.Error() != want {
		t.Fatalf("Parse(BOM + {}) error = %v, want %q", err, want)
	}
}
