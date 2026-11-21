package memorycardcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Resolver asks memorycard's read-only `resolve` verb which memory each legacy Engram
// observation number was imported as. It implements skills.LegacyOccurrenceResolver and
// reads no database itself.
type Resolver struct {
	Executable string
	Runner     CommandRunner
}

// ResolveLegacyOccurrences runs one `memorycard resolve --source engram --key <n> ... --json` for
// all numbers and returns number -> memory id for the numbers the import mapped. A null
// memory_id is an unknown key, which is not an error. An answer that is not the one asked
// for (another source, other keys, another order) is refused.
func (r Resolver) ResolveLegacyOccurrences(numbers []string) (map[string]int, error) {
	if r.Runner == nil {
		return nil, fmt.Errorf("memorycard resolve: command runner is required")
	}
	if len(numbers) == 0 {
		return nil, fmt.Errorf("memorycard resolve: at least one key is required")
	}
	bin := r.Executable
	if bin == "" {
		bin = "memorycard"
	}
	args := []string{"resolve", "--source", legacySource}
	for _, number := range numbers {
		args = append(args, "--key", number)
	}
	args = append(args, "--json")
	stdout, stderr, err := r.Runner.Output(context.Background(), nil, bin, args...)
	if err != nil {
		if detail := strings.TrimSpace(string(stderr)); detail != "" {
			return nil, fmt.Errorf("memorycard resolve: %s: %w", detail, err)
		}
		return nil, fmt.Errorf("memorycard resolve: %w", err)
	}
	mapped, err := decodeResolve(stdout, numbers)
	if err != nil {
		return nil, fmt.Errorf("memorycard resolve: decode: %w", err)
	}
	return mapped, nil
}

// legacySource is the `import_map` source the Engram importer writes.
const legacySource = "engram"

type resolvedKey struct {
	Key      string `json:"key"`
	MemoryID *int   `json:"memory_id"`
}

// decodeResolve refuses an absent or null ids array and an answer whose keys, or whose source,
// differ from what was asked, rather than reading a changed output format as "nothing mapped".
func decodeResolve(data []byte, asked []string) (map[string]int, error) {
	var envelope struct {
		Source string          `json:"source"`
		IDs    json.RawMessage `json:"ids"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.IDs) == 0 || string(envelope.IDs) == "null" {
		return nil, errors.New("resolve has no ids array")
	}
	if envelope.Source != legacySource {
		return nil, fmt.Errorf("resolve answered for source %q, asked %q", envelope.Source, legacySource)
	}
	var rows []resolvedKey
	if err := json.Unmarshal(envelope.IDs, &rows); err != nil {
		return nil, err
	}
	if len(rows) != len(asked) {
		return nil, fmt.Errorf("resolve answered %d keys, asked %d", len(rows), len(asked))
	}
	mapped := map[string]int{}
	for i, row := range rows {
		if row.Key != asked[i] {
			return nil, fmt.Errorf("resolve answered keys that differ from the keys asked: key %d is %q, asked %q", i+1, row.Key, asked[i])
		}
		if row.MemoryID != nil {
			mapped[row.Key] = *row.MemoryID
		}
	}
	return mapped, nil
}
