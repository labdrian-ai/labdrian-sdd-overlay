// Package contract reads a contract document: the markdown file whose frontmatter says which
// sub-agent phases a piece of guidance applies to, which it must be kept out of, under which
// heading of a prompt it is injected, and, optionally, for which languages and activations
// it applies at all. The package is pure. It reads text it is given and returns a value; it
// names no file, no process and no other package of the engine.
//
// There is one parse of the phase scope of the document (Parse), one of its context
// (ParseContext), ParseBoth for the reader of both, and one parse of a list (parseList). A reader parses only what it reads: the
// propagator that scopes a row of the skill registry and the status verb that checks a contract
// read the scope; the gate reads the scope and the context, and so does the OpenCode adapter
// for every contract but the unconditional minimalism one. A malformed context breaks the
// readers of the context and no other. Before this package three places read the frontmatter
// with three list parsers, and the lists of phases were read leniently:
// "applies_to_phases: sdd-tasks, sdd-apply" was taken as two phases and "[sdd-tasks" as one.
//
// The frontmatter is read the way the contracts in use have always been read, and that is
// kept: the lines between the first two delimiter lines, a delimiter being a line that is
// "---" and nothing else (a "---" inside a value, in a longer run of dashes or after other
// text on its line is part of the frontmatter, not its end); one "key: value" per line, with white space around the key and the value ignored;
// the later of two lines for a key wins; the body after the frontmatter is not read.
package contract

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// DefaultInjectionPoint is the heading a contract is injected under when its frontmatter does
// not name one.
const DefaultInjectionPoint = "## Skills to load before work"

// frontmatterDelimiter opens and closes the frontmatter.
const frontmatterDelimiter = "---"

// The keys of the frontmatter that are read.
const (
	keyAppliesTo         = "applies_to_phases"
	keyExcluded          = "excluded_phases"
	keyInjectionPoint    = "injection_point"
	keyLanguageContext   = "language_context"
	keyActivationContext = "activation_context"
	// keyContextOperator is read only to be refused: a contract that asks for an operator
	// between its contexts asks for something that is not supported, and is not guessed at.
	keyContextOperator = "context_operator"
)

// listOpen and listClose delimit an inline list: "[a, b, c]".
const (
	listOpen  = "["
	listClose = "]"
	// listSeparator separates the items of an inline list.
	listSeparator = ","
	// itemQuotes are the quotes an item may be wrapped in.
	itemQuotes = `"'`
)

// Contract is the phase scope a contract document's frontmatter describes.
type Contract struct {
	// AppliesTo is the phases the contract is injected into. It is never empty in a
	// Contract that Parse returns.
	AppliesTo []string
	// Excluded is the phases the contract is kept out of: it is stripped from a prompt that
	// carries it.
	Excluded []string
	// InjectionPoint is the heading the contract is injected under, as the frontmatter wrote
	// it, which is empty when it did not. Header says what to use.
	InjectionPoint string
}

// Context is the work a contract document's frontmatter says the contract applies to.
type Context struct {
	// LanguageContext and ActivationContext are the languages and the activations the
	// contract applies to, as written (the case is kept). Both empty means the contract does
	// not depend on the work it is injected for.
	LanguageContext   []string
	ActivationContext []string
}

// AppliesToPhase reports whether the contract is injected into phase.
func (c Contract) AppliesToPhase(phase string) bool { return slices.Contains(c.AppliesTo, phase) }

// ExcludesPhase reports whether the contract is kept out of phase.
func (c Contract) ExcludesPhase(phase string) bool { return slices.Contains(c.Excluded, phase) }

// Header is the heading the contract is injected under: the one the frontmatter names, or
// DefaultInjectionPoint.
func (c Contract) Header() string {
	if c.InjectionPoint != "" {
		return c.InjectionPoint
	}
	return DefaultInjectionPoint
}

// ContextRequired reports whether the contract applies only to work that matches its
// languages or its activations.
func (c Context) ContextRequired() bool {
	return len(c.LanguageContext) > 0 || len(c.ActivationContext) > 0
}

// The ways a contract document is refused. Parse and ParseContext return the zero value with each.
var (
	// ErrNoFrontmatter is a document without a frontmatter block.
	ErrNoFrontmatter = errors.New("contract file has no YAML frontmatter (expected content between " + frontmatterDelimiter + " delimiters)")
	// ErrNoAppliesTo is a document whose applies_to_phases is absent or empty: it says no
	// phase to inject into, and what a contract applies to is never guessed.
	ErrNoAppliesTo = errors.New("contract frontmatter missing or empty " + keyAppliesTo + ": " +
		"cannot derive scope without knowing which phases to inject into")
	// ErrUnsupportedContextOperator is a document that names a context_operator.
	ErrUnsupportedContextOperator = errors.New("unsupported " + keyContextOperator)
)

// MalformedListError is a list in the frontmatter that is not an inline list: "[a, b]".
type MalformedListError struct {
	// Key is the frontmatter key whose value is refused.
	Key string
	// Value is the value, with the white space around it removed.
	Value string
}

