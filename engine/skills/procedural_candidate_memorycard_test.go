package skills

import (
	"strings"
	"testing"
)

// The procedural-candidates skill and its shared contract record candidates and drafts with memorycard
// (T23). These tests pin the text an agent reads; the engine side that counts translated legacy ids is
// ParseCandidateRecordTranslated (candidate_legacy_test.go) and must agree with it.

func proceduralCandidateTexts(t *testing.T) (skill, contract string) {
	t.Helper()
	repoRoot := ooQualityContractRepoRoot(t)
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	return norm(readRepoFile(t, repoRoot, "skills/procedural-candidates/SKILL.md")),
		norm(readRepoFile(t, repoRoot, "skills/_shared/procedural-candidate-detection.md"))
}

func requireText(t *testing.T, name, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("%s must contain %q", name, want)
		}
	}
}

func forbidText(t *testing.T, name, text string, banned ...string) {
	t.Helper()
	for _, b := range banned {
		if strings.Contains(text, b) {
			t.Errorf("%s must not contain %q", name, b)
		}
	}
}

// Candidates and drafts are saved with memorycard and no new Engram write is told to the agent.
func TestProceduralCandidatesRecordWithMemorycardAndWriteNothingToEngram(t *testing.T) {
	skill, contract := proceduralCandidateTexts(t)

	requireText(t, "SKILL.md", skill,
		"memorycard `mem_save`",
		"beside `mem_session`",
		"never write to Engram",
	)
	forbidText(t, "SKILL.md", skill, "Engram `mem_save`", "mem_session_summary")

	requireText(t, "contract", contract,
		"Candidate records persist in memorycard, the sole durability store for this capability",
		"no new Engram write",
		"Each candidate is one memorycard memory",
		"- memorycard:3401 @ 2026-09-15T10:04:00Z",
		"| none | `observing` | agent | none | first occurrence (item 30) | `memorycard:<id>` |",
		"`mem_get` for every `memorycard:<m>` listed in `Occurrences`",
		"Every draft is stored as a memorycard memory",
		"beside `mem_session`",
	)
	forbidText(t, "contract", contract,
		"mem_get_observation",
		"mem_session_summary",
		"`engram:<id>`",
		"Engram observation",
		"sole durability store for this capability; no second SQLite store and no Go-side write path into Engram",
	)
}

// A legacy `engram:<n>` is the same occurrence as the `memorycard:<m>` the import mapped it to, and the
// counts use distinct translated ids, as the engine does.
func TestProceduralCandidateContractTranslatesLegacyOccurrenceIDsOnRead(t *testing.T) {
	skill, contract := proceduralCandidateTexts(t)

	requireText(t, "contract", contract,
		"memorycard resolve --source engram --key <n>",
		"[--key <n2> ...] --json",
		"an `engram:<n>` that `memorycard resolve` maps to `<m>` is the same occurrence as `memorycard:<m>`",
		"appending one while the other is listed is the same no-op",
		"is always derived**, equal to the number of distinct translated ids currently listed in `Occurrences`",
		"Two Engram ids the import merged into one memory resolve to the same `<m>` and count once",
		"id `resolve` reports unknown (`\"memory_id\": null`) stays as written and counts as itself",
		"legacy occurrence is read by resolving it, then `mem_get` with the `<m>` it maps to",
		"`OccurrencesSincePromotion` counts distinct translated ids",
	)
	requireText(t, "SKILL.md", skill,
		"that `memorycard resolve` (step 6) maps to `<m>` is the same occurrence as `memorycard:<m>`",
		"count distinct translated ids",
	)
}

// The record's next save rewrites each resolvable legacy id; there is no bulk migration.
func TestProceduralCandidateContractRewritesLegacyOccurrenceIDsOnSave(t *testing.T) {
	skill, contract := proceduralCandidateTexts(t)

	requireText(t, "contract", contract,
		"On every save of a record, rewrite each legacy `engram:<n>` in `Occurrences` that `memorycard resolve` maps to `<m>` as `memorycard:<m>`",
		"keeping its timestamp and summary",
		"When a rewrite makes two `Occurrences` entries one id, they are one observation: keep the entry with the earliest timestamp (not its list position), drop the other, and recompute `FirstObserved` and `LastObserved` from the remaining entries",
		"`OccurrenceCount` and `OccurrencesSincePromotion` are computed after the merge",
		"id reported unknown stays as written",
		"`History` lines are never rewritten",
		"There is no bulk migration",
		"a record never saved again keeps its legacy ids and is translated on read",
	)
	requireText(t, "SKILL.md", skill,
		"On every save of a candidate record, rewrite each legacy `engram:<n>` in `Occurrences` that `memorycard resolve` maps to `<m>` as `memorycard:<m>`",
		"id reported unknown stays as written",
		"migrate records in bulk",
		"keep the earliest-timestamp entry (shared contract section 3)",
		"If `resolve` fails or answers a different source, count or order, stop occurrence work and report it",
	)
}

