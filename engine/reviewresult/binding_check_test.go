package reviewresult_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
)

// A caller that holds a binding with no reply to hold to it asks the question Parse asks before it reads one: does
// the binding name a candidate? The stored exemption of a head is such a binding (own-rdd T4, S5): the delivery check
// reads it back and must never hand its base to git unless it is a full object name.
func TestBindingCheckAcceptsABindingThatNamesACandidate(t *testing.T) {
	if err := binding().Check(); err != nil {
		t.Errorf("Check() error = %v, want nil", err)
	}
	// A diff that changes no path is still a candidate.
	empty := binding()
	empty.Paths = nil
	if err := empty.Check(); err != nil {
		t.Errorf("Check() of a binding with no path error = %v, want nil", err)
	}
}

// The binding is refused for the reasons Parse refuses it, naming the member, because it is the same rule: the
// rows are the ones TestParseRefusesABindingThatNamesNoCandidate holds Parse to, and a path listed twice.
func TestBindingCheckRefusesABindingThatNamesNoCandidate(t *testing.T) {
	with := func(change func(*reviewresult.Binding)) reviewresult.Binding {
		b := binding()
		change(&b)
		return b
	}
	for _, tt := range []struct {
		name   string
		member string
		bound  reviewresult.Binding
	}{
		{"an empty binding", "base", reviewresult.Binding{}},
		{"a base that is empty", "base", with(func(b *reviewresult.Binding) { b.Base = "" })},
		{"a base that is an abbreviation", "base", with(func(b *reviewresult.Binding) { b.Base = baseSHA[:12] })},
		{"a base that would be read as an option", "base", with(func(b *reviewresult.Binding) { b.Base = "--output=forged" })},
		{"a head with an uppercase letter", "head", with(func(b *reviewresult.Binding) { b.Head = "22222222222222222222222222222222222222AB" })},
		{"a head that is a ref", "head", with(func(b *reviewresult.Binding) { b.Head = "HEAD" })},
		{"a diff_sha256 that is empty", "diff_sha256", with(func(b *reviewresult.Binding) { b.DiffSHA256 = "" })},
		{"a diff_sha256 of 63 characters", "diff_sha256", with(func(b *reviewresult.Binding) { b.DiffSHA256 = diffSHA[:63] })},
		{"a path listed twice", "paths", with(func(b *reviewresult.Binding) { b.Paths = []string{"a.go", "a.go"} })},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bound.Check()

			if !errors.Is(err, reviewresult.ErrInvalidBinding) {
				t.Fatalf("Check() error = %v, want one that wraps ErrInvalidBinding", err)
			}
			if !strings.Contains(err.Error(), tt.member) {
				t.Errorf("Check() error = %v, want it to name the member %q", err, tt.member)
			}
		})
	}
}
