package agentgen

// The tier x runtime table of skills/role-chain/references/models.md (S18): the one place that says
// which model each tier runs on each runtime. The generator reads it, so no id is written twice.
//
// The table is the Markdown table under the `## Tier table` heading: a header row `| tier | <runtime> | ... |`,
// a separator row, then one row per tier, one column per runtime, each cell the id, in backticks or bare.
// It ends at the first line that does not start with `|`; the next heading ends the section,
// and the heading and the rows are matched without surrounding space or carriage return.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// modelsPath is the file that holds the table, as a slash-separated path under the root of the
// repository.
const modelsPath = "skills/role-chain/references/models.md"

// The runtimes the generator needs an id for. OpenCode is read by the generator of its agent files.
const (
	runtimeClaude   = "claude"
	runtimePi       = "pi"
	runtimeOpenCode = "opencode"
)

var (
	requiredTiers    = []string{tierFrontier, tierMid, tierLight}
	requiredRuntimes = []string{runtimeClaude, runtimePi, runtimeOpenCode}
)

// ModelTable maps a tier and a runtime to a model id.
type ModelTable struct {
	ids map[string]map[string]string
}

// ID is the model id of the tier on the runtime. It fails for a tier or runtime the table does not
// hold.
func (t ModelTable) ID(tier, runtime string) (string, error) {
	id, ok := t.ids[tier][runtime]
	if !ok {
		return "", fmt.Errorf("the tier table of %s has no id for the %s tier on %s", modelsPath, tier, runtime)
	}
	return id, nil
}

// ReadModelTable reads and checks the table of the repository at repoRoot. It fails when the file or
// the table is missing, when a row or column is malformed, or when a tier or runtime the generator
// needs is absent or has an empty id.
func ReadModelTable(repoRoot string) (ModelTable, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(modelsPath)))
	if err != nil {
		return ModelTable{}, err
	}
	table, err := parseModelTable(string(data))
	if err != nil {
		return ModelTable{}, &tableError{err}
	}
	return table, nil
}

// tableError is a table that was read but cannot be used, as opposed to a file that could not be read.
type tableError struct{ err error }

func (e *tableError) Error() string { return e.err.Error() }
func (e *tableError) Unwrap() error { return e.err }

const tableHeading = "## Tier table"

// idShape is what a model id may hold: it is written verbatim into the frontmatter of a file.
var idShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:#-]*$`)

var separatorCell = regexp.MustCompile(`^:?-+:?$`)

func parseModelTable(text string) (ModelTable, error) {
	lines := strings.Split(text, "\n")
	start := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == tableHeading })
	if start < 0 {
		return ModelTable{}, fmt.Errorf("%s has no %q section", modelsPath, tableHeading)
	}
	// The header is the first table row of the section; the section ends at the next heading.
	header := -1
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "#") {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
			header = i
			break
		}
	}
	if header < 0 {
		return ModelTable{}, fmt.Errorf("the tier table of %s is missing: no table under %q", modelsPath, tableHeading)
	}
	head := splitRow(lines[header])
	if head[0] != "tier" {
		return ModelTable{}, fmt.Errorf("the tier table of %s must start with the header row `| tier | ... |`, got %q", modelsPath, lines[header])
	}
	runtimes := head[1:]
	for i, runtime := range runtimes {
		if slices.Contains(runtimes[:i], runtime) {
			return ModelTable{}, fmt.Errorf("the tier table of %s has a duplicate %s column", modelsPath, runtime)
		}
	}
	if header+1 >= len(lines) || !isSeparator(splitRow(lines[header+1]), len(head)) {
		return ModelTable{}, fmt.Errorf("the tier table of %s has no separator row of %d cells (`| --- | ...`) under its header", modelsPath, len(head))
	}
	ids := map[string]map[string]string{}
	for _, line := range lines[header+2:] {
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			break
		}
		cells := splitRow(line)
		if len(cells) != len(head) {
			return ModelTable{}, fmt.Errorf("the tier table of %s has a row of %d cells where the header has %d: %q", modelsPath, len(cells), len(head), line)
		}
		if _, dup := ids[cells[0]]; dup {
			return ModelTable{}, fmt.Errorf("the tier table of %s has a duplicate %s row", modelsPath, cells[0])
		}
		ids[cells[0]] = map[string]string{}
		for i, runtime := range runtimes {
			ids[cells[0]][runtime] = cells[i+1]
		}
	}
	for _, runtime := range requiredRuntimes {
		if !slices.Contains(runtimes, runtime) {
			return ModelTable{}, fmt.Errorf("the tier table of %s has no %s column", modelsPath, runtime)
		}
	}
	for _, tier := range requiredTiers {
		row, ok := ids[tier]
		if !ok {
			return ModelTable{}, fmt.Errorf("the tier table of %s has no %s row", modelsPath, tier)
		}
		for _, runtime := range requiredRuntimes {
			if row[runtime] == "" {
				return ModelTable{}, fmt.Errorf("the tier table of %s has no id for the %s tier on %s", modelsPath, tier, runtime)
			}
			if !idShape.MatchString(row[runtime]) {
				return ModelTable{}, fmt.Errorf("the tier table of %s has an id for the %s tier on %s that is not a model id: %q", modelsPath, tier, runtime, row[runtime])
			}
		}
	}
	return ModelTable{ids: ids}, nil
}

func isSeparator(cells []string, n int) bool {
	if len(cells) != n {
		return false
	}
	for _, c := range cells {
		if !separatorCell.MatchString(c) {
			return false
		}
	}
	return true
}

// splitRow is the cells of a table row, trimmed and without backticks; nil for a line that is not a row.
func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil
	}
	line = strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")
	cells := strings.Split(line, "|")
	for i, cell := range cells {
		cells[i] = strings.Trim(strings.TrimSpace(cell), "`")
	}
	return cells
}
