package contract_test

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
)

// doc is a contract document with the given frontmatter lines and a body that is not read.
func doc(frontmatter ...string) string {
	return "---\n" + strings.Join(frontmatter, "\n") + "\n---\n# A contract\n\nThe body is not read: applies_to_phases: nothing\n"
}

func TestParseReadsThePhaseScope(t *testing.T) {
	got, err := contract.Parse(doc(
		"applies_to_phases: [sdd-design, sdd-tasks, sdd-apply]",
		"excluded_phases: [sdd-propose, sdd-archive]",
		`injection_point: "## Skills to load before work"`,
	))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := contract.Contract{
		AppliesTo:      []string{"sdd-design", "sdd-tasks", "sdd-apply"},
		Excluded:       []string{"sdd-propose", "sdd-archive"},
		InjectionPoint: "## Skills to load before work",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %#v, want %#v", got, want)
	}
}

// Only applies_to_phases is required; every other key may be absent, and an absent key is an
// empty list or an empty injection point, which Header names the default for.
func TestParseNeedsOnlyTheAppliesToList(t *testing.T) {
	got, err := contract.Parse(doc("applies_to_phases: [sdd-apply]"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !reflect.DeepEqual(got.AppliesTo, []string{"sdd-apply"}) || got.Excluded != nil || got.InjectionPoint != "" {
		t.Errorf("Parse = %#v, want only AppliesTo", got)
	}
	if got.Header() != contract.DefaultInjectionPoint {
		t.Errorf("Header() = %q, want the default %q", got.Header(), contract.DefaultInjectionPoint)
	}
}

func TestInjectionPoint(t *testing.T) {
	for _, tc := range []struct{ line, want string }{
		{`injection_point: "## Quoted"`, "## Quoted"},
		{`injection_point: '## Single'`, "## Single"},
		{`injection_point: ## Bare`, "## Bare"},
		{`injection_point:`, ""},
		{`injection_point: ""`, ""},
	} {
		got, err := contract.Parse(doc("applies_to_phases: [a]", tc.line))
		if err != nil {
			t.Errorf("%s: %v", tc.line, err)
			continue
		}
		if got.InjectionPoint != tc.want {
			t.Errorf("%s: InjectionPoint = %q, want %q", tc.line, got.InjectionPoint, tc.want)
		}
		wantHeader := tc.want
		if wantHeader == "" {
			wantHeader = contract.DefaultInjectionPoint
		}
		if got.Header() != wantHeader {
			t.Errorf("%s: Header() = %q, want %q", tc.line, got.Header(), wantHeader)
		}
	}
}

// The shapes the frontmatter can have around its keys. None of them is a refusal, and a
// reader that tightened any of them would change what a contract already in use means.
func TestParseFrontmatterShapes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    []string
	}{
		{"indented keys", "---\n  applies_to_phases: [sdd-apply]\n---\n", []string{"sdd-apply"}},
		{"a tab before the key", "---\n\tapplies_to_phases: [sdd-apply]\n---\n", []string{"sdd-apply"}},
		{"CRLF line endings", "---\r\napplies_to_phases: [sdd-apply]\r\n---\r\n# c\r\n", []string{"sdd-apply"}},
		{"the later of two keys wins", doc("applies_to_phases: [sdd-tasks]", "applies_to_phases: [sdd-apply]"), []string{"sdd-apply"}},
		{"a preamble before the first delimiter", "preamble\n---\napplies_to_phases: [sdd-apply]\n---\nbody", []string{"sdd-apply"}},
		{"spaces around items", doc("applies_to_phases: [ sdd-apply ,  sdd-tasks ]"), []string{"sdd-apply", "sdd-tasks"}},
		{"an empty item between commas", doc("applies_to_phases: [sdd-apply, , sdd-tasks]"), []string{"sdd-apply", "sdd-tasks"}},
		{"quoted items", doc(`applies_to_phases: ["sdd-apply", 'sdd-tasks']`), []string{"sdd-apply", "sdd-tasks"}},
		{"a key that only starts with the name", doc("applies_to_phases_extra: [x]", "applies_to_phases: [sdd-apply]"), []string{"sdd-apply"}},
		{"a space before the colon is not the key", doc("applies_to_phases: [sdd-apply]", "applies_to_phases : [sdd-tasks]"), []string{"sdd-apply"}},
		{"a key name inside a value", doc("injection_point: applies_to_phases: [x]", "applies_to_phases: [sdd-apply]"), []string{"sdd-apply"}},
		{"a line that is not a key", doc("a note", "applies_to_phases: [sdd-apply]"), []string{"sdd-apply"}},
		{"the body is not read", "---\napplies_to_phases: [sdd-apply]\n---\napplies_to_phases: [x]\n", []string{"sdd-apply"}},
		// The context is ParseContext's: a reader of the scope alone is not broken by a malformed one.
		{"a malformed language_context is not read", doc("applies_to_phases: [sdd-apply]", "language_context: go"), []string{"sdd-apply"}},
		{"a malformed activation_context is not read", doc("applies_to_phases: [sdd-apply]", "activation_context:"), []string{"sdd-apply"}},
		{"a context_operator is not read", doc("applies_to_phases: [sdd-apply]", "context_operator: any"), []string{"sdd-apply"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := contract.Parse(tc.content)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(got.AppliesTo, tc.want) {
				t.Errorf("AppliesTo = %v, want %v", got.AppliesTo, tc.want)
			}
		})
	}
}

