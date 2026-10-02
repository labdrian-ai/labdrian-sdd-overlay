// Package contract reads a contract document: the markdown file whose frontmatter says which
// sub-agent phases a piece of guidance applies to, which it must be kept out of, under which
// heading of a prompt it is injected, and, optionally, for which languages and activations
// it applies at all. The package is pure. It reads text it is given and returns a value; it
// names no file, no process and no other package of the engine.
//
// There is one parse of the document (Parse) and one parse of a list (parseList). The gate
// that injects a contract into a prompt, the propagator that scopes a row of the skill
// registry from it, the runtime adapters that configure a plugin from it, and the status
// verb that checks it all call Parse, so a contract means the same thing to each of them.
// Before this package three places read the frontmatter with three list parsers, and the
// lists of phases were read leniently: "applies_to_phases: sdd-tasks, sdd-apply" was taken as
// two phases and "[sdd-tasks" as one.
//
// The frontmatter is read the way the contracts in use have always been read, and that is
// kept: the text between the first two "---" delimiters, which are found by text and not
// by line; one "key: value" per line, with white space around the key and the value ignored;
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

// Contract is what the frontmatter of a contract document says.
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
func (c Contract) ContextRequired() bool {
	return len(c.LanguageContext) > 0 || len(c.ActivationContext) > 0
}

// The ways a contract document is refused. Parse returns the zero Contract with each.
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

// Parse reads the contract a document's frontmatter describes. It fails, with the zero
// Contract, when the document has no frontmatter, when a list in it is not an inline list,
// when it names a context_operator, or when it names no phase to apply to.
func Parse(content string) (Contract, error) {
	parts := strings.SplitN(content, frontmatterDelimiter, 3)
	if len(parts) < 3 {
		return Contract{}, ErrNoFrontmatter
	}

	var c Contract
	lists := map[string]*[]string{
		keyAppliesTo:         &c.AppliesTo,
		keyExcluded:          &c.Excluded,
		keyLanguageContext:   &c.LanguageContext,
		keyActivationContext: &c.ActivationContext,
	}
	for _, line := range strings.Split(parts[1], "\n") {
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
		case key == keyContextOperator:
			return Contract{}, ErrUnsupportedContextOperator
		}
	}

	if len(c.AppliesTo) == 0 {
		return Contract{}, ErrNoAppliesTo
	}
	return c, nil
}

// parseList is the one parser of a list in the frontmatter, and it is strict: the value
// must be an inline list, "[a, b, c]", with the brackets, and nothing before or after them.
// Its items are separated by commas; white space and the quotes (either kind) at the ends
// of an item are removed, an empty item is dropped, and the case is kept. key only names the
// list in the error. The empty list "[]" is a list.
func parseList(key, value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, listOpen) || !strings.HasSuffix(value, listClose) {
		return nil, &MalformedListError{Key: key, Value: value}
	}
	inner := strings.TrimPrefix(strings.TrimSuffix(value, listClose), listOpen)
	var items []string
	for _, item := range strings.Split(inner, listSeparator) {
		if item = strings.Trim(strings.TrimSpace(item), itemQuotes); item != "" {
			items = append(items, item)
		}
	}
	return items, nil
}
