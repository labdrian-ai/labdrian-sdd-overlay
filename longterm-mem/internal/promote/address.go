package promote

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// AddressAllocator is the port through which promotion obtains the address of
// a page it is about to create: the next free "c-NNNNNN" of the vault. The
// package owns it and never runs the vault's allocator itself; the
// composition root hands the Writer an adapter (internal/vault's
// AddressAllocator, which runs scripts/allocate-address.sh under its lock),
// and a test hands it a fake.
//
// The context is the caller's: cancelling it abandons the allocation. An
// allocator that cannot allocate returns an error and no address; promote
// writes nothing for that memory. An address handed out is spent
// whether or not the page is ever written, as the vault's counter is.
type AddressAllocator interface {
	NextAddress(ctx context.Context) (string, error)
}

// errNoAddressAllocator is what a promotion that needs a new address answers
// when its Writer was built without an AddressAllocator.
var errNoAddressAllocator = errors.New("promote: the writer has no address allocator")

// takeAddress takes the next free address from addresses and enters it, through addressMap, in the vault's
// address map under the page's address-derived path, the manifest being dated at (UTC) when it has to be created.
// A page that already exists keeps its own address and never reaches it (R-028): the allocator is not asked, so a
// Writer that has none still re-promotes, and the address map is not written again. It is what every new page
// needs, whichever store the page is promoted from.
func takeAddress(ctx context.Context, addresses AddressAllocator, addressMap AddressMapRecorder, at time.Time) (string, error) {
	if memory.IsMissing(addresses) {
		return "", errNoAddressAllocator
	}
	if memory.IsMissing(addressMap) {
		return "", errNoAddressMapRecorder
	}
	address, err := addresses.NextAddress(ctx)
	if err != nil {
		return "", err
	}
	if address == "" {
		return "", errors.New("promote: the address allocator returned no address")
	}

	if err := addressMap.RecordAddress(vaultlayout.PageFile(address), address, at); err != nil {
		return "", err
	}
	return address, nil
}

// eachPage reads every page of the vault once, through pages, in the order the port lists them, and hands each
// to visit with the address, the whole content, and the error if the page could not be read (the content is
// then empty). A page that cannot be read does not stop the pass. Only a pages directory that cannot be listed,
// or a reader that was not wired, is an error: there is then no page to hand over.
func eachPage(pages PageReader, visit func(address, content string, err error)) error {
	if memory.IsMissing(pages) {
		return errNoPageReader
	}
	addresses, err := pages.ListPages()
	if err != nil {
		return fmt.Errorf("promote: list %s: %w", pages.PagesLocation(), err)
	}
	for _, address := range addresses {
		content, err := pages.ReadPage(address)
		visit(address, content, err)
	}
	return nil
}

// frontmatterBlock isolates the leading `---\n...\n---\n` frontmatter block
// from a full page's raw content, so parseFrontmatterFields (lint.go)
// never sees body prose that could collide with a field name.
func frontmatterBlock(raw string) (string, bool) {
	const delim = "---\n"
	parts := strings.SplitN(raw, delim, 3)
	if len(parts) < 3 {
		return "", false
	}
	return delim + parts[1] + delim, true
}
