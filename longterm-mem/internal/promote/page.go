package promote

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// pagePathPrefix is the vault-relative directory promoted pages live under
// (D7): `wiki/memory/<address>.md`.
const pagePathPrefix = "wiki/memory"

// nowFunc is swapped in tests for deterministic created/updated timestamps
// and golden comparisons.
var nowFunc = func() time.Time { return time.Now().UTC() }

// wikilinkPattern is the D7 alias wikilink shape, `[[c-NNNNNN|Title]]`,
// shared by wikilink's rendering and LintPage's resolvability check
// (4.12 REFACTOR).
var wikilinkPattern = regexp.MustCompile(`\[\[(c-\d{6})\|([^\]]*)\]\]`)

// Link is one already-resolved related-page reference (D7): the target's
// allocated address and the title EmitPage renders inside the wikilink
// alias.
type Link struct {
	Address string
	Title   string
}

// Page is one emitted vault page: its address-derived on-disk path (never
// derived from title, so a later retitle cannot rename it, R-027), its
// rendered frontmatter, and its rendered body.
type Page struct {
	Address     string
	Path        string
	Frontmatter string
	Body        string
}

// EmitPage renders obs as a contract-conformant vault page at address
// (R-027): frontmatter with the Engram type mapped onto the vault's own
// type contract (concept, D7) plus flat engram_* extras, related links
// from the caller-resolved related slice, and a body carrying obs.Content
// behind an H1 title (omitted when the content already opens with one)
// plus a footer naming the Engram source.
func EmitPage(obs engram.Observation, address string, related []Link) (Page, error) {
	if address == "" {
		return Page{}, fmt.Errorf("promote: emit page for observation %d: address is required", obs.ID)
	}

	today := nowFunc().Format("2006-01-02")
	fm := frontmatter{
		Title: obs.Title, Address: address, Aliases: []string{obs.Title},
		Created: today, Updated: today,
		Tags: []string{"memory", obs.Type}, Status: statusFor(obs),
		Related:        relatedWikilinks(related),
		EngramID:       obs.ID,
		EngramSyncID:   obs.SyncID,
		EngramType:     obs.Type,
		EngramRevision: obs.RevisionCount,
		Project:        obs.Project,
	}

	return Page{
		Address:     address,
		Path:        pagePathPrefix + "/" + address + ".md",
		Frontmatter: fm.Render(),
		Body:        renderBody(obs),
	}, nil
}

// statusFor derives the fresh-promotion status: mature for a pinned
// observation, developing otherwise; R-033 (slice 7) is the only place
// that later sets superseded/archived.
func statusFor(obs engram.Observation) string {
	if obs.Pinned {
		return "mature"
	}
	return "developing"
}

func relatedWikilinks(related []Link) []string {
	links := make([]string, 0, len(related))
	for _, l := range related {
		links = append(links, wikilink(l.Address, l.Title))
	}
	return links
}

// wikilink renders the D7 alias wikilink form.
func wikilink(address, title string) string {
	return fmt.Sprintf("[[%s|%s]]", address, title)
}

// renderBody renders obs.Content behind an H1 title (omitted when the
// content already opens with one) plus a footer naming the Engram source.
func renderBody(obs engram.Observation) string {
	content := strings.TrimRight(obs.Content, "\n")
	var b strings.Builder
	if !strings.HasPrefix(strings.TrimSpace(content), "# ") {
		b.WriteString("# ")
		b.WriteString(obs.Title)
		b.WriteString("\n\n")
	}
	b.WriteString(content)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "---\nPromoted from Engram observation %d, revision %d.\n", obs.ID, obs.RevisionCount)
	return b.String()
}
