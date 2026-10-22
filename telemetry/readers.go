// Package telemetry reads agent usage metadata without exposing conversation content.
package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Call is one normalized assistant usage record. Nil CostUSD means not recorded.
type Call struct {
	Source, Project, SessionID, Provider, Model, Role, AgentType, Day string
	Input, CacheRead, CacheWrite, Output, Reasoning                   int64
	CostUSD                                                           *float64
}

// Stats counts JSONL files, files read to the end, physical lines, emitted
// calls, unusable lines, and repeated IDs. UnreadableFiles counts files that
// could not be opened or completely read (they emit nothing) and directories
// that could not be listed. Non-assistant and usage-free lines count as
// Skipped; meta files do not count.
type Stats struct {
	Files, Read, Lines, Calls, Skipped, Duplicates, UnreadableFiles int64
}

// ReadClaude keeps the last occurrence of each message ID (or UUID) per file.
func ReadClaude(root string, emit func(Call)) (Stats, error) {
	return readLogs(root, "claude", emit)
}

// ReadPi keeps the first occurrence of each response ID across root; lines
// without one fall back to their line ID, deduplicated within their file.
func ReadPi(root string, emit func(Call)) (Stats, error) {
	return readLogs(root, "pi", emit)
}

// Narrow decoding deliberately excludes all conversation and tool content.
type record struct {
	Type, Timestamp, SessionID, UUID, ID string
	IsSidechain                          bool
	Message                              struct {
		ID, Role, Provider, Model, ResponseID string
		Usage                                 *struct {
			Input, Output, CacheRead, CacheWrite, Reasoning int64
			ClaudeInput                                     int64 `json:"input_tokens"`
			ClaudeOutput                                    int64 `json:"output_tokens"`
			ClaudeRead                                      int64 `json:"cache_read_input_tokens"`
			ClaudeWrite                                     int64 `json:"cache_creation_input_tokens"`
			Cost                                            json.RawMessage
		}
	}
}

type entry struct {
	call      Call
	stamp, id string
	local     bool // id is a Pi line ID, unique only within its file
}

func parse(line []byte, source string) (entry, bool) {
	var r record
	if !bytes.Contains(line, []byte(`"usage"`)) || json.Unmarshal(line, &r) != nil || r.Message.Usage == nil {
		return entry{}, false
	}
	u := r.Message.Usage
	c := Call{Source: source, SessionID: r.SessionID, Provider: r.Message.Provider, Model: r.Message.Model, Role: "main"}
	if stamp, err := time.Parse(time.RFC3339Nano, r.Timestamp); err == nil {
		c.Day = stamp.Format("2006-01-02")
	}
	id, local := r.Message.ID, false
	if source == "claude" {
		if r.Type != "assistant" || c.Model == "<synthetic>" {
			return entry{}, false
		}
		if id == "" {
			id = r.UUID
		}
		c.Provider = "anthropic"
		c.Input, c.Output, c.CacheRead, c.CacheWrite = u.ClaudeInput, u.ClaudeOutput, u.ClaudeRead, u.ClaudeWrite
		if r.IsSidechain {
			c.Role = "sub"
		}
	} else {
		if r.Type != "message" || r.Message.Role != "assistant" {
			return entry{}, false
		}
		id = r.Message.ResponseID
		if id == "" {
			id, local = r.ID, true
		}
		c.Input, c.Output, c.CacheRead, c.CacheWrite, c.Reasoning = u.Input, u.Output, u.CacheRead, u.CacheWrite, u.Reasoning
		if len(u.Cost) > 0 && string(u.Cost) != "null" {
			var value float64
			if json.Unmarshal(u.Cost, &value) == nil {
				c.CostUSD = &value
			} else {
				var cost struct{ Total *float64 }
				if json.Unmarshal(u.Cost, &cost) == nil {
					c.CostUSD = cost.Total
				}
			}
		}
	}
	return entry{c, r.Timestamp, id, local}, id != ""
}

