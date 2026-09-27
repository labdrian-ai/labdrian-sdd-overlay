# Laya System 1 Integration Plan for `labdrian-sdd-overlay`

Status: research complete; implementation not started.

## Objective

Introduce Laya as an optional, local System 1 decision layer for the Labdrian overlay. Laya must classify and score explicit questions quickly, while the existing overlay remains authoritative for permissions, orchestration, provider selection, memory writes, review gates, and delivery.

This plan is intentionally separate from `longterm-mem`: Laya is a decision adapter; `longterm-mem` is the Engram/Obsidian memory system.

## Repository and branch baseline

Repository: `labdrian-sdd-overlay`

Current branch at review time: `feat/procedural-memory-lifecycle-7b-retirement-detector`

Current HEAD: `f983e74 feat(skills): add read-only retirement detector`

Working tree at review time contained unrelated untracked installation/runtime artifacts:

- `.agents/`
- `.claude/skills/`
- `.pi/`
- `skills-lock.json`

No source modifications were made by this plan. The active feature commit is clean relative to its tracked files; its final verification and delivery work remain pending.

## Verified architecture map

### Runtime and installation layer

- `bin/labdrian-overlay` is the shell entrypoint and dispatches copy targets, package targets, and the `mcp` route.
- `overlay.manifest` already declares `longterm-mem/go.mod custom mcp`.
- `engine/runtime/` owns lifecycle adapters and capability statuses.
- `engine/runtime/longtermmem.go`, `longtermmem_config.go`, and `longtermmem_status.go` implement the aggregate longterm-mem install/status/uninstall contract across Claude, OpenCode, and Codex.
- The persistent longterm-mem binary path is `$STATE_DIR/bin/longterm-mem`, normally `~/.labdrian-overlay/bin/longterm-mem`.

### Memory layer

- `longterm-mem/` is a standalone Go module.
- `longterm-mem/internal/engram/` opens Engram read-only.
- `longterm-mem/internal/mcpserver/server.go` exposes only `query`, `get`, and `promote`.
- `longterm-mem/internal/promote/` owns vault page writes, provenance, precedence, and supersession handling.
- `longterm-mem/internal/vault/runner.go` is the only vault subprocess boundary; it is root-confined, argv-based, timeout-bounded, and receives a restricted environment.
- `longterm-mem/internal/embed/client.go` is the only production network client allowed by the module's egress guard, and it is loopback-only unless explicitly widened for one invocation.
- `engine/synctrigger/synctrigger.go` runs `longterm-mem sync` detached and bounded; it must not become an implicit Laya execution path.

### Active feature in progress

The active OpenSpec change is `openspec/changes/procedural-memory-lifecycle/`.

Its purpose is to close the procedural-memory loop:

```text
candidate detection -> drafting -> linting -> project registration
-> global promotion -> revision -> retirement
```

The current branch contains the read-only retirement detector slice (7b). The task document reports these as complete:

- `ReferencedPaths` and `ClassifyPaths` in `longterm-mem/internal/staleness/`;
- report-only `longterm-mem/internal/skillstale/` detector;
- `longterm-mem skills-stale` CLI;
- shared project-lock fixture pinning;
- focused and full tests for the completed slice.

The feature still has final verification and closure work unchecked in `openspec/changes/procedural-memory-lifecycle/tasks.md` and `apply-progress.md`:

- full `engine` vet/test;
- full `longterm-mem` vet/test;
- review that no project-tier path writes under global `skills/` outside `AddCore`;
- review that tests do not touch live user/runtime directories;
- real Engram/temp-repository acceptance checklist;
- final success-criteria reconciliation.

The Laya work must not be mixed into that active feature's remaining slice. It should depend on its closure or be explicitly staged as a separate change.

## Architectural decision

### Keep Laya out of `longterm-mem`

Do not add Laya inference, PyTorch, Python execution, routing policy, or decision schemas to `longterm-mem`.

`longterm-mem` should remain responsible for:

```text
Engram read -> query/get/promote -> Obsidian vault/index lifecycle
```

Laya should be a separate optional component:

```text
overlay request -> Laya adapter -> structured decision -> overlay policy/orchestration
```

This preserves:

- read-only Engram access;
- the longterm-mem network allowlist;
- the vault write boundary;
- MCP lifecycle ownership;
- independent failure and resource control;
- the ability to disable Laya without disabling memory.

## Laya provider and runtime boundary

Laya is published by Convai Innovations:

- runtime package: `laya` on PyPI;
- checkpoints: Hugging Face repositories;
- local execution: Python/PyTorch/Transformers/safetensors;
- output: typed `choice`, `score`, and `noul` decisions;
- no dependency on OpenAI, Anthropic, Gemini, or another generative provider.

The first integration should be a Python sidecar, not a Go reimplementation and not a direct library dependency of `engine`.

## Proposed component boundary

```text
labdrian-sdd-overlay/
├── laya-adapter/             # new standalone component, later slice
│   ├── cmd/laya-adapter/
│   ├── protocol/              # versioned JSON request/response contract
│   ├── runtime/               # Python process/model lifecycle
│   └── tests/
├── engine/                    # lifecycle/status only, if installation is added
├── longterm-mem/              # unchanged memory component
└── bin/labdrian-overlay       # later install/status dispatch
```

The initial implementation may live outside the Go overlay as a small Python executable launched by an explicit adapter command. Only after the protocol is stable should it receive an overlay lifecycle route.

## Adapter protocol

The adapter should accept one JSON request per invocation or a long-lived stdio session. The first implementation should prefer a bounded subprocess protocol to minimize lifecycle coupling.

Request shape:

