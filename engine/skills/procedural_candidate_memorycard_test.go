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
		`{"key": "<n>", "memory_id": <m>}`,
		"an `engram:<n>` that `memorycard resolve` maps to `<m>` is the same occurrence as `memorycard:<m>`",
		"appending one while the other is listed is the same no-op",
		"is always derived**, equal to the number of distinct translated ids currently listed in `Occurrences`",
		"Two Engram ids the import merged into one memory resolve to the same `<m>` and count once",
		"id `resolve` reports unknown (`\"memory_id\": null`) stays as written and counts as itself",
		"legacy occurrence is read by resolving it, then `mem_get` with the `<m>` it maps to",
		"`OccurrencesSincePromotion` counts distinct translated ids",
	)
	requireText(t, "SKILL.md", skill,
		"that `memorycard resolve --source engram --key <n> --json` maps to `<m>` is the same occurrence as `memorycard:<m>`",
		"count distinct translated ids",
	)
}

// The record's next save rewrites each resolvable legacy id; there is no bulk migration.
func TestProceduralCandidateContractRewritesLegacyOccurrenceIDsOnSave(t *testing.T) {
	skill, contract := proceduralCandidateTexts(t)

	requireText(t, "contract", contract,
		"On every save of a record, rewrite each legacy `engram:<n>` in `Occurrences` that `memorycard resolve` maps to `<m>` as `memorycard:<m>`",
		"keeping its timestamp and summary",
		"when two entries become one id, keep the earlier entry",
		"id reported unknown stays as written",
		"`History` lines are never rewritten",
		"There is no bulk migration",
		"a record never saved again keeps its legacy ids and is translated on read",
	)
	requireText(t, "SKILL.md", skill,
		"On every save of a candidate record, rewrite each legacy `engram:<n>` in `Occurrences` that `memorycard resolve` maps to `<m>` as `memorycard:<m>`",
		"id reported unknown stays as written",
		"migrate records in bulk",
	)
}
