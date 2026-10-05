package registryyaml

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
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

	schema schema
	// unread is what the parser left out of the file, as notes in the order of the file
	// (skills.Registry.Unread).
	unread []string
}

// decodeV1 is the decoder of version 1 of the file. The version itself was found and judged
// before it was chosen (versionOf), so it only has to step over the key.
func decodeV1(tokens []tok) (skills.Registry, error) {
	p := &tokParser{tokens: tokens, schema: schemaV1}
	reg, err := p.parseDocument()
	reg.Unread = p.unread
	return reg, err
}

// open says whether the key t, whose own column is col and which the parser has just stepped
// over, is in the shape its field has, so that what is under it can be read as that field. A
// scalar has no block under it; the others have no value on their line.
//
// A field in another shape is the one thing the reader does not read as it is. If the field is of
// the must-understand set the file is refused, naming the field and the line: reading the file
// without it would change what install, approval or projection does. Any other field has the part
// that is in the wrong shape left out and said: the block under a scalar is skipped (leaveOut),
// and the value on a block's line is noted and the block under it is read as ever.
func (p *tokParser) open(path string, t tok, col int) (bool, error) {
	f, ok := p.schema.byPath[path]
	if !ok {
		panic("registryyaml: the decoder asked for a field its schema does not have: " + path)
	}
	hasValue := t.kind == tokKeyValue || t.kind == tokSeqMapping
	next := p.peek()
	hasBlock := next != nil && next.indent > col
	if (f.shape == shapeScalar && !hasBlock) || (f.shape != shapeScalar && !hasValue) {
		return true, nil
	}
	if f.must {
		return false, fmt.Errorf("line %d: %s", t.lineNum, f.refusal(p.schema.childrenOf(path)))
	}
	if f.shape == shapeScalar {
		p.leaveOut(t.lineNum, col, f.note())
		return false, nil
	}
	p.unread = append(p.unread, fmt.Sprintf("line %d: %s", t.lineNum, f.strayValueNote()))
	return true, nil
}

// value reads the scalar field at path into into, unless the field is in another shape (open).
func (p *tokParser) value(path string, t tok, col int, into *string) error {
	read, err := p.open(path, t, col)
	if err != nil {
		return err
	}
	if read {
		*into = t.val
	}
	return nil
}

