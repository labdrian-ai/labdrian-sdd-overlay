package identity

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The vectors under testdata/ were recorded from the rules as longterm-mem had them before they
// moved here (Phase 9, D2), and they are the contract the engine and longterm-mem hold their own
// readers to. longterm-mem runs the same files through its adapter; the engine does not yet, so
// its agreement with them is not proved by a test of its own.

func TestNormalizeRemoteMatchesTheRecordedVectors(t *testing.T) {
	var vectors []struct {
		URL  string `json:"url"`
		Want string `json:"want"`
	}
	readVectors(t, "testdata/remote-vectors.json", &vectors)
	if len(vectors) < 30 {
		t.Fatalf("only %d vectors: the file was truncated", len(vectors))
	}
	for _, v := range vectors {
		v := v
		t.Run(v.URL, func(t *testing.T) {
			if got := NormalizeRemote(v.URL); got != v.Want {
				t.Errorf("NormalizeRemote(%q) = %q, want %q", v.URL, got, v.Want)
			}
		})
	}
}

func TestOriginRemoteMatchesTheRecordedVectors(t *testing.T) {
	var vectors []struct {
		Config string `json:"config"`
		Want   string `json:"want"`
	}
	readVectors(t, "testdata/origin-vectors.json", &vectors)
	if len(vectors) < 20 {
		t.Fatalf("only %d vectors: the file was truncated", len(vectors))
	}
	for _, v := range vectors {
		v := v
		t.Run(strings.ReplaceAll(v.Config, "\n", "|"), func(t *testing.T) {
			got, ok := OriginRemote(v.Config)
			if got != v.Want || ok != (v.Want != "") {
				t.Errorf("OriginRemote(%q) = %q, %v, want %q", v.Config, got, ok, v.Want)
			}
		})
	}
}

func readVectors(t *testing.T, path string, into any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
