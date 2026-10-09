package runtime

import "reflect"

// missing reports whether the HookInstaller port it is given is absent: an untyped nil, or a value
// of a nilable kind (a pointer, map, slice, func or channel) that is nil, whose methods could not be
// relied on. A nil pointer that implements the port passes `hooks == nil` and then fails on its
// first call, which is what the check is for.
//
// It answers for that one port: its parameter is a HookInstaller, so it is not a check for any
// port.
//
// The kinds are the ones reflect.Value.IsNil accepts and a Go value can have without being an
// interface. reflect.ValueOf unwraps an interface, so Interface is never the kind of the value it
// returns and has no case; UnsafePointer is left out on purpose because a port is never an
// unsafe.Pointer (the type would need methods, and nothing in the module declares one). reflect.Pointer
// is the name of the kind since Go 1.18 and reflect.Ptr only an alias kept for old code.
func missing(hooks HookInstaller) bool {
	if hooks == nil {
		return true
	}
	switch value := reflect.ValueOf(hooks); value.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return value.IsNil()
	}
	return false
}
