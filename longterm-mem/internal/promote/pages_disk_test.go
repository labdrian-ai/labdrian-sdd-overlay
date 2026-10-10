package promote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// diskPages is a PageRepository over a directory of the test's own, with the file system semantics the real
// adapter has (internal/vaultfs, which has its own tests, and which this package cannot import): the
// directory is listed by name, a link is followed, a page that is a directory reads as the file system says,
// and a write is the module's atomic replace, which keeps the mode of a page that exists and creates a new one
// owner-only. The promotion tests that put pages on disk with os and look at them there use it, as the ones
// that care only for what is asked of the port use memPages.
type diskPages struct{ dir string }

// pagesIn is the pages of the vault at root, where vaultlayout says they are.
func pagesIn(root string) diskPages { return diskPages{dir: filepath.Join(root, vaultlayout.PagesDir)} }

func (d diskPages) ListPages() ([]string, error) {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var addresses []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		addresses = append(addresses, strings.TrimSuffix(entry.Name(), ".md"))
	}
	return addresses, nil
}

func (d diskPages) ReadPage(address string) (string, error) {
	data, err := os.ReadFile(d.PageLocation(address))
	return string(data), err
}

func (d diskPages) PageExists(address string) (bool, error) {
	if _, err := os.Stat(d.PageLocation(address)); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (d diskPages) WritePage(address, content string) error {
	path := d.PageLocation(address)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("promote: create directory for %s: %w", path, err)
	}
	if err := durable.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("promote: %w", err)
	}
	return nil
}

func (d diskPages) PageLocation(address string) string { return filepath.Join(d.dir, address+".md") }

func (d diskPages) PagesLocation() string { return d.dir }
