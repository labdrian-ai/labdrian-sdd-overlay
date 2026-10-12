package query

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// mergeResults implements the amended R-006: vault rows precede Engram
// rows only when the vault is a requested source (matchLinkedObservation
// is the matcher; it consults both Engram-backed arms, see JD-4 there),
// and Engram's own requested sources are round-robin-interleaved by
// interleaveEngramSources, deduplicated by engram_id. When both engram-fts
// and engram-embed are requested, routeRank1 (R-058/R-059, Branch A: gate
// wired live, published routing accuracy 89%/86%) decides which of the two
// sources' own rows are offered first each round -- the only thing that
// decides is which row lands at rank 1; interleaveEngramSources itself
// never consults it, so the union guarantee is untouched by the decision.
func mergeResults(wantVault, wantFTS, wantEmbed bool, vaultRows []vault.Candidate, engramRows []memory.Row, embedRows []ResultRow, resolveLink func(string) (int64, bool), queryText, matchMode string) []ResultRow {
	consumed := make(map[int64]bool, len(engramRows)+len(embedRows))
	var merged []ResultRow

	if wantVault {
		for _, c := range vaultRows {
			if id, title, ok := matchLinkedObservation(c.PageAddress, engramRows, embedRows, resolveLink); ok && !consumed[id] {
				consumed[id] = true
				merged = append(merged, ResultRow{
					Sources: []string{SourceLinked}, PageAddress: c.PageAddress, PagePath: c.AbsolutePath,
					EngramID: id, Title: title, Snippet: c.Snippet,
					Score: &Score{BM25: c.BM25Score, Rerank: c.RerankScore},
				})
				continue
			}
			merged = append(merged, ResultRow{
				Sources: []string{SourceVault}, PageAddress: c.PageAddress, PagePath: c.AbsolutePath, Snippet: c.Snippet,
				Score: &Score{BM25: c.BM25Score, Rerank: c.RerankScore},
			})
		}
	}

	var engramSourceList []engramSourceRows

	if wantFTS {
		var ftsRows []ResultRow
		for _, er := range engramRows {
			if consumed[er.ID] {
				continue
			}
			ftsRows = append(ftsRows, ResultRow{
				EngramID: er.ID, Title: er.Title,
				Snippet: er.Snippet, SnippetTruncated: er.SnippetTruncated, FullLength: er.ContentLength,
				MatchOffset: er.MatchOffset, Content: er.Content,
			})
		}
		engramSourceList = append(engramSourceList, engramSourceRows{name: SourceEngramFTS, rows: ftsRows})
	}
	if wantEmbed {
		var embRows []ResultRow
		for _, er := range embedRows {
			if consumed[er.EngramID] {
				continue
			}
			embRows = append(embRows, er)
		}
		engramSourceList = append(engramSourceList, engramSourceRows{name: SourceEngramEmbed, rows: embRows})
	}

	if wantFTS && wantEmbed {
		// engramSourceList is [FTS, Embed] by construction above; swap
		// only when the gate names the embedding arm as rank 1's owner.
		tokens, _ := memory.SearchTokens(queryText)
		if routeRank1(tokens, matchMode) == SourceEngramEmbed {
			engramSourceList[0], engramSourceList[1] = engramSourceList[1], engramSourceList[0]
		}
	}
	merged = append(merged, interleaveEngramSources(engramSourceList)...)

	for i := range merged {
		merged[i].Rank = i + 1
	}
	return merged
}

// engramSourceRows is one requested Engram-backed source's own ranked
// rows, already in ResultRow shape (Sources not yet set).
type engramSourceRows struct {
	name string
	rows []ResultRow
}

// interleaveEngramSources round-robins across sources's own rows,
// preserving each source's native rank order as a subsequence of the
// result and deduplicating by EngramID -- the first occurrence is kept
// and every source that also produced it is added to its Sources (R-006's
// "a row found by both sources appears once, at its earliest rank").
func interleaveEngramSources(sources []engramSourceRows) []ResultRow {
	idx := make([]int, len(sources))
	seen := make(map[int64]int, len(sources))
	var merged []ResultRow
	for {
		progressed := false
		for s := range sources {
			if idx[s] >= len(sources[s].rows) {
				continue
			}
			progressed = true
			row := sources[s].rows[idx[s]]
			idx[s]++
			if pos, ok := seen[row.EngramID]; ok {
				merged[pos].Sources = appendSourceOnce(merged[pos].Sources, sources[s].name)
				continue
			}
			seen[row.EngramID] = len(merged)
			row.Sources = []string{sources[s].name}
			merged = append(merged, row)
		}
		if !progressed {
			break
		}
	}
	return merged
}

// appendSourceOnce appends name to list unless it is already there.
func appendSourceOnce(list []string, name string) []string {
	for _, s := range list {
		if s == name {
			return list
		}
	}
	return append(list, name)
}

// matchLinkedObservation resolves pageAddress's linked observation and
// finds it among the rows EITHER Engram-backed arm returned.
//
// It consults both arms because the linked-pair rule is about the
// observation, not about which retriever happened to surface it. Checking
// only the FTS rows -- which is what this did until JD-4 -- shipped the
// same observation twice whenever the embedding arm was the one that found
// it: once as a vault row and once as an engram_embed row. That failure
// got WORSE the better paraphrase retrieval worked, since the paraphrase
// queries the embedding arm exists to answer are exactly the ones FTS
// misses, so the duplication would have arrived with the feature.
//
// FTS is searched first so that when both arms hold the observation the
// consumed row is the FTS one, matching the pre-JD-4 behaviour exactly;
// the embed copy is then dropped by the consumed check downstream.
func matchLinkedObservation(pageAddress string, engramRows []memory.Row, embedRows []ResultRow, resolveLink func(string) (int64, bool)) (int64, string, bool) {
	if resolveLink == nil {
		return 0, "", false
	}
	id, ok := resolveLink(pageAddress)
	if !ok {
		return 0, "", false
	}
	for _, row := range engramRows {
		if row.ID == id {
			return row.ID, row.Title, true
		}
	}
	for _, row := range embedRows {
		if row.EngramID == id {
			return row.EngramID, row.Title, true
		}
	}
	return 0, "", false
}
