package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// This file pins the pipeline's PROSE vocabularies against their single
// machine owner, skills/_shared/entry-contract.schema.json.
//
// It exists because the chain-strategy vocabulary drifted across five separate
// surfaces (schema enum, orchestrator workflow, sdd-tasks emitter, sdd-apply
// consumer, chained-pr narrative) and only the schema token was enforced. Prose
// that nothing pins is a suggestion, so every correction landed in those files
// was reversible without a single test turning red. The precedent is
// TestNextRecommendationTokenDomainMatchesInceptionPipelineProse in
// token_domain_test.go: name one owner, then pin every mirror to it.

const (
	sddApplySkillRelPath      = "skills/sdd-apply/SKILL.md"
	sddTasksSkillRelPath      = "skills/sdd-tasks/SKILL.md"
	sddVerifySkillRelPath     = "skills/sdd-verify/SKILL.md"
	sddArchiveSkillRelPath    = "skills/sdd-archive/SKILL.md"
	orchestratorWorkflowPath  = "skills/_shared/sdd-orchestrator-workflow.md"
	timeEstimationSkillPath   = "skills/sdd-time-estimation/SKILL.md"
	chainStrategyField        = "chain_strategy"
	chainStrategyNoneToken    = "none"
	applyStep2aHeading        = "#### Step 2a: Enforce Review Workload Decision"
	chainingSpellingsMarker   = "**Chaining — one concept,"
	sliceRecordingWithinToken = "slices planned=P realized=R"
	sliceRecordingDriftToken  = "slice drift: planned=P realized=R"
)

func readRepoDoc(t *testing.T, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	return string(data)
}

// schemaEnum returns the enum of a top-level schema property, so a pin never
// hardcodes the domain it is supposed to be checking against.
func schemaEnum(t *testing.T, field string) []string {
	t.Helper()
	data, err := os.ReadFile(schemaPath(t))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		// Raw per property: sibling properties carry non-string enums (tier is
		// an integer enum), so decoding the whole map into a string-enum shape
		// fails on documents this pin is not looking at.
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	raw, ok := schema.Properties[field]
	if !ok {
		t.Fatalf("schema has no property %q", field)
	}
	var property struct {
		Enum []string `json:"enum"`
	}
	if err := json.Unmarshal(raw, &property); err != nil {
		t.Fatalf("decode schema property %q: %v", field, err)
	}
	if len(property.Enum) == 0 {
		t.Fatalf("schema property %q declares no enum", field)
	}
	return property.Enum
}

// chainTopologies is the chain_strategy domain minus the sentinel `none`:
// exactly the values that name a branch topology, and exactly the values
// sdd-apply has a branch for.
func chainTopologies(t *testing.T) []string {
	t.Helper()
	var topologies []string
	for _, token := range schemaEnum(t, chainStrategyField) {
		if token != chainStrategyNoneToken {
			topologies = append(topologies, token)
		}
	}
	sort.Strings(topologies)
	if len(topologies) == 0 {
		t.Fatal("chain_strategy enum carries no topology beyond the none sentinel")
	}
	return topologies
}

// markdownSection returns the body of the section introduced by heading, up to
// the next heading of the same or a shallower level.
func markdownSection(t *testing.T, doc, heading string) string {
	t.Helper()
	level := len(heading) - len(strings.TrimLeft(heading, "#"))
	lines := strings.Split(doc, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		t.Fatalf("document has no heading %q", heading)
	}
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		if depth := len(trimmed) - len(strings.TrimLeft(trimmed, "#")); depth <= level {
			return strings.Join(lines[start:i], "\n")
		}
	}
	return strings.Join(lines[start:], "\n")
}

// markdownTableAfter returns the rows of the first markdown table that begins
// at or after the line containing marker. The header and the separator row are
// dropped; each remaining row is returned as its trimmed cells.
func markdownTableAfter(t *testing.T, doc, marker string) [][]string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	start := -1
	for i, line := range lines {
		if strings.Contains(line, marker) {
			start = i
			break
		}
	}
	if start == -1 {
		t.Fatalf("document has no line containing %q", marker)
	}
	var rows [][]string
	inTable := false
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "|") {
			if inTable {
				break
			}
			continue
		}
		inTable = true
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		for j := range cells {
			cells[j] = strings.TrimSpace(cells[j])
		}
		rows = append(rows, cells)
	}
	if len(rows) < 3 {
		t.Fatalf("no markdown table with body rows found after %q", marker)
	}
	// Drop the header row and the |---|---| separator row.
	return rows[2:]
}

