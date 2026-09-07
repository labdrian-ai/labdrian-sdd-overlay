package guard

import (
	"sort"
	"strings"
	"testing"
)

// allowedNetImporters are the ONLY production files permitted to import a
// guardedImports entry (R-071). The embedding client is this module's
// first network dependency; every other file that reaches out over
// net/http or net is exactly the workaround that would defeat this guard,
// so the boundary is held here the same way R-021 holds os/exec.
//
// Adding a second entry is a change to the egress boundary, not a
// convenience — the same seriousness exec_allowlist_test.go asks for R-021.
var allowedNetImporters = map[string]bool{
	"internal/embed/client.go": true,
}

// guardedImports are the import paths this test refuses outside
// allowedNetImporters. "net" is guarded alongside "net/http" because raw
// net.Dial is the same egress boundary and is precisely the workaround
// that would defeat an http-only guard. "net/url" is deliberately NOT
// guarded: it parses and cannot itself egress.
var guardedImports = []string{"net/http", "net"}

// TestNetImportAllowlist statically parses every non-test .go file under
// the module root and fails if any file other than one named in
// allowedNetImporters imports a guardedImports entry (R-071).
func TestNetImportAllowlist(t *testing.T) {
	for _, importPath := range guardedImports {
		offenders, totalFiles, err := findImporters(".", importPath)
		if err != nil {
			t.Fatalf("walking module root for %q imports: %v", importPath, err)
		}
		if totalFiles == 0 {
			t.Fatal("no production .go files found under '.'; the allowlist walk may be broken")
		}
		for _, offender := range offenders {
			if !allowedNetImporters[offender] {
				t.Errorf("forbidden %q import in %s — only %s may reach the network (R-071)", importPath, offender, allowedNetNames())
			}
		}
	}
}

// allowedNetNames renders the allowlist for a failure message, sorted so
// the message is stable.
func allowedNetNames() string {
	names := make([]string, 0, len(allowedNetImporters))
	for name := range allowedNetImporters {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// TestNetImportAllowlistStillRefusesOthers: widening an allowlist is the
// easy way to turn a guard into a rubber stamp. This asserts the list is a
// list and not a shrug — three plausible other files (the vector index
// builder, the query package that must never egress mid-query, and the CLI
// entrypoint) are still forbidden, and the list holds exactly the one file
// this PR introduces.
func TestNetImportAllowlistStillRefusesOthers(t *testing.T) {
	for _, path := range []string{
		"internal/vecindex/build.go",
		"internal/query/query.go",
		"cmd/longterm-mem/main.go",
	} {
		if allowedNetImporters[path] {
			t.Errorf("%s must not be allowed to reach the network (R-071)", path)
		}
	}
	if len(allowedNetImporters) != 1 {
		t.Errorf("the allowlist holds %d entries; every addition is a change to the egress boundary and must be argued for, not slipped in", len(allowedNetImporters))
	}
}
