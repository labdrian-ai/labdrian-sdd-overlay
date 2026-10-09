package opencodeprompt

import (
	"errors"
	"testing"
)

// hashOf turns the result of marshalling into a hash. A config that could not be written has no
// hash: the error comes back, and never an empty string that a caller might record and later
// verify against.
func TestAConfigThatCannotBeWrittenHasNoHash(t *testing.T) {
	boom := errors.New("cannot marshal")

	hash, err := hashOf(nil, boom)

	if !errors.Is(err, boom) || hash != "" {
		t.Fatalf("hashOf = %q, %v, want no hash and the marshalling error", hash, err)
	}
}

func TestHashOfAnswersTheSHA256OfTheBytes(t *testing.T) {
	hash, err := hashOf([]byte("abc"), nil)
	if err != nil || hash != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("hashOf(abc) = %q, %v, want the SHA-256 of abc", hash, err)
	}
}

// If the current config cannot be hashed nothing can be called current, and the caller must not
// read that as a stale record: the error is not a mismatch.
func TestAConfigThatCannotBeHashedIsNotAMismatch(t *testing.T) {
	boom := errors.New("cannot marshal")
	failing := func(PromptConfig) (string, error) { return "", boom }
	config := PromptConfig{ContractPath: "a", InjectionPoint: "b"}

	err := verifyWith(failing, config, "anything", config)

	if !errors.Is(err, boom) || IsMismatch(err) {
		t.Fatalf("verifyWith = %v, want the hashing error and not a mismatch", err)
	}
}