// markdownTableWithHeader returns the body rows of the table whose header cells
// equal want exactly. Anchoring on a header CELL SET rather than a substring
// matters: this document carries a second table whose "Reads code, structural"
// cell matches a naive "| Reads" probe, so a substring anchor silently pinned
// the wrong table and reported the right one as empty.
func markdownTableWithHeader(t *testing.T, doc string, want []string) [][]string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		if !equalCells(splitTableRow(trimmed), want) {
			continue
		}
		var rows [][]string
		for j := i + 2; j < len(lines); j++ {
			rowText := strings.TrimSpace(lines[j])
			if !strings.HasPrefix(rowText, "|") {
				break
			}
			rows = append(rows, splitTableRow(rowText))
		}
		if len(rows) == 0 {
			t.Fatalf("table with header %v carries no body rows", want)
		}
		return rows
	}
	t.Fatalf("document has no markdown table with header %v", want)
	return nil
}

func splitTableRow(row string) []string {
	cells := strings.Split(strings.Trim(row, "|"), "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

func equalCells(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// chainStrategyLinePattern matches sdd-tasks' EMIT sites only — the forecast
// table row and the plain-text guard line — never the prose that explains them.
// Scoping matters both ways: a whole-file probe is satisfied by the rationale
// bullet that legitimately names the removed tokens, and a whole-file forbid
// list would forbid documenting why they were removed.
var chainStrategyLinePattern = regexp.MustCompile(`(?m)^(?:\| *Chain strategy *\|.*|Chain strategy:.*)$`)

// guardContractChainLinePattern matches the emitted plain-text guard line, the
// one downstream guards match literally.
var guardContractChainLinePattern = regexp.MustCompile(`(?m)^Chain strategy: (.+)$`)

// deprecatedChainTokens are the two values the chain field must never carry:
// `size-exception` is a delivery fact, not a topology, and `pending` is a null
// state. Neither answers "which branch does this PR target", and neither is
// accepted by the schema, so neither can ever reach an entry contract.
var deprecatedChainTokens = []string{"size-exception", "pending"}

// TestSddTasksChainStrategyEmitDomainMatchesSchema pins the emitter. sdd-tasks
// writes the `Chain strategy:` line every downstream guard matches literally,
// and it is the only producer of that line, so the domain it advertises IS the
// domain the pipeline sees.
func TestSddTasksChainStrategyEmitDomainMatchesSchema(t *testing.T) {
	doc := readRepoDoc(t, sddTasksSkillRelPath)
	// The EMITTER's domain is the whole schema enum, `none` included: the
	// forecast line answers "which branch does each PR target", and `none` is
	// the honest answer when nothing is chained. Only the CONSUMER's domain
	// (sdd-apply, below) is narrower, because `none` names no branch to take.
	stored := append([]string(nil), schemaEnum(t, chainStrategyField)...)
	sort.Strings(stored)

	lines := chainStrategyLinePattern.FindAllString(doc, -1)
	if len(lines) == 0 {
		t.Fatalf("%s no longer mentions the Chain strategy field at all", sddTasksSkillRelPath)
	}
	for _, line := range lines {
		for _, deprecated := range deprecatedChainTokens {
			if strings.Contains(line, deprecated) {
				t.Errorf("%s still emits the deprecated chain token %q on a Chain strategy line:\n  %s",
					sddTasksSkillRelPath, deprecated, strings.TrimSpace(line))
			}
		}
	}

	// The guard-contract literal is the line downstream guards match. Its token
	// SET must equal the schema's topology set; the order is the document's
	// business, so this compares sets rather than bytes.
	guardLines := guardContractChainLinePattern.FindAllStringSubmatch(doc, -1)
	if len(guardLines) == 0 {
		t.Fatalf("%s no longer emits a `Chain strategy: <domain>` guard-contract line", sddTasksSkillRelPath)
	}
	for _, guard := range guardLines {
		got := strings.Split(strings.Trim(strings.TrimSpace(guard[1]), "<>"), "|")
		for i := range got {
			got[i] = strings.TrimSpace(got[i])
		}
		sort.Strings(got)
		if !equalCells(got, stored) {
			t.Errorf("%s guard-contract line advertises the domain %v, but the schema chain_strategy enum is %v",
				sddTasksSkillRelPath, got, stored)
		}
	}
}

// applyChainRoutingTableHeader is the header of sdd-apply's chain-routing
// table: the machine-readable statement of what the consumer does with every
// value in the schema's closed domain.
var applyChainRoutingTableHeader = []string{"`chain_strategy`", "How apply routes it"}

// TestSddApplyChainStrategyConsumerDomainMatchesSchema pins the ONE consumer
// that actually branches on the value. The orchestrator's unknown-value guard
// lives in skills/_shared/sdd-orchestrator-workflow.md and therefore cannot
// fire on a token minted inside the phase agent, so sdd-apply must carry the
// closed domain itself.
//
// It reads a ROUTING TABLE rather than prose, and that is the whole point. The
// prose version of this rule contradicted itself inside one paragraph — first
// "Any other value — including `none` ... do NOT proceed: STOP ... return
// `blocked`", then "A `none` value means chaining is not required at all: route
// by `delivery_strategy` alone" — and an agent obeying the first half blocked
// apply on EVERY single-PR change, which is what `sdd-tasks` emits whenever its
// forecast says `Chained PRs recommended: No`. The earlier assertions could not
// see it: they only checked that the two topologies were named, that the two
// deprecated tokens were absent, and that the word STOP appeared somewhere —
// all three of which the contradictory paragraph satisfied. A table gives every
// value in the domain exactly one row and exactly one route, so "routes" and
// "blocks" can no longer both be true of the same value.
func TestSddApplyChainStrategyConsumerDomainMatchesSchema(t *testing.T) {
	doc := readRepoDoc(t, sddApplySkillRelPath)
	body := markdownSection(t, doc, applyStep2aHeading)

	// 1. The routed domain is exactly the schema's domain — `none` included.
	// A missing row means a legal value with no documented route; an extra row
	// means a route for a value the schema cannot store.
	stored := append([]string(nil), schemaEnum(t, chainStrategyField)...)
	sort.Strings(stored)
	var routed []string
	routes := map[string]string{}
	for _, row := range markdownTableWithHeader(t, body, applyChainRoutingTableHeader) {
		if len(row) < 2 {
			t.Fatalf("chain-routing row %v carries no route", row)
		}
		token := strings.Trim(row[0], "`")
		routed = append(routed, token)
		routes[token] = row[1]
	}
	sort.Strings(routed)
	if !equalCells(routed, stored) {
		t.Errorf("%s %s routes the domain %v, but the schema chain_strategy enum is %v",
			sddApplySkillRelPath, applyStep2aHeading, routed, stored)
	}

	// 2. Every value IN the domain routes. A row that tells the agent to stop
	// or return blocked is the defect this test exists to catch: it turns a
	// legal value into an unknown one.
	for token, route := range routes {
		if strings.Contains(route, "STOP") || strings.Contains(route, "blocked") {
			t.Errorf("%s %s routes the legal %s value %q to a STOP/blocked outcome: %q — the unknown-value guard fires on values OUTSIDE the domain, never on a member of it",
				sddApplySkillRelPath, applyStep2aHeading, chainStrategyField, token, route)
		}
	}

	// 3. The sentinel is routed by delivery_strategy, because it names no
	// branch to target and there is nothing to look up.
	if route, ok := routes[chainStrategyNoneToken]; ok && !strings.Contains(route, "delivery_strategy") {
		t.Errorf("%s %s routes %q as %q without naming `delivery_strategy` — `none` means chaining is not required, so delivery_strategy alone decides",
			sddApplySkillRelPath, applyStep2aHeading, chainStrategyNoneToken, route)
	}

	// 4. The topologies are still named, the deprecated tokens are still gone,
	// and the guard still exists for values outside the domain.
	for _, topology := range chainTopologies(t) {
		if !strings.Contains(body, topology) {
			t.Errorf("%s %s must name the chain topology %q it branches on", sddApplySkillRelPath, applyStep2aHeading, topology)
		}
	}
	for _, deprecated := range deprecatedChainTokens {
		if strings.Contains(body, deprecated) {
			t.Errorf("%s %s still consumes the deprecated chain token %q — sdd-apply has no branch for it",
				sddApplySkillRelPath, applyStep2aHeading, deprecated)
		}
	}
	if !strings.Contains(body, "STOP") {
		t.Errorf("%s %s must STOP on a chain_strategy value outside the schema domain", sddApplySkillRelPath, applyStep2aHeading)
	}
}

var spellingCountWords = map[string]int{
	"two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8,
}

// TestChainingSpellingsTableCountsEverySurface pins the reconciliation table
// against itself: the count word in its own headline must equal the number of
// surfaces the table lists, and sdd-apply's consumer prose must be one of them.
// The table previously claimed "four spellings" while omitting sdd-apply's own
// prose — a fifth spelling, and the only one downstream of the guard.
func TestChainingSpellingsTableCountsEverySurface(t *testing.T) {
	doc := readRepoDoc(t, orchestratorWorkflowPath)
	idx := strings.Index(doc, chainingSpellingsMarker)
	if idx == -1 {
		t.Fatalf("%s no longer carries the %q headline", orchestratorWorkflowPath, chainingSpellingsMarker)
	}
	headline := doc[idx:]
	if end := strings.Index(headline, "\n"); end != -1 {
		headline = headline[:end]
	}
	countWord := ""
	for word := range spellingCountWords {
		if strings.Contains(headline, word+" spellings") {
			countWord = word
			break
		}
	}
	if countWord == "" {
		t.Fatalf("headline %q does not state a spelling count in words", headline)
	}

	rows := markdownTableAfter(t, doc[idx:], "| Surface")
	if got, want := len(rows), spellingCountWords[countWord]; got != want {
		t.Errorf("headline claims %q (%d) but the table lists %d surfaces", countWord+" spellings", want, got)
	}

	var joined []string
	for _, row := range rows {
		joined = append(joined, strings.Join(row, " "))
	}
	if !strings.Contains(strings.Join(joined, "\n"), "sdd-apply") {
		t.Errorf("the chaining-spellings table omits sdd-apply's own prose, the one surface that branches on the value:\n%s",
			strings.Join(joined, "\n"))
	}
}

var backtickPattern = regexp.MustCompile("`([^`]+)`")

const chainStrategySectionHeading = "### Chain Strategy"

// chainStrategyStopPhrases are the ways prose says "this value has nowhere to
// go". Paired with a member of the stored domain in one sentence, each one
// contradicts the routing table that gives every member a route.
var chainStrategyStopPhrases = []string{"STOP", "blocked", "no route", "no branch"}

var (
	// codeSpanPattern matches one inline code span. A span whose content holds
	// whitespace is a quoted literal, not a token: the blocked-message text the
	// agent is told to return quotes tokens without routing them.
	codeSpanPattern = regexp.MustCompile("`[^`\n]*`")
)

// chainStrategyGuardMarker opens the ONE paragraph per section that is allowed
// to pair a member of the stored domain with a stop outcome — the unknown-value
// guard, which must name the topologies precisely because it exists to stop you
// guessing one.
//
// It is exempt by IDENTITY, not by description. Three scopes of description
// were tried and all three certified the contradiction they existed to catch:
//
//  1. Sentence-scoped: defeated by a full stop. The token moved into one
//     sentence and the stop into the next.
//  2. Paragraph-scoped: defeated by a BLANK LINE, which splits the paragraph
//     the same way the full stop split the sentence.
//  3. Paragraph-scoped with a "does it scope itself to values outside the
//     table" exemption regex: defeated by APPENDING. Both live guard
//     paragraphs match that regex — the exemption is what makes the live
//     assertion green — so a contradiction added to the end of the very
//     paragraph the pin exists to protect was invisible.
//
// Each fix widened the window and each was defeated by the next keystroke,
// because substring search cannot decide whether prose contradicts a table. So
// the guard paragraph is no longer DESCRIBED, it is QUOTED: the exempt
// paragraph must equal the pinned text byte for byte, and every other paragraph
// in the section gets the token-plus-stop check with no exemption at all. Split
// it, append to it, or reword it and the pin goes red and names the diff; the
// author must then change the pin deliberately, in the same commit, where a
// reviewer sees it.
//
// What would remove the pin entirely: make the routing table the single source
// and GENERATE the guard prose from it, so no hand-written sentence can
// disagree with the table it is derived from. Until the prose is generated,
// quoting it is the strongest check available, and a quoted paragraph cannot be
// green while a contradiction sits inside it.
const chainStrategyGuardMarker = "**Unknown-value guard (MANDATORY"

// pinnedChainStrategyGuards is the exact current text of each document's guard
// paragraph. Nothing derives it: it is a transcript, and that is the point.
var pinnedChainStrategyGuards = map[string]string{
	sddApplySkillRelPath: "**Unknown-value guard (MANDATORY, and it must fire HERE).** A value that is not a row in the table " +
		"above has no route in this skill. Do NOT pick the nearest topology, and do NOT default to " +
		"`stacked-to-main` because it is the common case. Do NOT proceed either: STOP before writing code and " +
		"return `blocked`, naming the unrecognised value and where it came from (tasks artifact, prompt, or " +
		"entry contract). The orchestrator carries the same guard, but it cannot fire on a value minted " +
		"inside this phase agent, so the check has to exist on both sides of the handoff.",
	orchestratorWorkflowPath: "**Unknown-value guard (MANDATORY).** Any `chain_strategy` value outside `stacked-to-main`, " +
		"`feature-branch-chain`, and `none` is invalid. Do NOT pick the nearest branch, and do NOT default to " +
		"`stacked-to-main` because it is the common case. Do NOT proceed either: STOP, report the " +
		"unrecognised value and where it came from (entry contract, tasks forecast, or session cache), and " +
		"re-collect the chain strategy before launching `sdd-apply`. This mirrors the identical rule for " +
		"`delivery_strategy` in the Review Workload Guard, which this section previously lacked. The exposure " +
		"is real and only latent: `sdd-apply` routes exactly the three stored values and nothing else, so a " +
		"value outside that domain reaches implementation with no route to take.",
}

// markdownParagraphs splits a section into RAW blank-line-separated paragraphs,
// unmasked, so the guard paragraph can be compared to its pin exactly as an
// author reads it.
func markdownParagraphs(body string) []string {
	var paragraphs []string
	for _, paragraph := range strings.Split(body, "\n\n") {
		if trimmed := strings.TrimSpace(paragraph); trimmed != "" {
			paragraphs = append(paragraphs, trimmed)
		}
	}
	return paragraphs
}

// chainStrategyProsePassages returns the PARAGRAPHS of a section that a reader
// takes as instruction: table rows removed, quoted message literals masked,
// blank-line boundaries preserved.
//
// Paragraphs, not sentences. A sentence-scoped read of this rule was defeated
// by a full stop — the contradiction simply moved the token into one sentence
// and the stop into the next — and a rule a keystroke defeats is worse than no
// rule, because it certifies the text it cannot read.
func chainStrategyProsePassages(body string) []string {
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			// Blanked, not dropped: a table between two paragraphs must not
			// splice them into one.
			kept = append(kept, "")
			continue
		}
		kept = append(kept, line)
	}
	prose := codeSpanPattern.ReplaceAllStringFunc(strings.Join(kept, "\n"), func(span string) string {
		if strings.ContainsAny(strings.Trim(span, "`"), " \t") {
			return "`quoted-literal`"
		}
		return span
	})
	var passages []string
	for _, passage := range strings.Split(prose, "\n\n") {
		if strings.TrimSpace(passage) != "" {
			passages = append(passages, passage)
		}
	}
	return passages
}

func namesToken(sentence, token string) bool {
	pattern := regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(token) + `(?:[^A-Za-z0-9_-]|$)`)
	return pattern.MatchString(sentence)
}

