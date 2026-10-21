package pipkg

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// sharedCitation is a shared document a skill names, with the skill that names it.
type sharedCitation struct{ skill, name string }

// sharedCitationRE matches a repository path of a shared document, skills/_shared/<name>.md.
// The name is one path element, so a citation cannot reach outside skills/_shared.
var sharedCitationRE = regexp.MustCompile(`skills/_shared/([A-Za-z0-9][A-Za-z0-9._-]*\.md)`)

// citedSharedDocuments returns, sorted and without repeats, the shared documents the markdown
// files of the skill directory src cite.
func citedSharedDocuments(src string) ([]string, error) {
	seen := map[string]bool{}
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range sharedCitationRE.FindAllSubmatch(data, -1) {
			seen[string(m[1])] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading the shared documents the skill cites: %w", err)
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
