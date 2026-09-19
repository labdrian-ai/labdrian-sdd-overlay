package skills

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// utf8BOM is the UTF-8 byte-order mark some editors write at the start of a
// file. A leading BOM must not falsely trip the frontmatter-fence hard rule.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// isFenceLine reports whether line is a `---` frontmatter fence, tolerating
// a trailing `\r` (CRLF line endings) and trailing horizontal whitespace
// (spaces or tabs) after the fence.
func isFenceLine(line string) bool {
	return strings.TrimRight(line, " \t\r") == "---"
}

// Severity classifies a lint rule as blocking (hard) or non-blocking
// (advisory). See the "Skill Lint Specification" (spec.md) and design.md's
// "Rule set and the deterministic body-budget proxy" decision.
type Severity string

const (
	// SeverityHard rules produce hard errors: at least one blocks
	// registration and promotion.
	SeverityHard Severity = "hard"
	// SeverityAdvisory rules produce warnings: they never block.
	SeverityAdvisory Severity = "advisory"
)

// Named constants for every numeric bound in the rule table. RenderLintRules
// renders every numeric bound from these constants; no rule Summary embeds a
// bare numeric literal.
const (
	// DescriptionMaxRunes is the hard upper bound on description length.
	DescriptionMaxRunes = 250
	// DescriptionShouldRunes is the advisory upper bound on description length.
	DescriptionShouldRunes = 160
	// BodyHardTokens is the hard upper bound on estimated body tokens.
	BodyHardTokens = 1000
	// BodyRecommendedTokens is the advisory upper bound on estimated body tokens.
	BodyRecommendedTokens = 700
	// BytesPerTokenProxy is the deterministic bytes-per-token divisor used by
	// EstimateBodyTokens, since engine/ carries no real tokenizer (ADR-15).
	BytesPerTokenProxy = 4
)

// LintError is a hard, blocking lint finding. It implements error so callers
// can print it directly, and carries a stable Rule ID so tests and callers
// can assert on identity rather than message text.
type LintError struct {
	Rule string
	Msg  string
}

func (e *LintError) Error() string {
	return fmt.Sprintf("[lint:%s] %s", e.Rule, e.Msg)
}

// Warning is a non-blocking, advisory lint finding.
type Warning struct {
	Rule string
	Msg  string
}

// EstimateBodyTokens is the deterministic body-size proxy: ceil(UTF-8 bytes
// / BytesPerTokenProxy). It performs no I/O and no tokenization.
func EstimateBodyTokens(body string) int {
	return (len(body) + BytesPerTokenProxy - 1) / BytesPerTokenProxy
}

// parsedFrontmatter is the result of the line-based frontmatter subset
// reader described in design.md's "LintSkill API shape" decision: top-level
// key: value pairs, one metadata: block with 2-space-indented pairs, and
// quote stripping. It is intentionally separate from parse.go, which is
// registry-specific and rejects unknown keys.
type parsedFrontmatter struct {
	Raw                  string
	Name                 string
	Description          string
	License              string
	MetaAuthor           string
	MetaVersion          string
	DescriptionMultiLine bool
}

