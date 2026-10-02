package capability

import (
	"errors"
	"fmt"
)

// TestCatalog is the port the evidence check asks its one question through: is there a
// test, by this name, directly in this directory of the engine, that go test would run?
// The domain owns the question; where the answer comes from (a scan of the test sources
// on disk, a table in memory) is an adapter's, so this package stays free of the file
// system. The adapter that reads the real tree is capabilitytest.NewCatalog.
type TestCatalog interface {
	// HasTest reports whether the directory dir holds a runnable test named name. dir is
	// a clean, slash-separated path relative to the engine root, and name is Test
	// followed by an identifier, exactly as ParseTestRef returns them; an adapter that
	// reads files may rely on dir never climbing out of the engine root. The error is
	// for a directory the catalog cannot answer for (it is missing, or unreadable); its
	// text says why and is reported as it is. A test that is not there is (false, nil).
	HasTest(dir, name string) (bool, error)
}

// CheckEvidence verifies that every test a declaration names exists in catalog. A
// reference <dir>:<TestName> is satisfied only by a catalog that has TestName in <dir>,
// and what counts as having it, a top-level function that go test would run, is the
// catalog's to decide. Every failing reference is reported at once, each naming its
// target and capability.
//
// References are checked for format first (ParseTestRef), so a reference that could name
// a path outside the engine root is refused before the catalog is asked anything. This is
// the whole check, and it is pure: it holds the rule that a named test must exist, and
// asks the world nothing but the one question of the port.
func CheckEvidence(catalog TestCatalog, d Declaration) error {
	var errs []error
	for _, c := range d.Claims {
		for _, ref := range c.Tests {
			where := fmt.Sprintf("target %s, capability %s", d.Target, c.Capability)
			dir, name, err := ParseTestRef(ref)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", where, err))
				continue
			}
			found, err := catalog.HasTest(dir, name)
			switch {
			case err != nil:
				errs = append(errs, fmt.Errorf("%s: test %q: %w", where, ref, err))
			case !found:
				errs = append(errs, fmt.Errorf("%s: test %q not found: no top-level func %s(t *testing.T) in a _test.go file of %q", where, ref, name, dir))
			}
		}
	}
	return errors.Join(errs...)
}