// TestChainStrategyProseNeverContradictsTheRoutingTable closes the hole the
// table-only pin left open. TestSddApplyChainStrategyConsumerDomainMatchesSchema
// reads table CELLS, so the original self-contradiction — "Any other value —
// including `none`, which names no branch to target — has no route in this
// skill" — could be restored VERBATIM into the guard paragraph with the table
// still correct and the suite still green. Prose is what the phase agent obeys,
// so the pin has to reach it: no sentence outside the table, in either the
// consumer skill or the orchestrator workflow that feeds it, may pair a member
// of the stored domain with a stop outcome.
func TestChainStrategyProseNeverContradictsTheRoutingTable(t *testing.T) {
	domain := schemaEnum(t, chainStrategyField)
	for _, target := range []struct{ path, heading string }{
		{sddApplySkillRelPath, applyStep2aHeading},
		{orchestratorWorkflowPath, chainStrategySectionHeading},
	} {
		body := markdownSection(t, readRepoDoc(t, target.path), target.heading)
		for _, violation := range chainStrategyProseViolations(body, domain, pinnedChainStrategyGuards[target.path]) {
			t.Errorf("%s %s: %s", target.path, target.heading, violation)
		}
	}
}

// chainStrategyProseViolations is the whole pin: the guard paragraph is checked
// by identity against its pin, and EVERY other paragraph is checked for a
// domain member paired with a stop outcome, with no exemption available to it.
func chainStrategyProseViolations(body string, domain []string, pinnedGuard string) []string {
	var violations []string

	var guards []string
	for _, paragraph := range markdownParagraphs(body) {
		if strings.HasPrefix(paragraph, chainStrategyGuardMarker) {
			guards = append(guards, paragraph)
		}
	}
	switch {
	case len(guards) == 0:
		violations = append(violations, fmt.Sprintf(
			"carries no paragraph opening %q. The unknown-value guard is the one paragraph allowed to name a "+
				"domain member beside a stop outcome, and it is exempt only because it is pinned verbatim; "+
				"remove or rename it and the exemption has nothing to stand on", chainStrategyGuardMarker))
	case len(guards) > 1:
		violations = append(violations, fmt.Sprintf(
			"carries %d paragraphs opening %q, so the guard is no longer one quotable paragraph. Splitting it "+
				"is exactly how a blank line used to hide a contradiction from this pin", len(guards), chainStrategyGuardMarker))
	case guards[0] != strings.TrimSpace(pinnedGuard):
		violations = append(violations, fmt.Sprintf(
			"the unknown-value guard paragraph is not the pinned text. It is the only paragraph here permitted "+
				"to pair a legal %s value with a stop outcome, so it is exempt by transcript, not by "+
				"description: anything added to, removed from, or reworded inside it must be re-read by a "+
				"human and re-pinned in the same commit.\n  found:  %q\n  pinned: %q",
			chainStrategyField, guards[0], strings.TrimSpace(pinnedGuard)))
	}

	for _, contradiction := range contradictingChainStrategyProse(body, domain) {
		violations = append(violations, fmt.Sprintf(
			"pairs the legal %s value %q with %q in prose: %q\nEvery member of the domain routes; only the "+
				"pinned unknown-value guard may name a member beside a stop",
			chainStrategyField, contradiction.token, contradiction.stop, contradiction.text))
	}
	return violations
}

