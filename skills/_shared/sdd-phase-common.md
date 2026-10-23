# SDD Phase — Common Protocol

Boilerplate identical across all SDD phase skills. Sub-agents MUST load this alongside their phase-specific SKILL.md.

Executor boundary: every SDD phase agent is an EXECUTOR, not an orchestrator. Do the phase work yourself. Do NOT launch sub-agents, do NOT call `delegate`/`task`, and do NOT bounce work back unless the phase skill explicitly says to stop and report a blocker.

## B. Artifact Retrieval

**`sdd-research` collector exception:** this output-only collector does not read local artifacts, repository state, or Engram state, and does not use artifact locators. It returns its evidence envelope to the orchestrator, which validates and persists it through the selected store route. Sections B and C do not apply to `sdd-research`; every other phase follows them unchanged.

The orchestrator injects the artifact store and the locators native status already resolved (`artifactStore` and `artifactPaths` from `gentle-ai sdd-status --json --instructions`). Read what you are given.

**Do NOT detect the artifact store, and do NOT branch on it.** The dispatcher resolved it from the store the workspace DECLARES. An agent that re-derives the store disagrees with the authority that launched it, which is exactly how a phase ends up reading a store the workspace never declared — or reading nothing at all and returning an empty result.

For each artifact your phase requires, read its locator:

| reported store | locator shape | how to read it |
|---|---|---|
| `openspec` | repo path, e.g. `openspec/changes/{change-name}/tasks.md` | read the file |
| `engram` | topic key, e.g. `sdd/{change-name}/tasks` | `mem_search(query: "<locator>", project: "{project}")` → `mem_get_observation(id)` |
| `hybrid` | either shape | read the file when the locator is a path, the observation when it is a topic key |

**CRITICAL for topic-key locators**: `mem_search` returns 300-char PREVIEWS, not full content. You MUST call `mem_get_observation(id)` for EVERY artifact. **Skipping this produces wrong output.** Do NOT use search previews as source material.

**Run all retrievals in parallel** — do NOT read sequentially.

A required locator reported as `<unresolved>` means the artifact does not exist. Report it as a blocker. Never substitute another store's copy, and never go looking for one.

## C. Artifact Persistence

Every artifact-producing phase other than the output-only `sdd-research` collector MUST persist it. Skipping this BREAKS the pipeline — downstream phases will not find your output.

Persist to the store the orchestrator reported, using that artifact's locator. As in section B, the store is told to you; do not detect it. The write mechanisms below differ because writing a file and saving an observation are genuinely different operations, not because the agent gets to choose between them.

Verification reports are optional diagnostics. Persist honest results without a validator or certificate; preserve historical findings and never fabricate a pass to enable archive.

### Engram mode

```
mem_save(
  title: "sdd/{change-name}/{artifact-type}",
  topic_key: "sdd/{change-name}/{artifact-type}",
  type: "architecture",
  project: "{project}",
  capture_prompt: false,
  content: "{your full artifact markdown}"
)
```

`topic_key` enables upserts — saving again updates, not duplicates.
`capture_prompt: false` is mandatory for SDD artifacts because they are automated pipeline outputs, not human/proactive memory saves. Set it when the Engram tool schema supports it; if an older schema rejects or does not expose the field, omit it rather than failing.

### OpenSpec mode

File was already written during the phase's main step. No additional action needed.

### Hybrid mode

Attempt BOTH declared writes and read back each successful write. Hybrid writes are not atomic: preserve successful writes and report partial persistence with the outstanding locator. Do not claim a successful mirror, silently substitute another store, or roll back valid progress.

### None mode

Return result inline only. Do not write any files or call `mem_save`.

## D. Return Envelope

> **CRITICAL — Response ordering**: Your FINAL output MUST be text (the return envelope), NOT a tool call. If you need to save to Engram (`mem_save`), do it BEFORE your final text response. Do NOT call `mem_session_summary` — that's for top-level agents only. **Why**: When a sub-agent's last action is a tool call, the parent agent receives only the tool result — your text response (the actual analysis) is lost.

Every phase MUST return a structured envelope to the orchestrator:

- `status`: `success`, `partial`, or `blocked`
- `executive_summary`: 1-3 sentence summary of what was done
- `detailed_report`: (optional) full phase output, or omit if already inline
- `artifacts`: list of artifact keys/paths written
- `next_recommended`: the next SDD phase to run, or "none"
- `risks`: risks discovered, or "None"
- `skill_resolution`: how skills were loaded — `paths-injected` (received exact skill paths from orchestrator), `fallback-registry` (self-loaded paths from registry), `fallback-path` (loaded via SKILL: Load path), or `none` (no skills loaded)

**Validating a delegated phase result.** A runtime whose host does not validate task results itself MUST run `gentle-ai sdd-task-result --phase <phase> --cwd <repo> --input <path|->` over the child's raw output before treating it as a result. It exits zero for a usable result and otherwise renders the typed terminal failure below, byte-identical to the one a validating host emits, because both read one definition. Never classify a task result by reading it yourself.

If terminal task-result validation reports `sdd_task_result_empty` or `sdd_task_result_malformed`, do not assume this envelope was delivered. Do not retry automatically or initiate another phase. The terminal value starts with `GENTLE_AI_SDD_FAILURE ` followed by a `gentle-ai.sdd-task-result-failure/v1` JSON handoff; preserve it unchanged, follow its `continuation` exactly once, and execute it only when supplied as a command. Never turn guidance into a guessed command: use only the coordinator's retained structured status for the selected change and artifact store; if unavailable, report the terminal failure and ask the user to select both. Do not infer either, run unscoped status discovery, retry, or launch another phase. Report the typed failure to the user and wait for an explicit decision. A later launch in the same session receives `sdd_task_dispatch_latched` instead: that launch never dispatched, so it names the phase it requested, the earlier phase and code that actually failed, and its `exit` -- start a new session to launch SDD phases again.

OpenCode `background: true` launch acknowledgements and progress signals are nonterminal. They must not produce either transport failure or a session latch; wait for the child to complete, then use the normal artifact/status route.

Example:

```markdown
**Status**: success
**Summary**: Proposal created for `{change-name}`. Defined scope, approach, and rollback plan.
**Artifacts**: Engram `sdd/{change-name}/proposal` | `openspec/changes/{change-name}/proposal.md`
**Next**: sdd-spec or sdd-design
**Risks**: None
**Skill Resolution**: paths-injected — 3 skills (react-19, typescript, tailwind-4)
(other values: `fallback-registry`, `fallback-path`, or `none — no registry found`)
```
