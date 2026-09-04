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
