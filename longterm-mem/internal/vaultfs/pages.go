package vaultfs

import (
	"os"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// pageSuffix is what a file of the pages directory ends in to be a page.
const pageSuffix = ".md"

// ListPages returns the address of every page in the vault, in the order of the directory (which is by name):
// the files of the pages directory whose name ends in .md and that are not directories. A link is a file here
// whatever it points at, as the directory lists it, so a link to a directory is listed and reading it is what
// fails. A vault with no pages directory, or whose pages directory is a link to nothing, has no pages; a
// directory that cannot be listed (it is a file, it cannot be opened) is the file system's own error, which
// names it.
func (v *Vault) ListPages() ([]string, error) {
	entries, err := os.ReadDir(v.PagesLocation())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var addresses []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), pageSuffix) {
			continue
		}
		addresses = append(addresses, strings.TrimSuffix(entry.Name(), pageSuffix))
	}
	return addresses, nil
}

// ReadPage returns the whole content of the page promoted under address, byte for byte, reading through a link
// to it. A page that is not there is the file system's error for it (errors.Is(err, fs.ErrNotExist)), and any
// other failure is the file system's own: both name the file, and neither carries the Vault's error prefix,
// because the callers word the failure around the page they were reading.
func (v *Vault) ReadPage(address string) (string, error) {
	data, err := os.ReadFile(v.PageLocation(address))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PageExists reports whether something is at the path of the page promoted under address, following a link:
// a file or a directory is a page that exists, a link to nothing is a page that does not. A path that cannot
// be examined for another reason (the pages directory is a file) is the file system's error, untouched, and
// no answer.
func (v *Vault) PageExists(address string) (bool, error) {
	if _, err := os.Stat(v.PageLocation(address)); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// WritePage replaces the page promoted under address with content, durably and atomically, creating the
// pages directory when the vault has none: through a link to the page, keeping the mode of the page that is
// there, and creating a new one owner-only. Its errors name what failed behind the Vault's error prefix, if
// it has one.
func (v *Vault) WritePage(address, content string) error {
	if err := writeFileAtomic(v.PageLocation(address), []byte(content)); err != nil {
		return v.fail(err)
	}
	return nil
}

// PageLocation is the path of the page promoted under address in this vault, as an operator would open it.
func (v *Vault) PageLocation(address string) string {
	return v.layout.Path(vaultlayout.PageFile(address))
}

// PagesLocation is the path of the pages directory of this vault.
func (v *Vault) PagesLocation() string {
	return v.layout.Path(vaultlayout.PagesDir)
}
