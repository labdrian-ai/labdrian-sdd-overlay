## Exploration: procedural-candidate-detection

### Current State

The overlay's memory stack has two working layers and one that does not exist:

- **Episodic**: Engram `mem_save`/`mem_search`/`mem_get_observation` — LLM-agent-driven, MCP-tool-mediated writes into Engram's own SQLite DB. No Go code in this repo writes to that DB; the only Go client (`longterm-mem/internal/engram/store.go`) opens it strictly **read-only** (`mode=ro&_query_only=true`, package doc: "no code path here can write to Engram's database").
- **Semantic (partial)**: `longterm-mem` — a separate Go subsystem with its own vault (`longterm-mem/internal/promote/writer.go`) and its own MCP stdio server (`longterm-mem/internal/mcpserver/server.go`, tools `query`/`get`/`promote`). This IS real Go-side automation, but it writes to a git-tracked vault, not to Engram's DB.
- **Procedural**: does not exist. `skills/` (managed by `engine/skills/*`) is structurally procedural memory, but every entry is added by a human running `engine skills add <id>`. Nothing detects a repeated approach and proposes it.

**Skill registry mechanics** (`engine/skills/`): `types.go` defines `Registry{Version, Skills []Entry}` with `Entry{ID, Path, Source, Install, Lifecycle}`. The hand-rolled parser (`parse.go`, not a generic YAML lib) recognizes exactly this fixed key set — **no trigger/pattern/keyword field exists anywhere in the schema**, confirmed against the live `skills.registry.yaml`. `AddEntry`/`AddCore` (`lifecycle.go:27-62`, `211-333`) is the only write path (slug guard, duplicate-id rejection, atomic manifest-then-registry write per ADR-9); `RenderStatusCore`/`RenderListCore` (`status.go:37`, `list.go:48`) are the only pure-read entrypoints, via `ParseRegistry` (`parse.go:35`).

**Session/hook lifecycle** (`engine/settings/settings.go`, the `~/.claude/settings.json` merger): exactly five hooks exist today — two `UserPromptSubmit` (propagate, anti-generic-design), two `PreToolUse`/`Agent` (gate-task, design), one `PreToolUse`/`Bash` (fail-closed review-receipt capture), one `SessionEnd` (`sync-trigger`, detached/`Setsid`, always exits 0, runs `longterm-mem sync` under a 60s bound in a child process — `synctrigger.go:92-239`). **No SessionStart, Stop, or PostToolUse hook exists anywhere in this repo.** There is no generic transcript/tool-call observer to piggyback on.

**CodeGraph**: no in-repo indexed graph or provenance mechanism. `.codegraph/` is just the external CLI's local cache; no Go code here tracks "this approach was used at this call site."

### Affected Areas

- `engine/skills/parse.go`, `types.go` — read-only reuse point for duplicate-candidate checking against `skills.registry.yaml` (R-004). No schema change needed for read-only matching against `id`/`path`; matching against skill *semantics* would require reading `SKILL.md` frontmatter/description, which is outside this package today.
- `engine/settings/settings.go` — needs a **new hook builder function** (following `buildSyncTriggerSessionEndEntry`, `settings.go:615-626`) if detection runs as a Go-side SessionEnd hook. No existing hook shape fits "observe what happened this session and count repeated patterns."
- `longterm-mem/internal/engram/store.go` — the only Go-side Engram reader; usable to pull this session's observations for pattern analysis, but it is read-only by construction (R-002), so any candidate-store write still has to go back through the MCP `mem_save`/`mem_update` tools, not Go code.
- `longterm-mem/internal/synctrigger/synctrigger.go` — the closest architectural precedent for a bounded, non-blocking, detached SessionEnd process (R-003: "the triggering host command must never observe a non-zero exit or block on this runner").
- `openspec/changes/procedural-candidate-detection/` (this change) and downstream umbrella items 2–6 (`procedural-skill-drafting`, `procedural-promotion-gate`, `procedural-loop-observability`, `procedural-runtime-projection`, `procedural-skill-retirement`) all inherit whatever candidate-identity contract this slice defines.

### Approaches