// chainStrategyContradiction is one passage that tells the agent a legal
// chain_strategy value has nowhere to go.
type chainStrategyContradiction struct{ token, stop, text string }

// contradictingChainStrategyProse returns the paragraphs of a section, OTHER
// than the pinned unknown-value guard, that pair a member of the stored domain
// with a stop outcome. There is no self-scoping exemption: a paragraph that
// describes itself as firing on values outside the table used to be excused,
// and that description was trivially attached to a contradiction. Only the
// guard paragraph is skipped, and only because chainStrategyProseViolations
// checks it against a verbatim pin instead.
func contradictingChainStrategyProse(body string, domain []string) []chainStrategyContradiction {
	var found []chainStrategyContradiction
	for _, passage := range chainStrategyProsePassages(body) {
		if strings.HasPrefix(strings.TrimSpace(passage), chainStrategyGuardMarker) {
			continue
		}
		for _, token := range domain {
			if !namesToken(passage, token) {
				continue
			}
			for _, stop := range chainStrategyStopPhrases {
				if strings.Contains(passage, stop) {
					found = append(found, chainStrategyContradiction{token, stop, strings.TrimSpace(passage)})
				}
			}
		}
	}
	return found
}

// --- The pin's own controls ------------------------------------------------
//
// A textual pin is worth exactly what its two error directions are worth, and
// this one had both backwards for three revisions running. The fixtures below
// are the three evasions that were each demonstrated against the live
// documents, plus the correct wording, so the pin is judged against text that
// cannot quietly change under it.