// parseFrontmatterFields parses the already fence-split frontmatter text.
// It never fails: a missing or empty field simply surfaces as a zero value,
// which the required-fields rule then reports.
func parseFrontmatterFields(raw string) parsedFrontmatter {
	fm := parsedFrontmatter{Raw: raw}
	lines := strings.Split(raw, "\n")
	inMetadata := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}

		if inMetadata && (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")) {
			key, val, ok := splitKeyValue(strings.TrimSpace(line))
			if ok {
				switch key {
				case "author":
					fm.MetaAuthor = unquote(val)
				case "version":
					fm.MetaVersion = unquote(val)
				}
			}
			continue
		}
		inMetadata = false

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			// A line indented by less than the documented 2-space metadata
			// pair (for example, a one-space-indented `author:`/`version:`
			// entry), or any other stray indented line. This line's own
			// key: value is NOT parsed, and it ends the metadata block: a
			// well-formed 2-space entry appearing after it is also not
			// reached, because inMetadata was already reset to false above.
			// Only the five required fields (name, description, license,
			// metadata.author, metadata.version) left unset this way are
			// reported, by the required-fields hard rule, which names the
			// missing field. Any other stray-indented key/value pair (for
			// example, a mis-indented, non-required top-level key) is
			// dropped silently, with no diagnostic at all.
			continue
		}

		key, val, ok := splitKeyValue(line)
		if !ok {
			continue
		}

		switch key {
		case "name":
			fm.Name = unquote(val)
		case "license":
			fm.License = unquote(val)
		case "metadata":
			inMetadata = true
		case "description":
			val = strings.TrimSpace(val)
			if val == "" {
				// An empty tag line followed by an indented continuation is
				// YAML plain multi-line scalar syntax: content is genuinely
				// present, just in the wrong shape, so it is classified as
				// DescriptionMultiLine (a description-one-line hard error),
				// never as a missing required field. Only an empty tag line
				// with nothing following it is genuinely absent.
				if hasIndentedContinuation(lines, i) {
					fm.DescriptionMultiLine = true
				}
				continue
			}
			if isBlockScalarIndicator(val) {
				fm.DescriptionMultiLine = true
				continue
			}
			fm.Description = unquote(val)
			if hasIndentedContinuation(lines, i) {
				fm.DescriptionMultiLine = true
			}
		}
	}

	return fm
}

// hasIndentedContinuation reports whether, after skipping any whitespace-only
// lines following lines[i], the next non-blank line exists and is indented
// with a leading space or tab. This is the shared test for YAML plain
// multi-line scalar continuation, used by both an empty and a non-empty
// description tag line. A whitespace-only line carries no actual value on
// its own, but in YAML a plain scalar can continue across one: it does not
// end the scalar, so it must not by itself decide the outcome. The decision
// is made on the next non-blank line: indented means a genuine continuation;
// unindented, or no such line before the frontmatter ends, means none.
func hasIndentedContinuation(lines []string, i int) bool {
	j := i + 1
	for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
		j++
	}
	if j >= len(lines) {
		return false
	}
	next := lines[j]
	return strings.HasPrefix(next, " ") || strings.HasPrefix(next, "\t")
}

// splitKeyValue splits a "key: value" or "key:" line into its parts. It
// returns ok=false when the line has no colon or an empty key.
func splitKeyValue(s string) (key, val string, ok bool) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(s[:idx])
	val = strings.TrimSpace(s[idx+1:])
	if key == "" {
		return "", "", false
	}
	return key, val, true
}

// unquote strips one layer of matching double or single quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// blockScalarIndicators is the authoritative list of YAML block-scalar
// indicators (folded or literal, with optional chomping). It is the single
// source of data for both isBlockScalarIndicator and the rendered
// description-one-line rule summary, so the two can never drift apart.
var blockScalarIndicators = []string{">", "|", ">-", "|-", ">+", "|+"}

// isBlockScalarIndicator reports whether val is a YAML block-scalar
// indicator (folded or literal, with optional chomping).
func isBlockScalarIndicator(val string) bool {
	for _, ind := range blockScalarIndicators {
		if val == ind {
			return true
		}
	}
	return false
}

// descriptionOneLineSummary renders the description-one-line rule's Summary
// from blockScalarIndicators so the list of rejected indicators can never
// drift from the data isBlockScalarIndicator actually checks against.
func descriptionOneLineSummary() string {
	quoted := make([]string, len(blockScalarIndicators))
	for i, ind := range blockScalarIndicators {
		quoted[i] = "`" + ind + "`"
	}
	return fmt.Sprintf(
		"`description` is a single physical line: no block scalar (%s) and no indented continuation line.",
		strings.Join(quoted, ", "),
	)
}

// lintRule is one entry in the authoritative rule table. check is nil for
// frontmatter-fence, which SplitSkillFile evaluates before LintSkill ever
// runs (see the "LintSkillFile Composes SplitSkillFile and LintSkill"
// requirement in spec.md); it still renders in RenderLintRules for
// documentation.
type lintRule struct {
	ID       string
	Severity Severity
	Summary  string
	check    func(fm parsedFrontmatter, body string) []string
}

