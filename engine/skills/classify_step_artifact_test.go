package skills

// The Classify step of the role chain (S6, T4b of odd/tasks/merge-playbook-into-sdd-overlay.md, L47, L55, L90): the parent
// launches the classifier after `review record` / `verify` and for each result the stop hook names. The step is the file
// skills/role-chain/references/classify.md, which one line of the Binding section of SKILL.md points at (no section of its
// own: the skill keeps its five sections and its recommended body budget, L94). These pins hold what the step says against the code and the texts it runs: the deployed paths of the
// evidence store, the exit of every command, the order of the steps, and the lines of a failed check.

import (
	"regexp"
	"strings"
	"testing"
)

const classifyRel = "skills/role-chain/references/classify.md"

// classifySteps are the numbered steps of classify.md, each with its text up to the next step, by number.
func classifySteps(t *testing.T, text string) map[int]string {
	t.Helper()
	number := regexp.MustCompile(`(?m)^([0-9]+)\. `)
	marks := number.FindAllStringSubmatchIndex(text, -1)
	steps := map[int]string{}
	for i, m := range marks {
		end := len(text)
		if i+1 < len(marks) {
			end = marks[i+1][0]
		}
		n := 0
		for _, digit := range text[m[2]:m[3]] {
			n = n*10 + int(digit-'0')
		}
		steps[n] = text[m[0]:end]
	}
	return steps
}

// inOrder reports the first of wants that is not found, each searched after the one before it.
func inOrder(text string, wants ...string) (missing string, ok bool) {
	at := 0
	for _, want := range wants {
		i := strings.Index(text[at:], want)
		if i < 0 {
			return want, false
		}
		at += i + len(want)
	}
	return "", true
}

// sentenceFrom is the text of step from marker to the next full stop that ends a sentence, or the end of the step.
func sentenceFrom(step, marker string) (string, bool) {
	_, rest, ok := strings.Cut(step, marker)
	if !ok {
		return "", false
	}
	sentence, _, _ := strings.Cut(rest, ". ")
	return sentence, true
}

// R1 (L47, `7704d28b9678-5` F3): the paths the step runs are the ones the manifest deploys the evidence store to for
// Claude Code, and the Claude skills root is the one `bin/lib/targets.sh` names.
func TestTheClassifyStepRunsTheDeployedEvidenceStore(t *testing.T) {
	root := skillsRepoRoot(t)
	const skillsRoot = "$HOME/.claude/skills"
	if targets := readRepoFile(t, root, "bin/lib/targets.sh"); !strings.Contains(targets, `[claude]="`+skillsRoot+`"`) {
		t.Fatalf("bin/lib/targets.sh does not hold [claude]=%q, so the paths of the step name a root no deploy uses", skillsRoot)
	}
	rows := ReadManifestRows([]byte(readRepoFile(t, root, "overlay.manifest")))
	routes, err := ResolveRoutes(rows, false, testOverlay, RouteDestinations{Target: "claude", SkillsRoot: skillsRoot, AgentsRoot: "$HOME/.claude/agents"})
	if err != nil {
		t.Fatalf("ResolveRoutes for claude: %v", err)
	}
	dest := map[string]string{}
	for _, r := range routes {
		dest[r.Path] = r.Dest
	}
	steps := classifySteps(t, readRepoFile(t, root, classifyRel))
	step, ok := steps[2]
	if !ok {
		t.Fatalf("%s has no step 2, the step that names <script> and <rules>", classifyRel)
	}
	for _, name := range []string{"evidence.sh", "evidence-rules.md"} {
		want := dest["_shared/evidence/"+name]
		if want == "" {
			t.Fatalf("no Claude route for _shared/evidence/%s", name)
		}
		if !strings.Contains(step, "`"+want+"`") {
			t.Errorf("step 2 of %s does not name `%s`, where the manifest deploys %s", classifyRel, want, name)
		}
	}
}

// R2 (`7704d28b9678-2` F3): steps 3 and 4 name each exit of the script that stops the step, and a stop is `blocked`.
func TestTheClassifyStepNamesEachExitOfTheScript(t *testing.T) {
	steps := classifySteps(t, readRepoFile(t, skillsRepoRoot(t), classifyRel))
	three, four := steps[3], steps[4]
	if three == "" || four == "" {
		t.Fatalf("%s lacks step 3 or step 4: %d steps", classifyRel, len(steps))
	}
	for _, want := range []string{"Exit 0", "Exit 1", "Exit 2", " 3 (", " 4 (", "stop, blocked", "Any other exit"} {
		if !strings.Contains(three, want) {
			t.Errorf("step 3 (reviewed) does not name %q", want)
		}
	}
	// A script that is not a regular file is not deployed: step 2 stops, blocked, before step 3 runs it (`8ee5ac95b287-5` F1, L98).
	if sentence, ok := sentenceFrom(steps[2], "not a regular file"); !ok || !strings.Contains(sentence, "stop, blocked") {
		t.Errorf("step 2 does not stop, blocked, on a <script> that is not a regular file: %q", sentence)
	}
	for _, exit := range []string{"Exit 2:", "Exit 3:"} {
		sentence, ok := sentenceFrom(four, exit)
		switch {
		case !ok:
			t.Errorf("step 4 (check) does not name %q", exit)
		case !strings.Contains(sentence, "blocked"):
			t.Errorf("step 4 names %q and does not say blocked: %q", exit, sentence)
		}
	}
	if !strings.Contains(four, "Exit 1:") || !strings.Contains(four, "Any other exit: blocked") {
		t.Errorf("step 4 (check) does not name exit 1 and every other exit")
	}
}