// splitSentenceEvasion moves the token into one sentence and the stop into the
// next. It defeated the sentence-scoped pin.
const splitSentenceEvasion = "**Unknown-value guard (MANDATORY).** The `none` value names no topology to target. " +
	"Such a value has no route in this skill: STOP before writing code and return blocked.\n"

// blankLineEvasion moves the stop into a new PARAGRAPH. It defeated the
// paragraph-scoped pin, because a blank line splits a paragraph exactly as a
// full stop splits a sentence.
const blankLineEvasion = "**Unknown-value guard (MANDATORY).** The `none` value names no topology to target.\n\n" +
	"Such a value has no route in this skill: STOP before writing code and return blocked.\n"

// appendedContradictionEvasion leaves the correct guard wording intact and adds
// the contradiction to the END of that same paragraph. It defeated the
// scope-regex exemption: the paragraph still described itself as firing on
// values outside the table, so the pin skipped the whole thing — including the
// sentence that contradicted the table.
const appendedContradictionEvasion = pinnedApplyGuardFixture + " The `none` value names no topology to target, " +
	"so it too has no route here: STOP before writing code and return `blocked`."

// pinnedApplyGuardFixture is the correct wording the three evasions are
// measured against; the live pin for the same paragraph lives in
// pinnedChainStrategyGuards.
const pinnedApplyGuardFixture = "**Unknown-value guard (MANDATORY).** A value that is not a row in the table above has no route " +
	"in this skill. Do NOT pick the nearest topology, and do NOT default to `stacked-to-main` because it is " +
	"the common case. Do NOT proceed either: STOP before writing code and return `blocked`, naming the " +
	"unrecognised value."

