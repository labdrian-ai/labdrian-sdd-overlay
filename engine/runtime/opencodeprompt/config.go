package opencodeprompt

import "encoding/json"

// PromptConfig is what the plugin is told about the prompt: the top-level fields are those of the
// minimalism contract, kept for the plugin versions that read only them, and Contracts lists every
// contract the plugin injects, the minimalism one first.
type PromptConfig struct {
	ContractPath           string           `json:"contract_path"`
	IncludedPhases         []string         `json:"included_phases"`
	ExcludedPhases         []string         `json:"excluded_phases"`
	InjectionPoint         string           `json:"injection_point"`
	LanguageContext        []string         `json:"language_context,omitempty"`
	ActivationContext      []string         `json:"activation_context,omitempty"`
	ContextOperator        *string          `json:"context_operator,omitempty"`
	ContextOperatorPresent bool             `json:"-"`
	Contracts              []ContractConfig `json:"contracts,omitempty"`
}

// ContractConfig is the plugin's entry for one contract: the phases it applies to and excludes,
// the line it is injected under, and the context it needs. ContextOperator is carried as the file
// has it, absent, null or a string, which ContextOperatorPresent tells apart.
type ContractConfig struct {
	ContractPath           string   `json:"contract_path"`
	IncludedPhases         []string `json:"included_phases"`
	ExcludedPhases         []string `json:"excluded_phases"`
	InjectionPoint         string   `json:"injection_point"`
	LanguageContext        []string `json:"language_context,omitempty"`
	ActivationContext      []string `json:"activation_context,omitempty"`
	ContextOperator        *string  `json:"context_operator,omitempty"`
	ContextOperatorPresent bool     `json:"-"`
}

// MarshalJSON writes the config as the plugin reads it: context_operator appears only when the
// config has one, and as null when it is present and empty.
func (c PromptConfig) MarshalJSON() ([]byte, error) {
	contextOperator, err := marshalContextOperator(c.ContextOperator, c.ContextOperatorPresent)
	if err != nil {
		return nil, err
	}
	type promptConfigJSON struct {
		ContractPath      string           `json:"contract_path"`
		IncludedPhases    []string         `json:"included_phases"`
		ExcludedPhases    []string         `json:"excluded_phases"`
		InjectionPoint    string           `json:"injection_point"`
		LanguageContext   []string         `json:"language_context,omitempty"`
		ActivationContext []string         `json:"activation_context,omitempty"`
		ContextOperator   json.RawMessage  `json:"context_operator,omitempty"`
		Contracts         []ContractConfig `json:"contracts,omitempty"`
	}
	return json.Marshal(promptConfigJSON{
		ContractPath:      c.ContractPath,
		IncludedPhases:    c.IncludedPhases,
		ExcludedPhases:    c.ExcludedPhases,
		InjectionPoint:    c.InjectionPoint,
		LanguageContext:   c.LanguageContext,
		ActivationContext: c.ActivationContext,
		ContextOperator:   contextOperator,
		Contracts:         c.Contracts,
	})
}

// UnmarshalJSON reads what MarshalJSON wrote, and records whether context_operator was there.
func (c *PromptConfig) UnmarshalJSON(data []byte) error {
	type promptConfigJSON struct {
		ContractPath      string           `json:"contract_path"`
		IncludedPhases    []string         `json:"included_phases"`
		ExcludedPhases    []string         `json:"excluded_phases"`
		InjectionPoint    string           `json:"injection_point"`
		LanguageContext   []string         `json:"language_context,omitempty"`
		ActivationContext []string         `json:"activation_context,omitempty"`
		ContextOperator   *string          `json:"context_operator,omitempty"`
		Contracts         []ContractConfig `json:"contracts,omitempty"`
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var decoded promptConfigJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = PromptConfig{
		ContractPath:           decoded.ContractPath,
		IncludedPhases:         decoded.IncludedPhases,
		ExcludedPhases:         decoded.ExcludedPhases,
		InjectionPoint:         decoded.InjectionPoint,
		LanguageContext:        decoded.LanguageContext,
		ActivationContext:      decoded.ActivationContext,
		ContextOperator:        decoded.ContextOperator,
		ContextOperatorPresent: hasJSONKey(raw, "context_operator"),
		Contracts:              decoded.Contracts,
	}
	return nil
}

// MarshalJSON writes the entry like PromptConfig writes its own fields.
func (c ContractConfig) MarshalJSON() ([]byte, error) {
	contextOperator, err := marshalContextOperator(c.ContextOperator, c.ContextOperatorPresent)
	if err != nil {
		return nil, err
	}
	type contractConfigJSON struct {
		ContractPath      string          `json:"contract_path"`
		IncludedPhases    []string        `json:"included_phases"`
		ExcludedPhases    []string        `json:"excluded_phases"`
		InjectionPoint    string          `json:"injection_point"`
		LanguageContext   []string        `json:"language_context,omitempty"`
		ActivationContext []string        `json:"activation_context,omitempty"`
		ContextOperator   json.RawMessage `json:"context_operator,omitempty"`
	}
	return json.Marshal(contractConfigJSON{
		ContractPath:      c.ContractPath,
		IncludedPhases:    c.IncludedPhases,
		ExcludedPhases:    c.ExcludedPhases,
		InjectionPoint:    c.InjectionPoint,
		LanguageContext:   c.LanguageContext,
		ActivationContext: c.ActivationContext,
		ContextOperator:   contextOperator,
	})
}

// UnmarshalJSON reads what MarshalJSON wrote.
func (c *ContractConfig) UnmarshalJSON(data []byte) error {
	type contractConfigJSON struct {
		ContractPath      string   `json:"contract_path"`
		IncludedPhases    []string `json:"included_phases"`
		ExcludedPhases    []string `json:"excluded_phases"`
		InjectionPoint    string   `json:"injection_point"`
		LanguageContext   []string `json:"language_context,omitempty"`
		ActivationContext []string `json:"activation_context,omitempty"`
		ContextOperator   *string  `json:"context_operator,omitempty"`
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var decoded contractConfigJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = ContractConfig{
		ContractPath:           decoded.ContractPath,
		IncludedPhases:         decoded.IncludedPhases,
		ExcludedPhases:         decoded.ExcludedPhases,
		InjectionPoint:         decoded.InjectionPoint,
		LanguageContext:        decoded.LanguageContext,
		ActivationContext:      decoded.ActivationContext,
		ContextOperator:        decoded.ContextOperator,
		ContextOperatorPresent: hasJSONKey(raw, "context_operator"),
	}
	return nil
}

func marshalContextOperator(value *string, present bool) (json.RawMessage, error) {
	if !present {
		return nil, nil
	}
	if value == nil {
		return json.RawMessage("null"), nil
	}
	encoded, err := json.Marshal(*value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func hasJSONKey(raw map[string]json.RawMessage, key string) bool {
	_, ok := raw[key]
	return ok
}