// leaveOut notes that what the file says at line was not read, and steps over what is under
// the key, which is everything that follows it in a column further in than the key's own.
func (p *tokParser) leaveOut(line, col int, note string) {
	p.unread = append(p.unread, fmt.Sprintf("line %d: %s", line, note))
	for next := p.peek(); next != nil && next.indent > col; next = p.peek() {
		p.advance()
	}
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

// parseDocument parses the top-level YAML document of version 1 and checks the fields it must
// have. On a fault it returns what it had decoded so far with the error (see Decode).
func (p *tokParser) parseDocument() (skills.Registry, error) {
	var reg skills.Registry
	seenSkills := false

	for p.peek() != nil {
		t := p.peek()
		if t.indent != 0 {
			return reg, fmt.Errorf("line %d: unexpected indentation at document root", t.lineNum)
		}
		if t.kind != tokKeyValue && t.kind != tokKeyOnly {
			return reg, fmt.Errorf("line %d: unexpected token at document root", t.lineNum)
		}
		p.advance()
		switch t.key {
		case "version":
			// Found and judged before this decoder was chosen (versionOf).
		case "skills":
			if _, err := p.open("skills", *t, 0); err != nil {
				return reg, err
			}
			seenSkills = true
			entries, err := p.parseSkillEntries(2)
			reg.Skills = entries
			if err != nil {
				return reg, err
			}
		default:
			p.leaveOut(t.lineNum, 0, fmt.Sprintf("unknown top-level key %q", t.key))
		}
	}

	if !seenSkills {
		return reg, fmt.Errorf("skills: missing required top-level field 'skills'")
	}

	return reg, nil
}

// parseSkillEntries parses the block sequence of skill entries at seqIndent. On a fault it returns
// the entries that were whole before it with the error.
func (p *tokParser) parseSkillEntries(seqIndent int) ([]skills.Entry, error) {
	var entries []skills.Entry

	for {
		t := p.peek()
		if t == nil || t.indent < seqIndent {
			break
		}
		if t.indent > seqIndent {
			return entries, fmt.Errorf("line %d: unexpected indentation inside skills sequence", t.lineNum)
		}
		if t.kind != tokSeqMapping && t.kind != tokSeqKeyOnly {
			return entries, fmt.Errorf("line %d: expected sequence item ('- key: value') in skills list", t.lineNum)
		}
		p.advance()

		entry, err := p.parseEntry(*t, seqIndent+2)
		if err != nil {
			return entries, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// parseEntry parses one skill entry. first is the sequence-item token (e.g. "- id: sdd-spec"),
// which has the entry's first key. Subsequent keys at entryIndent are consumed from the token
// stream. The entry is not judged: that is the domain's.
func (p *tokParser) parseEntry(first tok, entryIndent int) (skills.Entry, error) {
	var e skills.Entry
	keysSeen := make(map[string]bool)

	if err := p.applyEntryKey(&e, first, entryIndent, keysSeen); err != nil {
		return skills.Entry{}, err
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
			return skills.Entry{}, fmt.Errorf("line %d: duplicate key %q in skill entry", t.lineNum, t.key)
		}
		if err := p.applyEntryKey(&e, *t, entryIndent, keysSeen); err != nil {
			return skills.Entry{}, err
		}
	}

	return e, nil
}

// applyEntryKey dispatches a single key within a skill entry. t is the key's token, which the
// parser has stepped over; entryIndent is the column of the entry's keys.
func (p *tokParser) applyEntryKey(e *skills.Entry, t tok, entryIndent int, seen map[string]bool) error {
	seen[t.key] = true
	switch t.key {
	case "id":
		return p.value("id", t, entryIndent, &e.ID)
	case "path":
		return p.value("path", t, entryIndent, &e.Path)
	case "source":
		read, err := p.open("source", t, entryIndent)
		if err != nil || !read {
			return err
		}
		src, err := p.parseSource(entryIndent+2, t.lineNum, e.ID)
		if err != nil {
			return err
		}
		e.Source = src
	case "install":
		read, err := p.open("install", t, entryIndent)
		if err != nil || !read {
			return err
		}
		inst, err := p.parseInstall(entryIndent + 2)
		if err != nil {
			return err
		}
		e.Install = inst
	case "lifecycle":
		read, err := p.open("lifecycle", t, entryIndent)
		if err != nil || !read {
			return err
		}
		lc, err := p.parseLifecycle(entryIndent + 2)
		if err != nil {
			return err
		}
		e.Lifecycle = lc
	default:
		p.leaveOut(t.lineNum, entryIndent, fmt.Sprintf("unknown key %q in skill entry", t.key))
	}
	return nil
}

// parseSource parses the source mapping. entryID is the owning entry's id (may be
// empty if id appears after source in the YAML) and is used in cross-field error
// messages (R-114, R-115). sourceLineNum is the line of the "source:" key itself.
func (p *tokParser) parseSource(indent, sourceLineNum int, entryID string) (skills.Source, error) {
	var src skills.Source
	seen := make(map[string]bool)
	var repoLineNum, refLineNum int

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
			return skills.Source{}, fmt.Errorf("line %d: duplicate key %q in source", t.lineNum, t.key)
		}
		seen[t.key] = true
		switch t.key {
		case "type":
			if err := p.value("source.type", *t, indent, &src.Type); err != nil {
				return skills.Source{}, err
			}
		case "upstream":
			read, err := p.open("source.upstream", *t, indent)
			if err != nil {
				return skills.Source{}, err
			}
			if read {
				u, err := p.parseUpstream(indent + 2)
				if err != nil {
					return skills.Source{}, err
				}
				src.Upstream = &u
			}
		case "repo":
			if err := p.value("source.repo", *t, indent, &src.Repo); err != nil {
				return skills.Source{}, err
			}
			repoLineNum = t.lineNum
		case "ref":
			if err := p.value("source.ref", *t, indent, &src.Ref); err != nil {
				return skills.Source{}, err
			}
			refLineNum = t.lineNum
		default:
			p.leaveOut(t.lineNum, indent, fmt.Sprintf("unknown key %q in source", t.key))
		}
	}

	// Cross-field validation (R-114, R-115, ADR-11): lives here in parseSource
	// (not validateEntry) so it can reference per-field line numbers from repoLineNum/refLineNum.
	// These checks run after all source keys are consumed so YAML field order is irrelevant.
	if src.Type != "" && src.Type != skills.SourceExternal {
		// repo or ref on a non-external entry is a hard error (mirrors allowedProjects-on-global).
		if src.Repo != "" {
			return skills.Source{}, fmt.Errorf("skills: entry %q: source.repo is not allowed when source.type is %q (line %d)", entryID, src.Type, repoLineNum)
		}
		if src.Ref != "" {
			return skills.Source{}, fmt.Errorf("skills: entry %q: source.ref is not allowed when source.type is %q (line %d)", entryID, src.Type, refLineNum)
		}
	}
	if src.Type == skills.SourceExternal && src.Repo == "" {
		// repo is required for external entries; reference the source block's opening line
		// so the message carries a location that reviewers can find (R-114).
		return skills.Source{}, fmt.Errorf("skills: entry %q: source.repo is required when source.type is 'external' (line %d)", entryID, sourceLineNum)
	}

	return src, nil
}

// parseUpstream parses the upstream mapping.
func (p *tokParser) parseUpstream(indent int) (skills.Upstream, error) {
	var u skills.Upstream
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
			return skills.Upstream{}, fmt.Errorf("line %d: duplicate key %q in upstream", t.lineNum, t.key)
		}
		seen[t.key] = true
		switch t.key {
		case "owner":
			if err := p.value("source.upstream.owner", *t, indent, &u.Owner); err != nil {
				return skills.Upstream{}, err
			}
		default:
			p.leaveOut(t.lineNum, indent, fmt.Sprintf("unknown key %q in upstream", t.key))
		}
	}

	return u, nil
}