var (
	isoDateRe    = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	engramRefRe  = regexp.MustCompile(`engram:\d+`)
	prIssueRe    = regexp.MustCompile(`(?i)\bPR ?#\d+|#\d{3,}`)
	homePathRe   = regexp.MustCompile(`(/home/[^/\s]+/|/Users/[^/\s]+/|C:\\Users\\[^\\]+\\)`)
	inlineCodeRe = regexp.MustCompile("`([^`\n]+)`")
)

var bannedShellUtilities = map[string]bool{
	"cat":  true,
	"grep": true,
	"find": true,
	"sed":  true,
	"ls":   true,
}

// canonicalSections is the documented H2 heading order (design.md's
// section-order rule).
var canonicalSections = []string{
	"Activation Contract",
	"Hard Rules",
	"Decision Gates",
	"Execution Steps",
	"Output Contract",
	"References",
}

var lintRules = []lintRule{
	{
		ID:       "frontmatter-fence",
		Severity: SeverityHard,
		Summary:  "The file starts with a `---` line and has a closing `---` line.",
		check:    nil, // evaluated by SplitSkillFile, never by LintSkill
	},
	{
		ID:       "required-fields",
		Severity: SeverityHard,
		Summary:  "`name`, `description`, `license`, `metadata.author` and `metadata.version` are present and non-empty after unquoting.",
		check: func(fm parsedFrontmatter, body string) []string {
			return checkRequiredFields(fm)
		},
	},
	{
		ID:       "description-one-line",
		Severity: SeverityHard,
		Summary:  descriptionOneLineSummary(),
		check: func(fm parsedFrontmatter, body string) []string {
			return checkDescriptionOneLine(fm)
		},
	},
	{
		ID:       "description-max",
		Severity: SeverityHard,
		Summary:  fmt.Sprintf("`description` is at most %d characters (runes, after unquoting).", DescriptionMaxRunes),
		check: func(fm parsedFrontmatter, body string) []string {
			return checkDescriptionMax(fm)
		},
	},
	{
		ID:       "body-hard-budget",
		Severity: SeverityHard,
		Summary:  fmt.Sprintf("Estimated body tokens are at most %d.", BodyHardTokens),
		check: func(fm parsedFrontmatter, body string) []string {
			return checkBodyHardBudget(body)
		},
	},
	{
		ID:       "description-should",
		Severity: SeverityAdvisory,
		Summary:  fmt.Sprintf("`description` is at most %d characters.", DescriptionShouldRunes),
		check: func(fm parsedFrontmatter, body string) []string {
			return checkDescriptionShould(fm)
		},
	},
	{
		ID:       "body-recommended",
		Severity: SeverityAdvisory,
		Summary:  fmt.Sprintf("Estimated body tokens are at most %d.", BodyRecommendedTokens),
		check: func(fm parsedFrontmatter, body string) []string {
			return checkBodyRecommended(body)
		},
	},
	{
		ID:       "section-order",
		Severity: SeverityAdvisory,
		Summary:  sectionOrderSummary(),
		check: func(fm parsedFrontmatter, body string) []string {
			return checkSectionOrder(body)
		},
	},
	{
		ID:       "incident-log-shape",
		Severity: SeverityAdvisory,
		Summary:  "The body contains an ISO date, an observation reference, or a PR/issue number.",
		check: func(fm parsedFrontmatter, body string) []string {
			return checkIncidentLogShape(body)
		},
	},
	{
		ID:       "banned-shell-utility",
		Severity: SeverityAdvisory,
		Summary:  "An inline code span, or a line inside a fenced code block, whose first word is `cat`, `grep`, `find`, `sed` or `ls`.",
		check: func(fm parsedFrontmatter, body string) []string {
			return checkBannedShellUtility(body)
		},
	},
	{
		ID:       "home-path-leak",
		Severity: SeverityAdvisory,
		Summary:  "An absolute home-directory path (`/home/<name>/`, `/Users/<name>/` or `C:\\Users\\<name>\\`) appears anywhere in the file.",
		check: func(fm parsedFrontmatter, body string) []string {
			return checkHomePathLeak(fm, body)
		},
	},
}

