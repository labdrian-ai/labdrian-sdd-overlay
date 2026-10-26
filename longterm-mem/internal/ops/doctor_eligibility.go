package ops

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// ObservationEligibility is what promotion's eligibility rule (R-007, promote.Eligible) reads of one observation:
// whether it is pinned and its topic_key, with the id that says whose they are. The package owns it and states
// the port that returns it in these plain types: which store holds the observation, and which columns it reads
// them from, are the adapter's to say.
type ObservationEligibility struct {
	// ID is the observation's id, the engram_id of the page promoted from it.
	ID int64
	// Pinned is whether the observation is pinned in Engram.
	Pinned bool
	// TopicKey is the observation's topic_key, empty when it has none.
	TopicKey string
}

// errNoObservationEligibilityReader is what the promoted-pages-eligible check reports when Doctor was built
// without a LiveObservationEligibility and there is a promoted page to judge: a port the caller forgot to wire is
// named.
var errNoObservationEligibilityReader = errors.New("ops: no observation eligibility reader was wired")

// checkPromotedPagesEligible reports, as an advisory warning, every promoted page whose observation promotion's
// own rule no longer accepts: promote.Eligible(obs, false) is false for it, so sync skips the page and it stays
// as it was written, however the observation changes (issue #293). Such a page is frozen, and the finding names
// it, the observation, why the observation is not eligible and the two ways to deal with it: promote it again
// by hand, which is explicit and so eligible, or pin the observation so that sync keeps the page current.
//
// The pages are joined to their observations by the page's engram_id. Nothing on a page records that it was
// promoted explicitly, so a page that `promote --id N` wrote for an observation that never was eligible is
// reported like any other, until the observation is pinned. A page that names no observation, or whose
// observation is not among the live ones of the project (deleted since, or the row of another project), has no
// observation to be judged by, and is not this check's concern.
//
// The check answers PASS or warn and never FAIL, in any state it finds the vault in: it is a notice, not a
// verdict about the vault, so it can never be the reason the command's exit code says a vault has a problem. A
// vault it cannot judge -- the pages cannot be listed, the live observations cannot be read, a port was not
// wired -- is a warning that says so, in the words of what failed. A page that cannot be read is the page
// checks' finding, which they word, and does not stop the pages after it from being judged.
//
// The live observations are asked for once, and only when a page names an observation to look up.
func checkPromotedPagesEligible(promoted PageReader, eligibility func(project string) ([]ObservationEligibility, error), project string) Check {
	cannotBeJudged := func(err error) Check {
		return Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: "eligibility cannot be judged: " + err.Error()}
	}

	pages, _, err := loadPromotedPages(promoted)
	if err != nil {
		return cannotBeJudged(err)
	}
	type promotion struct {
		address       string
		observationID int64
	}
	var promotions []promotion
	for _, page := range pages {
		if id, named := promote.PageObservationID(page); named {
			promotions = append(promotions, promotion{address: page.Address, observationID: id})
		}
	}
	if len(promotions) == 0 {
		return Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}
	}

	if eligibility == nil {
		return cannotBeJudged(errNoObservationEligibilityReader)
	}
	live, err := eligibility(project)
	if err != nil {
		return cannotBeJudged(fmt.Errorf("could not read the live observations of %s: %w", project, err))
	}
	liveByID := make(map[int64]ObservationEligibility, len(live))
	for _, observation := range live {
		liveByID[observation.ID] = observation
	}

	var findings []string
	for _, p := range promotions {
		observation, isLive := liveByID[p.observationID]
		if !isLive {
			continue
		}
		reason := promote.IneligibleReason(memory.Observation{ID: observation.ID, Pinned: observation.Pinned, TopicKey: observation.TopicKey})
		if reason == "" {
			continue
		}
		findings = append(findings, fmt.Sprintf(
			"%s: observation %d %s, so sync does not refresh this page; run `longterm-mem promote --id %d` to refresh it now, or pin observation %d in Engram so sync keeps it current",
			p.address, observation.ID, reason, observation.ID, observation.ID))
	}
	if len(findings) == 0 {
		return Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}
	}
	return Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: strings.Join(findings, "; ")}
}
