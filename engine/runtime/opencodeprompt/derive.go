package opencodeprompt

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
)

// contractRole says what a contract is to the plugin, which decides what happens when its text
// does not parse.
type contractRole int

const (
	// requiredContract is one the plugin is not complete without: the anti-generic-design guard,
	// which the program embeds, and, outside onMalformed, the minimalism contract.
	requiredContract contractRole = iota
	// optionalContract is one the overlay may or may not ship: the oo-quality contract.
	optionalContract
)

// onMalformed is what the loader does with a contract whose frontmatter does not parse, and the
// one place that says so. It returns the error that abandons the whole prompt config, or nil to
// leave the contract out and go on.
//
// As it stands, a required contract abandons the config and an optional one is dropped without a
// word, so a plugin can ship without its oo-quality guard and nothing says why. (The minimalism
// contract is outside this function: the top-level fields are its, so Derive cannot go on without
// it.) That asymmetry is carried over from the loader as it was; whether it should stay is the owner's decision
// (R3-opencode-secondary-contract-silent-drop, deferred to H26 in batch 8), and taking it is a
// change to this function and to the cases in policy_internal_test.go, nothing else.
func onMalformed(role contractRole, parseErr error) error {
	if role == requiredContract {
		return parseErr
	}
	return nil
}

// Derive builds the prompt config from the contracts the source gives: the minimalism contract,
// which fills the top-level fields and is unconditional in OpenCode (it has no context to hand
// the plugin), then the anti-generic-design guard, then the oo-quality contract when the overlay
// has one. The unconditional contracts keep that order whatever the optional file holds.
//
// The source is asked in that order and no further after a failure. A source that cannot read a
// contract fails Derive with its own error; a contract that cannot be parsed follows onMalformed.
func Derive(source ContractSource) (PromptConfig, error) {
	text, err := source.Minimalism()
	if err != nil {
		return PromptConfig{}, err
	}
	// The minimalism contract fills the top-level fields, so there is no config to build without
	// it: a malformed one ends Derive whatever onMalformed says of the others.
	doc, err := contract.Parse(text)
	if err != nil {
		return PromptConfig{}, err
	}
	minimalism := entryFor(MinimalismContractPath, doc, contract.Context{})
	contracts := []ContractConfig{minimalism}

	contracts, err = withContract(contracts, requiredContract, AntiGenericDesignPath, source.AntiGenericDesign())
	if err != nil {
		return PromptConfig{}, err
	}
	ooText, present, err := source.OOQuality()
	if err != nil {
		return PromptConfig{}, err
	}
	if present {
		contracts, err = withContract(contracts, optionalContract, OOQualityContractPath, ooText)
		if err != nil {
			return PromptConfig{}, err
		}
	}
	return PromptConfig{
		ContractPath:   minimalism.ContractPath,
		IncludedPhases: minimalism.IncludedPhases,
		ExcludedPhases: minimalism.ExcludedPhases,
		InjectionPoint: minimalism.InjectionPoint,
		Contracts:      contracts,
	}, nil
}

// withContract adds the entry for the contract at path to contracts, or leaves it out when its
// text does not parse and onMalformed says to go on.
func withContract(contracts []ContractConfig, role contractRole, path, text string) ([]ContractConfig, error) {
	doc, needs, err := contract.ParseBoth(text)
	if err != nil {
		if abort := onMalformed(role, err); abort != nil {
			return nil, abort
		}
		return contracts, nil
	}
	return append(contracts, entryFor(path, doc, needs)), nil
}

// entryFor is the plugin's entry for the contract at path: its scope, its injection line and the
// context it needs.
func entryFor(path string, doc contract.Contract, needs contract.Context) ContractConfig {
	return ContractConfig{
		ContractPath:      path,
		IncludedPhases:    doc.AppliesTo,
		ExcludedPhases:    doc.Excluded,
		InjectionPoint:    doc.Header(),
		LanguageContext:   needs.LanguageContext,
		ActivationContext: needs.ActivationContext,
		ContextOperator:   nil,
	}
}