// A delimiter is a line that is "---" and nothing else (white space at the end of the line,
// and the CR of a CRLF ending, do not count). "---" inside a value, in a longer run of
// dashes or after other text on its line is part of the frontmatter, not the end of it.
func TestParseReadsADelimiterOnlyAsAWholeLine(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    contract.Contract
	}{
		{"a --- inside a value", doc(`injection_point: "## a --- b"`, "excluded_phases: [sdd-propose]", "applies_to_phases: [sdd-apply]"),
			contract.Contract{AppliesTo: []string{"sdd-apply"}, Excluded: []string{"sdd-propose"}, InjectionPoint: "## a --- b"}},
		{"a longer run of dashes is not a delimiter", "---\ninjection_point: x\n-----\napplies_to_phases: [sdd-apply]\n---\n",
			contract.Contract{AppliesTo: []string{"sdd-apply"}, InjectionPoint: "x"}},
		{"a delimiter with text after it is not a delimiter", "---\napplies_to_phases: [sdd-apply]\n--- end\nexcluded_phases: [sdd-propose]\n---\n",
			contract.Contract{AppliesTo: []string{"sdd-apply"}, Excluded: []string{"sdd-propose"}}},
		{"white space after a delimiter is not text", "---  \t\napplies_to_phases: [sdd-apply]\n---\t \nbody",
			contract.Contract{AppliesTo: []string{"sdd-apply"}}},
		{"an indented --- is not a delimiter", "---\napplies_to_phases: [sdd-apply]\n  ---\nexcluded_phases: [sdd-propose]\n---\n",
			contract.Contract{AppliesTo: []string{"sdd-apply"}, Excluded: []string{"sdd-propose"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := contract.Parse(tc.content)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Parse = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// Without a second delimiter line there is no frontmatter, whatever else contains "---".
func TestParseRefusesAFrontmatterWhoseClosingDelimiterIsNotAWholeLine(t *testing.T) {
	for name, content := range map[string]string{
		"dashes inside a value only":    "---\napplies_to_phases: [sdd-apply]\ninjection_point: a --- b\n",
		"a longer run of dashes closes": "---\napplies_to_phases: [sdd-apply]\n-----\n",
		"text after the closing dashes": "---\napplies_to_phases: [sdd-apply]\n--- end\n",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := contract.Parse(content)
			if !errors.Is(err, contract.ErrNoFrontmatter) || !reflect.DeepEqual(got, contract.Contract{}) {
				t.Errorf("Parse = %#v, %v, want the zero value and ErrNoFrontmatter", got, err)
			}
		})
	}
}

func TestParseRefusesADocumentWithoutFrontmatter(t *testing.T) {
	for name, content := range map[string]string{
		"empty":                "",
		"text only":            "no frontmatter here",
		"one delimiter":        "---\napplies_to_phases: [sdd-apply]\n",
		"two delimiters short": "---\n---",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := contract.Parse(content)
			if name == "two delimiters short" {
				// Three parts are enough to have a frontmatter, an empty one.
				if !errors.Is(err, contract.ErrNoAppliesTo) {
					t.Errorf("error = %v, want ErrNoAppliesTo", err)
				}
			} else if !errors.Is(err, contract.ErrNoFrontmatter) {
				t.Errorf("error = %v, want ErrNoFrontmatter", err)
			}
			if !reflect.DeepEqual(got, contract.Contract{}) {
				t.Errorf("Parse returned %#v with an error, want the zero value", got)
			}
		})
	}
	const want = "contract file has no YAML frontmatter (expected content between --- delimiters)"
	if _, err := contract.Parse("x"); err == nil || err.Error() != want {
		t.Errorf("error text = %v, want %q", err, want)
	}
}

