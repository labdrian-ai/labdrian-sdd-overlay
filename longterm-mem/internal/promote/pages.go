package promote

import "errors"

// PageExistence is the port through which the page check asks whether the page promoted under an address is
// there: the one question its wikilink rule puts to the pages.
type PageExistence interface {
	// PageExists reports whether the page promoted under address is in the vault. A page that is not there is
	// (false, nil); anything else that keeps the answer from being known (the pages directory is a file, a
	// path that cannot be read) is an error and no answer. What is at the path is not examined: a directory
	// there is a page that exists.
	PageExists(address string) (bool, error)
}

// PageReader is the port through which promotion reads the pages of one vault: the promoted notes, one file
// each, named by their address. The package owns it and never touches the pages directory itself; the
// composition root hands it an adapter (internal/vaultfs, which keeps the pages where vaultlayout says), and a
// test hands it a fake.
//
// It speaks of a page by its address and of its content as the whole file, frontmatter and body, as the
// vault's own tools read it. The failures it returns are the file system's own, untouched, because every
// caller words its failure around the cause (what it was reading, and for whom); the adapter adds nothing to
// them but the location they already name.
type PageReader interface {
	PageExistence
	// ListPages returns the address of every page in the vault, in the order of the directory: the files of
	// the pages directory whose name ends in .md and that are not directories. A vault with no pages
	// directory has no pages, which is (nil, nil), and not an error; a directory that cannot be listed is.
	ListPages() ([]string, error)
	// ReadPage returns the whole content of the page promoted under address, following a link to it. A page
	// that is not there is an error for which errors.Is(err, fs.ErrNotExist) holds.
	ReadPage(address string) (string, error)
	// PageLocation names the page promoted under address, as an operator would find it, for the messages
	// that tell which page failed. It touches no file.
	PageLocation(address string) string
	// PagesLocation names the pages directory, for the same reason.
	PagesLocation() string
}

// PageRepository is the port through which promotion reads and writes the pages of one vault: the read half,
// and the replacement of a page.
type PageRepository interface {
	PageReader
	// WritePage makes the page promoted under address hold content, whole. It is durable and atomic: a reader
	// sees the old page or the new one and never half of either, after it returns nil the new page survives
	// the process, and a failure leaves the old page as it was. The pages directory is created when it is not
	// there; a page that exists keeps its mode and a link to it is written through, because the vault is the
	// user's own and shared with other tools; a page it creates is readable by its owner alone.
	WritePage(address, content string) error
}

// errNoPageRepository is what a promotion answers when its Writer was built without a PageRepository: a port
// the caller forgot to wire is named, before anything is written.
var errNoPageRepository = errors.New("promote: the writer has no page repository")

// errNoPageReader is what a read of the pages answers when it was handed no PageReader, and what the wikilink
// rule of the page check reports when it was handed no PageExistence and the page has a link to resolve.
var errNoPageReader = errors.New("promote: no page reader was wired")
