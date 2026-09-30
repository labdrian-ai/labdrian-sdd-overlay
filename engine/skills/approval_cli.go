package skills

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RenderApproveCore is the testable CLI core for
//
//	labdrian skills approve --id <id> --approver <label>
//
// It records a human approval of skills/<id>/SKILL.md: a typed record bound to
// the SHA-256 of the exact file bytes, written next to the skill as
// ApprovalRecordName. `skills add` requires such a record for a global skill.
//
// The engine cannot prove that a human ran this verb: the approver label is
// whatever the caller passes, and any process that can write the file can write
// a record. What the verb guarantees, and what is tested, is that the record
// matches the exact bytes of the skill it sits next to.
//
// Preconditions, all checked before anything is written:
//   - --id is a valid skill slug, --approver is a non-blank single-line label
//     (there is deliberately no default: no engine convention records a human
//     identity, and a value taken from the environment would read as a person
//     even when an agent ran the command), and --source-root is given (no
//     cwd-derived fallback, the R-002 precedent from RenderValidateCore);
//   - <source-root>/<id>/SKILL.md exists and passes the same hard lint
//     `skills add` enforces (warnings never block);
//   - an existing record file is readable (an unreadable one is refused rather
//     than overwritten blind).
//
// Re-approving identical bytes is idempotent: a valid record is left exactly as
// it is, so the original approver and time stay the record of who approved
// these bytes. Approving changed bytes, or replacing a stale or malformed
// record, writes a new record atomically.
//
// Output, all on stdout, all paths with forward slashes: `approved: <id>` (or
// `unchanged: <id>` when a valid record already covered these bytes), then
// `sha256: <hex>` and `record: <path>`. Every refusal exits 1 with the reason on
// stderr and nothing on stdout.
//
// The `labdrian` wrapper appends `--registry <path> --manifest <path>
// --source-root <path>` after the verb's own arguments; --registry and
// --manifest are consumed and ignored. now returns the approval time as an
// RFC 3339 UTC timestamp (YYYY-MM-DDTHH:MM:SSZ); it is injected because this
// package's import allowlist (zero_fetch_test.go) does not admit "time". A nil
// now refuses the approval: the verb never invents a timestamp.
func RenderApproveCore(args []string, readFile readFileFn, now func() string, stdout, stderr io.Writer, exit func(int)) {
	const verb = "skills approve"
	var id, approver, sourceRoot string
	haveApprover := false
	endOfOptions := false
	i := 0

	fail := func(format string, a ...any) {
		fmt.Fprintf(stderr, "error: "+verb+": "+format+"\n", a...)
		exit(1)
	}
	consumeValue := func(flag string) (string, bool) {
		if i+1 >= len(args) {
			fail("flag %q requires a value", flag)
			return "", false
		}
		value := args[i+1]
		if strings.HasPrefix(value, "-") {
			fail("flag %q requires a value; got flag token %q", flag, value)
			return "", false
		}
		i++
		return value, true
	}

	for ; i < len(args); i++ {
		arg := args[i]
		if !endOfOptions {
			switch arg {
			case "--":
				endOfOptions = true
				continue
			case "--id":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				id = value
				continue
			case "--approver":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				approver = value
				haveApprover = true
				continue
			case "--source-root":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				sourceRoot = value
				continue
			case "--registry", "--manifest":
				// Wrapper-injected and unused here: consume the value so it is
				// never misread as a positional.
				if _, ok := consumeValue(arg); !ok {
					return
				}
				continue
			}
			if strings.HasPrefix(arg, "-") {
				fail("unknown flag %q", arg)
				return
			}
		}
		fail("unexpected argument %q (approve takes --id, not a positional)", arg)
		return
	}

	if id == "" {
		fail("requires --id <skill-id>")
		return
	}
	if !slugRe.MatchString(id) {
		fail("id %q: invalid slug (must match ^[a-z0-9][a-z0-9-]*$)", id)
		return
	}
	if !haveApprover {
		fail("requires --approver <label> (a name for the record; the engine cannot verify it)")
		return
	}
	approver = strings.TrimSpace(approver)
	if problem := ApproverLabelError(approver); problem != "" {
		fail("approver %s", problem)
		return
	}
	if sourceRoot == "" {
		fail("requires --source-root <skills-dir>")
		return
	}
	if now == nil {
		fail("no clock configured: the approval time cannot be recorded")
		return
	}

	skillPath := filepath.Join(sourceRoot, id, "SKILL.md")
	skillData, err := readFile(skillPath)
	if err != nil {
		fail("skill %q: SKILL.md not found or unreadable at %q: %v", id, skillPath, err)
		return
	}
	if hard, _ := LintSkillFile(skillData); len(hard) > 0 {
		for _, finding := range hard {
			fmt.Fprintln(stderr, finding)
		}
		exit(1)
		return
	}

	digest := SkillDigest(skillData)
	recordPath := ApprovalRecordPath(sourceRoot, id)
	status, err := ReadApprovalStatus(sourceRoot, id, skillData, readFile)
	if err != nil {
		fail("%v", err)
		return
	}

	verdict := "approved"
	if status.State == ApprovalValid {
		verdict = "unchanged"
	} else {
		recordBytes, err := SerializeApprovalRecord(ApprovalRecord{
			Skill:      id,
			SHA256:     digest,
			ApprovedAt: now(),
			Approver:   approver,
		})
		if err != nil {
			fail("%v", err)
			return
		}
		if err := writeApprovalRecord(recordPath, recordBytes); err != nil {
			fail("skill %q: %v", id, err)
			return
		}
	}

	fmt.Fprintf(stdout, "%s: %s\n", verdict, id)
	fmt.Fprintf(stdout, "sha256: %s\n", digest)
	fmt.Fprintf(stdout, "record: %s\n", filepath.ToSlash(recordPath))
	exit(0)
}

// writeApprovalRecord writes data to path atomically: a temp file in the same
// directory, made world-readable (the record is committed with the skill, and
// CreateTemp's 0600 would not survive a checkout), synced, then renamed over
// path. A failure at any step removes the temp file and leaves path as it was.
func writeApprovalRecord(path string, data []byte) error {
	tmp, err := writeFileAtomic(path, data)
	if err != nil {
		return fmt.Errorf("writing approval record: %w", err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("writing approval record: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("writing approval record: finalizing %q: %w", path, err)
	}
	return nil
}