// roleChainClassifyLine is the Classify step as SKILL.md holds it (L94): one line of the Binding section, right after the
// severe-finding line and before the next-subtask line, so the skill keeps its five sections and its recommended budget.
const roleChainClassifyLine = "- Classify, Claude Code only: after `review record` and `verify`: `references/classify.md`."

// R3 (L47, L94): the steps run in the order that keeps the page whole: findings, the reviewed question, the check and the
// index, the classifier, a backup before the append, the check again, the page put back, the one relaunch. SKILL.md points
// at the file with one line of Binding that says it is for Claude Code.
func TestTheClassifyStepBacksUpBeforeItAppendsAndPutsThePageBack(t *testing.T) {
	root := skillsRepoRoot(t)
	text := readRepoFile(t, root, classifyRel)
	if missing, ok := inOrder(text, "review findings", "reviewed", "check", "index", "Launch the classifier", "backup", "Append", "check", "copy the backup back", "once more"); !ok {
		t.Errorf("%s does not hold %q after the steps before it, in that order", classifyRel, missing)
	}
	_, _, body := roleChainSkill(t)
	_, under := roleChainSections(body)
	binding := under["Binding"]
	if !strings.Contains(binding, "\n"+roleChainClassifyLine+"\n") {
		t.Errorf("the Binding section of SKILL.md lacks the line, verbatim: %s", roleChainClassifyLine)
	}
	if missing, ok := inOrder(binding, "\n- A severe finding blocks only if", "\n"+roleChainClassifyLine, "\n- Clear review and a next subtask"); !ok {
		t.Errorf("the Binding section does not hold %q after the lines before it, in that order", missing)
	}
}

const failedLines = "FAIL <file>: <problem>"

// R6 (L55, `490a46c0b44b-3` F3): the lines of a failed check are named alike by the script that prints them, the step that
// hands them back and the body of the classifier that reads them.
func TestTheFailedLinesAreNamedAlikeByTheStepTheBodyAndTheScript(t *testing.T) {
	root := skillsRepoRoot(t)
	body := readRepoFile(t, root, "roles/classifier/body.md")
	sentence := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "<failed>") {
			sentence = line
		}
	}
	if !strings.Contains(sentence, "`"+failedLines+"`") {
		t.Errorf("the <failed> sentence of roles/classifier/body.md does not name the lines `%s`", failedLines)
	}
	if step := classifySteps(t, readRepoFile(t, root, classifyRel))[10]; !strings.Contains(step, "`"+failedLines+"`") {
		t.Errorf("step 10 of %s does not name the lines `%s`", classifyRel, failedLines)
	}
	header, _, _ := strings.Cut(readRepoFile(t, root, "skills/_shared/evidence/evidence.sh"), "\n\n")
	if !strings.Contains(header, `"`+failedLines+`"`) {
		t.Errorf("the header of evidence.sh does not print %q", failedLines)
	}
}

// R8 (L55): after a check that exits 1 the step puts the page back, then relaunches the classifier once with the failed lines,
// and a second failure stops, blocked. The other exits of step 9 stop before any relaunch.
func TestTheClassifyStepRelaunchesOnceWithTheFailedLines(t *testing.T) {
	steps := classifySteps(t, readRepoFile(t, skillsRepoRoot(t), classifyRel))
	nine, ten := steps[9], steps[10]
	if nine == "" || ten == "" {
		t.Fatalf("%s lacks step 9 or step 10: %d steps", classifyRel, len(steps))
	}
	if missing, ok := inOrder(nine+ten, "copy the backup back", "<failed>", failedLines, "once more", "When it was the relaunch, stop, blocked"); !ok {
		t.Errorf("steps 9 and 10 do not hold %q after the page is put back, in that order", missing)
	}
	other, ok := sentenceFrom(nine, "Any other exit:")
	if !ok || !strings.Contains(other, "stop, blocked") {
		t.Errorf("step 9 does not stop, blocked, on any other exit: %q", other)
	}
	// Every "Any other exit" sentence of step 9 stops, blocked: one of them is the exit of the `reviewed` run that follows a
	// passing check, which must never end in a report of success (L98 choice 3).
	for rest := nine; ; {
		_, after, found := strings.Cut(rest, "Any other exit")
		if !found {
			break
		}
		sentence, _, _ := strings.Cut(after, ". ")
		if !strings.Contains(sentence, "stop, blocked") {
			t.Errorf("a sentence of step 9 that starts %q does not stop, blocked: %q", "Any other exit", sentence)
		}
		rest = after
	}
	// The relaunch is for a failure the classifier can fix: step 10 opens with its condition, that every FAIL line of the check
	// names the page, and a line of another file (the general rules) stops, blocked, with no relaunch (`8ee5ac95b287-2` F1, L98).
	const pageLabel = "`FAIL evidence.md: `"
	if missing, ok := inOrder(ten, pageLabel, "launch the classifier again"); !ok {
		t.Errorf("step 10 does not hold %q before the relaunch, in that order", missing)
	}
	if sentence, ok := sentenceFrom(ten, "names another file"); !ok || !strings.Contains(sentence, "stop, blocked") {
		t.Errorf("step 10 does not stop, blocked, on a FAIL line of another file: %q", sentence)
	}
	for _, relaunch := range []string{"<failed>", "once more"} {
		if strings.Contains(nine, relaunch) {
			t.Errorf("step 9 names %q: a relaunch belongs to step 10 alone", relaunch)
		}
	}
}

