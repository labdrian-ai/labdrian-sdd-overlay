//go:build !linux

package fsstore

// names and create are what Names and Create do on this platform, which cannot hold a directory open: the store
// works by path (bypath.go).

func (s Store) names(root, feature string) ([]string, error) {
	return s.namesByPath(root, feature)
}

func (s Store) create(root, feature, name string, data []byte) error {
	return s.createByPath(root, feature, name, data)
}