// parseInstall parses the install mapping.
func (p *tokParser) parseInstall(indent int) (skills.Install, error) {
	var inst skills.Install
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
			return skills.Install{}, fmt.Errorf("line %d: duplicate key %q in install", t.lineNum, t.key)
		}
		seen[t.key] = true
		switch t.key {
		case "defaultScope":
			read, err := p.open("install.defaultScope", *t, indent)
			if err != nil {
				return skills.Install{}, err
			}
			if read {
				if t.val != skills.ScopeGlobal && t.val != skills.ScopeProject {
					return skills.Install{}, fmt.Errorf("line %d: install.defaultScope %q is not valid; must be 'global' or 'project'", t.lineNum, t.val)
				}
				inst.DefaultScope = t.val
			}
		case "targets":
			read, err := p.open("install.targets", *t, indent)
			if err != nil {
				return skills.Install{}, err
			}
			if read {
				targets, err := p.parseScalarSequence(indent+2, t.lineNum)
				if err != nil {
					return skills.Install{}, err
				}
				inst.Targets = targets
			}
		case "allowedProjects":
			read, err := p.open("install.allowedProjects", *t, indent)
			if err != nil {
				return skills.Install{}, err
			}
			if read {
				projects, err := p.parseScalarSequence(indent+2, t.lineNum)
				if err != nil {
					return skills.Install{}, err
				}
				inst.AllowedProjects = projects
			}
		default:
			p.leaveOut(t.lineNum, indent, fmt.Sprintf("unknown key %q in install", t.key))
		}
	}

	return inst, nil
}

// parseLifecycle parses the lifecycle mapping.
func (p *tokParser) parseLifecycle(indent int) (skills.Lifecycle, error) {
	var lc skills.Lifecycle
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
			return skills.Lifecycle{}, fmt.Errorf("line %d: duplicate key %q in lifecycle", t.lineNum, t.key)
		}
		seen[t.key] = true
		switch t.key {
		case "updateStrategy":
			if err := p.value("lifecycle.updateStrategy", *t, indent, &lc.UpdateStrategy); err != nil {
				return skills.Lifecycle{}, err
			}
		default:
			p.leaveOut(t.lineNum, indent, fmt.Sprintf("unknown key %q in lifecycle", t.key))
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