// R13 (`8ee5ac95b287-5` F1, L98 choice 5): a step that runs the script says what any other exit does, or points at the step
// that does, so no step runs it with an exit left open. The class is a step with a run and no ending for every exit.
func TestEveryStepThatRunsTheScriptSaysWhatAnyOtherExitDoes(t *testing.T) {
	steps := classifySteps(t, readRepoFile(t, skillsRepoRoot(t), classifyRel))
	runs := 0
	for n, text := range steps {
		if !strings.Contains(text, `sh "<script>"`) {
			continue
		}
		runs++
		if !strings.Contains(text, "Any other exit") && !strings.Contains(text, "exits as in step ") {
			t.Errorf("step %d of %s runs the script and says neither %q nor %q", n, classifyRel, "Any other exit", "exits as in step ")
		}
	}
	if runs < 4 {
		t.Errorf("%d steps of %s run the script, want at least 4 (reviewed, check, index, count): the scan reads nothing", runs, classifyRel)
	}
}

// classifiedWithoutEvidenceHeading, classifiedWithoutEvidenceHeader and classifiedWithoutEvidenceRow are what step 8 writes
// for a review the classifier found no evidence in (L98 choice 3); shelltest's R4 holds the same strings against evidence.sh.
const (
	classifiedWithoutEvidenceHeading = "## Classified without evidence"
	classifiedWithoutEvidenceHeader  = "| Review | Date |"
	classifiedWithoutEvidenceRow     = "| <id> | <date> |"
)

// R11 (`665b99f747e2-3.polisher` W2, L98 choice 3): a run of the step ends with the result classified or blocked, never a
// report of success with no row. An empty Evidence fence is no error (step 7); step 8 appends a row of the table of reviews
// classified without evidence; and after a passing check step 9 asks `reviewed` and goes back to the backup unless it exits 0.
func TestTheClassifyStepEndsClassifiedOrBlocked(t *testing.T) {
	steps := classifySteps(t, readRepoFile(t, skillsRepoRoot(t), classifyRel))
	if !strings.Contains(steps[7], "empty Evidence fence") {
		t.Errorf("step 7 does not say what an empty Evidence fence is: %q", steps[7])
	}
	for _, want := range []string{
		"`" + classifiedWithoutEvidenceHeading + "`", "`" + classifiedWithoutEvidenceHeader + "`", "`|--------|------|`",
		"`" + classifiedWithoutEvidenceRow + "`", "`date +%F`",
	} {
		if !strings.Contains(steps[8], want) {
			t.Errorf("step 8 does not hold %s", want)
		}
	}
	if missing, ok := inOrder(steps[9], "Exit 0", "reviewed", "count", "copy the backup back"); !ok {
		t.Errorf("step 9 does not hold %q after the words before it, in that order", missing)
	}
	if sentence, ok := sentenceFrom(steps[9], "Any other exit of that run:"); !ok || !strings.Contains(sentence, "no row carries `<id>`") {
		t.Errorf("step 9 does not say that no row carries <id> when `reviewed` does not exit 0: %q", sentence)
	}
}

// R14 (L102, L104): step 1 stops the run while a severe finding waits for the check it needs, and names what ends the wait:
// `reproduced` or `not-reproduced` stored by `review verify`. A stored `not-run` keeps it waiting. The `unverified` member and
// the `stop, not blocked` stay pinned by the cmd test only.
func TestTheClassifyStepWaitsUntilTheCheckOfASevereFindingHasRun(t *testing.T) {
	step := classifySteps(t, readRepoFile(t, skillsRepoRoot(t), classifyRel))[1]
	for _, want := range []string{"until its check has run", "`reproduced`", "`not-reproduced`"} {
		if !strings.Contains(step, want) {
			t.Errorf("step 1 does not hold %s: %q", want, step)
		}
	}
	if sentence, ok := sentenceFrom(step, "A stored `not-run`"); !ok || !strings.Contains(sentence, "keeps it waiting") {
		t.Errorf("step 1 does not say that a stored `not-run` keeps the finding waiting: %q", sentence)
	}
	if strings.Contains(step, "waits for its verdict") {
		t.Errorf("step 1 still says the finding waits for its verdict: %q", step)
	}
}
