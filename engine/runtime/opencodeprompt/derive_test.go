package opencodeprompt_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/opencodeprompt"
)

const minimalismText = `---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-verify]
injection_point: "## Skills to load before work"
language_context: [ignored-for-minimalism]
---
# Minimalism
`

const antiGenericText = `---
applies_to_phases: [sdd-design]
excluded_phases: [sdd-archive]
injection_point: "## Design guard"
language_context: [css]
activation_context: [ui]
---
# Anti-generic design
`

const ooQualityText = `---
applies_to_phases: [sdd-apply]
excluded_phases: [sdd-verify]
injection_point: "## Skills to load before work"
language_context: [typescript]
activation_context: [review]
---
# OO quality
`

// malformedText has no frontmatter, which contract.Parse refuses.
const malformedText = "# a contract with no frontmatter at all\n"

// fakeSource is a ContractSource that answers from fields and records which questions it was asked,
// in order, so a test can say what the loader asked and what it never reached.
type fakeSource struct {
	minimalism    string
	minimalismErr error
	antiGeneric   string
	ooQuality     string
	ooPresent     bool
	ooErr         error

	asked []string
}

func (f *fakeSource) Minimalism() (string, error) {
	f.asked = append(f.asked, "minimalism")
	return f.minimalism, f.minimalismErr
}

func (f *fakeSource) AntiGenericDesign() string {
	f.asked = append(f.asked, "anti-generic-design")
	return f.antiGeneric
}

func (f *fakeSource) OOQuality() (string, bool, error) {
	f.asked = append(f.asked, "oo-quality")
	return f.ooQuality, f.ooPresent, f.ooErr
}

func fullSource() *fakeSource {
	return &fakeSource{minimalism: minimalismText, antiGeneric: antiGenericText, ooQuality: ooQualityText, ooPresent: true}
}

func paths(config opencodeprompt.PromptConfig) []string {
	var out []string
	for _, c := range config.Contracts {
		out = append(out, c.ContractPath)
	}
	return out
}

func TestDeriveReadsTheThreeContractsInOrder(t *testing.T) {
	source := fullSource()
	config, err := opencodeprompt.Derive(source)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	wantPaths := []string{
		"skills/_shared/minimalism-contract.md",
		"skills/_shared/anti-generic-design.md",
		"skills/_shared/oo-quality-contract.md",
	}
	if got := paths(config); !reflect.DeepEqual(got, wantPaths) {
		t.Errorf("contracts = %v, want %v: the unconditional contracts first, the optional one last", got, wantPaths)
	}
	if want := []string{"minimalism", "anti-generic-design", "oo-quality"}; !reflect.DeepEqual(source.asked, want) {
		t.Errorf("the source was asked %v, want %v", source.asked, want)
	}
}

func TestDeriveTakesTheTopLevelFieldsFromTheMinimalismContract(t *testing.T) {
	config, err := opencodeprompt.Derive(fullSource())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if config.ContractPath != "skills/_shared/minimalism-contract.md" ||
		!reflect.DeepEqual(config.IncludedPhases, []string{"sdd-tasks", "sdd-apply"}) ||
		!reflect.DeepEqual(config.ExcludedPhases, []string{"sdd-propose", "sdd-verify"}) ||
		config.InjectionPoint != "## Skills to load before work" {
		t.Errorf("top level = %+v, want the minimalism contract's path, phases and injection point", config)
	}
	if config.LanguageContext != nil || config.ActivationContext != nil || config.ContextOperator != nil || config.ContextOperatorPresent {
		t.Errorf("top level carries a context (%+v): the minimalism contract is unconditional", config)
	}
	minimalism := config.Contracts[0]
	if minimalism.LanguageContext != nil || minimalism.ActivationContext != nil {
		t.Errorf("the minimalism entry carries the context %v / %v although it is unconditional in OpenCode", minimalism.LanguageContext, minimalism.ActivationContext)
	}
}

func TestDeriveGivesTheOtherContractsTheirContext(t *testing.T) {
	config, err := opencodeprompt.Derive(fullSource())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	design, oo := config.Contracts[1], config.Contracts[2]
	if !reflect.DeepEqual(design.LanguageContext, []string{"css"}) || !reflect.DeepEqual(design.ActivationContext, []string{"ui"}) {
		t.Errorf("anti-generic entry context = %v / %v, want [css] / [ui]", design.LanguageContext, design.ActivationContext)
	}
	if !reflect.DeepEqual(oo.IncludedPhases, []string{"sdd-apply"}) || !reflect.DeepEqual(oo.LanguageContext, []string{"typescript"}) || !reflect.DeepEqual(oo.ActivationContext, []string{"review"}) {
		t.Errorf("oo-quality entry = %+v, want its own phases and context", oo)
	}
	if design.InjectionPoint != "## Design guard" {
		t.Errorf("anti-generic injection point = %q, want the one its contract declares", design.InjectionPoint)
	}
}

