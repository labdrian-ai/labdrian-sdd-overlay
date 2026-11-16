package agentgen

// The tier x runtime table of skills/role-chain/references/models.md (S18): the one place that says
// which model each tier runs on each runtime. The generator reads it, so no id is written twice.
//
// The table is a Markdown table under the header row `| tier | <runtime> | ... |`: one row per tier,
// one column per runtime, each cell the id, in backticks or bare. It ends at the first line that does
// not start with `|`.

import (
	"fmt"
	"os"
	"path/filepath"
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
	return parseModelTable(string(data))
}

func parseModelTable(text string) (ModelTable, error) {
	lines := strings.Split(text, "\n")
	start := -1
	for i, line := range lines {
		if cells := splitRow(line); len(cells) > 0 && cells[0] == "tier" {
			start = i
			break
		}
	}
	if start < 0 {
		return ModelTable{}, fmt.Errorf("the tier table of %s is missing: no header row `| tier | ... |`", modelsPath)
	}
	runtimes := splitRow(lines[start])[1:]
	ids := map[string]map[string]string{}
	// The line after the header is the separator row; the rows are the lines after it that start with |.
	for _, line := range lines[min(start+2, len(lines)):] {
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			break
		}
		cells := splitRow(line)
		if len(cells) != len(runtimes)+1 {
			return ModelTable{}, fmt.Errorf("the tier table of %s has a row of %d cells where the header has %d: %q", modelsPath, len(cells), len(runtimes)+1, line)
		}
		ids[cells[0]] = map[string]string{}
		for i, runtime := range runtimes {
			ids[cells[0]][runtime] = cells[i+1]
		}
	}
	for _, runtime := range requiredRuntimes {
		if !contains(runtimes, runtime) {
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
		}
	}
	return ModelTable{ids: ids}, nil
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

func contains(list []string, item string) bool {
	for _, s := range list {
		if s == item {
			return true
		}
	}
	return false
}
