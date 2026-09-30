package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// approveSkillBody is a lint-clean SKILL.md: only hard findings block approval.
const approveSkillBody = "---\n" +
	"name: wall-clock\n" +
	"description: Prove the production entry point stamps a real approval time.\n" +
	"license: Apache-2.0\n" +
	"metadata:\n" +
	"  author: someone\n" +
	"  version: 1.0.0\n" +
	"---\n" +
	"\n" +
	"## Activation Contract\n" +
	"\n" +
	"Use only in the approve wiring test.\n"

// TestRunSkillsCore_ApproveStampsTheWallClock proves the production skills
// entry point supplies a real clock to `skills approve`. The engine/skills
// package cannot read the time itself (its import allowlist excludes "time"),
// so this wiring is the only place a record's approved_at is decided.
func TestRunSkillsCore_ApproveStampsTheWallClock(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "skills")
	skillDir := filepath.Join(root, "wall-clock")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(approveSkillBody), 0o644); err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC().Truncate(time.Second)
	var out, errBuf bytes.Buffer
	code := -1
	runSkillsCore("approve",
		// --registry only places the overlay lock file, in the temporary directory
		// and not in the working directory of the test.
		[]string{"approve", "--id", "wall-clock", "--approver", "test-reviewer", "--source-root", root, "--registry", filepath.Join(base, "skills.registry.yaml")},
		&out, &errBuf, func(c int) { code = c })
	after := time.Now().UTC()

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, errBuf.String())
	}
	data, err := os.ReadFile(filepath.Join(skillDir, skills.ApprovalRecordName))
	if err != nil {
		t.Fatalf("record not written: %v (stdout %q)", err, out.String())
	}
	rec, err := skills.ParseApprovalRecord(data)
	if err != nil {
		t.Fatalf("record does not parse: %v\n%s", err, data)
	}
	at, err := time.Parse(time.RFC3339, rec.ApprovedAt)
	if err != nil {
		t.Fatalf("approved_at %q is not RFC 3339: %v", rec.ApprovedAt, err)
	}
	if at.Before(before) || at.After(after) {
		t.Errorf("approved_at %s is outside the run window [%s, %s]", at, before, after)
	}
	if !strings.HasSuffix(rec.ApprovedAt, "Z") {
		t.Errorf("approved_at %q must be UTC", rec.ApprovedAt)
	}
}
