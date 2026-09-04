// Package promote implements longterm-mem's promotion writer: which
// Engram observations are eligible (R-007) and how an eligible observation
// becomes a contract-conformant vault page (R-027).
package promote

import "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"

// eligibleTypes are the Engram observation types R-007 treats as
// automatically eligible regardless of pin state or revision count.
var eligibleTypes = map[string]bool{
	"decision":     true,
	"architecture": true,
	"pattern":      true,
}

// minEligibleRevisionCount is R-007's revision-count eligibility threshold.
const minEligibleRevisionCount = 3

// Eligible reports whether obs is eligible for promotion (R-007): pinned,
// OR of an eligible type, OR at/above the revision-count threshold, OR
// explicitly targeted by a promote call, which overrides every other
// criterion.
func Eligible(obs engram.Observation, explicit bool) bool {
	if explicit {
		return true
	}
	return obs.Pinned || eligibleTypes[obs.Type] || obs.RevisionCount >= minEligibleRevisionCount
}
