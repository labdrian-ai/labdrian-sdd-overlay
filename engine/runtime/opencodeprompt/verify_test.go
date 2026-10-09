package opencodeprompt_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/opencodeprompt"
)

// mustHash is the hash of a config that can be written.
func mustHash(t *testing.T, config opencodeprompt.PromptConfig) string {
	t.Helper()
	hash, err := opencodeprompt.Hash(config)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	return hash
}

func derived(t *testing.T) opencodeprompt.PromptConfig {
	t.Helper()
	config, err := opencodeprompt.Derive(fullSource())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	return config
}

// reload is what the adapter does with a config it recorded: write it and read it back.
func reload(t *testing.T, config opencodeprompt.PromptConfig) opencodeprompt.PromptConfig {
	t.Helper()
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var back opencodeprompt.PromptConfig
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	return back
}

func TestHashIsTheSHA256OfTheJSONTheConfigIsWrittenAs(t *testing.T) {
	config := derived(t)
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got, want := mustHash(t, config), hex.EncodeToString(sum[:]); got != want {
		t.Errorf("Hash = %q, want %q", got, want)
	}
}

func TestHashChangesWithAnyContract(t *testing.T) {
	a := derived(t)
	b := derived(t)
	b.Contracts[2].ActivationContext = []string{"something else"}
	if mustHash(t, a) == mustHash(t, b) {
		t.Error("two configs that differ in a contract have the same hash")
	}
}

func TestACurrentConfigVerifiesAfterAWriteAndARead(t *testing.T) {
	current := derived(t)
	if err := opencodeprompt.Verify(reload(t, current), mustHash(t, current), current); err != nil {
		t.Fatalf("Verify of the config as recorded = %v, want nil", err)
	}
}

func TestAStaleConfigIsAMismatchThatNamesTheField(t *testing.T) {
	current := derived(t)
	hash := mustHash(t, current)
	str := func(s string) *string { return &s }
	cases := map[string]struct {
		change func(*opencodeprompt.PromptConfig)
		want   string
	}{
		"an empty contract path":                  {func(c *opencodeprompt.PromptConfig) { c.ContractPath = "" }, "prompt_config.contract_path is empty"},
		"an empty injection point":                {func(c *opencodeprompt.PromptConfig) { c.InjectionPoint = "" }, "prompt_config.injection_point is empty"},
		"other included phases":                   {func(c *opencodeprompt.PromptConfig) { c.IncludedPhases = []string{"sdd-apply"} }, "prompt_config.included_phases"},
		"other excluded phases":                   {func(c *opencodeprompt.PromptConfig) { c.ExcludedPhases = nil }, "prompt_config.excluded_phases"},
		"another contract path":                   {func(c *opencodeprompt.PromptConfig) { c.ContractPath = "skills/other.md" }, "prompt_config.contract_path"},
		"another injection point":                 {func(c *opencodeprompt.PromptConfig) { c.InjectionPoint = "## Elsewhere" }, "prompt_config.injection_point"},
		"a language context":                      {func(c *opencodeprompt.PromptConfig) { c.LanguageContext = []string{"go"} }, "prompt_config.language_context"},
		"an activation context":                   {func(c *opencodeprompt.PromptConfig) { c.ActivationContext = []string{"x"} }, "prompt_config.activation_context"},
		"a context operator":                      {func(c *opencodeprompt.PromptConfig) { c.ContextOperator, c.ContextOperatorPresent = str("or"), true }, "prompt_config.context_operator"},
		"a contract left out":                     {func(c *opencodeprompt.PromptConfig) { c.Contracts = c.Contracts[:2] }, "prompt_config.contracts"},
		"a contract path changed":                 {func(c *opencodeprompt.PromptConfig) { c.Contracts[1].ContractPath = "skills/x.md" }, "prompt_config.contracts"},
		"a contract's included phases changed":    {func(c *opencodeprompt.PromptConfig) { c.Contracts[1].IncludedPhases = nil }, "prompt_config.contracts"},
		"a contract's excluded phases changed":    {func(c *opencodeprompt.PromptConfig) { c.Contracts[2].ExcludedPhases = []string{"x"} }, "prompt_config.contracts"},
		"a contract's language context changed":   {func(c *opencodeprompt.PromptConfig) { c.Contracts[2].LanguageContext = nil }, "prompt_config.contracts"},
		"a contract's activation context changed": {func(c *opencodeprompt.PromptConfig) { c.Contracts[2].ActivationContext = nil }, "prompt_config.contracts"},
		"a contract's context operator changed": {func(c *opencodeprompt.PromptConfig) {
			c.Contracts[2].ContextOperator, c.Contracts[2].ContextOperatorPresent = str("or"), true
		}, "prompt_config.contracts"},
		"a contract changed": {func(c *opencodeprompt.PromptConfig) { c.Contracts[1].InjectionPoint = "## Elsewhere" }, "prompt_config.contracts"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			recorded := reload(t, current)
			tc.change(&recorded)
			err := opencodeprompt.Verify(recorded, hash, current)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Verify = %v, want a mismatch that says %q", err, tc.want)
			}
			if !opencodeprompt.IsMismatch(err) {
				t.Errorf("Verify = %T, want a mismatch the caller can tell from a failure to read", err)
			}
		})
	}
}