func TestParseRefusesAContractThatAppliesToNothing(t *testing.T) {
	for name, content := range map[string]string{
		"no key":                  doc("excluded_phases: [sdd-propose]"),
		"an empty list":           doc("applies_to_phases: []"),
		"only empty items":        doc("applies_to_phases: [ , ]"),
		"a later empty list wins": doc("applies_to_phases: [sdd-apply]", "applies_to_phases: []"),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := contract.Parse(content)
			if !errors.Is(err, contract.ErrNoAppliesTo) {
				t.Errorf("error = %v, want ErrNoAppliesTo", err)
			}
			if !reflect.DeepEqual(got, contract.Contract{}) {
				t.Errorf("Parse returned %#v with an error, want the zero value", got)
			}
		})
	}
	const want = "contract frontmatter missing or empty applies_to_phases: cannot derive scope without knowing which phases to inject into"
	if _, err := contract.Parse(doc("excluded_phases: [x]")); err == nil || err.Error() != want {
		t.Errorf("error text = %v, want %q", err, want)
	}
}

// Every list is read by the one strict parser: it is an inline list or it is refused, and the
// refusal names the key and what was there. A parser that took "sdd-tasks, sdd-apply" as two
// phases, or "[sdd-tasks" as one, made a contract mean what its author did not write.
func TestParseRefusesAListThatIsNotAnInlineList(t *testing.T) {
	scope := func(content string) error { _, err := contract.Parse(content); return err }
	context := func(content string) error { _, err := contract.ParseContext(content); return err }
	for key, parse := range map[string]func(string) error{
		"applies_to_phases": scope, "excluded_phases": scope, "language_context": context, "activation_context": context,
	} {
		for _, value := range []string{
			"sdd-tasks, sdd-apply",
			"sdd-tasks",
			"[sdd-tasks, sdd-apply",
			"sdd-tasks, sdd-apply]",
			"",
			"(sdd-tasks)",
			"[sdd-tasks] # the phases",
			"[sdd-tasks][sdd-apply]",
			"[[sdd-tasks, sdd-apply]]",
			"- sdd-tasks",
			// A bracket inside a quoted item is a bracket among the items: the strict list has
			// no way to write one, and no phase or language has one in its name.
			`["sdd[1]"]`,
			`['sdd]', "sdd-apply"]`,
		} {
			t.Run(key+"="+value, func(t *testing.T) {
				err := parse(doc("applies_to_phases: [sdd-apply]", key+": "+value))
				var malformed *contract.MalformedListError
				if !errors.As(err, &malformed) {
					t.Fatalf("error = %v, want a *MalformedListError", err)
				}
				if malformed.Key != key || malformed.Value != strings.TrimSpace(value) {
					t.Errorf("error = %+v, want key %q and value %q", *malformed, key, strings.TrimSpace(value))
				}
				wantText := "malformed " + key + ": expected an inline list such as [a, b], got " + strconv.Quote(strings.TrimSpace(value))
				if err.Error() != wantText {
					t.Errorf("error text = %q, want %q", err, wantText)
				}
			})
		}
	}
}

// A later key does not hide an earlier one that is wrong: every line is read.
func TestParseRefusesAMalformedListEvenWhenALaterLineIsFine(t *testing.T) {
	_, err := contract.Parse(doc("excluded_phases: sdd-propose", "excluded_phases: [sdd-propose]", "applies_to_phases: [sdd-apply]"))
	var malformed *contract.MalformedListError
	if !errors.As(err, &malformed) || malformed.Key != "excluded_phases" {
		t.Errorf("error = %v, want a *MalformedListError for excluded_phases", err)
	}
}

func TestParseContextRefusesAContextOperator(t *testing.T) {
	got, err := contract.ParseContext(doc("language_context: [go]", "context_operator: prompt_contains"))
	if !errors.Is(err, contract.ErrUnsupportedContextOperator) {
		t.Errorf("error = %v, want ErrUnsupportedContextOperator", err)
	}
	if !reflect.DeepEqual(got, contract.Context{}) {
		t.Errorf("ParseContext returned %#v with an error, want the zero value", got)
	}
	if err != nil && err.Error() != "unsupported context_operator" {
		t.Errorf("error text = %q, want %q", err, "unsupported context_operator")
	}
}

// The context lists keep the case they were written in: matching them against the work is the
// caller's, and a reader that folded the case would change what a runtime is configured with.
func TestParseContextKeepsTheCaseAndStripsTheQuotes(t *testing.T) {
	got, err := contract.ParseContext(doc(`language_context: ["TypeScript", 'NestJS']`, "activation_context: [ Review , ]"))
	if err != nil {
		t.Fatalf("ParseContext: %v", err)
	}
	if !reflect.DeepEqual(got.LanguageContext, []string{"TypeScript", "NestJS"}) || !reflect.DeepEqual(got.ActivationContext, []string{"Review"}) {
		t.Errorf("context = %v and %v, want the case kept and the quotes stripped", got.LanguageContext, got.ActivationContext)
	}
	if !got.ContextRequired() {
		t.Error("ContextRequired() = false for a contract with context metadata")
	}
}

