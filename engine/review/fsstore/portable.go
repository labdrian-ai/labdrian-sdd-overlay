//go:build !linux

package fsstore

import (
	"fmt"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// names and create are what Names and Create do on this platform, which cannot hold a directory open: the store
// works by path (bypath.go).

func (s Store) names(root, feature string) ([]string, error) {
	return s.namesByPath(root, feature)
}

func (s Store) create(root, feature, name string, data []byte) error {
	return s.createByPath(root, feature, name, data)
}

// Lock holds no lock on this platform: it only requires the directory of the feature to be there. Two
// verdicts that block, verified at the same time on two heads of one base, can then both be stored; the
// commands are run one after another here.
func (s Store) Lock(root, feature string) (func(), error) {
	_, found, err := s.walk(root, feature, false)
	switch {
	case err != nil:
		return nil, err
	case !found:
		return nil, fmt.Errorf("%w: %s", app.ErrNotFound, filepath.Join(root, review.ResultsDir, feature))
	}
	return func() {}, nil
}
