package runtime

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// A HookInstaller of each kind that can be nil. The methods do nothing: the test never calls them,
// it asks only whether the port is there.
type (
	ptrHooks   struct{}
	mapHooks   map[string]string
	sliceHooks []string
	funcHooks  func()
	chanHooks  chan int
	// valueHooks is a struct: it can hold no nil.
	valueHooks struct{}
)

func (*ptrHooks) Install(string, string) error               { return nil }
func (*ptrHooks) Uninstall(string, string) error             { return nil }
func (*ptrHooks) Inspect(string, string) (bool, bool, error) { return false, false, nil }
func (*ptrHooks) InspectMainAgent(string) (settings.MainAgentState, string, error) {
	return settings.MainAgentOwned, "", nil
}
func (*ptrHooks) InspectOutputStyle(string) (settings.OutputStyleState, string, error) {
	return settings.OutputStyleOwned, "", nil
}

func (mapHooks) Install(string, string) error               { return nil }
func (mapHooks) Uninstall(string, string) error             { return nil }
func (mapHooks) Inspect(string, string) (bool, bool, error) { return false, false, nil }
func (mapHooks) InspectMainAgent(string) (settings.MainAgentState, string, error) {
	return settings.MainAgentOwned, "", nil
}
func (mapHooks) InspectOutputStyle(string) (settings.OutputStyleState, string, error) {
	return settings.OutputStyleOwned, "", nil
}

func (sliceHooks) Install(string, string) error               { return nil }
func (sliceHooks) Uninstall(string, string) error             { return nil }
func (sliceHooks) Inspect(string, string) (bool, bool, error) { return false, false, nil }
func (sliceHooks) InspectMainAgent(string) (settings.MainAgentState, string, error) {
	return settings.MainAgentOwned, "", nil
}
func (sliceHooks) InspectOutputStyle(string) (settings.OutputStyleState, string, error) {
	return settings.OutputStyleOwned, "", nil
}

func (funcHooks) Install(string, string) error               { return nil }
func (funcHooks) Uninstall(string, string) error             { return nil }
func (funcHooks) Inspect(string, string) (bool, bool, error) { return false, false, nil }
func (funcHooks) InspectMainAgent(string) (settings.MainAgentState, string, error) {
	return settings.MainAgentOwned, "", nil
}
func (funcHooks) InspectOutputStyle(string) (settings.OutputStyleState, string, error) {
	return settings.OutputStyleOwned, "", nil
}

func (chanHooks) Install(string, string) error               { return nil }
func (chanHooks) Uninstall(string, string) error             { return nil }
func (chanHooks) Inspect(string, string) (bool, bool, error) { return false, false, nil }
func (chanHooks) InspectMainAgent(string) (settings.MainAgentState, string, error) {
	return settings.MainAgentOwned, "", nil
}
func (chanHooks) InspectOutputStyle(string) (settings.OutputStyleState, string, error) {
	return settings.OutputStyleOwned, "", nil
}

func (valueHooks) Install(string, string) error               { return nil }
func (valueHooks) Uninstall(string, string) error             { return nil }
func (valueHooks) Inspect(string, string) (bool, bool, error) { return false, false, nil }
func (valueHooks) InspectMainAgent(string) (settings.MainAgentState, string, error) {
	return settings.MainAgentOwned, "", nil
}
func (valueHooks) InspectOutputStyle(string) (settings.OutputStyleState, string, error) {
	return settings.OutputStyleOwned, "", nil
}

// A port is missing when it is an untyped nil or a nil of any kind a value can have, and present
// otherwise, including an empty one: a map, slice or channel that is allocated, a func that is
// set, a pointer that points, a struct.
func TestMissingTellsAbsentPortsOfEveryNilableKind(t *testing.T) {
	var nilPtr *ptrHooks
	for _, c := range []struct {
		name  string
		hooks HookInstaller
		want  bool
	}{
		{"untyped nil", nil, true},
		{"nil pointer", nilPtr, true},
		{"nil map", mapHooks(nil), true},
		{"nil slice", sliceHooks(nil), true},
		{"nil func", funcHooks(nil), true},
		{"nil channel", chanHooks(nil), true},
		{"pointer that points", &ptrHooks{}, false},
		{"empty map", mapHooks{}, false},
		{"empty slice", sliceHooks{}, false},
		{"func that is set", funcHooks(func() {}), false},
		{"channel that is made", make(chanHooks), false},
		{"struct", valueHooks{}, false},
	} {
		if got := missing(c.hooks); got != c.want {
			t.Errorf("%s: missing = %v, want %v", c.name, got, c.want)
		}
	}
}
