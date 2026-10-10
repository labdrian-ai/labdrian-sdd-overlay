package memory

import "reflect"

// IsMissing reports whether a port a consumer was handed is not there: an absent interface, or an
// interface that holds a nil pointer (or a nil function, map, slice or channel). A plain comparison
// with nil sees the first and not the second, and the second is how a store that failed to open
// reaches a consumer: the call that would have used it then panics inside the store instead of failing
// in the consumer's own words. A consumer that checks its ports on entry asks this.
func IsMissing(port any) bool {
	if port == nil {
		return true
	}
	switch value := reflect.ValueOf(port); value.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan, reflect.Interface:
		return value.IsNil()
	default:
		return false
	}
}
