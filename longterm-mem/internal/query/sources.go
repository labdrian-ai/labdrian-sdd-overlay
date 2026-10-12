package query

// knownSources is every name Request.Sources may contain.
var knownSources = map[string]bool{
	SourceEngramFTS:   true,
	SourceEngramEmbed: true,
	SourceVault:       true,
}

// defaultSources is what Run queries when Request.Sources is empty:
// engram-fts and engram-embed together (R-060's own final shape) the
// moment project's embedding index actually exists, since a caller has no
// way to know from the outside whether one has ever been built -- and
// engram-fts alone otherwise, since asking the embedding arm to run on
// every query for a project with nothing indexed would cost a network
// round trip for a guaranteed-empty answer (embedarm.go's own
// "no index built yet" degradation covers a caller who names engram-embed
// explicitly against such a project; this is only about what an omitted
// Sources defaults to).
//
// The vault is unaffected either way: it is opt-in, queried only when a
// caller names it explicitly (R-060), regardless of what the two Engram
// arms default to. A caller that wants it names it:
// sources: ["vault", "engram-fts"].
func defaultSources(deps Deps, project string) []string {
	if _, err := loadEmbeddingIndex(deps.Index, project); err == nil {
		return []string{SourceEngramFTS, SourceEngramEmbed}
	}
	return []string{SourceEngramFTS}
}

// containsSource reports whether name is in sources.
func containsSource(sources []string, name string) bool {
	for _, s := range sources {
		if s == name {
			return true
		}
	}
	return false
}