func (e *MalformedListError) Error() string {
	return fmt.Sprintf("malformed %s: expected an inline list such as [a, b], got %q", e.Key, e.Value)
}

// frontmatterLines is the lines of the frontmatter of content, or ErrNoFrontmatter: the lines
// between the first two delimiter lines.
func frontmatterLines(content string) ([]string, error) {
	lines := strings.Split(content, "\n")
	opening := slices.IndexFunc(lines, isDelimiter)
	if opening < 0 {
		return nil, ErrNoFrontmatter
	}
	body := lines[opening+1:]
	closing := slices.IndexFunc(body, isDelimiter)
	if closing < 0 {
		return nil, ErrNoFrontmatter
	}
	return body[:closing], nil
}

// isDelimiter reports whether line is a delimiter: "---" and nothing else. White space at
// the end of the line, which includes the CR of a CRLF ending, is not something else.
func isDelimiter(line string) bool {
	return strings.TrimRight(line, " \t\r") == frontmatterDelimiter
}

// Parse reads the phase scope a document's frontmatter describes. It fails, with the zero
// Contract, when the document has no frontmatter, when a list of the scope is not an inline
// list, or when it names no phase to apply to. It does not read the context (ParseContext does).
func Parse(content string) (Contract, error) {
	lines, err := frontmatterLines(content)
	if err != nil {
		return Contract{}, err
	}
	return parseScope(lines)
}

// ParseContext reads the context a document's frontmatter describes. It fails, with the zero
// Context, when the document has no frontmatter, when a list of the context is not an inline
// list, or when it names a context_operator. It does not read the phase scope (Parse does).
func ParseContext(content string) (Context, error) {
	lines, err := frontmatterLines(content)
	if err != nil {
		return Context{}, err
	}
	return parseContext(lines)
}

// ParseBoth reads the phase scope and the context of a document, as Parse and ParseContext
// would, and splits its frontmatter once. It is for the reader that needs both, which would
// otherwise find the frontmatter twice. It fails with what Parse fails with, and when the
// scope is readable, with what ParseContext fails with; the zero values come with the error.
func ParseBoth(content string) (Contract, Context, error) {
	lines, err := frontmatterLines(content)
	if err != nil {
		return Contract{}, Context{}, err
	}
	c, err := parseScope(lines)
	if err != nil {
		return Contract{}, Context{}, err
	}
	needs, err := parseContext(lines)
	if err != nil {
		return Contract{}, Context{}, err
	}
	return c, needs, nil
}

// parseScope is Parse over the lines of the frontmatter.
func parseScope(lines []string) (Contract, error) {
	var c Contract
	lists := map[string]*[]string{
		keyAppliesTo: &c.AppliesTo,
		keyExcluded:  &c.Excluded,
	}
	for _, line := range lines {
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found {
			continue
		}
		switch {
		case lists[key] != nil:
			list, err := parseList(key, value)
			if err != nil {
				return Contract{}, err
			}
			*lists[key] = list
		case key == keyInjectionPoint:
			c.InjectionPoint = strings.Trim(strings.TrimSpace(value), itemQuotes)
		}
	}

	if len(c.AppliesTo) == 0 {
		return Contract{}, ErrNoAppliesTo
	}
	return c, nil
}

// parseContext is ParseContext over the lines of the frontmatter.
func parseContext(lines []string) (Context, error) {
	var c Context
	for _, line := range lines {
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		var err error
		switch {
		case !found:
		case key == keyLanguageContext:
			c.LanguageContext, err = parseList(key, value)
		case key == keyActivationContext:
			c.ActivationContext, err = parseList(key, value)
		case key == keyContextOperator:
			err = ErrUnsupportedContextOperator
		}
		if err != nil {
			return Context{}, err
		}
	}
	return c, nil
}

// parseList is the one parser of a list in the frontmatter, and it is strict: the value
// must be an inline list, "[a, b, c]", with the brackets, and nothing before or after them.
// The brackets are the ends of the value and nothing else: a bracket among the items
// ("[a][b]", "[a, [b]]") is not a list of one odd item, it is refused, and so is one inside
// a quoted item (["a[1]"]): the list has no escape for a bracket, and the name of a phase or
// a language has none. Its items are
// separated by commas; white space and the quotes (either kind) at the ends of an item are
// removed, an empty item is dropped, and the case is kept. key only names the list in the
// error. The empty list "[]" is a list.
func parseList(key, value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, listOpen) || !strings.HasSuffix(value, listClose) {
		return nil, &MalformedListError{Key: key, Value: value}
	}
	inner := strings.TrimPrefix(strings.TrimSuffix(value, listClose), listOpen)
	if strings.ContainsAny(inner, listOpen+listClose) {
		return nil, &MalformedListError{Key: key, Value: value}
	}
	var items []string
	for _, item := range strings.Split(inner, listSeparator) {
		if item = strings.Trim(strings.TrimSpace(item), itemQuotes); item != "" {
			items = append(items, item)
		}
	}
	return items, nil
}