1. **LLM-agent-driven detection (no new Go binary code)** — a skill/prompt convention where the agent itself, at session end or after N tool sequences, calls `mem_search`/`mem_get_observation` to look for repeated patterns and calls `mem_save` to persist/update a candidate record under a stable topic key (e.g. `procedural/{project}/candidates/{candidate-id}`), matching R-001. Duplicate rejection (R-004) reads `skills.registry.yaml` via a small new read-only helper (or reuses `ParseRegistry` directly, since it's already exported).
   - Pros: no new Go-side write path required (there isn't one into Engram's DB today); works with the existing MCP-only integration; fastest to ship within this slice's small/medium sizing; naturally fits "sub-step/debugging-pattern granularity" (OQ-1) since the agent already operates at that granularity mid-session.
   - Cons: detection quality depends on the agent reliably self-triggering it — there's no hard hook forcing the check to run; occurrence counting across sessions relies on `mem_search` recall being complete, which is the same recall dependency the rest of this memory stack already has.
   - Effort: Low–Medium.

2. **Go-side SessionEnd hook (new `buildProceduralDetectSessionEndEntry`, modeled on `synctrigger`)** — a new detached, timeout-bounded child process that reads this session's Engram observations read-only via `longterm-mem/internal/engram.Store`, runs pattern-matching, and either (a) writes results to the `longterm-mem` vault via `promote.Writer` (not Engram's DB), or (b) shells out to the `engram` CLI/MCP indirectly to write a candidate record.
   - Pros: deterministic, always-runs-once-per-session, matches the repo's own architectural precedent (`synctrigger`) and its non-blocking/fail-open contract.
   - Cons: **no Go-side write path into Engram's own DB exists today** — this approach either has to write into the `longterm-mem` vault instead (semantic drift: candidates aren't "the same kind of thing" R-001 asks for) or shell out to the `engram` binary/CLI from Go, which is unverified/untested territory this exploration did not confirm is even possible (the CLAUDE.md memory rule elsewhere notes `ENGRAM_DATABASE_URL` isn't honored by the CLI — writes have their own quirks). Also requires new `Merger` builder + settings.json wiring, and reading Engram observations read-only mid-hook still can't itself detect "success" vs "failure" semantically without re-deriving what the agent already knows better mid-conversation.
   - Effort: Medium–High.

3. **Hybrid: agent-driven detection, but with the Go skill-registry duplicate check exposed as a small reusable read-only entrypoint** — approach 1's LLM-driven loop, plus a genuinely new (currently absent) `engine/skills` read-only function like `MatchCandidate(registry, candidateID/path) (matched bool, skillPath string)` that `sdd-propose` or a future skill-promotion command can call directly instead of re-parsing YAML ad hoc.
   - Pros: keeps R-001–R-003 fully agent-driven (matches OQ-1's confirmed granularity and today's real integration surface), while giving R-004 a proper, testable, reusable Go seam instead of another one-off parser.
   - Cons: still two code paths (agent logic + one small Go helper) to keep in sync; the Go helper needs its own unit tests per R-004's acceptance criteria.
   - Effort: Low–Medium.

### Recommendation

**Approach 3.** The repo's actual Engram integration is asymmetric — reads are Go-capable (via `longterm-mem`), but writes into Engram's own DB are only ever agent-driven via MCP tools, and that's true for every other part of this memory stack already (episodic saves, session summaries). Approach 2 would be the first Go-side write path into Engram in the whole repo, which is a much bigger and riskier architectural commitment than this "small/medium, detection-and-storage-only" slice (per the existing estimate, obs #3399) should be taking on. Approach 3 keeps R-001–R-003 consistent with how every other Engram write in this repo already happens, and gives R-004 a real, tested, read-only Go seam (`engine/skills`) instead of ad hoc re-parsing — cheap to add since `ParseRegistry` is already exported and pure.

### Risks

- **No Go-side write path into Engram's DB** (confirmed, not assumed): any design that implicitly expects "the detector" to be a background process writing candidates without an LLM turn in the loop is not buildable against this repo's real architecture today. This must be stated plainly in the design doc so `sdd-design` doesn't silently pick Approach 2's premise.
- **No trigger/pattern field in the skill schema**: R-004's duplicate matching can only use `id`/`path` today, or off-schema reads of `SKILL.md` frontmatter/description — semantic similarity matching (mentioned as an open question in requirements brief obs #3395) has no existing schema support and would need either a schema extension (out of this slice's declared scope) or a text-similarity heuristic over `SKILL.md` bodies read separately.
- **No existing observer hook**: OQ-2 ("where does detection run") is still open per pipeline-state obs #3400. This exploration found zero existing hook this can extend — whatever `sdd-design` picks (agent-invoked skill, new SessionEnd hook, or both) is net-new surface, not a reuse of dormant infrastructure.
- **Cross-session occurrence counting depends on `mem_search` recall**, the same dependency this repo's own CLAUDE.md already flags as imperfect for the *forgetting* layer (zero `supersedes`/`conflicts_with` ever fired in 169 relations) — R-001/R-002's acceptance criteria assume recall is reliable enough to find prior occurrences, which this exploration did not independently verify.

### Ready for Proposal

**Yes** — OQ-1 (granularity) is confirmed (obs #3400); this exploration resolves enough of OQ-2 (no existing hook to reuse, so it becomes a first-class open design decision, not a blocker) and OQ-6 remains legitimately open for `sdd-design` (evidence source: this exploration shows Engram observations are Go-readable read-only, so `sdd-design` has a real option to name, not a guess). Recommend `sdd-propose` proceed with Approach 3 as the proposed direction, explicitly carrying forward the "no Go-side Engram write path" constraint so the proposal doesn't promise Go-side automation this repo can't deliver.
