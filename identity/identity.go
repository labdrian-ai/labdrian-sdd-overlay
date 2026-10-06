// Package identity holds the rules that turn what a git repository says about
// itself into the name of its project. It is pure: strings in, strings out, no
// file, no process, no clock, and nothing but the standard library, so the engine
// and longterm-mem both depend on it and neither depends on the other.
//
// Reading the repository (finding .git, following a linked worktree to its common
// directory, opening the config file) is not here. Each module keeps its own
// reader, an adapter that hands the text it read to the rules below, so the rule
// that two modules must agree on has one owner and the I/O that differs between
// them stays where it belongs (Phase 9, decision D2).
package identity

import "strings"

// OriginRemote reads the text of a git config file and returns the normalized
// form (see NormalizeRemote) of the URL of the remote called origin. It reports
// false when the config names no origin, or when none of its urls has a host to
// key on.
//
// It parses the one section header and the one key it needs rather than the whole
// grammar of a git config: [remote "origin"] and the equivalent [remote.origin]
// spelling, the section name case-insensitively as git is, the key url in any
// case, and the comment lines # and ;. The first url of the section that
// normalizes to something wins, so a local-path url ahead of a hosted one is
// skipped, which is the answer this rule has always given.
func OriginRemote(config string) (string, bool) {
	inOrigin := false
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inOrigin = sectionIsOrigin(line)
			continue
		}
		if !inOrigin {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.ToLower(strings.TrimSpace(key)) != "url" {
			continue
		}
		if normalized := NormalizeRemote(value); normalized != "" {
			return normalized, true
		}
	}
	return "", false
}

// sectionIsOrigin recognizes `[remote "origin"]` and the equivalent
// `[remote.origin]` spelling, case-insensitively on the section name as
// git itself is.
func sectionIsOrigin(header string) bool {
	inner := strings.TrimSpace(strings.Trim(header, "[]"))
	inner = strings.ReplaceAll(inner, "\"", "")
	inner = strings.ReplaceAll(inner, ".", " ")
	fields := strings.Fields(inner)
	return len(fields) == 2 && strings.EqualFold(fields[0], "remote") && fields[1] == "origin"
}

// NormalizeRemote reduces a remote URL to its normal form, "host/path":
// the host lowercased, the path stripped of its leading slash, any
// trailing slash and any ".git" suffix. All of
//
//	git@github.com:acme/widgets.git
//	https://github.com/acme/widgets
//	https://github.com/acme/widgets.git
//	ssh://git@github.com/acme/widgets.git
//
// collapse to "github.com/acme/widgets". The path's case is preserved --
// forge hosts differ on whether it is significant, and folding it would
// merge two repositories that a case-sensitive host keeps apart, which is
// the one mistake this rule must never make.
//
// A local-filesystem remote ("/srv/git/widgets.git", "../widgets") has no
// host to key on; it returns "" so the caller falls through to the next rule,
// which for a local remote is the more stable answer anyway.
//
// Two normalizations are deliberately NOT done, because each would merge
// what might be two repositories, and this rule's one unforgivable
// mistake is merging: a port is kept on the host ("host:2222/x" stays
// distinct from "host/x", since a second daemon on one machine is a
// different forge), and a ".GIT" suffix is left in place, since the path's
// case is preserved for the same reason. Both cost at worst a second
// identity for one repository -- visible, and fixable with a declared
// name -- where folding them costs one identity for two repositories,
// which is silent.
func NormalizeRemote(url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return ""
	}

	var host, path string
	if scheme, rest, ok := strings.Cut(url, "://"); ok {
		if strings.EqualFold(scheme, "file") {
			return ""
		}
		hostPart, p, _ := strings.Cut(rest, "/")
		host, path = hostPart, p
	} else if before, after, ok := strings.Cut(url, ":"); ok && !strings.Contains(before, "/") {
		// scp-like: [user@]host:path
		host, path = before, after
	} else {
		return ""
	}

	if _, h, ok := strings.Cut(host, "@"); ok {
		host = h
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}

	path = strings.Trim(strings.TrimSpace(path), "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	return host + "/" + path
}
