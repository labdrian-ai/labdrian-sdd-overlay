package hookwire

import (
	"errors"
	"reflect"
	"testing"
)

// renamedView stands for guardHookInput after somebody renames it.
type renamedView struct{}

// The legacy wording of the clearance guard's denial must survive a rename of the type that is
// decoded into: the rewrite finds the type by the type itself, not by a copy of its name, and
// that is only visible when the type is named something else. While it is named as it was
// (guardHookInput), the second rewrite of ClearanceGuardDetail replaces a string with itself, and
// no input of the real type can tell whether it is there.
func TestClearanceGuardDetailKeepsItsWordsWhenTheViewIsRenamed(t *testing.T) {
	saved := guardViewType
	t.Cleanup(func() { guardViewType = saved })
	guardViewType = reflect.TypeOf(renamedView{})

	for name, tc := range map[string]struct{ cause, want string }{
		"a value that is not an object": {
			"json: cannot unmarshal array into Go value of type hookwire.renamedView",
			"json: cannot unmarshal array into Go value of type shaper.guardHookInput",
		},
		"a field of the wrong type": {
			"json: cannot unmarshal number into Go struct field renamedView.tool_input of type string",
			"json: cannot unmarshal number into Go struct field guardHookInput.tool_input of type string",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := ClearanceGuardDetail(&toolCallError{cause: errors.New(tc.cause)})
			if got != tc.want {
				t.Errorf("ClearanceGuardDetail = %q, want %q", got, tc.want)
			}
		})
	}
}
