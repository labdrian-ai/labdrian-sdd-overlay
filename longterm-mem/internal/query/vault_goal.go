package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// VaultGoalCandidateBound caps how many candidates the vault is asked for under a goal (ADR C23 M5, decision 4).
// Under a goal the vault is asked for three times top, so that a goal among many pages of other goals still gets
// its share once those are left out; the bound keeps that from growing with top. 100 leaves every top up to 33
// its full threefold (the default 5 asks for 15) and keeps the cost bounded where it is paid: the vault script
// reranks every candidate it returns, and each one is a frontmatter read here. 3 x the CLI's largest --top (50)
// would be 150.
const VaultGoalCandidateBound = 100

// vaultGoalOverfetch is how many times top the vault is asked for under a goal (decision 4).
const vaultGoalOverfetch = 3

// VaultPageTags is the port through which the vault source reads the frontmatter tags of a page the vault offered,
// to tell under a goal which pages belong to another goal. The package owns it; an adapter that reads the page
// file satisfies it (internal/vault), so the query never reads a file. An error is a page whose tags cannot be
// read.
type VaultPageTags interface {
	PageTags(ctx context.Context, candidate VaultCandidate) ([]string, error)
}

// vaultRetrievalSize is how many candidates the vault is asked for: top without a goal, exactly as before goals;
// three times top under one, up to VaultGoalCandidateBound.
func vaultRetrievalSize(goal string, top int) int {
	if goal == "" {
		return top
	}
	return min(vaultGoalOverfetch*top, VaultGoalCandidateBound)
}

// keepGoalPages keeps, in the vault's order, the first top candidates whose tags name goal or name no goal
// (memory.TagsVisibleUnderGoal). A page whose tags cannot be read is left out, because it may be another goal's and
// a goal's query must not show another goal's records, and the pages left out that way are named in one
// diagnostic so the narrowing is not silent. Only the candidates needed to fill top are read.
func keepGoalPages(ctx context.Context, tags VaultPageTags, candidates []VaultCandidate, goal string, top int) ([]VaultCandidate, []Diagnostic) {
	var kept []VaultCandidate
	var unreadable []string
	for _, c := range candidates {
		if len(kept) >= top {
			break
		}
		pageTags, err := readPageTags(ctx, tags, c)
		if err != nil {
			unreadable = append(unreadable, fmt.Sprintf("%s (%v)", c.PageAddress, err))
			continue
		}
		if memory.TagsVisibleUnderGoal(pageTags, goal) {
			kept = append(kept, c)
		}
	}
	if len(unreadable) == 0 {
		return kept, nil
	}
	return kept, []Diagnostic{{
		Code: DiagnosticVaultPageTagsUnreadable,
		Detail: fmt.Sprintf("under goal %q these vault pages were left out because their frontmatter tags could not be read, so whether they belong to another goal is unknown: %s",
			goal, strings.Join(unreadable, "; ")),
	}}
}

// readPageTags reads c's tags through the port, or answers errNoVaultTags when none was wired.
func readPageTags(ctx context.Context, tags VaultPageTags, c VaultCandidate) ([]string, error) {
	if memory.IsMissing(tags) {
		return nil, errNoVaultTags
	}
	return tags.PageTags(ctx, c)
}

// errNoVaultTags is what a page's tags are when no VaultPageTags was wired.
var errNoVaultTags = fmt.Errorf("query: no reader of vault page tags is configured")