```json
{
  "protocol_version": "laya.v1",
  "project": "labdrian-sdd-overlay",
  "request_id": "run-123",
  "mode": "shadow",
  "state": {"request": "..."},
  "questions": {
    "task_type": {
      "type": "choice",
      "instructions": "What type of task is this?",
      "criteria": {
        "coding": "software work",
        "research": "investigation",
        "planning": "architecture or planning",
        "other": "other"
      }
    }
  },
  "model": "",
  "task": "",
  "lang": ""
}
```

Response shape:

```json
{
  "protocol_version": "laya.v1",
  "request_id": "run-123",
  "decision": {
    "answers": {},
    "routing": {},
    "model": "laya-rl-agent"
  },
  "diagnostics": {
    "latency_ms": 0,
    "fallback": false,
    "error": ""
  }
}
```

The protocol must preserve raw Laya answers and routing metadata. The adapter must not turn a probability into an authorization by itself.

## Integration modes

### Stage 1: shadow mode

```text
request -> existing overlay route
        \-> Laya classification and telemetry
```

Laya cannot change the selected provider, execute tools, write memory, or trigger review. Record agreement, latency, fallback, and errors.

### Stage 2: advisory routing

The overlay may use Laya as a suggestion for:

- coding vs research vs planning;
- difficulty;
- tool requirement;
- probable risk;
- memory candidate classification;
- human-escalation recommendation.

Existing deterministic policy remains authoritative.

### Stage 3: bounded policy signals

Laya may provide an additional signal for prompt injection, sensitive data, or operational risk. High-risk actions still require the overlay's existing explicit authorization path. A missing, timed-out, low-confidence, or contradictory Laya result must fail open to the existing safe policy, not to automatic execution.

## Memory integration

Laya must not write Engram or the vault directly.

Recommended flow:

```text
Laya: "This looks like a durable architecture decision"
       ↓
agent/overlay applies existing mem_save convention
       ↓
longterm-mem query/promote handles retrieval and vault promotion
```

For external knowledge ingestion, preserve the active `longterm-mem-knowledge-ingestion` contract:

```text
agent fetch/read -> Engram mem_save -> longterm-mem promote
```

Laya can classify or tag the material, but it must not create a parallel identity or ingestion path.

## Resource and lifecycle constraints

The host currently has enough capacity for a controlled CPU-only pilot, but model weights and PyTorch are materially heavier than longterm-mem.

Initial limits:

- one checkpoint resident;
- CPU-only fallback accepted;
- explicit startup and inference timeout;
- bounded request size;
- no model preload during normal overlay installation;
- no model download during `overlay apply` or `longterm-mem sync`;
- model cache outside the repository and vault;
- adapter failure must not fail memory sync or runtime startup.

Use Laya's `Router` only after the single-checkpoint path works. Start with an explicit checkpoint and `max_loaded=1`; do not preload all three models.

## Security and privacy requirements

- Do not send secrets, raw credentials, private prompts, or unredacted customer data to a remote model provider.
- Public Hugging Face weights may be downloaded during explicit setup, not silently during a runtime request.
- The adapter receives only the minimum state required by its question schema.
- Project identity is explicit in the protocol; do not derive it from the adapter process working directory.
- Laya cannot authorize SSH, destructive commands, pushes, releases, review acknowledgement, or remote operations.
- The adapter must have no Engram write capability and no vault write capability.
- If a remote inference mode is ever added, it requires a separate explicit egress contract; local Laya is the default.

## Verification plan

### Adapter tests

- protocol JSON round-trip;
- malformed request rejection;
- missing/empty questions rejection;
- timeout and process termination;
- model unavailable fallback;
- deterministic fixture decisions;
- preservation of routing metadata;
- request-size limit;
- no filesystem writes outside the configured cache;
- no Engram/vault writes.

### Overlay tests

- shadow mode never changes the existing route;
- adapter failure preserves the existing route;
- high-risk result still reaches the existing authorization gate;
- `longterm-mem sync` succeeds when Laya is unavailable;
- lifecycle status reports Laya separately from longterm-mem;
- uninstalling Laya does not remove longterm-mem registration;
- explicit project identity is preserved across worktrees.

### Evaluation tests

- synthetic coding/research/planning corpus;
- multilingual inputs;
- prompt-injection corpus;
- memory-worthy vs transient observations;
- calibration: Brier score and ECE;
- false-positive and false-negative escalation rates;
- CPU latency and resident memory under one loaded checkpoint.

## Delivery slices

1. **Protocol and fixture** — standalone, no overlay behavior change.
2. **Python sidecar** — explicit model loading, JSON stdio, timeout, and offline fixture mode.
3. **Shadow instrumentation** — adapter invocation with no routing authority.
4. **Evaluation and calibration** — record results in the Laya Obsidian vault.
5. **Advisory routing** — opt-in feature flag, deterministic overlay policy remains authoritative.
6. **Optional lifecycle integration** — only after the sidecar and protocol are stable; add an `mcp`-like component route if operationally justified.
7. **Memory classification integration** — only after routing behavior is stable; keep writes on existing Engram/longterm-mem paths.

## Explicit non-goals

- replacing OpenAI/Anthropic/Gemini or local generative models;
- adding Laya to `longterm-mem`;
- direct URL fetching from Go;
- direct vault writes from Laya;
- automatic destructive actions based on confidence;
- training or fine-tuning Laya as part of the first integration;
- mixing this work into the pending procedural-memory-lifecycle closure slice;
- changing the existing review/RDD authority model.

## Decision gate before implementation

Implementation should begin only after the owner accepts these boundaries:

1. Laya is a separate optional sidecar.
2. First behavior is shadow-only.
3. `longterm-mem` remains unchanged.
4. Existing overlay policy remains authoritative.
5. The pending procedural-memory-lifecycle and longterm-mem-ingestion work are tracked separately.
6. CPU-only, one-checkpoint operation is acceptable for the pilot.
