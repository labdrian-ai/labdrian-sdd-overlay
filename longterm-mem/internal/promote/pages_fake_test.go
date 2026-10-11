package promote

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
)

// pageWrite is one call of WritePage, as it was asked.
type pageWrite struct{ address, content string }

// memPages is a PageRepository kept in memory: pages are strings under their address, a missing page reads as
// the file system says one does, and the fake can fail each operation on demand and tells what it was asked.
// The file system adapter has its own tests (internal/vaultfs); the promotion that has to be seen to go
// through the port, and the failures a disk will not produce on demand, are tested here against this fake.
type memPages struct {
	// pages holds the content of each page by address.
	pages map[string]string
	// dir is what PagesLocation answers; the location of a page is under it.
	dir string
	// listErr, existsErr and writeErr, when set, are what the matching call answers.
	listErr, existsErr, writeErr error
	// readErrs makes the read of an address fail; readHook, when set, is asked before every read with the
	// address and how many times it has been read, and a non-nil answer fails that read.
	readErrs map[string]error
	readHook func(address string, nth int) error

	// reads counts the reads asked for by address, failed ones included; exists lists the addresses
	// PageExists was asked about; writes lists every call of WritePage, failed ones included.
	reads  map[string]int
	exists []string
	writes []pageWrite
}

func newMemPages(pages map[string]string) *memPages {
	if pages == nil {
		pages = map[string]string{}
	}
	return &memPages{pages: pages, dir: filepath.Join("memory-vault", "wiki", "memory"), reads: map[string]int{}}
}

func (m *memPages) ListPages() ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	addresses := make([]string, 0, len(m.pages))
	for address := range m.pages {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	return addresses, nil
}

func (m *memPages) ReadPage(address string) (string, error) {
	m.reads[address]++
	if err := m.readErrs[address]; err != nil {
		return "", err
	}
	if m.readHook != nil {
		if err := m.readHook(address, m.reads[address]); err != nil {
			return "", err
		}
	}
	content, ok := m.pages[address]
	if !ok {
		return "", fmt.Errorf("open %s: %w", m.PageLocation(address), fs.ErrNotExist)
	}
	return content, nil
}

func (m *memPages) PageExists(address string) (bool, error) {
	m.exists = append(m.exists, address)
	if m.existsErr != nil {
		return false, m.existsErr
	}
	_, ok := m.pages[address]
	return ok, nil
}

func (m *memPages) WritePage(address, content string) error {
	m.writes = append(m.writes, pageWrite{address: address, content: content})
	if m.writeErr != nil {
		return m.writeErr
	}
	m.pages[address] = content
	return nil
}

func (m *memPages) PageLocation(address string) string { return filepath.Join(m.dir, address+".md") }

func (m *memPages) PagesLocation() string { return m.dir }