func checkRequiredFields(fm parsedFrontmatter) []string {
	var msgs []string
	// description's presence is not fm.Description alone: a block scalar or
	// an indented continuation (fm.DescriptionMultiLine) means content is
	// genuinely present, just in the wrong shape, and is reported by
	// description-one-line instead — never as a missing required field.
	required := []struct {
		field   string
		present bool
	}{
		{"name", fm.Name != ""},
		{"description", fm.Description != "" || fm.DescriptionMultiLine},
		{"license", fm.License != ""},
		{"metadata.author", fm.MetaAuthor != ""},
		{"metadata.version", fm.MetaVersion != ""},
	}
	for _, r := range required {
		if !r.present {
			msgs = append(msgs, fmt.Sprintf("required field %q is missing or empty", r.field))
		}
	}
	return msgs
}

func checkDescriptionOneLine(fm parsedFrontmatter) []string {
	if fm.DescriptionMultiLine {
		return []string{"description must be a single physical line; block scalars and indented continuation lines are not allowed"}
	}
	return nil
}

func checkDescriptionMax(fm parsedFrontmatter) []string {
	if fm.DescriptionMultiLine {
		return nil
	}
	n := len([]rune(fm.Description))
	if n > DescriptionMaxRunes {
		return []string{fmt.Sprintf("description is %d runes, exceeds the hard bound of %d", n, DescriptionMaxRunes)}
	}
	return nil
}

func checkDescriptionShould(fm parsedFrontmatter) []string {
	if fm.DescriptionMultiLine {
		return nil
	}
	n := len([]rune(fm.Description))
	if n > DescriptionShouldRunes {
		return []string{fmt.Sprintf("description is %d runes, exceeds the recommended bound of %d", n, DescriptionShouldRunes)}
	}
	return nil
}

func checkBodyHardBudget(body string) []string {
	n := EstimateBodyTokens(body)
	if n > BodyHardTokens {
		return []string{fmt.Sprintf("body is an estimated %d tokens, exceeds the hard budget of %d", n, BodyHardTokens)}
	}
	return nil
}

func checkBodyRecommended(body string) []string {
	n := EstimateBodyTokens(body)
	if n > BodyRecommendedTokens {
		return []string{fmt.Sprintf("body is an estimated %d tokens, exceeds the recommended budget of %d", n, BodyRecommendedTokens)}
	}
	return nil
}

func checkSectionOrder(body string) []string {
	var present []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "## ") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
		for _, canon := range canonicalSections {
			if heading == canon {
				present = append(present, canon)
				break
			}
		}
	}

	lastIdx := -1
	for _, p := range present {
		idx := indexOfSection(p)
		if idx < lastIdx {
			return []string{fmt.Sprintf("section %q appears out of canonical order", p)}
		}
		lastIdx = idx
	}
	return nil
}

// sectionOrderSummary renders the section-order rule's Summary from
// canonicalSections so the documented section list can never drift from
// the data checkSectionOrder actually enforces.
func sectionOrderSummary() string {
	return fmt.Sprintf(
		"The canonical H2 headings that are present (%s) appear in canonical order.",
		strings.Join(canonicalSections, ", "),
	)
}

func indexOfSection(name string) int {
	for i, s := range canonicalSections {
		if s == name {
			return i
		}
	}
	return -1
}

func checkIncidentLogShape(body string) []string {
	var msgs []string
	if isoDateRe.MatchString(body) {
		msgs = append(msgs, "body contains an ISO date, which reads as an incident-log entry rather than a lesson")
	}
	if engramRefRe.MatchString(body) {
		msgs = append(msgs, "body contains an observation reference (engram:<id>), which reads as an incident-log entry rather than a lesson")
	}
	if prIssueRe.MatchString(body) {
		msgs = append(msgs, "body contains a PR or issue number, which reads as an incident-log entry rather than a lesson")
	}
	return msgs
}