func TestARecordedHashThatIsNotCurrentIsAMismatchToo(t *testing.T) {
	current := derived(t)
	err := opencodeprompt.Verify(reload(t, current), "0000", current)
	want := `prompt_config_hash "0000" is not current "` + mustHash(t, current) + `"`
	if err == nil || err.Error() != want {
		t.Fatalf("Verify = %v, want %q", err, want)
	}
	if !opencodeprompt.IsMismatch(err) {
		t.Errorf("a stale hash is not reported as a mismatch: %T", err)
	}
}

func TestAnErrorThatIsNotAMismatchIsNotOne(t *testing.T) {
	if opencodeprompt.IsMismatch(errors.New("open: no such file")) || opencodeprompt.IsMismatch(nil) {
		t.Error("IsMismatch said yes for an error that is not a mismatch")
	}
}

// context_operator is carried as the file has it: absent, null and a string are three states.
func TestTheContextOperatorKeepsItsThreeStatesThroughTheFile(t *testing.T) {
	str := "or"
	for name, tc := range map[string]struct {
		present bool
		value   *string
		json    string
	}{
		"absent": {false, nil, ``},
		"null":   {true, nil, `"context_operator":null`},
		"string": {true, &str, `"context_operator":"or"`},
	} {
		t.Run(name, func(t *testing.T) {
			config := derived(t)
			config.ContextOperator, config.ContextOperatorPresent = tc.value, tc.present
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			// Contracts also carry the key through their own entries only when set; look at the top level.
			var top map[string]json.RawMessage
			if err := json.Unmarshal(data, &top); err != nil {
				t.Fatal(err)
			}
			raw, has := top["context_operator"]
			if has != tc.present {
				t.Fatalf("context_operator written = %v, want %v (%s)", has, tc.present, data)
			}
			if tc.json != "" && !strings.Contains(`"context_operator":`+string(raw), tc.json) {
				t.Errorf("context_operator = %s, want %s", raw, tc.json)
			}
			back := reload(t, config)
			if back.ContextOperatorPresent != tc.present || (back.ContextOperator == nil) != (tc.value == nil) {
				t.Errorf("after a round trip: present=%v value=%v, want present=%v value=%v", back.ContextOperatorPresent, back.ContextOperator, tc.present, tc.value)
			}
		})
	}
}

// A value without the flag that says it was in the file is not written: the flag decides.
func TestAContextOperatorThatWasNotInTheFileIsNotWritten(t *testing.T) {
	or := "or"
	config := derived(t)
	config.ContextOperator, config.ContextOperatorPresent = &or, false
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	if raw, has := top["context_operator"]; has {
		t.Errorf("context_operator = %s was written although the config did not have it", raw)
	}
}