// Either context list alone makes the contract depend on the work.
func TestEitherContextListRequiresContext(t *testing.T) {
	for _, line := range []string{"language_context: [go]", "activation_context: [review]"} {
		got, err := contract.ParseContext(doc(line))
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		if !got.ContextRequired() {
			t.Errorf("%s: ContextRequired() = false, want true", line)
		}
	}
}

func TestEmptyContextListsRequireNothing(t *testing.T) {
	got, err := contract.ParseContext(doc("language_context: []", "activation_context: []"))
	if err != nil {
		t.Fatalf("ParseContext: %v", err)
	}
	if got.ContextRequired() {
		t.Error("ContextRequired() = true for empty context lists")
	}
}

// ParseContext needs a frontmatter and no phase scope.
func TestParseContextNeedsAFrontmatterAndNothingElse(t *testing.T) {
	if got, err := contract.ParseContext("no frontmatter here"); !errors.Is(err, contract.ErrNoFrontmatter) || !reflect.DeepEqual(got, contract.Context{}) {
		t.Errorf("ParseContext = %#v, %v, want the zero value and ErrNoFrontmatter", got, err)
	}
	if got, err := contract.ParseContext(doc("excluded_phases: sdd-propose", "language_context", "context_operator")); err != nil || got.ContextRequired() {
		t.Errorf("ParseContext of a document with no context = %#v, %v, want an empty context and no error", got, err)
	}
}

// ParseBoth is Parse and ParseContext over one split of the frontmatter: for every document
// it returns what the two return, and the error of the scope first, then the context's.
func TestParseBothIsParseAndParseContext(t *testing.T) {
	documents := map[string]string{
		"both":                  doc("applies_to_phases: [sdd-apply]", "excluded_phases: [sdd-propose]", "language_context: [go]", "activation_context: [review]"),
		"scope only":            doc("applies_to_phases: [sdd-apply]"),
		"no frontmatter":        "no frontmatter here",
		"no scope":              doc("language_context: [go]"),
		"malformed scope":       doc("applies_to_phases: sdd-apply", "language_context: [go"),
		"malformed context":     doc("applies_to_phases: [sdd-apply]", "language_context: go"),
		"an operator":           doc("applies_to_phases: [sdd-apply]", "context_operator: prompt_contains"),
		"both malformed":        doc("applies_to_phases: sdd-apply", "language_context: go"),
		"no scope and operator": doc("context_operator: prompt_contains"),
	}
	for name, content := range documents {
		t.Run(name, func(t *testing.T) {
			gotScope, gotContext, gotErr := contract.ParseBoth(content)
			wantScope, scopeErr := contract.Parse(content)
			wantContext, contextErr := contract.ParseContext(content)
			wantErr := scopeErr
			if wantErr == nil {
				wantErr = contextErr
			}
			if !reflect.DeepEqual(gotErr, wantErr) {
				t.Fatalf("ParseBoth error = %v, want %v", gotErr, wantErr)
			}
			if wantErr != nil {
				wantScope, wantContext = contract.Contract{}, contract.Context{}
			}
			if !reflect.DeepEqual(gotScope, wantScope) || !reflect.DeepEqual(gotContext, wantContext) {
				t.Errorf("ParseBoth = %#v and %#v, want %#v and %#v", gotScope, gotContext, wantScope, wantContext)
			}
		})
	}
}

func TestPhaseMembership(t *testing.T) {
	c := contract.Contract{AppliesTo: []string{"sdd-tasks", "sdd-apply"}, Excluded: []string{"sdd-propose"}}
	for phase, wantApplies := range map[string]bool{"sdd-tasks": true, "sdd-apply": true, "sdd-propose": false, "sdd-explore": false, "": false, "SDD-APPLY": false} {
		if got := c.AppliesToPhase(phase); got != wantApplies {
			t.Errorf("AppliesToPhase(%q) = %v, want %v", phase, got, wantApplies)
		}
	}
	for phase, wantExcludes := range map[string]bool{"sdd-propose": true, "sdd-apply": false, "": false} {
		if got := c.ExcludesPhase(phase); got != wantExcludes {
			t.Errorf("ExcludesPhase(%q) = %v, want %v", phase, got, wantExcludes)
		}
	}
}
