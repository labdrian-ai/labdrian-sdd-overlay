package promote

import (
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// diskFile is one file of a vault, as the catalog is: an IndexReader over a path of the test's own, with the file
// system semantics the real adapter has (internal/vaultfs, which has its own tests, and which this package cannot
// import): a file is read whole through a link, and a failure is the file system's own. The promotion tests that
// put the catalog on disk with os and look at it there use it, as the ones that care only for what is asked of
// the port use memFile.
type diskFile struct{ path string }

// indexIn is the catalog of the vault at root, where vaultlayout says it is.
func indexIn(root string) diskFile { return diskFile{path: filepath.Join(root, vaultlayout.IndexFile)} }

func (f diskFile) ReadIndex() (string, error) {
	data, err := os.ReadFile(f.path)
	return string(data), err
}
