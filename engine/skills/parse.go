// Package skills provides parsing and validation for skills.registry.yaml.
package skills

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// tokenKind identifies the syntactic role of a parsed YAML line.
type tokenKind int

const (
	tokKeyValue   tokenKind = iota // key: value
	tokKeyOnly                     // key:
	tokSeqMapping                  // - key: value  (first key of a sequence-item mapping)
	tokSeqKeyOnly                  // - key:         (first key of a sequence-item mapping, no inline value)
	tokSeqScalar                   // - value        (bare scalar inside a sequence)
)

type tok struct {
	kind    tokenKind
	indent  int
	key     string
	val     string
	lineNum int
}

// ParseRegistry parses a strict YAML subset from r and returns a Registry.
// It rejects every construct outside the documented subset with a line-numbered
// error. The parser never returns partial data on error (R-011).
// minimal: forced — ADR-1 (zero-dep invariant)
func ParseRegistry(r io.Reader) (Registry, error) {
	tokens, err := tokenize(r)
	if err != nil {
		return Registry{}, err
	}
	p := &tokParser{tokens: tokens}
	return p.parseDocument()
}

// tokenize scans the reader line-by-line, checks for forbidden constructs,
// and emits a flat []tok slice. Any out-of-subset construct causes an
// immediate line-numbered error.
func tokenize(r io.Reader) ([]tok, error) {
	var tokens []tok
	scanner := bufio.NewScanner(r)
	lineNum := 0
	seenContent := false
	seenDocMarker := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Reject tabs anywhere in the line; the constraint targets indentation.
		if strings.ContainsRune(line, '\t') {
			return nil, fmt.Errorf("line %d: tab character not allowed; use spaces for indentation", lineNum)
		}

		trimmed := strings.TrimSpace(line)

		// Skip blank lines and full-line comments.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// YAML document markers.
		if trimmed == "---" || trimmed == "..." {
			if seenContent || seenDocMarker {
				return nil, fmt.Errorf("line %d: multi-document YAML is not supported", lineNum)
			}
			seenDocMarker = true
			continue
		}

		seenContent = true

		// Reject flow sequences and mappings.
		if strings.ContainsAny(trimmed, "{}[]") {
			return nil, fmt.Errorf("line %d: flow sequences/mappings ({}, []) are not supported", lineNum)
		}

		// Reject anchors and aliases.
		if strings.ContainsAny(trimmed, "&*") {
			return nil, fmt.Errorf("line %d: YAML anchors (&) and aliases (*) are not supported", lineNum)
		}

		// Reject YAML tags.
		if strings.ContainsRune(trimmed, '!') {
			return nil, fmt.Errorf("line %d: YAML tags (!) are not supported", lineNum)
		}

		// Reject block scalars: any value starting with | or > after a key (covers
		// |, >, |-, |+, >-, >+, |2, etc.).
		if idx := strings.Index(trimmed, ": "); idx >= 0 {
			valPart := strings.TrimSpace(trimmed[idx+2:])
			if len(valPart) > 0 && (valPart[0] == '|' || valPart[0] == '>') {
				return nil, fmt.Errorf("line %d: block scalars (| and >) are not supported", lineNum)
			}
		}

		indent := countIndent(line)
		t, err := parseLine(trimmed, indent, lineNum)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("skills: read error: %w", err)
	}

	return tokens, nil
}

// countIndent returns the number of leading space characters on a line.
func countIndent(line string) int {
	for i, ch := range line {
		if ch != ' ' {
			return i
		}
	}
	return len(line)
}

// parseLine converts a single trimmed, non-blank, non-comment line to a tok.
func parseLine(trimmed string, indent, lineNum int) (tok, error) {
	if strings.HasPrefix(trimmed, "- ") {
		return parseContent(trimmed[2:], indent, lineNum, true)
	}
	if trimmed == "-" {
		return tok{kind: tokSeqScalar, indent: indent, val: "", lineNum: lineNum}, nil
	}
	return parseContent(trimmed, indent, lineNum, false)
}

