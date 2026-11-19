package promote

import (
	"fmt"
	"regexp"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

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

// EmitPage renders m as a contract-conformant vault page at address (R-027), in the style of the pages of the
// store m comes from (pageStyle): the frontmatter with the store's type mapped onto the vault's own type
// contract (concept, D7) plus the flat store-agnostic keys that name the memory (source, source_id, source_type,
// source_revision), related links from the caller-resolved related slice, and a body carrying the memory's content
// behind an H1 title (omitted when the content already opens with one) plus a footer naming the store.
//
// at is the instant the page is rendered at: its created and updated dates
// are the day it is in UTC, whatever zone at carries. The caller reads the
// time (the Writer from its Clock), so the same inputs always render the
// same page.
func EmitPage(m memory.Memory, address string, related []Link, at time.Time) (Page, error) {
	style, err := styleOf(m.Identity.Source)
	if err != nil {
		return Page{}, err
	}
	return style.emit(m, address, related, at)
}

// pageTags are a promoted page's tags: memory and the memory's type, and,
// when the memory's topic carries the goal prefix (memory.GoalOfTopic),
// the goal's tag after them (ADR C23 M5, decision 2), so a query under a goal
// can tell the page's goal from its frontmatter. An observation of no goal
// gets the two tags every page had before goals.
func pageTags(m memory.Memory) []string {
	tags := []string{"memory", m.Type}
	if goal, ok := memory.GoalOfTopic(m.TopicKey); ok {
		tags = append(tags, memory.GoalTag(goal))
	}
	return tags
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