func TestDeriveWithoutTheOptionalContractHasTwo(t *testing.T) {
	source := fullSource()
	source.ooPresent = false
	source.ooQuality = ""
	config, err := opencodeprompt.Derive(source)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if got := len(config.Contracts); got != 2 {
		t.Errorf("%d contracts (%v), want the two unconditional ones", got, paths(config))
	}
}

// ---- a malformed contract ----
//
// The owner decided (2026-10-09, H26) that a contract whose frontmatter does not parse aborts the
// whole prompt config, whichever contract it is, and that the error names the file. Until then an
// optional contract was dropped in silence, and the config was built without it.

func TestAMalformedContractAbortsTheWholePromptConfigAndNamesTheFile(t *testing.T) {
	cases := map[string]struct {
		spoil func(*fakeSource)
		path  string
	}{
		"the minimalism contract": {func(s *fakeSource) { s.minimalism = malformedText }, "skills/_shared/minimalism-contract.md"},
		"the anti-generic guard":  {func(s *fakeSource) { s.antiGeneric = malformedText }, "skills/_shared/anti-generic-design.md"},
		"the oo-quality contract": {func(s *fakeSource) { s.ooQuality = malformedText }, "skills/_shared/oo-quality-contract.md"},
		"a minimalism contract with a malformed list": {func(s *fakeSource) {
			s.minimalism = "---\napplies_to_phases: [a][b]\n---\n"
		}, "skills/_shared/minimalism-contract.md"},
		"an oo-quality contract with a malformed context list": {func(s *fakeSource) {
			s.ooQuality = "---\napplies_to_phases: [sdd-apply]\nlanguage_context: typescript\n---\n"
		}, "skills/_shared/oo-quality-contract.md"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			source := fullSource()
			tc.spoil(source)
			config, err := opencodeprompt.Derive(source)
			if err == nil {
				t.Fatalf("Derive built %+v from a malformed contract, want an error", config)
			}
			if len(config.Contracts) != 0 || config.ContractPath != "" {
				t.Errorf("an aborted Derive returned a config (%+v)", config)
			}
			var malformed *opencodeprompt.MalformedContractError
			if !errors.As(err, &malformed) || malformed.Path != tc.path {
				t.Fatalf("Derive = %v, want a *MalformedContractError for %s", err, tc.path)
			}
			if !strings.HasPrefix(err.Error(), tc.path+": ") {
				t.Errorf("message = %q, want it to begin with the file, %q", err.Error(), tc.path+": ")
			}
			if malformed.Err == nil || !strings.HasSuffix(err.Error(), malformed.Err.Error()) {
				t.Errorf("message = %q, want it to end with the parse error (%v)", err.Error(), malformed.Err)
			}
		})
	}
}

// The parse error stays reachable behind the file name, so a caller can still tell a list that is
// malformed from a frontmatter that is missing.
func TestTheParseErrorOfAMalformedContractIsStillReachable(t *testing.T) {
	source := fullSource()
	source.ooQuality = "---\napplies_to_phases: [sdd-apply]\nlanguage_context: typescript\n---\n"
	_, err := opencodeprompt.Derive(source)
	var list *contract.MalformedListError
	if !errors.As(err, &list) {
		t.Fatalf("Derive = %v, want the list error of the contract package behind the file name", err)
	}
}

// Dropping applies to the text that does not parse. A source that cannot read the optional
// contract is a different failure and aborts, so a broken file system is not mistaken for an
// absent contract.
func TestAnOptionalContractThatCannotBeReadAborts(t *testing.T) {
	boom := errors.New("permission denied")
	source := fullSource()
	source.ooErr = boom
	if _, err := opencodeprompt.Derive(source); !errors.Is(err, boom) {
		t.Fatalf("Derive = %v, want the read error", err)
	}
}

func TestAMinimalismContractThatCannotBeReadAbortsBeforeTheOthersAreAsked(t *testing.T) {
	boom := errors.New("open /overlay/skills/_shared/minimalism-contract.md: no such file or directory")
	source := fullSource()
	source.minimalismErr = boom
	_, err := opencodeprompt.Derive(source)
	if !errors.Is(err, boom) || err.Error() != boom.Error() {
		t.Fatalf("Derive = %v, want the read error as it came, unwrapped", err)
	}
	if want := []string{"minimalism"}; !reflect.DeepEqual(source.asked, want) {
		t.Errorf("the source was asked %v, want only %v: nothing after a failed required read", source.asked, want)
	}
}

// A malformed required contract is reported before an unreadable optional one is even asked
// for, which is the order the loader has always had: the message a person sees does not depend
// on a file they have not touched.
func TestAMalformedRequiredContractIsReportedBeforeTheOptionalOneIsRead(t *testing.T) {
	source := fullSource()
	source.antiGeneric = malformedText
	source.ooErr = errors.New("permission denied")
	_, err := opencodeprompt.Derive(source)
	if err == nil || strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("Derive = %v, want the parse error of the required contract", err)
	}
	for _, asked := range source.asked {
		if asked == "oo-quality" {
			t.Errorf("the optional contract was read although a required one had already failed: %v", source.asked)
		}
	}
}