// The context operator is compared by whether it was there and, when it was, by its value, with
// null a value of its own: the file written by an older plugin and the one written today differ
// in exactly these.
func TestTheContextOperatorIsComparedByPresenceAndValue(t *testing.T) {
	or, and := "or", "and"
	state := func(present bool, value *string) opencodeprompt.PromptConfig {
		config := derived(t)
		config.ContextOperator, config.ContextOperatorPresent = value, present
		return config
	}
	cases := []struct {
		name              string
		recorded, current opencodeprompt.PromptConfig
		same              bool
	}{
		{"both absent", state(false, nil), state(false, nil), true},
		{"both null", state(true, nil), state(true, nil), true},
		{"both the same string", state(true, &or), state(true, &or), true},
		{"recorded absent, current there", state(false, nil), state(true, &or), false},
		{"recorded there, current absent", state(true, &or), state(false, nil), false},
		{"null against a string", state(true, nil), state(true, &or), false},
		{"a string against null", state(true, &or), state(true, nil), false},
		{"two different strings", state(true, &or), state(true, &and), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := opencodeprompt.Verify(tc.recorded, mustHash(t, tc.current), tc.current)
			if tc.same && err != nil {
				t.Errorf("Verify = %v, want nil: the operator is the same", err)
			}
			if !tc.same && (err == nil || !strings.Contains(err.Error(), "prompt_config.context_operator")) {
				t.Errorf("Verify = %v, want a mismatch on the context operator", err)
			}
		})
	}
}

// The message shows an operator the way a person reads it: absent, null, or quoted.
func TestTheMismatchOfAContextOperatorShowsBothSides(t *testing.T) {
	or := "or"
	recorded, current := derived(t), derived(t)
	recorded.ContextOperator, recorded.ContextOperatorPresent = &or, true
	current.ContextOperatorPresent = true
	err := opencodeprompt.Verify(recorded, mustHash(t, current), current)
	want := `prompt_config.context_operator "or" is not current null`
	if err == nil || err.Error() != want {
		t.Fatalf("Verify = %v, want %q", err, want)
	}
	recorded.ContextOperatorPresent = false
	err = opencodeprompt.Verify(recorded, mustHash(t, current), current)
	if want := `prompt_config.context_operator <absent> is not current null`; err == nil || err.Error() != want {
		t.Fatalf("Verify = %v, want %q", err, want)
	}
}

// A recorded hash that is empty is not the hash of anything: it never verifies, however the
// current config looks. (Hash used to answer "" for a config it could not write, so a record made
// from that answer would have verified.)
func TestAnEmptyRecordedHashIsAMismatch(t *testing.T) {
	current := derived(t)
	err := opencodeprompt.Verify(reload(t, current), "", current)
	if err == nil || !opencodeprompt.IsMismatch(err) {
		t.Fatalf("Verify with an empty recorded hash = %v, want a mismatch", err)
	}
}

// When the current config is itself empty in a field, the recorded one is not blamed for being
// empty: the two agree, and a config that Derive could never have built is not Verify's to judge.
func TestARecordedFieldThatIsEmptyLikeTheCurrentOneIsNotBlamed(t *testing.T) {
	current := opencodeprompt.PromptConfig{}
	if err := opencodeprompt.Verify(current, mustHash(t, current), current); err != nil {
		t.Fatalf("Verify of two empty configs = %v, want nil", err)
	}
}

// The empty recorded field is still named when the current one has a value.
func TestAnEmptyRecordedFieldIsNamedWhenTheCurrentOneIsNot(t *testing.T) {
	current := derived(t)
	recorded := reload(t, current)
	recorded.InjectionPoint = ""
	err := opencodeprompt.Verify(recorded, mustHash(t, current), current)
	if err == nil || err.Error() != "prompt_config.injection_point is empty" {
		t.Fatalf("Verify = %v, want %q", err, "prompt_config.injection_point is empty")
	}
}