func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(fields[0], "$")
}

func checkBannedShellUtility(body string) []string {
	seen := map[string]bool{}
	var msgs []string

	for _, m := range inlineCodeRe.FindAllStringSubmatch(body, -1) {
		w := firstWord(m[1])
		if bannedShellUtilities[w] && !seen[w] {
			seen[w] = true
			msgs = append(msgs, fmt.Sprintf("banned shell utility %q found in an inline code span; use the approved replacement instead", w))
		}
	}

	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			continue
		}
		w := firstWord(line)
		if bannedShellUtilities[w] && !seen[w] {
			seen[w] = true
			msgs = append(msgs, fmt.Sprintf("banned shell utility %q found in a fenced code block; use the approved replacement instead", w))
		}
	}

	return msgs
}

func checkHomePathLeak(fm parsedFrontmatter, body string) []string {
	combined := fm.Raw + "\n" + body
	if homePathRe.MatchString(combined) {
		return []string{"an absolute home-directory path leaks into the file"}
	}
	return nil
}

// SplitSkillFile splits a full SKILL.md file into its frontmatter and body
// text. It is where the frontmatter-fence hard rule is evaluated: a missing
// opening or closing `---` fence fails here, before LintSkill is ever
// called (see LintSkillFile).
func SplitSkillFile(data []byte) (frontmatter, body string, err error) {
	data = bytes.TrimPrefix(data, utf8BOM)
	lines := strings.Split(string(data), "\n")

	if len(lines) == 0 || !isFenceLine(lines[0]) {
		return "", "", &LintError{Rule: "frontmatter-fence", Msg: "file does not start with a `---` frontmatter fence"}
	}

	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if isFenceLine(lines[i]) {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return "", "", &LintError{Rule: "frontmatter-fence", Msg: "file has no closing `---` frontmatter fence"}
	}

	frontmatter = strings.Join(lines[1:closeIdx], "\n")
	body = strings.Join(lines[closeIdx+1:], "\n")
	return frontmatter, body, nil
}

// LintSkill evaluates every rule in lintRules against already-split
// frontmatter and body text. It is a pure function: no filesystem I/O, no
// network I/O, no os/exec, stdlib only (ADR-15). Given the same input it
// always returns the same hard errors and warnings.
func LintSkill(frontmatter, body string) (hard []error, warnings []Warning) {
	fm := parseFrontmatterFields(frontmatter)

	for _, r := range lintRules {
		if r.check == nil {
			continue
		}
		for _, msg := range r.check(fm, body) {
			switch r.Severity {
			case SeverityHard:
				hard = append(hard, &LintError{Rule: r.ID, Msg: msg})
			default:
				warnings = append(warnings, Warning{Rule: r.ID, Msg: msg})
			}
		}
	}

	return hard, warnings
}

// LintSkillFile composes SplitSkillFile and LintSkill: it first splits the
// raw file, and only calls LintSkill on a successful split. When
// SplitSkillFile fails, LintSkillFile surfaces that failure as its own hard
// error and never calls LintSkill.
func LintSkillFile(data []byte) (hard []error, warnings []Warning) {
	frontmatter, body, err := SplitSkillFile(data)
	if err != nil {
		return []error{err}, nil
	}
	return LintSkill(frontmatter, body)
}

// RenderLintRules renders the authoritative rule table as a Markdown table,
// with every numeric bound sourced from the named constants above (never a
// hardcoded literal in the rendered text). It is the single generator for
// the style guide's machine-checkable section (slice 1b).
func RenderLintRules() string {
	var b strings.Builder
	b.WriteString("| ID | Severity | Check |\n")
	b.WriteString("|---|---|---|\n")
	for _, r := range lintRules {
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", r.ID, r.Severity, escapeTableCell(r.Summary))
	}
	return b.String()
}

// escapeTableCell escapes GFM table-breaking pipe characters in s so a
// Summary containing a literal `|` (for example, to document YAML block
// scalar indicators) still renders as a single table cell, even inside a
// code span, since a raw `|` splits a Markdown table row regardless of
// backticks around it.
func escapeTableCell(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