func agentType(path string) string {
	f, err := os.Open(strings.TrimSuffix(path, ".jsonl") + ".meta.json")
	if err != nil {
		return ""
	}
	defer f.Close()
	var meta struct{ AgentType string }
	decoder := json.NewDecoder(f)
	if decoder.Decode(&meta) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return ""
	}
	return meta.AgentType
}

// Files are walked lexically; each file emits in timestamp/ID order. Memory is
// one line plus normalized metadata for one file and, for Pi, run-wide seen IDs.
// Conversation payloads are never retained. The caller must not mutate logs
// during a read if it needs reproducible results.
func readLogs(root, source string, emit func(Call)) (Stats, error) {
	var stats Stats
	info, err := os.Stat(root)
	if err != nil {
		return stats, err
	}
	if !info.IsDir() {
		return stats, fmt.Errorf("log root is not a directory: %s", root)
	}
	seen := make(map[string]bool) // Pi response IDs kept by earlier files
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			stats.UnreadableFiles++
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		stats.Files++
		f, err := os.Open(path)
		if err != nil {
			stats.UnreadableFiles++
			return nil
		}
		defer f.Close()
		rel, _ := filepath.Rel(root, path)
		var rows map[string]entry
		var ok bool
		if source == "claude" {
			if rows, ok = readFile(f, source, keepLast, &stats); ok {
				tagClaude(rows, path, rel)
			}
		} else {
			keep := func(rows map[string]entry, e entry) bool { return keepFirst(rows, seen, e) }
			if rows, ok = readFile(f, source, keep, &stats); ok {
				tagPi(rows, d.Name(), seen)
			}
		}
		if !ok {
			stats.UnreadableFiles++
			return nil
		}
		stats.Read++
		project, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		for _, e := range ordered(rows) {
			e.call.Project = project
			emit(e.call)
			stats.Calls++
		}
		return nil
	})
	return stats, err
}

// readFile parses one log, letting keep store each parsed row and report
// whether it repeated an ID. It returns false when the file cannot be read to
// the end; its rows are then dropped so a file counts wholly or not at all.
func readFile(r io.Reader, source string, keep func(map[string]entry, entry) bool, stats *Stats) (map[string]entry, bool) {
	rows := make(map[string]entry)
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			stats.Lines++
			if e, ok := parse(line, source); !ok {
				stats.Skipped++
			} else if keep(rows, e) {
				stats.Duplicates++
			}
		}
		if err == io.EOF {
			return rows, true
		}
		if err != nil {
			return nil, false
		}
	}
}

// keepLast is the Claude rule: the last line per ID in a file wins.
func keepLast(rows map[string]entry, e entry) bool {
	_, dup := rows[e.id]
	rows[e.id] = e
	return dup
}

// keepFirst is the Pi rule: the first line per ID wins. Response IDs are
// unique across the root; fallback line IDs only within their file.
func keepFirst(rows map[string]entry, seen map[string]bool, e entry) bool {
	if _, dup := rows[e.id]; dup || (!e.local && seen[e.id]) {
		return true
	}
	rows[e.id] = e
	return false
}

// tagClaude adds what the file path and its meta.json say about every row.
func tagClaude(rows map[string]entry, path, rel string) {
	agent := agentType(path)
	sub := strings.Contains("/"+filepath.ToSlash(rel), "/subagents/")
	for id, e := range rows {
		e.call.AgentType = agent
		if sub {
			e.call.Role = "sub"
		}
		rows[id] = e
	}
}

// tagPi sets the session from the file name and records the file's response
// IDs only after it was read completely.
func tagPi(rows map[string]entry, name string, seen map[string]bool) {
	_, session, _ := strings.Cut(strings.TrimSuffix(name, ".jsonl"), "_")
	for id, e := range rows {
		e.call.SessionID = session
		rows[id] = e
		if !e.local {
			seen[id] = true
		}
	}
}

func ordered(rows map[string]entry) []entry {
	list := make([]entry, 0, len(rows))
	for _, e := range rows {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].stamp == list[j].stamp {
			return list[i].id < list[j].id
		}
		return list[i].stamp < list[j].stamp
	})
	return list
}
