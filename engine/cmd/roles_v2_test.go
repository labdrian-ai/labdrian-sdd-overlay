package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

const rolesGoalEngramDirective = `{"version": 1, "scope": "goal", "sources": ["engram"], "write": "none"}`

// rolesV2HandoffJSON is rolesHandoffJSON as a completed version 2 record, with directive as
// its memory_directive when not empty.
func rolesV2HandoffJSON(seq int, prevSHA, from, to, directive string) string {
	data := strings.Replace(rolesHandoffJSON(seq, prevSHA, from, to, "completed", ""), `"version": 1`, `"version": 2`, 1)
	if directive != "" {
		data = strings.Replace(data, `"status": "completed"`, `"status": "completed", "memory_directive": `+directive, 1)
	}
	return data
}

func rolesChainFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(os.Getenv("XDG_STATE_HOME"), "labdrian", "role-chains", "proj-1", "goal-1", "chain-1", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestRolesValidateAcceptsAV2RecordAndShowsItsDirective(t *testing.T) {
	path := filepath.Join(t.TempDir(), "handoff.json")
	if err := os.WriteFile(path, []byte(rolesV2HandoffJSON(1, "", "shaper", "estimator", rolesGoalEngramDirective)), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runRolesTest([]string{"validate", "--file", path}, "")
	want := `{
  "valid": true,
  "from_role": "shaper",
  "to_role": "estimator",
  "status": "completed",
  "memory_directive": {
    "version": 1,
    "scope": "goal",
    "sources": [
      "engram"
    ],
    "write": "none"
  },
  "authority": "` + rolesAuthorityNote + `"
}
`
	if r.code != 0 || r.stdout != want || r.stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q, want 0 and %q", r.code, r.stdout, r.stderr, want)
	}
}

// TestRolesValidateV1OutputIsUnchanged pins the v1 output word for word: no directive member.
func TestRolesValidateV1OutputIsUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "handoff.json")
	if err := os.WriteFile(path, []byte(rolesHandoffJSON(1, "", "shaper", "estimator", "completed", "")), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runRolesTest([]string{"validate", "--file", path}, "")
	want := `{
  "valid": true,
  "from_role": "shaper",
  "to_role": "estimator",
  "status": "completed",
  "authority": "` + rolesAuthorityNote + `"
}
`
	if r.code != 0 || r.stdout != want {
		t.Fatalf("code=%d stdout=%q stderr=%q, want 0 and %q", r.code, r.stdout, r.stderr, want)
	}
}

func TestRolesValidateRefusesADirectiveOnAV1Record(t *testing.T) {
	path := filepath.Join(t.TempDir(), "handoff.json")
	data := strings.Replace(rolesHandoffJSON(1, "", "shaper", "estimator", "completed", ""), `"status": "completed"`, `"status": "completed", "memory_directive": `+rolesGoalEngramDirective, 1)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runRolesTest([]string{"validate", "--file", path}, "")
	want := "error: roles validate: parse role handoff: unknown role handoff field \"memory_directive\"\n"
	if r.code != 1 || r.stderr != want || r.stdout != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q, want 1 and %q", r.code, r.stdout, r.stderr, want)
	}
}

func TestRolesAppendMovesAChainFromV1ToV2ButNeverBack(t *testing.T) {
	rolesTestStateHome(t)
	flags := []string{"append", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1", "--stdin"}
	r1 := rolesHandoffJSON(1, "", "shaper", "estimator", "completed", "")
	if a := runRolesTest(flags, r1); a.code != 0 {
		t.Fatalf("append v1 code=%d stderr=%q, want 0", a.code, a.stderr)
	}
	r2 := rolesV2HandoffJSON(2, sha256hex.Sum([]byte(r1)), "estimator", "builder", rolesGoalEngramDirective)
	if a := runRolesTest(flags, r2); a.code != 0 {
		t.Fatalf("append v2 code=%d stderr=%q, want 0", a.code, a.stderr)
	}
	files := rolesChainFiles(t)
	if len(files) != 2 {
		t.Fatalf("chain files = %v, want 2 records stored", files)
	}
	stored, err := os.ReadFile(files[1])
	if err != nil || string(stored) != r2 {
		t.Fatalf("stored seq 2 = %q, %v; want the exact v2 bytes appended", stored, err)
	}

	r3 := rolesHandoffJSON(3, sha256hex.Sum([]byte(r2)), "builder", "reviewer", "completed", "")
	back := runRolesTest(flags, r3)
	want := "error: roles append: role chain store: append: role chain: seq 3 has version 1 after seq 2 version 2; a chain never moves back to an older version\n"
	if back.code != 1 || back.stderr != want {
		t.Fatalf("append v1 after v2 code=%d stderr=%q, want 1 and %q", back.code, back.stderr, want)
	}
	if files := rolesChainFiles(t); len(files) != 2 {
		t.Fatalf("chain files after a refused append = %v, want still 2", files)
	}

	resume := runRolesTest([]string{"resume", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1"}, "")
	if resume.code != 0 || !strings.Contains(resume.stdout, `"role": "builder"`) || !strings.Contains(resume.stdout, `"interrupted": false`) {
		t.Fatalf("resume code=%d stdout=%q stderr=%q, want role=builder, interrupted=false", resume.code, resume.stdout, resume.stderr)
	}
	next := runRolesTest([]string{"next", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1"}, "")
	if next.code != 0 || !strings.Contains(next.stdout, `"reviewer"`) {
		t.Fatalf("next code=%d stdout=%q stderr=%q, want reviewer listed", next.code, next.stdout, next.stderr)
	}
}
