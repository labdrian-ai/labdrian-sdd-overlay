package projectid_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// The vectors are the ones of the identity module (identity/testdata), the rules both modules
// share (Phase 9, D2): the same files, run here through this module's own reader of the repository.
// Each case builds a repository whose .git/config is the vector, with no git process, and checks
// the identity the whole chain answers.

const vectorsDir = "../../../identity/testdata"

func readVectors(t *testing.T, name string, into any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(vectorsDir, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// repositoryWithConfig is a main checkout whose git config is exactly config.
func repositoryWithConfig(t *testing.T, config string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(repo, ".git", "config"), config)
	return repo
}

func TestResolve_EveryOriginConfigVectorGivesTheRecordedIdentity(t *testing.T) {
	var vectors []struct {
		Config string `json:"config"`
		Want   string `json:"want"`
	}
	readVectors(t, "origin-vectors.json", &vectors)
	if len(vectors) < 20 {
		t.Fatalf("only %d vectors: the file was truncated", len(vectors))
	}
	for _, v := range vectors {
		v := v
		t.Run(strings.ReplaceAll(v.Config, "\n", "|"), func(t *testing.T) {
			id := resolve(t, repositoryWithConfig(t, v.Config))
			if v.Want == "" {
				if id.Rule != projectid.RuleCommonDir {
					t.Errorf("config %q resolved by rule %q to %q, want the common dir rule: it names no origin to key on", v.Config, id.Rule, id.Project)
				}
				return
			}
			if id.Rule != projectid.RuleRemote || id.Project != v.Want {
				t.Errorf("config %q resolved to %q via %q, want %q via %q", v.Config, id.Project, id.Rule, v.Want, projectid.RuleRemote)
			}
		})
	}
}

func TestResolve_EveryRemoteVectorGivesTheRecordedIdentity(t *testing.T) {
	var vectors []struct {
		URL  string `json:"url"`
		Want string `json:"want"`
	}
	readVectors(t, "remote-vectors.json", &vectors)
	if len(vectors) < 30 {
		t.Fatalf("only %d vectors: the file was truncated", len(vectors))
	}
	for _, v := range vectors {
		v := v
		t.Run(v.URL, func(t *testing.T) {
			id := resolve(t, repositoryWithConfig(t, "[remote \"origin\"]\n\turl = "+v.URL+"\n"))
			if v.Want == "" {
				if id.Rule != projectid.RuleCommonDir {
					t.Errorf("remote %q resolved by rule %q to %q, want the common dir rule: it has no host to key on", v.URL, id.Rule, id.Project)
				}
				return
			}
			if id.Rule != projectid.RuleRemote || id.Project != v.Want {
				t.Errorf("remote %q resolved to %q via %q, want %q via %q", v.URL, id.Project, id.Rule, v.Want, projectid.RuleRemote)
			}
		})
	}
}
