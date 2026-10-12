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
//
// An index that is there and cannot be read leaves the embedding source out as one nobody built does, and says
// so in the diagnostic it returns (unreadableIndexDiagnostic).
func defaultSources(deps Deps, project string) ([]string, []Diagnostic) {
	_, err := loadEmbeddingIndex(deps.Index, project)
	if err == nil {
		return []string{SourceEngramFTS, SourceEngramEmbed}, nil
	}
	if d, unreadable := unreadableIndexDiagnostic(project, err); unreadable {
		return []string{SourceEngramFTS}, []Diagnostic{d}
	}
	return []string{SourceEngramFTS}, nil
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
