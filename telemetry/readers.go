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

// Stats counts JSONL files, physical lines, emitted calls, unusable lines,
// repeated IDs, and files that could not be opened or completely read.
// Non-assistant and usage-free lines count as Skipped; meta files do not count.
type Stats struct {
	Files, Lines, Calls, Skipped, Duplicates, UnreadableFiles int64
}

// ReadClaude keeps the last occurrence of each message ID (or UUID) per file.
func ReadClaude(root string, emit func(Call)) (Stats, error) {
	return readLogs(root, "claude", emit)
}

// ReadPi keeps the first occurrence of each response ID (or line ID) across root.
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
	id := r.Message.ID
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
			id = r.ID
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
	return entry{c, r.Timestamp, id}, id != ""
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
	seen := make(map[string]bool)
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
		parts := strings.Split(filepath.ToSlash(rel), "/")
		project := parts[0]
		rows := make(map[string]entry)
		var add func(entry)
		if source == "claude" {
			agent := agentType(path)
			sub := strings.Contains("/"+filepath.ToSlash(rel), "/subagents/")
			// Claude: the last line per ID in this file wins.
			add = func(e entry) {
				e.call.AgentType = agent
				if sub {
					e.call.Role = "sub"
				}
				if _, exists := rows[e.id]; exists {
					stats.Duplicates++
				}
				rows[e.id] = e
			}
		} else {
			_, session, _ := strings.Cut(strings.TrimSuffix(d.Name(), ".jsonl"), "_")
			// Pi: the first line per ID across the whole root wins.
			add = func(e entry) {
				if seen[e.id] {
					stats.Duplicates++
					return
				}
				seen[e.id] = true
				e.call.SessionID = session
				rows[e.id] = e
			}
		}
		reader := bufio.NewReader(f)
		for {
			line, readErr := reader.ReadBytes('\n')
			if len(line) > 0 {
				stats.Lines++
				if e, ok := parse(line, source); ok {
					e.call.Project = project
					add(e)
				} else {
					stats.Skipped++
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					stats.UnreadableFiles++
				}
				break
			}
		}
		ordered := make([]entry, 0, len(rows))
		for _, e := range rows {
			ordered = append(ordered, e)
		}
		sort.Slice(ordered, func(i, j int) bool {
			if ordered[i].stamp == ordered[j].stamp {
				return ordered[i].id < ordered[j].id
			}
			return ordered[i].stamp < ordered[j].stamp
		})
		for _, e := range ordered {
			emit(e.call)
			stats.Calls++
		}
		return nil
	})
	return stats, err
}