func TestChainStrategyProsePinCatchesEveryDemonstratedEvasion(t *testing.T) {
	domain := schemaEnum(t, chainStrategyField)
	for name, prose := range map[string]string{
		"split_sentence":        splitSentenceEvasion,
		"blank_line":            blankLineEvasion,
		"appended_to_the_guard": appendedContradictionEvasion,
	} {
		t.Run(name, func(t *testing.T) {
			if len(chainStrategyProseViolations(prose, domain, pinnedApplyGuardFixture)) == 0 {
				t.Errorf("the prose pin is green on a guard that contradicts the routing table:\n%s\nA pin that certifies the text it cannot read is worse than no pin", prose)
			}
		})
	}
}

func TestChainStrategyProsePinDoesNotFireOnThePinnedGuard(t *testing.T) {
	domain := schemaEnum(t, chainStrategyField)
	if found := chainStrategyProseViolations(pinnedApplyGuardFixture, domain, pinnedApplyGuardFixture); len(found) != 0 {
		t.Errorf("the prose pin fires on its own pinned guard %+v", found)
	}
}

// TestChainStrategyGuardExemptionIsScopedToTheGuardParagraph proves the
// exemption is narrow: the SAME sentences, moved out of the guard slot into an
// ordinary paragraph, are caught. Nothing about their wording earns silence —
// only being the pinned guard does.
func TestChainStrategyGuardExemptionIsScopedToTheGuardParagraph(t *testing.T) {
	domain := schemaEnum(t, chainStrategyField)
	relocated := strings.Replace(pinnedApplyGuardFixture, "**Unknown-value guard (MANDATORY).**", "**Note.**", 1)
	body := pinnedApplyGuardFixture + "\n\n" + relocated
	if found := chainStrategyProseViolations(body, domain, pinnedApplyGuardFixture); len(found) == 0 {
		t.Errorf("the guard's wording buys silence outside the guard paragraph:\n%s", body)
	}
}

