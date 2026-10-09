package opencodeprompt

// The paths the contracts are known by in a PromptConfig, relative to the overlay and always
// written with forward slashes: they are names the plugin matches, not locations on a disk.
const (
	MinimalismContractPath = "skills/_shared/minimalism-contract.md"
	AntiGenericDesignPath  = "skills/_shared/anti-generic-design.md"
	OOQualityContractPath  = "skills/_shared/oo-quality-contract.md"
)

// ContractSource is where Derive gets the text of the contracts. The adapter implements it over
// the overlay on disk and the guard the program embeds; a test implements it over strings. Derive
// asks in the order the methods are listed and stops at the first failure, so a source is never
// asked for a contract after a required one has failed.
type ContractSource interface {
	// Minimalism is the text of the minimalism contract. A source that cannot give it returns the
	// error, which Derive returns as it came.
	Minimalism() (string, error)
	// AntiGenericDesign is the text of the anti-generic-design guard, which the program carries
	// itself and so can always give.
	AntiGenericDesign() string
	// OOQuality is the text of the oo-quality contract. present is false when the overlay has
	// none, which is not an error; an error means it has one and could not read it.
	OOQuality() (text string, present bool, err error)
}
