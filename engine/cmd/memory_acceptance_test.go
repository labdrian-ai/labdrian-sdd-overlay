package main

// C23 M11, ADR decisions 1–2: process-level delivery, not live CLI sessions.
// The table below is the runtime -> surface -> claim -> proof contract. Unsupported
// rows prove the declaration/limit, not the absence of every possible integration.
// All engine processes use phase7World's isolated HOME, state and empty PATH;
// only the overlay sources are read from the checkout. No runtime CLI is started.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

func TestMemoryWiringAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs engine processes")
	}
	binary := phase6BuildEngineBinary(t)
	w := newPhase7World(t, binary)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, label string, got phase7Run, stdout string) {
		t.Helper()
		if want := (phase7Run{code: 0, stdout: stdout}); got != want {
			t.Fatalf("%s: got %+v, want %+v", label, got, want)
		}
	}
	unchanged := func(t *testing.T, before map[string]string) {
		t.Helper()
		if !reflect.DeepEqual(before, w.stateSnapshot()) {
			t.Fatal("read-only/refused process changed workflow state or evidence")
		}
	}

	// A single bound workflow supplies every proof. sdd includes Engram and
	// excludes longterm-mem, without editing any stored workflow data.
	goalText := memoryTestGoalJSON("proj-1", "goal-1")
	goal := writeMemoryTestFile(t, w.dir, "goal.json", goalText)
	// The CLI reports the canonical Goal digest, not the authored JSON's hash.
	const digest = "3fa80a4c34f1198a2fe03c95ecaaab97721dd5a94dce91612f2612b4208e54ac"
	stateOutput := func(status string) string {
		return fmt.Sprintf("{\n  \"classification\": \"owned\",\n  \"status\": %q,\n  \"profile\": \"sdd\",\n  \"goal_id\": \"goal-1\",\n  \"goal_digest\": \"%s\",\n  \"stages\": [],\n  \"last_verified_seq\": -1\n}\n", status, digest)
	}
	check(t, "create", w.verb("proj-1", "wf-1", "create", "--goal", goal, "--profile", "sdd"), stateOutput("created"))
	check(t, "start", w.verb("proj-1", "wf-1", "start"), stateOutput("running"))
	bound := w.engine(w.repo, "", "workflow", "bind", "--project", "proj-1", "--workflow", "wf-1")
	// Only the wall-clock timestamp varies; validate it, then pin every output byte.
	var binding struct {
		Binding struct {
			BoundAt string `json:"bound_at"`
		} `json:"binding"`
	}
	if err := json.Unmarshal([]byte(bound.stdout), &binding); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse(time.RFC3339Nano, binding.Binding.BoundAt); err != nil || !strings.HasSuffix(binding.Binding.BoundAt, "Z") {
		t.Fatalf("invalid binding timestamp %q: %v", binding.Binding.BoundAt, err)
	}
	check(t, "bind", bound, fmt.Sprintf("{\n  \"classification\": \"owned\",\n  \"binding\": {\n    \"version\": 1,\n    \"repo_key\": %q,\n    \"project_id\": \"proj-1\",\n    \"workflow_id\": \"wf-1\",\n    \"bound_at\": %q\n  }\n}\n", wantRepoKey(t, filepath.Join(w.repo, ".git")), binding.Binding.BoundAt))

	const block = "labdrian memory instructions (workflow wf-1, scope project):\n" +
		"- Read memory only from: engram. A read from any other source is denied.\n" +
		"- Engram reads (mem_search, mem_context) name project \"proj-1\".\n" +
		"- Memory writes (mem_save, promote) are not limited by the plan.\n" +
		"- Procedural candidates: T1 right after a proactive mem_save of type bugfix, pattern, discovery or decision; T2 at session close, beside mem_session_summary (skills/_shared/procedural-candidate-detection.md, section 5). Load skills/procedural-candidates/SKILL.md for the procedure; run `skills candidates` before creating or updating a candidate."
	const unavailable = "- Do not read engram: it is unavailable in this session (the engram program was not found).\n"
	sessionOutput := func(instructions string) string {
		return `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":` + strconv.Quote(instructions) + "}}\n"
	}
	instructions := func(t *testing.T) {
		t.Helper()
		before := w.stateSnapshot()
		check(t, "bound instructions", w.engine(w.repo, "", "memory", "plan", "--instructions"), block+"\n")
		unchanged(t, before)
	}
	allowedTool := "mcp__plugin_engram_engram__mem_get_observation"
	allowedInput := map[string]any{"id": 4216}
	denied := func(t *testing.T) {
		t.Helper()
		before := w.stateSnapshot()
		reason := "labdrian: the memory plan of workflow wf-1 of project proj-1 (profile \"sdd\") does not include source \"longterm-mem\", so this longterm-mem get is denied. Stop following this workflow with 'labdrian workflow unbind' to read it."
		check(t, "excluded source", w.toolHook(w.repo, "mcp__longterm-mem__get", map[string]any{"id": "obs-1"}),
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":`+strconv.Quote(reason)+"}}\n")
		unchanged(t, before)
	}
	claudeInstructions := func(t *testing.T) {
		instructions(t)
		before := w.stateSnapshot()
		degraded := strings.Replace(block, "- Engram reads", unavailable+"- Engram reads", 1)
		check(t, "SessionStart", w.engine(w.dir, sessionStartInput(t, w.repo, "startup"), "projection", "hook", "--event", "SessionStart"), sessionOutput(degraded))
		contract := writeMemoryTestFile(t, w.dir, "contract.md", contractContentForE2E)
		prompt := "Do the task.\n\n## Skills to load before work\n" + contract + "\n\n" + block + "\n"
		check(t, "Agent rewrite", w.engine(w.dir, agentInputIn(w.repo, "sdd-apply", "Do the task."), "gate-task", "--contract-file", contract, "--contract-path", contract),
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"description":"d","prompt":`+strconv.Quote(prompt)+`,"subagent_type":"sdd-apply"}}}`+"\n")
		unchanged(t, before)
	}
	claudeEnforcement := func(t *testing.T) {
		before := w.stateSnapshot()
		check(t, "included source", w.toolHook(w.repo, allowedTool, allowedInput), "")
		unchanged(t, before)
		denied(t)
	}
	claudeEvidence := func(t *testing.T) {
		path := strings.TrimSuffix(phase6WorkflowLogPath(w.state, "proj-1", "wf-1"), ".jsonl") + ".memory.jsonl"
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("evidence exists before a completed read: %v", err)
		}
		before := w.stateSnapshot()
		const response = "MEMORY-RESPONSE-CANARY"
		start := time.Now().UTC()
		check(t, "PostToolUse", w.engine(w.dir, postToolUseInput(t, w.repo, allowedTool, allowedInput, response), "projection", "hook", "--event", "PostToolUse"), "")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var record map[string]any
		if json.Unmarshal(data, &record) != nil || strings.Count(string(data), "\n") != 1 {
			t.Fatalf("not exactly one evidence line: %q", data)
		}
		at, _ := record["at"].(string)
		when, err := time.Parse(time.RFC3339Nano, at)
		if err != nil || when.Before(start) || when.After(time.Now().UTC()) || !strings.HasSuffix(at, "Z") {
			t.Fatalf("invalid evidence timestamp %q: %v", at, err)
		}
		sum := sha256.Sum256([]byte(strconv.Quote(response)))
		want := fmt.Sprintf(`{"version":1,"at":%q,"tool":%q,"source":"engram","project":"","goal":"","ids":["4216"],"skill":"","call_id":"toolu_1","session":"session-1","response_sha256":%q,"response_bytes":%d}`+"\n", at, allowedTool, hex.EncodeToString(sum[:]), len(strconv.Quote(response)))
		if string(data) != want || strings.Contains(string(data), response) {
			t.Fatalf("evidence = %q, want metadata only %q", data, want)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("sidecar mode: %v, %v", info, err)
		}
		after := w.stateSnapshot()
		rel, err := filepath.Rel(w.state, path)
		if err != nil {
			t.Fatal(err)
		}
		delete(after, rel)
		delete(after, strings.TrimSuffix(rel, ".jsonl")+".lock")
		if !reflect.DeepEqual(before, after) {
			t.Fatal("PostToolUse changed files other than its sidecar and lock")
		}
		// A refused read has no PostToolUse: the runtime never ran it.
		denied(t)
	}
	claudeAvailability := func(t *testing.T) {
		// The earlier SessionStart pinned the unavailable line. Now a scratch
		// Engram stub answers stats; no real memory service can be contacted.
		marker := filepath.Join(w.dir, "engram-args")
		writeFixtureFile(t, filepath.Join(w.bin, "engram"), "#!/bin/sh\nprintf '%s\\n' \"$*\" > '"+marker+"'\n")
		if err := os.Chmod(filepath.Join(w.bin, "engram"), 0o700); err != nil {
			t.Fatal(err)
		}
		before := w.stateSnapshot()
		check(t, "available SessionStart", w.engine(w.dir, sessionStartInput(t, w.repo, "resume"), "projection", "hook", "--event", "SessionStart"), sessionOutput(block))
		args, err := os.ReadFile(marker)
		if err != nil || string(args) != "stats\n" {
			t.Fatalf("session probe args %q, %v, want stats", args, err)
		}
		unchanged(t, before)
	}

	// Read citations independently of the package builder's citation parser.
	// Inspect every Markdown document under the two skill directories, not just
	// today's SKILL.md files, so a newly nested normative reference is covered.
	shared := map[string]bool{}
	for _, name := range []string{"memory-plan", "procedural-candidates"} {
		err := filepath.WalkDir(filepath.Join(root, "skills", name), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				for _, citation := range regexp.MustCompile(`skills/_shared/[A-Za-z0-9_.-]+\.md`).FindAllString(string(data), -1) {
					shared[citation] = true
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !shared["skills/_shared/procedural-candidate-detection.md"] {
		t.Fatal("memory skills no longer cite the normative detector contract")
	}
	codexDelivery := func(t *testing.T) {
		skillsRoot := filepath.Join(w.dir, "codex-skills")
		before := w.stateSnapshot()
		r := w.engine(w.dir, "", "manifest", "routes", "--overlay-root", root, "--target", "codex", "--skills-root", skillsRoot, "--agents-root", filepath.Join(w.dir, "codex-agents"))
		if r.code != 0 || r.stderr != "" || !strings.HasSuffix(r.stdout, "\n") {
			t.Fatalf("manifest routes: %+v", r)
		}
		rows := map[string]string{}
		for _, row := range strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n") {
			cols := strings.Split(row, "\t")
			if len(cols) != 5 || rows[cols[0]] != "" {
				t.Fatalf("malformed/duplicate route %q", row)
			}
			rows[cols[0]] = row + "\n"
		}
		wanted := []string{"skills/memory-plan/SKILL.md", "skills/procedural-candidates/SKILL.md"}
		for citation := range shared {
			wanted = append(wanted, citation)
		}
		for _, rel := range wanted {
			path := strings.TrimPrefix(rel, "skills/")
			want := strings.Join([]string{path, "skill", rel, filepath.Join(root, rel), filepath.Join(skillsRoot, path)}, "\t") + "\n"
			if rows[path] != want {
				t.Errorf("codex %s: row %q, want %q", rel, rows[path], want)
			}
		}
		if _, err := os.Stat(skillsRoot); !os.IsNotExist(err) {
			t.Fatalf("routes must describe delivery, not install skills: %v", err)
		}
		unchanged(t, before)
		instructions(t)
	}
	var codexCommand string              // the installed SessionStart command, used by the availability proof
	codexCommands := map[string]string{} // installed tool commands, not hand-written invocations
	codexHook := func(t *testing.T) {
		t.Helper()
		// The hook is merged into a scratch CODEX_HOME and run as installed, through sh -c, with
		// the SessionStart input Codex defines (rust-v0.148.0 hooks schema): it runs outside the
		// agent shell, so the hidden $HOME of the permission profile does not reach it.
		hooks := filepath.Join(w.dir, "codex-home", "hooks.json")
		if err := os.MkdirAll(filepath.Dir(hooks), 0o700); err != nil {
			t.Fatal(err)
		}
		writeMemoryTestFile(t, filepath.Dir(hooks), "hooks.json", `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"moshi notify"}]}]}}`)
		before := w.stateSnapshot()
		merged := w.engine(w.dir, "", "merge-codex-hooks", "--hooks", hooks, "--hook-command", w.binary)
		if merged.code != 0 || merged.stderr != "" {
			t.Fatalf("merge-codex-hooks: %+v", merged)
		}
		data, err := os.ReadFile(hooks)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Hooks map[string][]struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		for _, event := range []string{"SessionStart", "PreToolUse", "PostToolUse"} {
			for _, group := range doc.Hooks[event] {
				for _, h := range group.Hooks {
					if !strings.Contains(h.Command, "projection hook") {
						continue
					}
					if codexCommands[event] != "" {
						t.Fatalf("duplicate owned %s handler", event)
					}
					codexCommands[event] = h.Command
					if event == "SessionStart" {
						if group.Matcher != "startup|resume|clear|compact" {
							t.Fatalf("matcher %q", group.Matcher)
						}
						continue
					}
					if !strings.Contains(h.Command, "--source codex") {
						t.Fatalf("%s lacks the boundary adapter: %q", event, h.Command)
					}
					matcher := regexp.MustCompile(group.Matcher)
					for _, tool := range []string{allowedTool, "mcp__longterm_mem__get", "mcp__longterm_mem__query"} {
						if !matcher.MatchString(tool) {
							t.Fatalf("%s matcher excludes acceptance tool %s", event, tool)
						}
					}
				}
			}
			if codexCommands[event] == "" {
				t.Fatalf("missing owned %s handler", event)
			}
		}
		ours := codexCommands["SessionStart"]
		codexCommand = ours
		if ours == "" || !strings.Contains(string(data), "moshi notify") {
			t.Fatalf("our entry or the foreign one is missing: %s", data)
		}
		// sh falls back to its own default PATH when PATH is empty, which on a developer machine can
		// find a real engram; the probe is made deterministic with a PATH that holds a stub that
		// answers `engram stats`, so the block carries no availability line.
		hw := w
		hw.bin = t.TempDir()
		if err := os.WriteFile(filepath.Join(hw.bin, "engram"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		for _, source := range []string{"startup", "resume", "clear", "compact"} {
			check(t, "codex SessionStart "+source, hw.run(hw.dir, sessionStartInput(t, hw.repo, source), "sh", "-c", ours), sessionOutput(block))
		}
		unchanged(t, before)
	}
	codexAvailability := func(t *testing.T) {
		t.Helper()
		// The hook shares Claude's SessionStart renderer, so the session-time probe of C23 M8 runs
		// in the hook process too: a stub engram that fails is reported in the block.
		if codexCommand == "" {
			t.Fatal("the instructions proof did not install the Codex hook")
		}
		hw := w
		hw.bin = t.TempDir()
		if err := os.WriteFile(filepath.Join(hw.bin, "engram"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		line := "- Do not read engram: it is unavailable in this session (the engram program failed: exit status 1).\n"
		want := sessionOutput(strings.Replace(block, "- Engram reads", line+"- Engram reads", 1))
		check(t, "codex SessionStart availability", hw.run(hw.dir, sessionStartInput(t, hw.repo, "startup"), "sh", "-c", codexCommand), want)
	}
	codexTool := func(t *testing.T, event, tool string, input any) phase7Run {
		t.Helper()
		command := codexCommands[event]
		if command == "" {
			t.Fatalf("instructions proof did not install %s", event)
		}
		return w.run(w.dir, codexSourceInput(t, w.repo, event, tool, input), "sh", "-c", command)
	}
	codexEnforcement := func(t *testing.T) {
		before := w.stateSnapshot()
		check(t, "Codex included source", codexTool(t, "PreToolUse", allowedTool, allowedInput), "")
		for _, tt := range []struct{ name, action string }{{"query", "query"}, {"get", "read it"}} {
			reason := "labdrian: the memory plan of workflow wf-1 of project proj-1 (profile \"sdd\") does not include source \"longterm-mem\", so this longterm-mem " + tt.name + " is denied. Stop following this workflow with 'labdrian workflow unbind' to " + tt.action + "."
			check(t, "Codex excluded "+tt.name, codexTool(t, "PreToolUse", "mcp__longterm_mem__"+tt.name, map[string]any{"id": "obs-1", "project": "proj-1", "query": "q"}),
				`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":`+strconv.Quote(reason)+"}}\n")
		}
		unchanged(t, before) // includes the existing Claude evidence and its counters
	}
	codexEvidence := func(t *testing.T) {
		path := sourceSidecar(w)
		previous := sourceRead(t, path) // Claude's record remains intact in the shared sidecar
		before := w.stateSnapshot()
		check(t, "Codex successful MCP PostToolUse", codexTool(t, "PostToolUse", allowedTool, allowedInput), "")
		data := sourceRead(t, path)
		if !strings.HasPrefix(string(data), string(previous)) || strings.Count(string(data[len(previous):]), "\n") != 1 {
			t.Fatalf("not exactly one append: %q -> %q", previous, data)
		}
		sourceRecord(t, data[len(previous):], allowedTool, "engram", "", "4216")
		if strings.Contains(string(data), "response-content-canary") {
			t.Fatal("Codex response content leaked into evidence")
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("Codex sidecar mode: %v, %v", info, err)
		}
		after := w.stateSnapshot()
		rel, err := filepath.Rel(w.state, path)
		if err != nil {
			t.Fatal(err)
		}
		before[rel] = after[rel]
		if !reflect.DeepEqual(before, after) {
			t.Fatal("Codex PostToolUse changed files other than the existing sidecar")
		}
		codexEnforcement(t) // refused calls do not run or append a PostToolUse record
	}
	piDelivery := func(t *testing.T) {
		dest := filepath.Join(w.dir, "pkg")
		before := w.stateSnapshot()
		check(t, "pipkg build", w.engine(w.dir, "", "pipkg", "build", "--overlay-root", root, "--registry", filepath.Join(root, "skills.registry.yaml"), "--dest-dir", dest), "pipkg build: labdrian-pi package written to "+dest+"\n")
		wanted := []string{"skills/memory-plan/SKILL.md", "skills/procedural-candidates/SKILL.md"}
		for citation := range shared {
			wanted = append(wanted, citation)
		}
		for _, rel := range wanted {
			original, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatal(err)
			}
			packaged, err := os.ReadFile(filepath.Join(dest, rel))
			if err != nil || string(original) != string(packaged) {
				t.Errorf("Pi %s missing or not byte-identical: %v", rel, err)
			}
		}
		unchanged(t, before)
		instructions(t)
	}

	type claim struct {
		status capability.Status
		limit  string
		proof  func(*testing.T)
	}
	unsupported := func(limit string) claim {
		return claim{capability.Unsupported, limit, func(t *testing.T) { t.Helper(); t.Log(limit) }}
	}
	table := map[string]map[string]claim{
		"claude": {
			"instructions": {capability.Supported, "Engine hook processes only; no live session.", claudeInstructions},
			"enforcement":  {capability.Partial, "Recognized source gate only; Bash and project of reads by id are not enforced.", claudeEnforcement},
			"evidence":     {capability.Supported, "Runtime-supplied PostToolUse metadata, not proof of response truth.", claudeEvidence},
			"availability": {capability.Partial, "Session-time Engram availability in this plan, not runtime authentication.", claudeAvailability},
		},
		"codex": {
			"instructions": {capability.Supported, "Engine hook process only, once the owner trusts the hook in the Codex TUI; the skills stay deliverable but Codex's sandbox may hide them; no live session before trust.", func(t *testing.T) { codexDelivery(t); codexHook(t) }},
			"enforcement":  {capability.Partial, "Trusted MCP read hooks only; Codex fails open; no procedural skills, Bash or apply_patch enforcement, no project validation for reads by id; agent_id ignored. Engine processes, not live validation.", codexEnforcement},
			"evidence":     {capability.Supported, "Successful MCP reads only, using the shared metadata/hash sidecar; failed calls have no PostToolUse. Engine processes, not live validation.", codexEvidence},
			"availability": {capability.Partial, "Session-time Engram availability shared with Claude through the same hook renderer, not runtime authentication; only after the owner trusts the hook.", codexAvailability},
		},
		"pi": {
			"instructions": {capability.Partial, "Package delivery and manual skill invocation, not automatic projection.", piDelivery},
			"enforcement":  unsupported("Pi has no memory enforcement; M12 remains pending."),
			"evidence":     unsupported("Pi has no memory-read evidence hook; M12 remains pending."),
			"availability": unsupported("Pi has no session memory availability probe; M12 remains pending."),
		},
	}
	before := w.stateSnapshot()
	r := w.engine(w.dir, "", "runtime", "capabilities", "--target", "all")
	report := phase7DecodeReport(t, r.stdout)
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	check(t, "capabilities", r, string(encoded)+"\n")
	unchanged(t, before)
	for _, runtime := range []string{"claude", "codex", "pi"} {
		found := 0
		for _, declaration := range report.Declarations {
			if declaration.Target != runtime {
				continue
			}
			for _, c := range declaration.Claims {
				if c.Capability == capability.MemoryEnforcement {
					found++
					if c.Status != table[runtime]["enforcement"].status || c.Detail == "" {
						t.Fatalf("%s declares memory enforcement %s (%s), table expects %s", runtime, c.Status, c.Detail, table[runtime]["enforcement"].status)
					}
				}
			}
		}
		if found != 1 {
			t.Fatalf("%s has %d memory enforcement declarations, want one", runtime, found)
		}
		if len(table[runtime]) != 4 {
			t.Fatalf("%s must declare exactly four memory surfaces", runtime)
		}
		// Validate every row, even an accidentally added surface not in the
		// execution order below: a claim without a proof must fail closed.
		for surface, c := range table[runtime] {
			if c.proof == nil || c.limit == "" || (c.status != capability.Supported && c.status != capability.Partial && c.status != capability.Unsupported) {
				t.Fatalf("%s/%s has no valid claim, written limit or proof", runtime, surface)
			}
		}
		for _, surface := range []string{"instructions", "enforcement", "evidence", "availability"} {
			c, ok := table[runtime][surface]
			if !ok {
				t.Fatalf("%s has no %s claim", runtime, surface)
			}
			if !t.Run(runtime+"/"+surface, c.proof) {
				t.FailNow() // later steps rely on this one bound workflow's chain
			}
		}
	}
}
