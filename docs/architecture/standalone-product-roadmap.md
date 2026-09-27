# Labdrian: standalone product rationale and source crosswalk

**Role:** supporting product rationale and requirements source; **not** an active roadmap or schedule. The sole active standalone-product roadmap is [Standalone Platform Master Roadmap](../../openspec/decisions/standalone-platform-roadmap.md), which owns canonical Phase 0–10 IDs, status, sequencing, and crosswalk decisions. The P0–P8 labels below are retained only as historical source aliases; they are not an ordered work queue, and their numbering does not map ordinally to phases.

Product direction is approved; implementation status and authorization must be verified from current evidence. This document preserves the unique rationale, constraints, acceptance details, and open decisions behind the source packages. Read [START-HERE](standalone-product-start-here.md) for recovery. Earlier context: [Laya integration proposal](laya-system-one-overlay-plan.md), provenance only.

This rationale extends that proposal. Its distinctions between proposed behavior and verified capability take precedence over the earlier document's broad claims. It does not rewrite historical artifacts or authorize changes to existing review authority.

## Approved direction and limits

| Topic | Settled direction |
| --- | --- |
| Product | Independent platform on existing Pi, Claude Code, OpenCode and Codex runtimes; incremental support, not immediate parity |
| Independence | A supported end-to-end path requires neither gentle-ai nor gentle-pi; third-party runtimes and model providers remain dependencies |
| Gentle AI | Optional compatibility integration, not a mandatory core dependency |
| Laya | First specialized decision backend; optional, local, shadow-first |
| Future models | Allow other specialized decision models through explicit capabilities; no universal framework upfront |
| Jev | User named a future candidate; exact artifact, API, availability and license remain unverified |
| Memory | Keep longterm-mem separate and preserve existing Engram/vault responsibilities |
| Authority | Humans and actual runtime controls authorize operations; model outputs cannot grant authority |
| Current scope | Document the plan and recovery instructions only; no implementation, install, model download, commit, push or migration |

An existing workflow or SDD skill may itself call Gentle AI. Installing it under a different name does not establish independence. All reachable execution paths in the standalone profile must be checked.

## Product promise and MVP

Primary hypothesis: developers can carry governed skills, project context and recoverable workflows across supported agent runtimes without maintaining a vendor overlay by hand. Validate this with a user other than the maintainer; technical independence alone does not prove product value.

First useful journey:

1. Install Labdrian into a clean environment without Gentle AI.
2. Attach one supported runtime through its supported extension or configuration mechanism.
3. Open an explicitly identified project and discover project skills.
4. Run a bounded, human-authorized task using the runtime's existing agent execution.
5. Record actual checks and recover task progress after restarting the session.
6. Retrieve existing project memory when configured; memory absence is disclosed rather than fabricated.
7. Optionally observe Laya shadow decisions, without changing task routing or permissions.
8. Update, roll back and uninstall Labdrian without deleting foreign configuration or user memory.

MVP: one fully supported runtime, own install/doctor/configuration, minimal recoverable workflow, skills lifecycle, optional memory integration and optional Laya experiment. A second runtime demonstrates that the contract is genuinely reusable before a broad multi-runtime claim.

Non-goals: agent-loop replacement, universal model gateway, mandatory new daemon, marketplace, cloud service, training Laya, cloning all RDD semantics, immediate four-runtime parity, automatic global skill promotion or autonomous memory writes by decision models.

## Evidence and provenance

This document is grounded in earlier repository reads and explicit user choices, not a new exhaustive audit. A delegated planner could not launch because this session and the target are different Git clones. No successful independent review or runtime verification is claimed.

Earlier observed surfaces to revalidate:

| Surface | Prior observation / transition concern |
| --- | --- |
| README.md and bin/labdrian-overlay | Vendor capture, upstream/main merge model, CLI/TUI and install/update paths |
| engine/runtime/ and engine/pipkg/ | Runtime and memory lifecycle; Pi packaging and runner dependencies |
| engine/cmd/main.go | Skills, prespec, runtime, sync and review-receipt command surface |
| skills/inception-pipeline/ | Pre-SDD flow tied to Gentle dispatcher and artifact conventions |
| engine/reviewreceipt/ | Gentle-specific receipt capture; never silently reinterpret |
| longterm-mem/ | Separate Go module; MCP query/get/promote, read-only Engram source and vault promotion |
| openspec/changes/procedural-memory-lifecycle/ | Active work; completion must be freshly reconciled |
| openspec/changes/longterm-mem-knowledge-ingestion/ | Separate ingestion work; preserve fetch/read -> mem_save -> promote boundary |

At documentation time, Git reported modified procedural-memory-lifecycle/tasks.md and apply-progress.md, plus untracked verify-report.md in that change. These files were not edited or evaluated by this planning task. Earlier F.1-F.6 pending claims are historical, not current completion evidence. Existing untracked .agents/, .claude/skills/, .pi/ and skills-lock.json were preserved.

The earlier 31 GiB / 20-thread host snapshot is not a performance guarantee or a release support target. CPU latency and memory for Laya have not been benchmarked here.

## Proposed architecture

```text
Labdrian CLI / TUI and project configuration
  |-- workflow progress and actual verification evidence
  |-- skills ownership / installation / revision / retirement
  |-- runtime capability adapters -> existing execution runtimes
  |-- memory integration -> longterm-mem -> Engram / vault
  |-- optional decision interface -> Laya Python sidecar
  `-- optional Gentle compatibility -> vendor workflows / receipts