// parseContent tokenizes a content string (after stripping any "- " prefix).
// The isSeqItem flag controls which token kinds are returned for plain scalars.
func parseContent(content string, indent, lineNum int, isSeqItem bool) (tok, error) {
	// Locate the first ': ' boundary or a trailing ':' to find the key.
	colonIdx := -1
	for i := 0; i < len(content); i++ {
		if content[i] == ':' {
			if i+1 == len(content) || content[i+1] == ' ' {
				colonIdx = i
				break
			}
		}
	}

	if colonIdx < 0 {
		// SUGGESTION-2: block scalar indicator on a standalone line (e.g. just "|-").
		if len(content) > 0 && (content[0] == '|' || content[0] == '>') {
			return tok{}, fmt.Errorf("line %d: block scalars (| and >) are not supported", lineNum)
		}
		if isSeqItem {
			// FIX 1: reject inline trailing comment in unquoted sequence scalar.
			if !isQuotedScalar(content) && strings.Contains(content, " # ") {
				return tok{}, fmt.Errorf("line %d: inline comments are not supported", lineNum)
			}
			val, err := unquoteScalar(content, lineNum)
			if err != nil {
				return tok{}, err
			}
			return tok{kind: tokSeqScalar, indent: indent, val: val, lineNum: lineNum}, nil
		}
		return tok{}, fmt.Errorf("line %d: expected 'key: value' or 'key:' format, got %q", lineNum, content)
	}

	key := content[:colonIdx]
	if key == "" {
		return tok{}, fmt.Errorf("line %d: empty key", lineNum)
	}

	// key: (no value)
	if colonIdx+1 == len(content) {
		if isSeqItem {
			return tok{kind: tokSeqKeyOnly, indent: indent, key: key, lineNum: lineNum}, nil
		}
		return tok{kind: tokKeyOnly, indent: indent, key: key, lineNum: lineNum}, nil
	}

	// key: value
	rawVal := strings.TrimSpace(content[colonIdx+2:])
	// FIX 1: reject inline trailing comment in unquoted values.
	if !isQuotedScalar(rawVal) && strings.Contains(rawVal, " # ") {
		return tok{}, fmt.Errorf("line %d: inline comments are not supported", lineNum)
	}
	// FIX 2: detect unterminated quoted strings.
	val, err := unquoteScalar(rawVal, lineNum)
	if err != nil {
		return tok{}, err
	}
	if isSeqItem {
		return tok{kind: tokSeqMapping, indent: indent, key: key, val: val, lineNum: lineNum}, nil
	}
	return tok{kind: tokKeyValue, indent: indent, key: key, val: val, lineNum: lineNum}, nil
}

