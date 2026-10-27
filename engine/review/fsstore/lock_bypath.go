//go:build unix && !linux

package fsstore

// Lock takes an exclusive flock(2) on the directory of the feature, reached by path (lockByPath).
func (s Store) Lock(root, feature string) (func(), error) {
	return s.lockByPath(root, feature)
}