```

These are logical boundaries, not a mandate to reorganize every directory or introduce services. Prefer extracting existing behavior in place. Keep one repository initially unless evidence justifies a split.

### Ownership and capability contract

- Runtime adapters describe hooks, tools, subagent execution, cancellation, session events and permissions actually available in a tested version.
- Classify each guarantee as **enforced**, **advisory** or **unavailable**. A prompt instruction is not an enforced security boundary.
- If a required control is unavailable, block the affected operation or explicitly offer a narrower safe workflow; never silently downgrade it.
- Model provider connections initially remain runtime-owned. Do not build another SDK gateway just to route existing tasks.
- The workflow owns progress and evidence references, not a copy of every runtime session or review engine.
- Memory and decision services do not own command permissions.
- Project identity and authorized roots are explicit. Credentials, ambient SSH sessions and remote access never become permissions merely because discoverable.

### Profiles and compatibility

Proposed profiles: standalone and Gentle-compatible. This is product design, not an instruction to change the current session's installed harness.

Standalone must have no hidden Gentle invocation, copied dispatcher requirement or gentle-pi-only subagent dependency. Compatible mode may retain Gentle integration, including genuine receipts, under its own contracts. Missing native review is reported as unavailable/not run, never approved. Functional check evidence and review authority are separate concepts.

## Historical package source scopes and acceptance

The following P-label sections preserve the source package's scope and acceptance rationale for the master crosswalk. Their old dependency statements and proof descriptions are planning provenance, not current status or a separate active sequence. The master records any verified status and canonical relationship; absent current evidence, status is unverified.

### P0 source — Reconcile dependencies, ownership and licenses

Source-plan relation: no prerequisite stated; read-only first.

Outputs: current change inventory; dependency table covering build, install, startup, ordinary task, resume, archive, review and uninstall; license/provenance inventory for vendor assets, code, models and weights.

Acceptance: each mandatory Gentle dependency has an exact callsite or artifact reference and a disposition (retain behind adapter, replace minimally, or exclude from standalone). Active/uncommitted work has an owner/boundary. The first-runtime recommendation is based on actual capabilities without gentle-pi. Redistribution obligations are identified before copying/relabeling vendor assets.

P0 stability follow-ups carried into the standalone transition:

- **Codex body-loading proof:** discovery/name exposure is not sufficient. A clean-environment smoke test MUST prove that a supported Codex version loads the complete project skill body, or the capability MUST remain explicitly `unverified` and outside the supported-runtime claim.
- **Retirement rollback-failure proof:** the retirement engine MUST exercise both partial rollback and rollback-of-rollback failure paths, including the repo-relative diagnostic, before the standalone lifecycle contract is called release-ready.
- **Release smoke gate:** a clean supported environment MUST cover install, first bounded workflow, restart/resume, update, rollback, and uninstall. A green unit suite alone does not satisfy this gate.

Proof: bounded source references and observed read-only status; unknowns remain marked. Do not claim a byte-for-byte audit from a sample. Preserve all unrelated work. Each follow-up retains an explicit `passed`, `failed`, `skipped`, or `unverified` result with its environment and command evidence.

### P1 source — Specify minimal standalone contracts

Source-plan relation: P0 and owner decision D1 were stated as prerequisites; current dependency is unverified. Shared Goal and Workflow Profile acceptance maps once to Phases 1 and 2; the additional contracts here remain distinct.

Outputs: versioned configuration, explicit project identity, workflow progress/evidence contract, runtime capability matrix and decision capability draft. Separate advisory policy from enforceable runtime controls.

Acceptance: the MVP workflow can be represented without Gentle commands or receipt schemas. Unsupported capabilities and version mismatches have explicit outcomes. Scope and authority do not grow through delegation or decision outputs.

Proof: fixture scenarios for restart, wrong project, unavailable capability and contradictory configuration; review each proposed abstraction against two runtime shapes before building it.

### P2 source — Independent installation and reversible migration

Source-plan relation: P0-P1 and licensing disposition were stated as prerequisites; current dependency is unverified. This install/migration acceptance remains distinct from Phase 6 workflow lifecycle.

Outputs: standalone installation path and doctor; versioned state ownership; optional compatibility installation; migration preview, backup and rollback design for existing ~/.labdrian-overlay state. Choose final paths before migration, not from this illustrative plan.

Acceptance: fresh install does not require vendor branches, backups or Gentle binaries. Migration preserves settings, symlinks, ownership, memory and provenance. It refuses ambiguous foreign-owned destinations. Updating one component does not implicitly install a model or alter another runtime.

Proof: isolated HOME/config-root fixtures covering fresh install, repeated install, interrupted migration, collisions, rollback and uninstall. No tests against live user configuration.

### P3 source — One independent vertical workflow

Source-plan relation: P1-P2 and a selected reference runtime were stated as prerequisites; current dependency is unverified. Its first useful user journey remains distinct from lifecycle and adapter contracts.

Outputs: one adapter, a small independently owned workflow and actual check evidence with restartable progress. Preserve useful existing skills rather than recreating a full orchestrator.

Acceptance: in an environment with Gentle AI and gentle-pi absent, the MVP journey completes. Tool execution stays native to the runtime. Cancellation and resumption do not duplicate writes. If subagents are essential, an independent supported runner exists; otherwise advertise a bounded single-agent path honestly.

Proof: clean-environment end-to-end recording; runtime/version/capability report; denied action and restart tests. Capture exact commands and results when the implementation determines them; this plan invents none.

### P4 source — Compatibility isolation and memory continuity

Source-plan relation: P2-P3 and reconciliation with active memory changes were stated as prerequisites; current dependency is unverified. Compatibility behavior and memory continuity remain linked to, but not presumed identical with, Phases 5/6.

Outputs: optional Gentle adapter and regression coverage; independent lifecycle registration for existing longterm-mem; migration support for legacy workflows and artifacts.

Acceptance: core works with compatibility disabled; enabling it preserves vendor semantics. Original receipts remain owned/interpreted by their issuer. Memory query/get/promote and sync retain project confinement and provenance. No Laya code or inference dependencies enter longterm-mem.

Proof: compatible/standalone profile tests, memory unavailable tests, existing module regression commands, foreign configuration preservation and uninstall isolation. Do not broaden longterm-mem tool or network permissions as a migration shortcut.

### P5 source — Laya protocol and offline adapter contract

Source-plan relation: P1; the source allowed independent work relative to P3 after interface stabilization. Current dependency is unverified. This optional decision-backend contract has no asserted phase equivalent.

Outputs: decision provider interface with declared supported question kinds; deterministic fake backend; proposed Python stdio JSONL contract; configuration and fixture tests. Validate exact upstream question/answer semantics before finalizing schemas.

Acceptance: explicit project/request identity, protocol and model revision, question validation, bounded response size, timeout, cancellation, abstention and error semantics. Preserve raw answer meaning while exposing a small normalized envelope; do not manufacture probability distributions for backends that return only scores.

Proof: malformed JSON, oversized input/output, unknown protocol/question, mismatched response ID, unsupported capability, EOF/crash, cancellation and schema round-trip tests. All tests in this package can run without weights or a network.

### P6 source — Local Laya lifecycle and shadow observation

Source-plan relation: P3, P5 and upstream license/runtime validation were stated as prerequisites; current dependency is unverified. This remains distinct from procedural memory and general observability.

Outputs: optional pinned Python environment/checkpoint installation; one resident worker initially; bounded queue and deadlines; shadow-only hook and privacy-conscious telemetry.

Acceptance: explicit setup/download consent; no downloads during routine apply, sync or request handling. Sidecar is optional and has no Engram/vault write capability. Shadow cannot change routing, prompts, permissions or delivered answers; synchronous overhead is bounded and measured. Absence/failure uses the existing authorized baseline and does not weaken a safety gate.

Proof: model absent, slow startup, hung inference, queue saturation, wrong-model response and worker crash cases; decision-disabled baseline comparison; clean removal without memory loss. Record CPU cold/warm timings and peak/steady RSS rather than extrapolating upstream GPU claims.

### P7 source — Evaluate specialized decisions and gate advisory use

Source-plan relation: P6 and owner-approved evaluation thresholds D2 were stated as prerequisites; current dependency is unverified. This evaluation/admission scope remains distinct from general product observability.

Outputs: versioned evaluation corpus and report comparing Laya with deterministic rules and current behavior. Use research/planning/implementation classification first; evaluate choice, ordinal and uncertainty outputs separately where supported. Include Spanish, English, mixed-language, ambiguous and adversarial requests.

Acceptance: human labels and ambiguity policy documented; separate calibration/tuning and held-out data. Report classwise precision/recall, confusion, abstention coverage, latency percentiles and memory. Use Brier/ECE only for appropriate probabilistic outputs, not arbitrary scores. No claims that calibrated training prevents all errors.

Proof: reproducible run with code/model/corpus versions; error analysis and threshold rationale. Enabling advisory mode requires an explicit decision and evidence of useful improvement within the resource budget. If it fails, retain shadow or disable it; the independent platform can still release.

### P8 source — Prove portability and ship the independent product

Source-plan relation: P3-P4, and P6 for a Laya preview claim, were stated as prerequisites; P7 was required only for advisory claims, not platform release. Current dependency is unverified. Product release acceptance is distinct from overlay-versioned-releases.

Outputs: second-runtime capability-tested adapter, onboarding, supported-version matrix, release assets, upgrade/rollback procedure, troubleshooting and migration notes. Define reference-runtime MVP separately from general multi-runtime support.

Acceptance: another user completes the clean-install journey without maintainer intervention. No Gentle/gentle-pi dependency in standalone smoke tests. Unsupported runtime controls are disclosed. Laya is visibly experimental until validated. Published assets retain required licenses/notices and have reproducible version/provenance information. The release smoke gate passes install → bounded workflow → restart/resume → update → rollback → uninstall; Codex body loading is either proven for the advertised version or excluded from support; retirement rollback and rollback-of-rollback diagnostics are exercised and recorded.

Proof: fresh environment and upgrade-from-legacy journeys; uninstall preservation; supported runtime/version matrix; successful and failed task evidence; exact Codex smoke output; retirement rollback-failure output; release-gate command log. Owner separately authorizes commit, publishing and release operations.

## Laya adapter design details to validate in P5

Proposed transport: persistent Python child process, one JSON object per line on stdin/stdout, logs on stderr. Prefer this over per-request model loading or a loopback server initially; benchmark startup and recovery before finalizing. No implicit MCP server is required.

Request envelope should contain protocol version, request ID, explicit project, bounded state, typed questions and a deadline. Backend selection is trusted configuration, not an arbitrary model identifier or executable supplied in the user's state.

Response envelope should identify protocol/request/project, backend and checkpoint revision, status, typed answers and timing. Suggested statuses: ok, abstained, unavailable, timeout, invalid_request, invalid_response, internal_error. These names are proposals, not a published protocol.

Capabilities describe supported question types, languages tested, batching and score semantics. Treat choice/score/noul as upstream terms until checked against a pinned Laya version. A new model can support a subset. Jev is not an implementation commitment without identifying its actual artifact and license.

Resource policy: one model worker, bounded input/tokens and queue; separate startup/inference deadlines; terminate and reap hung children; limited restarts and a disabled/unhealthy state after exhaustion. Weight cache remains outside the repository/vault. Offline runtime operation after explicit setup must be verified rather than assumed from library defaults.

Privacy policy: metadata-only telemetry by default (version, duration, status, aggregate outcome); no raw prompt, secret or memory content. Define retention and deletion before enabling telemetry. Content-bearing evaluation samples require explicit permission/redaction. Untrusted document text is data, not configuration or executable instructions.

## Verification and release evidence

For each work package, record: actual changed surfaces, commands run and observed results, failed/skipped checks with reasons, proof location, open risks and next step. Resolve exact test runners and configured TDD from the target repository when implementation is authorized. Existing tests do not imply TDD is enabled.

Normalize before verification. Run applicable functional checks; preserve native review policy where it applies without inventing approval or turning it on. These roadmap items are not review receipts or commit authorization. Split future implementation into coherent work units with tests/docs, not a single accumulated migration patch.

Key release invariants:

- No required Gentle AI or gentle-pi in the standalone install and execution path.
- No permission escalation through Laya, a skill, a model suggestion or missing runtime hook.
- No writes to foreign runtime configuration or memory outside proven ownership.
- Project identity, stored progress and memory remain consistent across restart/worktrees.
- Failed or absent decision inference preserves the authorized baseline.
- Legacy migration and rollback preserve data and accurately report partial outcomes.
- No performance, multilingual or calibration guarantee without local evidence.

## Open decisions and risks

| ID | Decision | When needed |
| --- | --- | --- |
| D1 | First runtime and essential enforced capabilities; recommend from P0 evidence, do not ask the user to guess APIs | Before P1/P3 implementation |
| D2 | CPU/RAM/latency budget and acceptable classification/abstention quality | Before P7 advisory admission |
| D3 | Final config/state paths and supported migration versions | Before P2 migration implementation |
| D4 | Exact Laya package/checkpoint revisions, licenses and question schemas | Before P5 schema freeze and P6 download |
| D5 | Runtime/version/OS support and release license/distribution obligations | Before P8 release |
| D6 | Exact identity/API/license of Jev | Only if a future adapter is proposed |

Primary risks: hidden Gentle coupling inside skills; runtime hooks that cannot enforce promised policy; model license changes; destructive state migration; resource overhead from shadow inference; uncalibrated confidence; adding too much framework before one useful journey works. Mitigation is evidence-first extraction with optional components, not immediate replacement of every subsystem.

## Status and next step

This source plan's earlier “all P0–P8 planned” statements and checkbox ledger are historical, not current status evidence. Use the master roadmap for canonical status and next action. Reconcile current implementation, task evidence, and the historical SDD source before making completion or dependency claims. No implementation is authorized by this supporting rationale.

## References and persistence

- [Sole active master roadmap](../../openspec/decisions/standalone-platform-roadmap.md)
- [Fresh-session guide](standalone-product-start-here.md)
- [Earlier Laya-only plan](laya-system-one-overlay-plan.md) — historical context; current completion claims require revalidation
- [Laya model card](https://huggingface.co/convaiinnovations/laya)
- [Laya source](https://github.com/NandhaKishorM/laya)
- [Laya package](https://pypi.org/project/laya/)

External links identify sources, not validated/pinned dependencies. P0/P5 must inspect current licenses and exact versions. The repository document is the primary recovery artifact; Engram is a secondary locator/context copy. Files are local and uncommitted until the owner authorizes delivery; a separate clone will not receive them automatically.