// TestOrchestratorConsumerRowStatesTheWholeApplyDomain pins the one row of the
// chaining-spellings table that describes sdd-apply. It claimed sdd-apply's
// domain was the two topologies, which was the premise of the neighbouring
// instruction to withhold `none` from the apply prompt — on every single-PR
// change, i.e. most of them. The row is a claim about another file, so it is
// pinned to that file's routing table and to the schema both derive from.
func TestOrchestratorConsumerRowStatesTheWholeApplyDomain(t *testing.T) {
	doc := readRepoDoc(t, orchestratorWorkflowPath)
	idx := strings.Index(doc, chainingSpellingsMarker)
	if idx == -1 {
		t.Fatalf("%s no longer carries the %q headline", orchestratorWorkflowPath, chainingSpellingsMarker)
	}
	// The domain cell escapes its alternation bar (`a` \| `b`), and the row
	// splitter is naive about that, so the domain arrives as every cell between
	// the Surface cell and the trailing "How to read it" cell.
	var domainCell string
	for _, row := range markdownTableAfter(t, doc[idx:], "| Surface") {
		if len(row) >= 3 && strings.Contains(row[0], "sdd-apply") {
			domainCell = strings.Join(row[1:len(row)-1], " ")
			break
		}
	}
	if domainCell == "" {
		t.Fatalf("the chaining-spellings table has no sdd-apply consumer row")
	}
	var stated []string
	for _, match := range backtickPattern.FindAllStringSubmatch(domainCell, -1) {
		stated = append(stated, match[1])
	}
	sort.Strings(stated)
	stored := append([]string(nil), schemaEnum(t, chainStrategyField)...)
	sort.Strings(stored)
	if !equalCells(stated, stored) {
		t.Errorf("%s describes sdd-apply's consumer domain as %v, but sdd-apply routes the schema domain %v — a short domain here is the premise for withholding a legal value from the apply prompt",
			orchestratorWorkflowPath, stated, stored)
	}
}