// isQuotedScalar reports whether s is a properly paired quoted string
// (starts and ends with the same quote character, length >= 2).
func isQuotedScalar(s string) bool {
	if len(s) < 2 {
		return false
	}
	return (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')
}

// unquoteScalar strips a matching surrounding pair of single or double quotes.
// FIX 2: if the scalar starts with a quote character but has no matching closing
// quote it returns a line-numbered error (unterminated quoted string).
func unquoteScalar(s string, lineNum int) (string, error) {
	if len(s) == 0 {
		return "", nil
	}
	if s[0] == '"' || s[0] == '\'' {
		q := s[0]
		if len(s) >= 2 && s[len(s)-1] == q {
			return s[1 : len(s)-1], nil
		}
		return "", fmt.Errorf("line %d: unterminated quoted string", lineNum)
	}
	return s, nil
}

// --- token-stream parser ---

type tokParser struct {
	tokens []tok
	pos    int
}

func (p *tokParser) peek() *tok {
	if p.pos < len(p.tokens) {
		return &p.tokens[p.pos]
	}
	return nil
}

func (p *tokParser) advance() *tok {
	if p.pos < len(p.tokens) {
		t := &p.tokens[p.pos]
		p.pos++
		return t
	}
	return nil
}

// parseDocument parses the top-level YAML document and validates required fields.
func (p *tokParser) parseDocument() (Registry, error) {
	var reg Registry
	seenVersion := false
	seenSkills := false

	for p.peek() != nil {
		t := p.peek()
		if t.indent != 0 {
			return Registry{}, fmt.Errorf("line %d: unexpected indentation at document root", t.lineNum)
		}
		switch t.kind {
		case tokKeyValue:
			p.advance()
			switch t.key {
			case "version":
				seenVersion = true
				reg.Version = t.val
			default:
				return Registry{}, fmt.Errorf("line %d: unknown top-level key %q", t.lineNum, t.key)
			}
		case tokKeyOnly:
			p.advance()
			switch t.key {
			case "skills":
				seenSkills = true
				entries, err := p.parseSkillEntries(2)
				if err != nil {
					return Registry{}, err
				}
				reg.Skills = entries
			default:
				return Registry{}, fmt.Errorf("line %d: unknown top-level key %q", t.lineNum, t.key)
			}
		default:
			return Registry{}, fmt.Errorf("line %d: unexpected token at document root", t.lineNum)
		}
	}

	if !seenVersion {
		return Registry{}, fmt.Errorf("skills: missing required top-level field 'version'")
	}
	if reg.Version != "1" {
		return Registry{}, fmt.Errorf("skills: version %q is not supported; only version \"1\" is valid", reg.Version)
	}
	if !seenSkills {
		return Registry{}, fmt.Errorf("skills: missing required top-level field 'skills'")
	}

	return reg, nil
}

// parseSkillEntries parses the block sequence of skill entries at seqIndent.
func (p *tokParser) parseSkillEntries(seqIndent int) ([]Entry, error) {
	var entries []Entry
	seen := make(map[string]bool)

	for {
		t := p.peek()
		if t == nil || t.indent < seqIndent {
			break
		}
		if t.indent > seqIndent {
			return nil, fmt.Errorf("line %d: unexpected indentation inside skills sequence", t.lineNum)
		}
		if t.kind != tokSeqMapping && t.kind != tokSeqKeyOnly {
			return nil, fmt.Errorf("line %d: expected sequence item ('- key: value') in skills list", t.lineNum)
		}
		p.advance()

		entry, err := p.parseEntry(t.key, t.val, seqIndent+2, t.lineNum)
		if err != nil {
			return nil, err
		}

		if seen[entry.ID] {
			return nil, fmt.Errorf("skills: duplicate id %q", entry.ID)
		}
		seen[entry.ID] = true
		entries = append(entries, entry)
	}

	return entries, nil
}

// parseEntry parses one skill entry. firstKey/firstVal come from the sequence-item
// token (e.g. "- id: sdd-spec"). Subsequent keys at entryIndent are consumed from
// the token stream.
func (p *tokParser) parseEntry(firstKey, firstVal string, entryIndent, lineNum int) (Entry, error) {
	var e Entry
	keysSeen := make(map[string]bool)

	if err := p.applyEntryKey(&e, firstKey, firstVal, entryIndent, lineNum, keysSeen); err != nil {
		return Entry{}, err
	}

	for {
		t := p.peek()
		if t == nil || t.indent != entryIndent {
			break
		}
		if t.kind != tokKeyValue && t.kind != tokKeyOnly {
			break
		}
		p.advance()
		if keysSeen[t.key] {
			return Entry{}, fmt.Errorf("line %d: duplicate key %q in skill entry", t.lineNum, t.key)
		}
		if err := p.applyEntryKey(&e, t.key, t.val, entryIndent, t.lineNum, keysSeen); err != nil {
			return Entry{}, err
		}
	}

	if err := validateEntry(&e); err != nil {
		return Entry{}, err
	}

	return e, nil
}

// applyEntryKey dispatches a single key within a skill entry.
func (p *tokParser) applyEntryKey(e *Entry, key, val string, entryIndent, lineNum int, seen map[string]bool) error {
	seen[key] = true
	switch key {
	case "id":
		e.ID = val
	case "path":
		e.Path = val
	case "source":
		src, err := p.parseSource(entryIndent+2, lineNum)
		if err != nil {
			return err
		}
		e.Source = src
	case "install":
		inst, err := p.parseInstall(entryIndent+2, lineNum)
		if err != nil {
			return err
		}
		e.Install = inst
	case "lifecycle":
		lc, err := p.parseLifecycle(entryIndent+2, lineNum)
		if err != nil {
			return err
		}
		e.Lifecycle = lc
	default:
		return fmt.Errorf("line %d: unknown key %q in skill entry", lineNum, key)
	}
	return nil
}

// parseSource parses the source mapping.
func (p *tokParser) parseSource(indent, lineNum int) (Source, error) {
	var src Source
	seen := make(map[string]bool)

	for {
		t := p.peek()
		if t == nil || t.indent != indent {
			break
		}
		if t.kind != tokKeyValue && t.kind != tokKeyOnly {
			break
		}
		p.advance()
		if seen[t.key] {
			return Source{}, fmt.Errorf("line %d: duplicate key %q in source", t.lineNum, t.key)
		}
		seen[t.key] = true
		switch t.key {
		case "type":
			src.Type = t.val
		case "upstream":
			u, err := p.parseUpstream(indent+2, t.lineNum)
			if err != nil {
				return Source{}, err
			}
			src.Upstream = &u
		default:
			return Source{}, fmt.Errorf("line %d: unknown key %q in source", t.lineNum, t.key)
		}
	}

	return src, nil
}

// parseUpstream parses the upstream mapping.
func (p *tokParser) parseUpstream(indent, lineNum int) (Upstream, error) {
	var u Upstream
	seen := make(map[string]bool)

	for {
		t := p.peek()
		if t == nil || t.indent != indent {
			break
		}
		if t.kind != tokKeyValue && t.kind != tokKeyOnly {
			break
		}
		p.advance()
		if seen[t.key] {
			return Upstream{}, fmt.Errorf("line %d: duplicate key %q in upstream", t.lineNum, t.key)
		}
		seen[t.key] = true
		switch t.key {
		case "owner":
			u.Owner = t.val
		default:
			return Upstream{}, fmt.Errorf("line %d: unknown key %q in upstream", t.lineNum, t.key)
		}
	}

	return u, nil
}

// parseInstall parses the install mapping.
func (p *tokParser) parseInstall(indent, lineNum int) (Install, error) {
	var inst Install
	seen := make(map[string]bool)

	for {
		t := p.peek()
		if t == nil || t.indent != indent {
			break
		}
		if t.kind != tokKeyValue && t.kind != tokKeyOnly {
			break
		}
		p.advance()
		if seen[t.key] {
			return Install{}, fmt.Errorf("line %d: duplicate key %q in install", t.lineNum, t.key)
		}
		seen[t.key] = true
		switch t.key {
		case "defaultScope":
			if t.val != "global" && t.val != "project" {
				return Install{}, fmt.Errorf("line %d: install.defaultScope %q is not valid; must be 'global' or 'project'", t.lineNum, t.val)
			}
			inst.DefaultScope = t.val
		case "targets":
			targets, err := p.parseScalarSequence(indent+2, t.lineNum)
			if err != nil {
				return Install{}, err
			}
			inst.Targets = targets
		case "allowedProjects":
			projects, err := p.parseScalarSequence(indent+2, t.lineNum)
			if err != nil {
				return Install{}, err
			}
			inst.AllowedProjects = projects
		default:
			return Install{}, fmt.Errorf("line %d: unknown key %q in install", t.lineNum, t.key)
		}
	}

	return inst, nil
}

// parseLifecycle parses the lifecycle mapping.
func (p *tokParser) parseLifecycle(indent, lineNum int) (Lifecycle, error) {
	var lc Lifecycle
	seen := make(map[string]bool)

	for {
		t := p.peek()
		if t == nil || t.indent != indent {
			break
		}
		if t.kind != tokKeyValue && t.kind != tokKeyOnly {
			break
		}
		p.advance()
		if seen[t.key] {
			return Lifecycle{}, fmt.Errorf("line %d: duplicate key %q in lifecycle", t.lineNum, t.key)
		}
		seen[t.key] = true
		switch t.key {
		case "updateStrategy":
			lc.UpdateStrategy = t.val
		default:
			return Lifecycle{}, fmt.Errorf("line %d: unknown key %q in lifecycle", t.lineNum, t.key)
		}
	}

	return lc, nil
}

// parseScalarSequence parses a block sequence of plain scalars at the given indent.
func (p *tokParser) parseScalarSequence(indent, lineNum int) ([]string, error) {
	var items []string

	for {
		t := p.peek()
		if t == nil || t.indent != indent {
			break
		}
		if t.kind != tokSeqScalar {
			break
		}
		p.advance()
		items = append(items, t.val)
	}

	return items, nil
}

// --- schema validation ---

var validSourceTypes = map[string]bool{"core": true, "custom": true}
var validTargets = map[string]bool{"claude": true, "opencode": true, "codex": true}
var validUpdateStrategies = map[string]bool{"vendor-merge": true, "overlay-only": true}

// validateEntry enforces the schema-level constraints on a single Entry.
func validateEntry(e *Entry) error {
	if e.ID == "" {
		return fmt.Errorf("skills: entry is missing required field 'id'")
	}
	if !validSourceTypes[e.Source.Type] {
		return fmt.Errorf("skills: entry %q: source.type %q is not valid; must be 'core' or 'custom'", e.ID, e.Source.Type)
	}
	if e.Source.Type == "custom" && e.Source.Upstream != nil {
		return fmt.Errorf("skills: entry %q: source.upstream is not allowed when source.type is 'custom'", e.ID)
	}
	if e.Source.Type == "core" && e.Source.Upstream != nil && e.Source.Upstream.Owner == "" {
		return fmt.Errorf("skills: entry %q: source.upstream.owner must not be empty", e.ID)
	}
	// Scope enum is validated with a line number in parseInstall; by the time
	// validateEntry runs, DefaultScope is always "global" or "project".
	if e.Install.DefaultScope == "global" && len(e.Install.AllowedProjects) > 0 {
		return fmt.Errorf("skills: entry %q: allowedProjects is only valid for project-scoped entries", e.ID)
	}
	if len(e.Install.Targets) == 0 {
		return fmt.Errorf("skills: entry %q: install.targets must not be empty (R-007)", e.ID)
	}
	for _, target := range e.Install.Targets {
		if !validTargets[target] {
			return fmt.Errorf("skills: entry %q: install.targets contains invalid value %q; must be one of: claude, opencode, codex", e.ID, target)
		}
	}
	if !validUpdateStrategies[e.Lifecycle.UpdateStrategy] {
		return fmt.Errorf("skills: entry %q: lifecycle.updateStrategy %q is not valid; must be 'vendor-merge' or 'overlay-only'", e.ID, e.Lifecycle.UpdateStrategy)
	}
	return nil
}
