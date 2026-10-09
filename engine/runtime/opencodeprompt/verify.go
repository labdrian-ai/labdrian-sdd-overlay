package opencodeprompt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// Hash fingerprints the config: the hex SHA-256 of the JSON it is written as, or "" for a config
// that cannot be written.
func Hash(config PromptConfig) string {
	data, err := json.Marshal(config)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// MismatchError says that a recorded prompt config is not the current one: it is stale, or it was
// edited. It is not a failure to read or derive a config, which a caller reports differently.
type MismatchError struct {
	Err error
}

func (e MismatchError) Error() string { return e.Err.Error() }

// Unwrap lets errors.Is and errors.As see the field that differs.
func (e MismatchError) Unwrap() error { return e.Err }

// IsMismatch reports whether err is, or wraps, a MismatchError.
func IsMismatch(err error) bool {
	var mismatch MismatchError
	return errors.As(err, &mismatch)
}

// Verify says whether recorded, with the hash that was recorded beside it, is the current config.
// It returns a MismatchError naming the first thing that differs, the hash last, and nil when
// nothing does.
func Verify(recorded PromptConfig, recordedHash string, current PromptConfig) error {
	if err := validate(recorded, current); err != nil {
		return MismatchError{Err: err}
	}
	if expected := Hash(current); recordedHash != expected {
		return MismatchError{Err: fmt.Errorf("prompt_config_hash %q is not current %q", recordedHash, expected)}
	}
	return nil
}

// validate compares the fields of got with those of want in the order a person would expect them
// reported, and names the first that differs.
func validate(got, want PromptConfig) error {
	if got.ContractPath == "" {
		return fmt.Errorf("prompt_config.contract_path is empty")
	}
	if got.InjectionPoint == "" {
		return fmt.Errorf("prompt_config.injection_point is empty")
	}
	if !equalStringSlices(got.IncludedPhases, want.IncludedPhases) {
		return fmt.Errorf("prompt_config.included_phases %v is not current %v", got.IncludedPhases, want.IncludedPhases)
	}
	if !equalStringSlices(got.ExcludedPhases, want.ExcludedPhases) {
		return fmt.Errorf("prompt_config.excluded_phases %v is not current %v", got.ExcludedPhases, want.ExcludedPhases)
	}
	if got.ContractPath != want.ContractPath {
		return fmt.Errorf("prompt_config.contract_path %q is not current %q", got.ContractPath, want.ContractPath)
	}
	if got.InjectionPoint != want.InjectionPoint {
		return fmt.Errorf("prompt_config.injection_point %q is not current %q", got.InjectionPoint, want.InjectionPoint)
	}
	if !equalStringSlices(got.LanguageContext, want.LanguageContext) {
		return fmt.Errorf("prompt_config.language_context %v is not current %v", got.LanguageContext, want.LanguageContext)
	}
	if !equalStringSlices(got.ActivationContext, want.ActivationContext) {
		return fmt.Errorf("prompt_config.activation_context %v is not current %v", got.ActivationContext, want.ActivationContext)
	}
	if !equalOptionalString(got.ContextOperator, got.ContextOperatorPresent, want.ContextOperator, want.ContextOperatorPresent) {
		return fmt.Errorf("prompt_config.context_operator %s is not current %s", formatOptionalString(got.ContextOperator, got.ContextOperatorPresent), formatOptionalString(want.ContextOperator, want.ContextOperatorPresent))
	}
	if !equalContractConfigs(got.Contracts, want.Contracts) {
		return fmt.Errorf("prompt_config.contracts is not current")
	}
	return nil
}

func equalContractConfigs(a, b []ContractConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ContractPath != b[i].ContractPath || a[i].InjectionPoint != b[i].InjectionPoint ||
			!equalStringSlices(a[i].IncludedPhases, b[i].IncludedPhases) ||
			!equalStringSlices(a[i].ExcludedPhases, b[i].ExcludedPhases) ||
			!equalStringSlices(a[i].LanguageContext, b[i].LanguageContext) ||
			!equalStringSlices(a[i].ActivationContext, b[i].ActivationContext) ||
			!equalOptionalString(a[i].ContextOperator, a[i].ContextOperatorPresent, b[i].ContextOperator, b[i].ContextOperatorPresent) {
			return false
		}
	}
	return true
}

func equalOptionalString(a *string, aPresent bool, b *string, bPresent bool) bool {
	if aPresent != bPresent {
		return false
	}
	if !aPresent {
		return true
	}
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func formatOptionalString(value *string, present bool) string {
	if !present {
		return "<absent>"
	}
	if value == nil {
		return "null"
	}
	return fmt.Sprintf("%q", *value)
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