// The full envelope, what a legacy id is, and when resolve is never called.
func TestProceduralCandidateContractNamesTheResolveEnvelope(t *testing.T) {
	skill, contract := proceduralCandidateTexts(t)
	requireText(t, "contract", contract,
		`{"source":"engram","ids":[{"key":"<n>","memory_id":<m>|null}]}`,
		"one entry per key asked, in the order asked",
		"a different `source`, count or order of keys",
		"A legacy id is `engram:` followed by digits",
		"When a record lists no legacy id, never call `resolve`",
	)
	requireText(t, "SKILL.md", skill, "Never call `resolve` when the record lists no legacy id")
}

// Item 6 and the skill both carry the lifecycle exception, and the merge tie and baseline rules.
func TestProceduralCandidateContractCarriesTheExceptionAndTheMergeTies(t *testing.T) {
	skill, contract := proceduralCandidateTexts(t)
	requireText(t, "contract", contract,
		"keeping its timestamp and summary; when `resolve` failed (item 2), nothing is rewritten and the lifecycle exception of item 2 applies",
		"Ask `resolve` in one call, with each distinct legacy number once, sorted as strings and as written (a number with leading zeros is asked as written)",
		"on equal timestamps keep the entry listed first",
		"Merged entries that straddle a `Registered` or `Promoted` `at:` count once, before the baseline, because they are one observation",
		"A record already `emitted` or `rejected` whose count is at or below N after a merge (section 3) keeps its `Status` and is never re-emitted",
	)
	requireText(t, "SKILL.md", skill,
		"on a resolve failure a lifecycle transition whose external action already happened is still saved with `Occurrences` exactly as stored",
		"Ask `resolve` in one call, with each distinct legacy number once, sorted as strings and as written",
		"sorted as strings and as written: `memorycard resolve --source engram --key <n> [--key <n2> ...] --json`",
	)
	forbidText(t, "contract", contract, "once for all of them")
}

// The stop covers occurrence work only; a lifecycle History line whose action already happened is saved.
func TestProceduralCandidateContractScopesTheResolveStop(t *testing.T) {
	_, contract := proceduralCandidateTexts(t)
	requireText(t, "contract", contract,
		"A lifecycle transition whose external action already happened (`Promoted`, registration, retirement, revision draft opened) is still saved with its `Status` and the `History` evidence and fields its section 9 row requires, with `Occurrences` left exactly as stored and no count change, and the failure reported",
		"On read, entries that translate to one id count once by the merge rule of item 6, so a read and a later save derive the same counts",
	)
}

// Acceptance item 3 no longer says the count is incremented; section 4 says a merge never unlatches Status.
func TestProceduralCandidateContractCountIsRederivedAndMergeKeepsTheLatch(t *testing.T) {
	_, contract := proceduralCandidateTexts(t)
	requireText(t, "contract", contract,
		"`Occurrences`, `OccurrenceCount` re-derived)",
		"A count that falls below `Threshold` after a merge (section 3) leaves the latched `Status` unchanged and is not an anomaly of the record",
	)
	forbidText(t, "contract", contract, "`OccurrenceCount` incremented")
}

// The acceptance checklist covers translate on read, rewrite on save, merge, unknown and resolve failure.
func TestProceduralCandidateContractHasALegacyIDAcceptanceChecklist(t *testing.T) {
	_, contract := proceduralCandidateTexts(t)
	requireText(t, "contract", contract,
		"## Acceptance checklist (legacy occurrence ids, executed during `sdd-verify`)",
		"**Translate on read**",
		"**Rewrite on save**",
		"**Merge keeps the earliest timestamp**",
		"**Unknown stays**",
		"**A failed resolve stops**",
	)
}

// A failed or unavailable resolve never turns legacy ids into unknown ones: the record is left alone.
func TestProceduralCandidateContractStopsWhenResolveFails(t *testing.T) {
	_, contract := proceduralCandidateTexts(t)
	requireText(t, "contract", contract,
		"When `resolve` fails or is unavailable",
		"no legacy id is treated as unknown: stop occurrence work (do not count, append or rewrite) and report the failure.",
	)
}
