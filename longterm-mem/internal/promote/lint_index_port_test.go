package promote

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// indexOf is an IndexReader that answers a content and an error, and counts how often it was asked.
type indexOf struct {
	content string
	err     error
	reads   int
}

func (i *indexOf) ReadIndex() (string, error) {
	i.reads++
	return i.content, i.err
}

func pageForTheInboundLink(t *testing.T, address string) Page {
	t.Helper()
	obs := memory.Observation{ID: 44, Type: "decision", Title: "Named", Content: "Body.", Project: "labdrian-sdd-overlay"}
	page, err := EmitPage(obs, address, nil, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	return page
}

// The rule asks the port for the catalog and judges the content it is given: a link to the page's address is
// enough, and the bytes the port answers are all the rule knows of the file.
func TestCheckInboundIndexLinkJudgesTheContentThePortAnswers(t *testing.T) {
	page := pageForTheInboundLink(t, "c-000044")

	t.Run("a catalog that links to the page", func(t *testing.T) {
		index := &indexOf{content: "- [[c-000044|Named]]\n"}
		if diag, ok := CheckInboundIndexLink(page, index); !ok || diag != (Diagnostic{}) {
			t.Fatalf("CheckInboundIndexLink = (%+v, %v), want the page found", diag, ok)
		}
		if index.reads != 1 {
			t.Errorf("the catalog was read %d times, want once", index.reads)
		}
	})

	t.Run("a catalog with no link to the page", func(t *testing.T) {
		diag, ok := CheckInboundIndexLink(page, &indexOf{content: "- [[c-000099|Other]]\n"})
		if ok || diag.Rule != "inbound-index-link" || diag.Detail != "wiki/index.md has no link to c-000044" {
			t.Fatalf("CheckInboundIndexLink = (%+v, %v), want the missing link named", diag, ok)
		}
	})

	t.Run("an address in the prose of the catalog is not a link to the page", func(t *testing.T) {
		diag, ok := CheckInboundIndexLink(page, &indexOf{content: "Notes: c-000044 was promoted, see the log.\n"})
		if ok || diag.Detail != "wiki/index.md has no link to c-000044" {
			t.Fatalf("CheckInboundIndexLink = (%+v, %v), want the missing link named", diag, ok)
		}
	})

	t.Run("an empty catalog is a catalog with no link, not a missing one", func(t *testing.T) {
		diag, ok := CheckInboundIndexLink(page, &indexOf{})
		if ok || diag.Detail != "wiki/index.md has no link to c-000044" {
			t.Fatalf("CheckInboundIndexLink = (%+v, %v), want the missing link named", diag, ok)
		}
	})
}

// A catalog the port cannot read, for whatever reason, is a catalog that is missing as far as the rule can tell:
// not there, a directory, a link to nothing, a wiki directory that is a file.
func TestCheckInboundIndexLinkTakesACatalogItCannotReadForMissing(t *testing.T) {
	page := pageForTheInboundLink(t, "c-000044")
	for name, cause := range map[string]error{
		"not there":                       fmt.Errorf("open wiki/index.md: %w", fs.ErrNotExist),
		"something else":                  errors.New("read wiki/index.md: is a directory"),
		"a wiki directory that is a file": errors.New("open wiki/index.md: not a directory"),
	} {
		t.Run(name, func(t *testing.T) {
			diag, ok := CheckInboundIndexLink(page, &indexOf{content: "- [[c-000044|Named]]\n", err: cause})
			if ok || diag.Rule != "inbound-index-link" || diag.Detail != "wiki/index.md is missing" {
				t.Fatalf("CheckInboundIndexLink = (%+v, %v), want the catalog named missing, whatever the content beside the error", diag, ok)
			}
		})
	}
}

// A port that was not wired is named, rather than called, as the other ports of the page check are.
func TestCheckInboundIndexLinkNamesAReaderThatWasNotWired(t *testing.T) {
	page := pageForTheInboundLink(t, "c-000044")
	var typedNil *indexOf
	for name, index := range map[string]IndexReader{"no reader": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			diag, ok := CheckInboundIndexLink(page, index)
			if ok || diag.Rule != "inbound-index-link" || diag.Detail != "promote: no index reader was wired" {
				t.Fatalf("CheckInboundIndexLink = (%+v, %v), want the missing port named", diag, ok)
			}
		})
	}
}

// LintPage hands the rule the reader it was given.
func TestLintPageReadsTheCatalogThroughItsPort(t *testing.T) {
	page := pageForTheInboundLink(t, "c-000044")
	index := &indexOf{content: "- [[c-000044|Named]]\n"}

	for _, d := range LintPage(page, newMemPages(nil), &memAddressMap{}, index) {
		if d.Rule == "inbound-index-link" {
			t.Errorf("LintPage reports %+v although the catalog the port answers links to the page", d)
		}
	}
	if index.reads != 1 {
		t.Errorf("the catalog was read %d times, want once", index.reads)
	}
}
