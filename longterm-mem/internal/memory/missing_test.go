package memory_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

type fakePort struct{}

func (*fakePort) Name() string { return "fake" }

type valuePort struct{}

func (valuePort) Name() string { return "value" }

type namer interface{ Name() string }

func TestIsMissingTellsAPortThatIsNotThereFromOneThatIs(t *testing.T) {
	var typedNil *fakePort
	var nilInterface namer
	var nilFunc func()
	var nilMap map[string]int
	cases := []struct {
		name    string
		port    any
		missing bool
	}{
		{"an untyped nil", nil, true},
		{"a nil interface value", nilInterface, true},
		{"a nil pointer inside an interface", typedNil, true},
		{"a nil function", nilFunc, true},
		{"a nil map", nilMap, true},
		{"a pointer to something", &fakePort{}, false},
		{"a struct value", valuePort{}, false},
		{"a function", func() {}, false},
		{"an empty but made map", map[string]int{}, false},
	}
	for _, tc := range cases {
		if got := memory.IsMissing(tc.port); got != tc.missing {
			t.Errorf("IsMissing(%s) = %v, want %v", tc.name, got, tc.missing)
		}
	}
}

// A port held in a variable of its interface type is the common case: the interface is non-nil exactly
// when something was assigned to it, and a nil pointer assigned to it is the case a plain comparison
// with nil does not see.
func TestIsMissingSeesThroughAnInterfaceVariable(t *testing.T) {
	var held namer = (*fakePort)(nil) // non-nil as an interface, nil behind it
	if !memory.IsMissing(held) {
		t.Error("IsMissing did not see the nil pointer behind the interface")
	}
}
