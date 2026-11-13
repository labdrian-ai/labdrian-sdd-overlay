package ops

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// MemoryEligibility is what promotion's eligibility rule (R-007, promote.Eligible) reads of one memorycard memory:
// whether it is pinned and its topic_key, with the ids that say whose they are. The package owns it and states the
// port that returns it in these plain types: which store holds the memory, and which columns it reads them from,
// are the adapter's to say.
type MemoryEligibility struct {
	// ID is the memory's memorycard id, the memorycard_id of the page promoted from it.
	ID int64
	// EngramID is the id of the Engram observation the memory was imported from, or 0 when it was not imported
	// from one: the engram_id of a page an earlier release promoted from that observation.
	EngramID int64
	// Superseded is whether memorycard has retired the memory for another: such a memory is never judged, for the page
	// of the memory that replaced it is the one sync keeps current.
	Superseded bool
	// Pinned is whether the memory is pinned.
	Pinned bool
	// TopicKey is the memory's topic_key, empty when it has none.
	TopicKey string
}

// errNoMemoryEligibilityReader is what the promoted-pages-eligible check reports when Doctor was built without a
// LiveMemoryEligibility and there is a promoted page to judge: a port the caller forgot to wire is named.
var errNoMemoryEligibilityReader = errors.New("ops: no memory eligibility reader was wired")

// checkPromotedPagesEligible reports, as an advisory warning, every promoted page whose memory promotion's own
// rule no longer accepts: promote.Eligible(memory, false) is false for it, so sync skips the page and it stays as
// it was written, however the memory changes (issue #293). Such a page is frozen, and the finding names it, the
// memory, why the memory is not eligible and the two ways to deal with it: promote it again by hand, which is
// explicit and so eligible, or pin the memory so that sync keeps the page current.
//
// The pages are joined to their memories by the identity the page names: a page's memorycard_id is the memory's
// id, and a page of an earlier release names an Engram observation by its engram_id, which is the memory
// memorycard imported from that observation (its import_map row). Nothing on a page records that it was promoted
// explicitly, so a page that `promote --source memorycard --id N` wrote for a memory that never was eligible is
// reported like any other, until the memory is pinned. A page that names no memory, or whose memory is not among
// the live ones of the project (deleted since, or the row of another project), has no memory to be judged by, and
// is not this check's concern.
//
// The check answers PASS or warn and never FAIL, in any state it finds the vault in: it is a notice, not a
// verdict about the vault, so it can never be the reason the command's exit code says a vault has a problem. A
// vault it cannot judge -- the pages cannot be listed, the live memories cannot be read, a port was not wired --
// is a warning that says so, in the words of what failed. A page that cannot be read is the page checks' finding,
// which they word, and does not stop the pages after it from being judged.
//
// A superseded memory is never judged: only the live ones (not superseded, not archived) of a page's memory count. A page
// of an earlier release may name an observation memorycard imported as several parts; it is kept current when any live part
// is eligible, and a warning names the first live part.
//
// The live memories are asked for once, and only when a page names a memory to look up.
func checkPromotedPagesEligible(promoted PageReader, eligibility func(project string) ([]MemoryEligibility, error), project string) Check {
	cannotBeJudged := func(err error) Check {
		return Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: "eligibility cannot be judged: " + err.Error()}
	}

	pages, _, err := loadPromotedPages(promoted)
	if err != nil {
		return cannotBeJudged(err)
	}
	type promotion struct {
		address string
		ref     promote.MemoryRef
	}
	var promotions []promotion
	for _, page := range pages {
		if ref, named := promote.PageMemoryRef(page); named {
			promotions = append(promotions, promotion{address: page.Address, ref: ref})
		}
	}
	if len(promotions) == 0 {
		return Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}
	}

	if eligibility == nil {
		return cannotBeJudged(errNoMemoryEligibilityReader)
	}
	live, err := eligibility(project)
	if err != nil {
		return cannotBeJudged(fmt.Errorf("could not read the live memories of %s: %w", project, err))
	}
	liveByMemoryID := make(map[int64]MemoryEligibility, len(live))
	liveByEngramID := map[int64][]MemoryEligibility{}
	for _, m := range live {
		if m.Superseded {
			continue
		}
		liveByMemoryID[m.ID] = m
		if m.EngramID > 0 {
			liveByEngramID[m.EngramID] = append(liveByEngramID[m.EngramID], m)
		}
	}

	var findings []string
	for _, p := range promotions {
		var candidates []MemoryEligibility
		legacy := p.ref.Source != "memorycard"
		if legacy {
			candidates = liveByEngramID[p.ref.ID]
		} else if m, isLive := liveByMemoryID[p.ref.ID]; isLive {
			candidates = []MemoryEligibility{m}
		}
		// No live memory: nothing to judge the page by. One observation imported as several parts has several: the page
		// is kept current when any of them is eligible, and otherwise the first (lowest id) is the one named.
		if len(candidates) == 0 {
			continue
		}
		reason := ""
		for _, m := range candidates {
			reason = promote.IneligibleReason(memory.Memory{Pinned: m.Pinned, TopicKey: m.TopicKey})
			if reason == "" {
				break
			}
		}
		if reason == "" {
			continue
		}
		m := candidates[0]
		findings = append(findings, adviceFor(p.address, m, reason, legacy))
	}
	if len(findings) == 0 {
		return Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}
	}
	return Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: strings.Join(findings, "; ")}
}

// adviceFor words one finding. A page that names its memorycard memory is refreshed by promoting that memory again
// or kept current by pinning it. A page of an earlier release is neither: promote refuses it as bridged until the
// one-time migration gives it its memorycard identity, so the advice names no promote command, only the pin that
// takes effect after the migration.
func adviceFor(address string, m MemoryEligibility, reason string, legacy bool) string {
	if legacy {
		return fmt.Sprintf("%s: memory %d %s, so sync does not refresh this page; the page predates memorycard identity and is refreshed only after the one-time vault migration; pin memory %d in memorycard so sync keeps it current after it", address, m.ID, reason, m.ID)
	}
	return fmt.Sprintf("%s: memory %d %s, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id %d` to refresh it now, or pin memory %d in memorycard so sync keeps it current", address, m.ID, reason, m.ID, m.ID)
}
