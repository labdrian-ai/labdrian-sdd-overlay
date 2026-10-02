package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

func sha256HexTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type rolesRun struct {
	code   int
	stdout string
	stderr string
}

func runRolesTest(args []string, stdin string) rolesRun {
	var out, errBuf bytes.Buffer
	code := -1
	exited := false
	runRolesCore(args, strings.NewReader(stdin), &out, &errBuf, func(c int) {
		if !exited {
			code = c
			exited = true
		}
	})
	if !exited {
		code = 0
	}
	return rolesRun{code: code, stdout: out.String(), stderr: errBuf.String()}
}

func rolesTestStateHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

func rolesHandoffJSON(seq int, prevSHA, from, to, status, resumeReason string) string {
	if prevSHA == "" {
		prevSHA = strings.Repeat("0", 64)
	}
	statusField := `"status": "` + status + `"`
	if resumeReason != "" {
		statusField += `, "resume_reason": "` + resumeReason + `"`
	}
	return `{
  "version": 1,
  "project_id": "proj-1",
  "goal_id": "goal-1",
  "chain_id": "chain-1",
  "seq": ` + itoaTest(seq) + `,
  "from_role": "` + from + `",
  "to_role": "` + to + `",
  "prev_sha256": "` + prevSHA + `",
  "payload_kind": "diff",
  "payload_sha256": "` + strings.Repeat("a", 64) + `",
  "evidence": [],
  "context": {"summary": "s", "decisions": [], "open_questions": []},
  ` + statusField + `
}`
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func TestRunRolesCoreRequiresVerb(t *testing.T) {
	r := runRolesTest(nil, "")
	if r.code != 1 {
		t.Fatalf("code = %d, want 1", r.code)
	}
}

func TestRunRolesCoreUnknownVerb(t *testing.T) {
	r := runRolesTest([]string{"astronaut"}, "")
	if r.code != 1 || !strings.Contains(r.stderr, "unknown verb") {
		t.Fatalf("code=%d stderr=%q, want exit 1 with an unknown-verb message", r.code, r.stderr)
	}
}

func TestRolesValidateAcceptsValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handoff.json")
	data := rolesHandoffJSON(1, "", "shaper", "estimator", "completed", "")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runRolesTest([]string{"validate", "--file", path}, "")
	if r.code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q, want 0", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "\"valid\": true") {
		t.Fatalf("stdout = %q, want it to report valid: true", r.stdout)
	}
	if !strings.Contains(r.stdout, "no roles command launches an agent") {
		t.Fatalf("stdout = %q, want the no-authority disclosure", r.stdout)
	}
}

func TestRolesValidateRejectsInvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handoff.json")
	if err := os.WriteFile(path, []byte(`{"version": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runRolesTest([]string{"validate", "--file", path}, "")
	if r.code != 1 {
		t.Fatalf("code = %d, want 1 for an invalid record", r.code)
	}
	if !strings.Contains(r.stderr, "error") {
		t.Fatalf("stderr = %q, want an error message", r.stderr)
	}
}

func TestRolesValidateRejectsMissingFile(t *testing.T) {
	r := runRolesTest([]string{"validate", "--file", "/nonexistent/handoff.json"}, "")
	if r.code != 1 {
		t.Fatalf("code = %d, want 1 for a missing file", r.code)
	}
}

func TestRolesAppendNextResumeRoundTrip(t *testing.T) {
	rolesTestStateHome(t)
	r1 := rolesHandoffJSON(1, "", "shaper", "estimator", "completed", "")
	appended := runRolesTest([]string{"append", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1", "--stdin"}, r1)
	if appended.code != 0 {
		t.Fatalf("append code=%d stdout=%q stderr=%q, want 0", appended.code, appended.stdout, appended.stderr)
	}
	if !strings.Contains(appended.stdout, "no roles command launches an agent") {
		t.Fatalf("append stdout = %q, want the no-authority disclosure", appended.stdout)
	}

	resume := runRolesTest([]string{"resume", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1"}, "")
	if resume.code != 0 {
		t.Fatalf("resume code=%d stdout=%q stderr=%q, want 0", resume.code, resume.stdout, resume.stderr)
	}
	if !strings.Contains(resume.stdout, "\"role\": \"estimator\"") || !strings.Contains(resume.stdout, "\"interrupted\": false") {
		t.Fatalf("resume stdout = %q, want role=estimator, interrupted=false", resume.stdout)
	}

	next := runRolesTest([]string{"next", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1"}, "")
	if next.code != 0 {
		t.Fatalf("next code=%d stdout=%q stderr=%q, want 0", next.code, next.stdout, next.stderr)
	}
	if !strings.Contains(next.stdout, "\"builder\"") {
		t.Fatalf("next stdout = %q, want builder listed as the next role from estimator", next.stdout)
	}
}

func TestRolesResumeReportsInterrupted(t *testing.T) {
	rolesTestStateHome(t)
	r1 := rolesHandoffJSON(1, "", "shaper", "estimator", "completed", "")
	if a := runRolesTest([]string{"append", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1", "--stdin"}, r1); a.code != 0 {
		t.Fatalf("append seq1 code=%d stderr=%q", a.code, a.stderr)
	}
	r2 := rolesHandoffJSON(2, "", "estimator", "builder", "interrupted", "network down")
	// prev_sha256 must be the real digest of r1's bytes; recompute via a
	// second append attempt using the CLI's own validate path is overkill
	// here, so compute it the same way the store does.
	r2 = withPrevSHA(r2, sha256HexTest([]byte(r1)))
	if a := runRolesTest([]string{"append", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1", "--stdin"}, r2); a.code != 0 {
		t.Fatalf("append seq2 code=%d stdout=%q stderr=%q", a.code, a.stdout, a.stderr)
	}

	resume := runRolesTest([]string{"resume", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1"}, "")
	if resume.code != 0 {
		t.Fatalf("resume code=%d stderr=%q", resume.code, resume.stderr)
	}
	if !strings.Contains(resume.stdout, "\"interrupted\": true") || !strings.Contains(resume.stdout, "network down") {
		t.Fatalf("resume stdout = %q, want interrupted=true with the reason", resume.stdout)
	}
}

func TestRolesResumeAndNextErrorOnMissingChain(t *testing.T) {
	rolesTestStateHome(t)
	for _, verb := range []string{"resume", "next"} {
		r := runRolesTest([]string{verb, "--project", "nope", "--goal", "nope", "--chain", "nope"}, "")
		if r.code != 1 {
			t.Fatalf("%s code = %d, want 1 for a chain with no records", verb, r.code)
		}
	}
}

func TestRolesAppendRejectsIdentityMismatch(t *testing.T) {
	rolesTestStateHome(t)
	r1 := rolesHandoffJSON(1, "", "shaper", "estimator", "completed", "")
	r := runRolesTest([]string{"append", "--project", "other-project", "--goal", "goal-1", "--chain", "chain-1", "--stdin"}, r1)
	if r.code != 1 || !strings.Contains(r.stderr, "project") {
		t.Fatalf("code=%d stderr=%q, want a refusal naming the project_id mismatch", r.code, r.stderr)
	}
}

// The append verb reads stdin through the domain's record bound, not a CLI-wide cap, so
// every record it accepts is one a chain store can load back: a larger stdin is refused
// before parsing, naming roles.MaxRecordBytes.
func TestRolesAppendBoundsStdinAtTheRecordBound(t *testing.T) {
	rolesTestStateHome(t)
	r := runRolesTest([]string{"append", "--project", "p3", "--goal", "g3", "--chain", "c3", "--stdin"}, strings.Repeat(" ", roles.MaxRecordBytes+1))
	want := fmt.Sprintf("stdin exceeds %d bytes", roles.MaxRecordBytes)
	if r.code != 1 || !strings.Contains(r.stderr, want) {
		t.Fatalf("code=%d stderr=%q, want a refusal containing %q", r.code, r.stderr, want)
	}
}

// rolesPaddedRecord is the first record of proj-1/goal-1/chain-1 grown to exactly size
// bytes with blanks before the closing brace: still the same one valid document.
func rolesPaddedRecord(size int) string {
	record := rolesHandoffJSON(1, "", "shaper", "estimator", "completed", "")
	return strings.Replace(record, "\n}", strings.Repeat(" ", size-len(record))+"\n}", 1)
}

// The bound is exact on the accepting side too: the verb takes a record of exactly
// roles.MaxRecordBytes, and the reading verbs load that chain back, because the store
// reads through the same bound. Nothing the verb accepts is a record the chain cannot hold.
func TestRolesAppendAcceptsARecordOfExactlyTheBoundAndResumeLoadsItBack(t *testing.T) {
	rolesTestStateHome(t)
	record := rolesPaddedRecord(roles.MaxRecordBytes)
	if len(record) != roles.MaxRecordBytes {
		t.Fatalf("the fixture is %d bytes, want exactly roles.MaxRecordBytes (%d)", len(record), roles.MaxRecordBytes)
	}
	appended := runRolesTest([]string{"append", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1", "--stdin"}, record)
	if appended.code != 0 {
		t.Fatalf("append code=%d stderr=%q, want 0 for a record of exactly the bound", appended.code, appended.stderr)
	}
	for _, verb := range []string{"resume", "next"} {
		r := runRolesTest([]string{verb, "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1"}, "")
		if r.code != 0 {
			t.Fatalf("%s code=%d stderr=%q, want 0: the chain holding a record of the bound must load", verb, r.code, r.stderr)
		}
	}
	resume := runRolesTest([]string{"resume", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1"}, "")
	if !strings.Contains(resume.stdout, `"role": "estimator"`) {
		t.Fatalf("resume stdout = %q, want the loaded record's role (estimator)", resume.stdout)
	}
}

// The validate verb reads its file through the same bound as the append verb reads stdin,
// so a file of any size costs the bound and not its size, and the refusal names the bound
// before anything is parsed.
func TestRolesValidateBoundsItsFileAtTheRecordBound(t *testing.T) {
	dir := t.TempDir()
	over := filepath.Join(dir, "over.json")
	if err := os.WriteFile(over, []byte(rolesPaddedRecord(roles.MaxRecordBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runRolesTest([]string{"validate", "--file", over}, "")
	want := fmt.Sprintf("file %q exceeds %d bytes", over, roles.MaxRecordBytes)
	if r.code != 1 || !strings.Contains(r.stderr, want) {
		t.Fatalf("code=%d stderr=%q, want a refusal containing %q", r.code, r.stderr, want)
	}

	exact := filepath.Join(dir, "exact.json")
	if err := os.WriteFile(exact, []byte(rolesPaddedRecord(roles.MaxRecordBytes)), 0o600); err != nil {
		t.Fatal(err)
	}
	r = runRolesTest([]string{"validate", "--file", exact}, "")
	if r.code != 0 || !strings.Contains(r.stdout, `"valid": true`) {
		t.Fatalf("code=%d stdout=%q stderr=%q, want a file of exactly the bound validated", r.code, r.stdout, r.stderr)
	}
}

// A file far over the bound is refused without being read in full: the read stops one byte
// past the bound. A sparse file occupies almost no disk but reports its full size.
func TestRolesValidateDoesNotReadAFileFarOverTheBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(512 << 20); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	r := runRolesTest([]string{"validate", "--file", path}, "")
	runtime.ReadMemStats(&after)
	want := fmt.Sprintf("file %q exceeds %d bytes", path, roles.MaxRecordBytes)
	if r.code != 1 || !strings.Contains(r.stderr, want) {
		t.Fatalf("code=%d stderr=%q, want a refusal containing %q", r.code, r.stderr, want)
	}
	// Reading the bound plus one byte grows the read buffer a few times over (ReadAll
	// doubles it), so the limit is eight times the bound: room for that growth and for the
	// command's own work, and 16 times less than reading the 512 MiB file whole would
	// allocate. Tighten it only with a measurement of the read; loosen it and the test
	// stops telling a bounded read from an unbounded one.
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8*roles.MaxRecordBytes {
		t.Errorf("validate of a 512 MiB file allocated %d bytes, want at most %d: the file must not be read past the bound", allocated, 8*roles.MaxRecordBytes)
	}
}

func TestRolesAppendRejectsInvalidTransition(t *testing.T) {
	rolesTestStateHome(t)
	bad := rolesHandoffJSON(1, "", "builder", "sweeper", "completed", "")
	r := runRolesTest([]string{"append", "--project", "p2", "--goal", "g2", "--chain", "c2", "--stdin"}, bad)
	if r.code != 1 {
		t.Fatalf("code = %d, want 1 for a first record whose from_role is not prototyper or shaper", r.code)
	}
}

func TestRolesMatchShaperReportsVocabularyMatches(t *testing.T) {
	root, _ := shaperWorktree(t, shaperTestHandoffV3, shaperTestGoal("standalone-shaper-handoff", `[]`))
	r := runRolesTest([]string{"match-shaper", "--root", root, "--handoff", "handoff.json"}, "")
	if r.code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q, want 0", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, `"role": "reviewer"`) || !strings.Contains(r.stdout, `"matches_vocabulary": true`) {
		t.Fatalf("stdout = %q, want reviewer reported as a vocabulary match", r.stdout)
	}
	if !strings.Contains(r.stdout, `"role": "implementer"`) || !strings.Contains(r.stdout, `"matches_vocabulary": false`) {
		t.Fatalf("stdout = %q, want implementer reported as not matching the vocabulary", r.stdout)
	}
}

func withPrevSHA(data, digest string) string {
	return strings.Replace(data, `"prev_sha256": "`+strings.Repeat("0", 64)+`"`, `"prev_sha256": "`+digest+`"`, 1)
}
