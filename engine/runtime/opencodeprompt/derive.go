package opencodeprompt

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
)

// MalformedContractError says that the text of the contract at Path does not parse. It names the
// file, so a person can open it, and wraps the parse error, so a caller can still tell what is
// wrong with it.
//
// Every contract that does not parse ends Derive with this error, the optional one too. That is
// the owner's decision of 2026-10-09 (R3-opencode-secondary-contract-silent-drop, deferred to H26
// in batch 8). Before it, an optional contract that did not parse was dropped without a word, so a
// plugin could ship without its oo-quality guard and nothing said why. No branch decides anything
// now: the two places that parse a contract return this error, and a different decision is a
// change to those two places and to the tests in derive_test.go.
type MalformedContractError struct {
	// Path is the contract as the prompt config knows it (skills/_shared/...).
	Path string
	// Err is what the contract parser said.
	Err error
}

func (e *MalformedContractError) Error() string { return e.Path + ": " + e.Err.Error() }

// Unwrap lets errors.Is and errors.As reach the parse error.
func (e *MalformedContractError) Unwrap() error { return e.Err }

// Derive builds the prompt config from the contracts the source gives: the minimalism contract,
// which fills the top-level fields and is unconditional in OpenCode (it has no context to hand
// the plugin), then the anti-generic-design guard, then the oo-quality contract when the overlay
// has one. The unconditional contracts keep that order whatever the optional file holds.
//
// The source is asked in that order and no further after a failure. A source that cannot read a
// contract fails Derive with its own error; a contract that cannot be parsed fails it with a
// *MalformedContractError, whichever contract it is.
func Derive(source ContractSource) (PromptConfig, error) {
	text, err := source.Minimalism()
	if err != nil {
		return PromptConfig{}, err
	}
	doc, err := contract.Parse(text)
	if err != nil {
		return PromptConfig{}, &MalformedContractError{Path: MinimalismContractPath, Err: err}
	}
	minimalism := entryFor(MinimalismContractPath, doc, contract.Context{})
	contracts := []ContractConfig{minimalism}

	contracts, err = withContract(contracts, AntiGenericDesignPath, source.AntiGenericDesign())
	if err != nil {
		return PromptConfig{}, err
	}
	ooText, present, err := source.OOQuality()
	if err != nil {
		return PromptConfig{}, err
	}
	if present {
		contracts, err = withContract(contracts, OOQualityContractPath, ooText)
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

// withContract adds the entry for the contract at path to contracts, or fails when its text does
// not parse.
func withContract(contracts []ContractConfig, path, text string) ([]ContractConfig, error) {
	doc, needs, err := contract.ParseBoth(text)
	if err != nil {
		return nil, &MalformedContractError{Path: path, Err: err}
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
