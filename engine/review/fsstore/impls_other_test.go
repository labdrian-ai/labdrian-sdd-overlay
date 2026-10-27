//go:build !linux && !windows

package fsstore

func impls() []impl {
	return []impl{byPath()}
}