// TestPhaseIOTableReadsAreReferencedByPhaseSkills pins the phase I/O table
// against the skills it describes. A backticked artifact in the Reads column is
// a claim that the phase agent reads it; if that phase's own SKILL.md never
// mentions the artifact, the claim is documentation of behaviour nothing
// implements.
func TestPhaseIOTableReadsAreReferencedByPhaseSkills(t *testing.T) {
	doc := readRepoDoc(t, orchestratorWorkflowPath)
	rows := markdownTableWithHeader(t, doc, []string{"Phase", "Reads", "Writes"})

	phaseSkills := map[string]string{
		"sdd-explore": "skills/sdd-explore/SKILL.md",
		"sdd-propose": "skills/sdd-propose/SKILL.md",
		"sdd-spec":    "skills/sdd-spec/SKILL.md",
		"sdd-design":  "skills/sdd-design/SKILL.md",
		"sdd-tasks":   sddTasksSkillRelPath,
		"sdd-apply":   sddApplySkillRelPath,
		"sdd-verify":  sddVerifySkillRelPath,
		"sdd-archive": sddArchiveSkillRelPath,
	}

	checked := 0
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		phase := strings.Trim(row[0], "`")
		relPath, ok := phaseSkills[phase]
		if !ok {
			continue
		}
		skill := readRepoDoc(t, relPath)
		for _, match := range backtickPattern.FindAllStringSubmatch(row[1], -1) {
			artifact := match[1]
			checked++
			// The topic-key tail, not the bare word: every phase skill names the
			// artifacts it reads as `sdd/{change-name}/<artifact>`, so requiring
			// the "/<artifact>" form is what separates a read instruction from an
			// incidental mention (sdd-tasks names "entry contract" once, in a
			// deprecated-token footnote that instructs no read at all).
			if !strings.Contains(skill, "/"+artifact) {
				t.Errorf("%s declares that %s reads %q, but %s carries no `sdd/{change-name}/%s` read instruction",
					orchestratorWorkflowPath, phase, artifact, relPath, artifact)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("%s phase I/O table declares no backticked Reads artifact — the pin is vacuous", orchestratorWorkflowPath)
	}
}

// TestApplyProgressSliceRecordingRuleHasAnOwner pins the Plan-vs-Realized slice
// rule to the only agent that can honour it. The orchestrator table mandates
// recording the figures IN apply-progress, and apply-progress is written by
// sdd-apply alone, so a rule stated only in the orchestrator doc is a rule with
// no owner.
func TestApplyProgressSliceRecordingRuleHasAnOwner(t *testing.T) {
	workflow := readRepoDoc(t, orchestratorWorkflowPath)
	apply := readRepoDoc(t, sddApplySkillRelPath)

	for _, token := range []string{sliceRecordingWithinToken, sliceRecordingDriftToken} {
		if !strings.Contains(workflow, token) {
			t.Fatalf("%s no longer states the apply-progress slice rule %q — this pin is checking a rule that moved", orchestratorWorkflowPath, token)
		}
		if !strings.Contains(apply, token) {
			t.Errorf("%s mandates recording %q in apply-progress, but %s (its only writer) was never told the rule exists",
				orchestratorWorkflowPath, token, sddApplySkillRelPath)
		}
	}
}

var reviewSliceExamplePattern = regexp.MustCompile("`([a-z0-9-]+)` planned ([0-9]+) slices and delivered ([0-9]+)")

// countEntryContractReviewSlices counts the review_slices entries of a change's
// committed entry contract, searching both the live and the archived change
// directories.
func countEntryContractReviewSlices(t *testing.T, change string) (int, string) {
	t.Helper()
	root := repoRoot(t)
	var found string
	err := filepath.Walk(filepath.Join(root, "openspec", "changes"), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || found != "" {
			return nil
		}
		if filepath.Base(path) != "entry.json" {
			return nil
		}
		dir := filepath.Base(filepath.Dir(path))
		if dir == change || strings.HasSuffix(dir, "-"+change) {
			found = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking openspec/changes: %v", err)
	}
	if found == "" {
		t.Fatalf("no committed entry.json found for change %q — the worked example cites a record that does not exist", change)
	}
	data, err := os.ReadFile(found)
	if err != nil {
		t.Fatalf("read %s: %v", found, err)
	}
	var contract struct {
		ReviewSlices []json.RawMessage `json:"review_slices"`
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatalf("decode %s: %v", found, err)
	}
	rel, relErr := filepath.Rel(root, found)
	if relErr != nil {
		rel = found
	}
	return len(contract.ReviewSlices), filepath.ToSlash(rel)
}

// TestReviewSlicesWorkedExampleReproduces pins sdd-time-estimation's review-slice
// worked example against the record it cites. The plan side is
// `len(review_slices)` in that change's entry contract, and the skill's own
// SINGLE-SAMPLE HONESTY rule demands a figure be reproducible from a cited
// record — so an example whose plan number disagrees with the contract shows no
// variance at all, it just misreads one.
func TestReviewSlicesWorkedExampleReproduces(t *testing.T) {
	doc := readRepoDoc(t, timeEstimationSkillPath)
	matches := reviewSliceExamplePattern.FindAllStringSubmatch(doc, -1)
	if len(matches) == 0 {
		t.Fatalf("%s carries no `<change>` planned N slices and delivered M worked example", timeEstimationSkillPath)
	}
	for _, match := range matches {
		change, plannedText, deliveredText := match[1], match[2], match[3]
		planned := mustAtoi(t, plannedText)
		delivered := mustAtoi(t, deliveredText)
		actual, source := countEntryContractReviewSlices(t, change)
		if planned != actual {
			t.Errorf("worked example claims %s planned %d slices, but %s carries %d review_slices",
				change, planned, source, actual)
		}
		if delivered == planned {
			t.Errorf("worked example for %s shows no variance (planned %d, delivered %d) — it cannot demonstrate the failure it is cited for",
				change, planned, delivered)
		}
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		t.Fatalf("parse %q as an integer: %v", s, err)
	}
	return n
}
