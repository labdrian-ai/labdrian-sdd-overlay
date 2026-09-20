package skills

import (
	"regexp"
	"strings"
	"testing"
)

// fenceOpenRE matches a CommonMark fence line: leading indent up to 3 spaces,
// three or more backticks or tildes, then an optional info string.
var fenceOpenRE = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})[^`~]*$")

// scanFences walks the document line by line applying CommonMark fenced-code
// rules: a fence closes on the first later line consisting only of the same
// fence character, at least as long as the opening fence, with no info
// string. It returns an error naming the first fence that is still open at
// end of document (unterminated) or nil when every opened fence closed.
func scanFences(doc string) error {
	lines := strings.Split(doc, "\n")
	var openChar byte
	var openLen int
	var openLineNum int
	for i, line := range lines {
		if openChar == 0 {
			m := fenceOpenRE.FindStringSubmatch(line)
			if m != nil {
				openChar = m[1][0]
				openLen = len(m[1])
				openLineNum = i + 1
			}
			continue
		}
		// Inside a fence: check whether this line is a valid closing fence.
		trimmed := strings.TrimLeft(line, " ")
		if len(trimmed) >= openLen {
			allSame := true
			for j := 0; j < len(trimmed); j++ {
				if trimmed[j] != openChar {
					allSame = false
					break
				}
			}
			if allSame {
				openChar = 0
				openLen = 0
			}
		}
	}
	if openChar != 0 {
		return errFenceUnterminated(openLineNum)
	}
	return nil
}

type fenceUnterminatedError struct{ line int }

func (e fenceUnterminatedError) Error() string {
	return "fence opened but never closed, starting at line"
}

func errFenceUnterminated(line int) error {
	return fenceUnterminatedError{line: line}
}

func TestProceduralCandidateContractArtifact(t *testing.T) {
	repoRoot := ooQualityContractRepoRoot(t)
	contract := readRepoFile(t, repoRoot, "skills/_shared/procedural-candidate-detection.md")

	t.Run("R001_topic_key_shapes_present_verbatim", func(t *testing.T) {
		for _, required := range []string{
			"procedural/candidates/repeated-success/{approach-slug}",
			"procedural/candidates/failure-recovery/{failure-slug}/{recovery-slug}",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must contain topic-key shape %q verbatim", required)
			}
		}
	})

	t.Run("R001_record_field_block_present", func(t *testing.T) {
		for _, field := range []string{
			"Kind",
			"Candidate",
			"Aliases",
			"Status",
			"RejectionReason",
			"MatchedSkillPath",
			"Threshold",
			"OccurrenceCount",
			"FirstObserved",
			"LastObserved",
			"Occurrences",
			"Summary",
		} {
			marker := "**" + field + "**"
			if !strings.Contains(contract, marker) {
				t.Fatalf("contract must contain record field %q verbatim", marker)
			}
		}
	})

	t.Run("R002_R003_emission_decision_table_present", func(t *testing.T) {
		for _, row := range []string{
			"| — (no record) | — | 1 | not called | `observing` | No |",
			"| `observing` | yes | unchanged | not called | `observing` | No |",
			"| `observing` | no | < N | not called | `observing` | No |",
			"| `observing` | no | = N | no match | `emitted` | **Yes, once** |",
			"| `observing` | no | = N | match | `rejected` | No |",
			"| `emitted` | no | > N | not called | `emitted` | No |",
			"| `rejected` | no | > N | not called | `rejected` | No |",
		} {
			if !strings.Contains(contract, row) {
				t.Fatalf("contract must contain emission decision table row %q verbatim", row)
			}
		}
	})

	t.Run("R002_R003_threshold_default_present", func(t *testing.T) {
		if !strings.Contains(contract, "Threshold: 3") {
			t.Fatalf("contract must contain %q verbatim", "Threshold: 3")
		}
	})

	t.Run("R004_rejection_record_fields_present", func(t *testing.T) {
		for _, required := range []string{
			"**RejectionReason**",
			"**MatchedSkillPath**",
			"present only when Status=rejected",
			"present only when RejectionReason=duplicate",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must contain rejection field marker %q verbatim", required)
			}
		}
	})

	t.Run("R004_match_candidate_call_site_present", func(t *testing.T) {
		if !strings.Contains(contract, "MatchCandidate(registry, slug)") {
			t.Fatalf("contract must contain the MatchCandidate call-site description %q verbatim", "MatchCandidate(registry, slug)")
		}
	})

	t.Run("R004_untruncated_comparison_rule_present", func(t *testing.T) {
		required := "Comparison always uses the untruncated normalized form, never the truncated form `NormalizeSlug` returns."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the untruncated-comparison rule verbatim: %q", required)
		}
	})

	t.Run("R004_sweep_memo_key_untruncated_present", func(t *testing.T) {
		required := "Any per-sweep reuse or memoization of a `MatchCandidate` answer MUST be keyed by the untruncated normalized form, never by the truncated `NormalizeSlug` topic-key slug, because two long candidates sharing a truncated prefix would otherwise share one cached answer and reintroduce the false duplicate this section exists to prevent."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the sweep-memoization key rule verbatim: %q", required)
		}
	})

	t.Run("Section7_DoNotCaptureCategoriesPresentVerbatim", func(t *testing.T) {
		for _, required := range []string{
			"**Environment-dependent failure**",
			"**Negative claim about a tool with no independent verification**",
			"**Transient error**",
			"**One-off narrative**",
			"**Unresolved failure presented as workflow**",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must contain do-not-capture category %q verbatim", required)
			}
		}
	})

	t.Run("Section7_DoNotCaptureRejectionReasonFormatPresent", func(t *testing.T) {
		required := "RejectionReason: do-not-capture:<class>"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the do-not-capture rejection reason format verbatim: %q", required)
		}
	})

	t.Run("Section7_DoNotCaptureSingleListEnforcedTwice", func(t *testing.T) {
		required := "This is the single do-not-capture list for this contract. It is defined exactly once, here, and enforced at two independent points in the lifecycle: at emission (this document's own section 4, before a candidate that reaches `Threshold` is allowed to advance past `emitted`) and at draft time (`procedural-skill-drafting`, before any draft record is saved). Neither enforcement point supersedes the other: a candidate that slips past emission-time enforcement is still refused at draft time."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the single-list-enforced-twice rule verbatim: %q", required)
		}
	})

	t.Run("Section8_LessonShapeRulePresentVerbatim", func(t *testing.T) {
		required := "the summary MUST be phrased as an imperative rule followed by exactly one clause explaining why, and MUST NOT include observation ids, dates, or PR/issue numbers."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the lesson-shape rule verbatim: %q", required)
		}
	})

	t.Run("Section8_DispositionRulePresentVerbatim", func(t *testing.T) {
		required := "Every candidate MUST have a `Disposition` of exactly `new` or `extend:<skill-id>`, computed before drafting."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the Disposition rule verbatim: %q", required)
		}
	})

	t.Run("Section8_DraftTopicKeyShapePresent", func(t *testing.T) {
		required := "procedural/drafts/{kind}/{slug}"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must contain the draft topic-key shape %q verbatim", required)
		}
	})

	t.Run("Section8_DraftNoFileUnderSkillsPresent", func(t *testing.T) {
		required := "No draft file is ever written under `skills/` at any point in the drafting lifecycle"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the no-draft-file-under-skills rule verbatim: %q", required)
		}
	})

	t.Run("Section8_NestedFencesCloseInOrder", func(t *testing.T) {
		if err := scanFences(contract); err != nil {
			t.Fatalf("contract has an unterminated or early-closing fenced block: %v", err)
		}
	})

	t.Run("Section8_DraftRecordFieldBlockPresent", func(t *testing.T) {
		const sec8Heading = "## 8. Lesson shape, Disposition, and draft records"
		const sec9Heading = "## 9. Extended Status vocabulary, transition table, and History"
		start := strings.Index(contract, sec8Heading)
		if start < 0 {
			t.Fatalf("contract must contain section 8 heading %q", sec8Heading)
		}
		end := strings.Index(contract, sec9Heading)
		if end < 0 || end <= start {
			t.Fatalf("contract must contain section 9 heading %q after section 8", sec9Heading)
		}
		section8 := contract[start:end]

		const anchor = "The draft record's field block:"
		anchorIdx := strings.Index(section8, anchor)
		if anchorIdx < 0 {
			t.Fatalf("section 8 must introduce the draft record's field block with %q", anchor)
		}
		fieldBlock := section8[anchorIdx:]

		pos := 0
		for _, field := range []string{
			"**Candidate**",
			"**Disposition**",
			"**Status**",
			"**ForRevision**",
			"**Lint**",
			"**History**",
			"**Body**",
		} {
			idx := strings.Index(fieldBlock[pos:], field)
			if idx < 0 {
				t.Fatalf("section 8's draft field block must contain field %q, in order, after position %d", field, pos)
			}
			pos += idx + len(field)
		}
	})

	t.Run("Section9_StatusVocabularyPresentVerbatim", func(t *testing.T) {
		required := "observing | emitted | rejected | drafted | registered | promoted | retired"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must contain the extended Status vocabulary %q verbatim", required)
		}
	})

	t.Run("Section9_StatusVocabularyScopedToCandidateRecord", func(t *testing.T) {
		required := "The candidate record's `Status` field takes exactly one value from:"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must scope the extended Status vocabulary to the candidate record verbatim: %q", required)
		}
		normalized := strings.Join(strings.Fields(contract), " ")
		draftVocab := "The draft record (section 8) has its own, separate `Status` field with its own vocabulary, `open | registered | abandoned`"
		if !strings.Contains(normalized, draftVocab) {
			t.Fatalf("contract must state the draft record's own separate Status vocabulary verbatim: %q", draftVocab)
		}
	})

	t.Run("Section9_TransitionTableRowsPresentVerbatim", func(t *testing.T) {
		for _, row := range []string{
			"| none | `observing` | agent | none | first occurrence (item 30) | `engram:<id>` |",
			"| `observing` | `emitted` | agent | none | count reaches `Threshold`, no `MatchCandidate` match (item 30) | count |",
			"| `observing` | `rejected` | agent | none | `MatchCandidate` match (item 30) | `MatchedSkillPath` |",
			"| `observing` or `emitted` | `rejected` | agent | none | do-not-capture class matched at emission or at draft time | `RejectionReason: do-not-capture:<class>` |",
			"| `emitted` | `drafted` | agent | project | draft record written, `LintSkill` hard = 0 (new) or diff applies cleanly (extend) | draft topic key, `Disposition`, lint counts |",
			"| `drafted` | `registered` | agent | project | `project-register` and commit (new), or `project-revise` of an agent-owned target (extend) | skill id, `sha256`, commit, rev |",
			"| `registered` | `registered` | agent | project | revision (ownership OK, trigger reached) | `sha256`, commit, rev |",
			"| `drafted` or `registered` | `promoted` | human | global | `engine skills add` merged in the overlay | overlay commit, `sha256` |",
			"| `registered` | `retired` | agent | project | `project-retire` and commit | `RetirementReason`, commit |",
			"| `promoted` | `retired` | human | global | `engine skills remove` merged | `RetirementReason`, overlay commit |",
		} {
			if !strings.Contains(contract, row) {
				t.Fatalf("contract must contain transition table row %q verbatim", row)
			}
		}
	})

	t.Run("Section9_TransitionTableRowCountPinned", func(t *testing.T) {
		const header = "| From | To | Actor | Tier | Event | Required `History` evidence |"
		idx := strings.Index(contract, header)
		if idx < 0 {
			t.Fatalf("contract must contain transition table header %q", header)
		}
		lines := strings.Split(contract[idx:], "\n")
		// lines[0] is the header row, lines[1] is the separator row.
		rowCount := 0
		for _, line := range lines[2:] {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "|") {
				break
			}
			rowCount++
		}
		const expectedRows = 10 // task 2.1: "the full transition table (10 rows)"
		if rowCount != expectedRows {
			t.Fatalf("transition table must have exactly %d data rows per task 2.1, found %d", expectedRows, rowCount)
		}
	})

	t.Run("Section9_PromotedRetirePreservesStatusPresent", func(t *testing.T) {
		required := "it is NOT the `registered -> retired` transition in the table above — `Status` stays `promoted`, and only a `promoted -> promoted` `History` line records the removal."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the promoted-retire status-preservation rule verbatim: %q", required)
		}
	})

	t.Run("Section9_HistoryFormatLinesPresentVerbatim", func(t *testing.T) {
		for _, line := range []string{
			"- 2026-09-20T10:00:00Z | observing -> emitted | agent | count 3/3, no registry match",
			"- 2026-09-20T10:05:00Z | emitted -> drafted | agent | draft procedural/drafts/repeated-success/probe-engram-with-home-override; Disposition new; lint hard 0 warnings 1 (body-recommended)",
			"- 2026-09-20T10:09:00Z | drafted -> registered | agent | skill probe-engram-with-home-override rev 1 sha256:3f2a...c1 commit a1b2c3d",
		} {
			if !strings.Contains(contract, line) {
				t.Fatalf("contract must contain History example line %q verbatim", line)
			}
		}
	})

	t.Run("Section9_HistoryWriteRulePrefixInvariantPresent", func(t *testing.T) {
		required := "After the write, the old lines MUST be a prefix of the new ones"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the History prefix-invariant write rule verbatim: %q", required)
		}
	})

	t.Run("Section9_NewCandidateRecordFieldsPresentVerbatim", func(t *testing.T) {
		for _, field := range []string{
			"`**Disposition**: new | extend:<skill-id>`",
			"`**Draft**: procedural/drafts/...`",
			"`**Registered**: <id> rev:<n> sha256:<hex> commit:<sha> at:<RFC3339>`",
			"`**Promoted**: <id> path:skills/<id>/SKILL.md sha256:<hex> commit:<sha> at:<RFC3339>`",
			"`**OccurrencesSincePromotion**: <n>`",
			"`**RetirementReason**: stale-reference | superseded | absorbed | promoted | quiet | human-request`",
			"`**AbsorbedInto**: <skill-id>`",
		} {
			if !strings.Contains(contract, field) {
				t.Fatalf("contract must contain new candidate-record field %q verbatim", field)
			}
		}
	})

	t.Run("Section10_RuntimeTargetsTableRowsPresent", func(t *testing.T) {
		for _, required := range []string{
			"| `.claude/skills/` | Claude Code | verified |",
			"| `.agents/skills/` | Pi (always, after project trust); Codex (discovery only, on this host) | verified |",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must contain runtime targets table row starting %q verbatim", required)
			}
		}
	})

	t.Run("Section10_CodexStatusCellDiscoveryScopedVerbatim", func(t *testing.T) {
		required := "recorded verdict `PASS`, scoped to discovery of `.agents/skills/<id>/` (name and description exposed in Codex's skill list); loading the skill body is unverified on this host"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the scoped Codex verdict verbatim: %q", required)
		}
	})

	t.Run("Section10_PiTrustNotePresent", func(t *testing.T) {
		required := "**Pi trust note**:"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must contain the Pi trust note marker %q verbatim", required)
		}
	})

	t.Run("Section10_NeverWritesPiSkillsDirectory", func(t *testing.T) {
		const header = "| Directory | Runtime(s) | Status | Evidence |"
		idx := strings.Index(contract, header)
		if idx < 0 {
			t.Fatalf("contract must contain runtime targets table header %q", header)
		}
		lines := strings.Split(contract[idx:], "\n")
		// lines[0] is the header row, lines[1] is the separator row.
		for _, line := range lines[2:] {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "|") {
				break
			}
			if strings.Contains(trimmed, ".pi/skills") {
				t.Fatalf("runtime targets table must never name .pi/skills/ in any cell of any row, found: %q", trimmed)
			}
		}
		required := "`.pi/skills/` is never written by this capability."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the never-write-.pi/skills rule verbatim: %q", required)
		}
	})

	// --- section 11: registration and commit procedure --------------------

	// section11 is the slice between section 11's heading and the heading
	// that follows it, so every assertion below proves its subject appears
	// INSIDE section 11 rather than anywhere in the document. A whole-file
	// Contains would still pass with section 11 deleted, because most of
	// these strings also occur in design.md's own prose — not here, but the
	// habit is what makes the check vacuous.
	section11 := func(t *testing.T) string {
		t.Helper()
		const head = "## 11. Registration and commit procedure"
		const tail = "## Acceptance checklist (procedural-skill-registration"
		start := strings.Index(contract, head)
		if start < 0 {
			t.Fatalf("contract must contain section 11 heading %q", head)
		}
		end := strings.Index(contract, tail)
		if end <= start {
			t.Fatalf("section 11 must be followed by its acceptance checklist %q (start=%d, end=%d)", tail, start, end)
		}
		return contract[start:end]
	}

	// orderedMarkers walks markers with a moving cursor, so they are proven
	// to appear IN ORDER inside the slice, not merely to be present.
	orderedMarkers := func(t *testing.T, slice, what string, markers []string) {
		t.Helper()
		pos := 0
		for _, marker := range markers {
			idx := strings.Index(slice[pos:], marker)
			if idx < 0 {
				t.Fatalf("%s must contain %q, in order, after position %d", what, marker, pos)
			}
			pos += idx + len(marker)
		}
	}

	t.Run("Section11_TenGitStepsPresentInOrder", func(t *testing.T) {
		orderedMarkers(t, section11(t), "section 11", []string{
			"`git -C R rev-parse --show-toplevel`",
			"`git -C R symbolic-ref -q HEAD`",
			"`git -C R diff --cached --name-only`",
			"`labdrian skills project-register --dry-run",
			"`git -C R check-ignore -- <plan paths>`",
			"`wrote: <rel>`",
			"`git -C R add -- <wrote paths>`",
			"`git -C R commit -m \"<conventional message>\" -- <wrote paths>`",
			"`labdrian skills project-status --project-root R <id>`",
			"`git -C R rev-parse --short HEAD`",
		})
	})

	t.Run("Section11_StepCountPinnedAtTen", func(t *testing.T) {
		slice := section11(t)
		steps := 0
		for _, line := range strings.Split(slice, "\n") {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) < 3 {
				continue
			}
			// An ordered-list item: one or two digits, then ". ".
			digits := 0
			for digits < len(trimmed) && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
				digits++
			}
			if digits == 0 || digits > 2 {
				continue
			}
			if strings.HasPrefix(trimmed[digits:], ". ") {
				steps++
			}
		}
		const expectedSteps = 10 // design.md decision (c): the ten ordered agent git steps
		if steps != expectedSteps {
			t.Fatalf("section 11 must carry exactly %d numbered steps per design decision (c), found %d", expectedSteps, steps)
		}
	})

	t.Run("Section11_CommitMessagesPresentVerbatimInOrder", func(t *testing.T) {
		slice := section11(t)
		const stepEight = "`git -C R commit -m \"<conventional message>\" -- <wrote paths>`"
		const stepNine = "`labdrian skills project-status --project-root R <id>`"
		start := strings.Index(slice, stepEight)
		if start < 0 {
			t.Fatalf("section 11 must contain the commit step %q", stepEight)
		}
		end := strings.Index(slice, stepNine)
		if end <= start {
			t.Fatalf("the commit step must precede the ownership-confirmation step")
		}
		orderedMarkers(t, slice[start:end], "section 11's commit step", []string{
			"`feat(skills): register project skill <id>`",
			"`feat(skills): revise project skill <id>`",
			"`chore(skills): retire project skill <id>`",
			"No AI attribution, no `-a`, no `--no-verify`, no amend, no push.",
		})
	})

	t.Run("Section11_CrashRecoveryRulePresentVerbatim", func(t *testing.T) {
		slice := section11(t)
		for _, required := range []string{
			"The newly written SKILL.md files are always untracked (each lives under a brand-new skill directory), and are deleted only when their bytes hash to the sha256 the failed run printed.",
			"when it existed in `HEAD` before this run, restore it byte-for-byte with `git -C R restore --source=HEAD -- <lock path>`; when this run created it for the first time in the repository (so it is untracked), delete it instead, after the same hash check.",
			"A revision or retirement runs `git -C R restore --source=HEAD --staged --worktree -- <wrote paths>`. `git clean` and `git reset --hard` are never used.",
		} {
			if !strings.Contains(slice, required) {
				t.Fatalf("section 11 must state the crash-recovery rule verbatim: %q", required)
			}
		}
	})

	t.Run("Section11_StagedSetMismatchUnstagesAndRefuses", func(t *testing.T) {
		required := "On a mismatch, run `git -C R restore --staged -- <wrote paths>` and refuse."
		if !strings.Contains(section11(t), required) {
			t.Fatalf("section 11 must state the staged-set mismatch rule verbatim: %q", required)
		}
	})

	t.Run("Section11_RefusalTableRowsPresentVerbatim", func(t *testing.T) {
		slice := section11(t)
		for _, row := range []string{
			"| The command runs from a subdirectory, a nested repository or a submodule | `git -C R rev-parse --show-toplevel` does not equal `R` | Refuse before step 4; nothing is planned, read or written |",
			"| `HEAD` is detached | `git -C R symbolic-ref -q HEAD` fails | Refuse before step 4 |",
			"| A merge, cherry-pick, revert or rebase is in progress | `MERGE_HEAD`, `CHERRY_PICK_HEAD`, `REVERT_HEAD`, `rebase-merge` or `rebase-apply` exists under `git -C R rev-parse --git-dir` | Refuse before step 4 |",
			"| Unrelated changes are already staged | `git -C R diff --cached --name-only` is non-empty | Refuse, naming every staged path, and leave the index untouched |",
			"| A planned target is gitignored | `git -C R check-ignore -- <plan paths>` prints a path | Refuse; `git add -f` is forbidden |",
			"| The staged set does not equal the wrote set | `git -C R diff --cached --name-only` after `git -C R add` differs | Unstage with `git -C R restore --staged -- <wrote paths>` and refuse |",
		} {
			if !strings.Contains(slice, row) {
				t.Fatalf("section 11 must contain refusal table row %q verbatim", row)
			}
		}
	})

	t.Run("Section11_RefusalTableRowCountPinned", func(t *testing.T) {
		slice := section11(t)
		const header = "| Refusal condition | Detected by | What the agent does |"
		idx := strings.Index(slice, header)
		if idx < 0 {
			t.Fatalf("section 11 must contain refusal table header %q", header)
		}
		lines := strings.Split(slice[idx:], "\n")
		// lines[0] is the header row, lines[1] is the separator row.
		rowCount := 0
		for _, line := range lines[2:] {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "|") {
				break
			}
			rowCount++
		}
		const expectedRows = 6
		if rowCount != expectedRows {
			t.Fatalf("section 11's refusal table must have exactly %d data rows, found %d", expectedRows, rowCount)
		}
	})

	t.Run("Section11_TrustNotePrintedOnSuccessOnlyAndNeverAPathspec", func(t *testing.T) {
		slice := section11(t)
		for _, required := range []string{
			"note: Pi loads .agents/skills only after the project is trusted; Pi may prompt once for project trust.",
			"The agent relays every `note:` line to the user and never uses one as a pathspec.",
		} {
			if !strings.Contains(slice, required) {
				t.Fatalf("section 11 must state the Pi trust note rule verbatim: %q", required)
			}
		}
	})

	t.Run("Section11_EngineNeverRunsGit", func(t *testing.T) {
		required := "The engine prints the exact path set and never runs git; the agent runs every git command as `git -C <project-root>`."
		if !strings.Contains(section11(t), required) {
			t.Fatalf("section 11 must state the trust boundary verbatim: %q", required)
		}
	})

	t.Run("Section11_AcceptanceChecklistPresent", func(t *testing.T) {
		const head = "## Acceptance checklist (procedural-skill-registration, executed during `sdd-verify`)"
		if !strings.Contains(contract, head) {
			t.Fatalf("contract must contain the registration acceptance checklist heading %q", head)
		}
	})
}
