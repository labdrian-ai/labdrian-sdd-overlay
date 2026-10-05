# Hexagonal target architecture (Phase 9, C17)

> Produced by the read-only C17 audit on 2026-10-01 (ultracode workflow `wf_2e59e99b-d33`: seven package-group mappers, one adversarial challenger, one planner) against `main` `4505259`. It is the reference for every Phase 9 refactor. Governing principle (user): hexagonal architecture and clean code to the maximum. Decisions D1–D6 below were taken by the user on 2026-10-01 and override the recommendations in section 5 where they differ.

Source: repo at `main` 4505259, read-only. Inputs are the reader findings and the adversarial verdicts. Everything below is confirmed or confirmed-with-correction. Duplicates are merged and refuted items are listed in §6. I re-checked these myself:
- `tui/run.go:599` sets `allSelected` from the length of `AllTargets()`, and `bin/labdrian-overlay:133` adds `[pi]`.
- `engine/shaper/readiness.go:7` imports `gitprov`.
- `engine/cmd/main.go:917` calls `syscall.Flock` with `LOCK_EX` and no bound.
- `go list`: `capability` imports `workflow`, `os`, `go/parser`; `projection` imports `workflow` and `capability`.

---

## 1. Target picture

**Dependency rule.** Imports point inward only: composition root → driving adapters → application → domain, and driven adapters → ports (which domain or application owns) → domain.
- Domain packages import only the pure standard library and other domain packages. That excludes `os`, `os/exec`, `syscall`, `net`, `io/fs` walking, `time.Now`, `crypto/rand`, and file-layout formats.
- Only `engine/cmd/*`, `longterm-mem/cmd/longterm-mem` and `tui/main.go` build concrete adapters.
- A go-list architecture test enforces this (H1).

| Domain (bounded context) | Domain packages (pure) | Ports it owns | Driven adapters | Driving adapters / root |
|---|---|---|---|---|
| Skills lifecycle | `skills` (model, Validate, planners, lint, match), `skills/app` (use cases) | RegistryRepository, ProjectFS, SkillSource, ManifestRepository, ApprovalRecordStore, ProjectLockStore, Locker (exists), ProjectIdentity, PathResolver, Clock (exists) | `skills/registryyaml` (versioned, tolerant), `skills/skillsfs`, identity chain (explicit → git-config origin → dir name), `filelock` | skills CLI adapter in `cmd`; `cmd/main.go` |
| Workflow | `workflow` (events, hash chain, state machine, Lifecycle), `workflowprofile`, `goal`, `roles`, `memoryscope` | EventLog, ProfileCatalog, GoalReader / RoleChainReader / DependencyProber (exist), Clock (exists) | `workflow/filelog`, `roles/filechain`, `capability/presence` | `cmd/workflow.go` |
| Projection and gating | `projection` (Project, Gate, Binding), `gate`, `contract` (new), `propagator` | BindingStore, WorkflowReader, RepoLocator, RegistryStore | `projection/fsstore`, `gitfs` (RepoKey, pointer parse shared with gitprov), `hookwire` (Claude hook protocol) | hook commands in `cmd` |
| Readiness / shaping | `shaper`, `prespec` | ContainedSource, ClearanceStore, WorktreeProvenance (value type) | `shaper/fsadapter`, `gitprov` | `cmd/shaper.go`, `cmd/prespec.go` |
| Review receipts | `reviewreceipt` (receipt parsing, approval selection, hook verdict) | TransactionStores, ReceiptSource, ReceiptSink, ChangeCatalog | `reviewreceipt/fsstore` (on top of gitprov) | `cmd` hook |
| Runtime projection | `runtime/core` (Target vocabulary, Adapter port, prompt rules), `capability` | Adapter + Registry (`Factory(Config)`), CommandRunner, PackageBuilder, HookInstaller | `runtime/<claude,codex,opencode,pi,longtermmem>`, `settings` (Claude settings.json), `pipkg` (+ SourceRepo port, exec-git adapter) | `cmd` runtime/status |
| Shared infrastructure | `jsonstrict`, `pathguard` (pure half) | — | `statestore` (StateHome, private dirs, OpenNoFollow, AtomicPublish, SyncDir), `atomicfile`, `filelock`, `pathguard/fsresolve` | — |
| Memory (longterm-mem) | `internal/memory` (new: Observation, Row, Standing, Edge, tokenizer, snippet), promote rules, staleness findings, `projectid` rules, `ingest` | ObservationLister/Searcher/StandingReader/CoverageReader, VaultRepository, AddressAllocator, IndexRebuilder, VaultRetriever, EmbeddingIndexRepository, RepoEvidence, RepositoryInspector, SkillSource, CommandResolver, SourceReader, Clock | `engram` (SQLite), `vaultfs` (new), `vault` (script runner), `vecindex`, `embed`, `repohistory`, `identityledger`, `register`, `durable` | `mcpserver`, `cmd/longterm-mem` |
| Operator TUI | `tui/internal/domain` | Backend, TargetCatalog, BackupQuery, RepoLocator | `tui/internal/adapters/overlaycli` | `tui/internal/ui`; `tui/main.go` |

## 2. Current state per package group

| Group | Packages | Alignment | Already well aligned (keep, do not churn) |
|---|---|---|---|
| Pure domain | `goal`, `workflowprofile`, `memoryscope`, `jsonstrict`, `propagator`, `gate` (no I/O), `ingest` | **good** | Strict jsonstrict parsing; `Resolve` returns clones; narrowing-only memory scope; `ingest` is pure (go list) |
| Workflow core | `workflow`, `roles`, `projection` | **partial** | Lifecycle already injects clock, Provenance, GoalReader, RoleChainReader, DependencyProber (`lifecycle.go:66-102,206`). `statemachine.go` and the hash chain are pure. `Project`/`Gate` are pure (`context.go:244`, `gate.go:149`). `cmd/workflow.go:192-210` is textbook wiring |
| Shaper / prespec / capability | `shaper`, `prespec`, `capability` | **partial** | `Evaluate` does no I/O. `NewIDFrom(t, r)` is injectable (`brief.go:21`). `capability.go`, `validate.go`, `declarations.go` are pure. PresenceProber uses an injected StatFS |
| Skills | `skills`, `pipkg`, `pathguard` | **poor** (`skills`), **partial** (others) | Locker port + `cmd/skills_lock.go` adapter. Clock injected (`skills.go:34`). Pure planners (`PlanInstall`, `EvaluateOwnership`). The `projectFS` interface (`project_register.go:1043`). `zero_fetch_test.go` guard. `SkipWhenCopying` shared with pipkg |
| Runtime / settings | `runtime`, `settings`, `capability/presence` | **partial** | `runtime.Adapter` port (`runtime.go:69`). Pure `longtermmem_status.go`. Fixed-argv exec. Constructor injection of roots. `hookFamily` (`family.go`) is the target shape. `exec_allowlist_test.go` |
| Infra utilities | `filelock`, `gitprov`, `synctrigger`, `gadu`, `assets` | **good** (adapters in the right ring) | filelock Clock and typed BusyError. gitprov Runner port, scrubbed env, fail-closed checks. synctrigger takes env defaults from its caller |
| Review receipts | `reviewreceipt` | **poor** | Typed fail-closed errors. Idempotent refuse-to-overwrite write. One ApprovedSummary parser reused by archive-anchor-gate |
| Engine root | `engine/cmd` | **partial** (holds use cases) | `wallClockUTC` injection, `runtimeAdapterForTarget` selection, thin roles/memory/capabilities adapters; every `run*Core` takes injected I/O |
| longterm-mem | `engram`, `promote`, `query`, `ops`, `skillstale`, `staleness`, `projectid`, `vaultreg` | **poor** (domain types live in the SQLite adapter) | `mcpserver` Deps of functions. `ExplicitPromote`'s ObservationLookup port. `vecindex` Embedder port with injected `now`. `embed` is the only net importer. `durable` reused. `identityledger` injects its clock. `repohistory` |
| Bash | `bin/labdrian-overlay` | **partial** | Most verbs are pass-throughs to the engine. pipkg and settings delegate. `engine_binary_is_stale` is the single staleness rule |
| TUI | `tui` (package main) | **poor** internally; **good** boundary | Out-of-process boundary (no engine import). `classify`, `buildArgSets`, `ParseSyncCheck` are pure and tested. `pendingTargets` invariant. View has no I/O |

## 3. Confirmed work units (ODD-ready)

Severity uses the corrected values. Size: S ≲100, M ≲250, L ≲400 authored lines, XL = several slices. "Prec." means must precede.

### Phase A: guard and shared infrastructure primitives (enable every later cut)

| ID | Title | Violations covered | Packages | Size | Risk | Deps | Phase 9 relation |
|---|---|---|---|---|---|---|---|
| H1 | Architecture fitness test: per-package ring declaration; domain must not reach `os`/`os/exec`/`syscall`/`net`/adapter packages; today's violations allowlisted and removed as each H lands | engine-no-architecture-fitness-test | engine (and the same test in longterm-mem) | S | low | — | **New**; prec. all |
| H2 | `engine/statestore` + `engine/atomicfile`: one StateHome, private dirs, OpenNoFollow, AtomicPublish, SyncDir, WriteAtomic(perm, backup, fsync) | state-store-plumbing-triplicated, workflow-statehome-infra-helper, skills-atomic-write-dup (cluster incl. runtime/cmd/settings copies) | new packages only (adoption happens in later H items) | M | low | H1 | **New**; prec. H5–H9, H16, C19 |
| H3 | Single flock: cmd `acquireRegistryLock` → `filelock.Acquire` (fixes the unbounded wait); workflow/projection use filelock; flock syscall becomes a field, not a package var | filelock-flock-reimplemented-elsewhere, filelock-flock-var-seam | filelock, cmd, workflow, projection | M | medium (lock semantics) | H1 | **New** |

### Phase B: cut domain free of infrastructure (moves, with the port defined at the cut)

| ID | Title | Violations covered | Packages | Size | Risk | Deps | Phase 9 relation |
|---|---|---|---|---|---|---|---|
| H4 | shaper owns `WorktreeProvenance`; cmd maps `gitprov.Observation` | shaper-domain-imports-gitprov-adapter (high) | shaper, cmd/shaper.go | S | low | H1 | **New** |
| H5 | Workflow ports: `EventLog` + `ProfileCatalog` injected via NewLifecycle; every operation uses them | workflow-lifecycle-depends-on-concrete-store (high), workflow-resolveprofile-seam-bypassed | workflow, cmd | S | low | H1 | **New**; prec. C20 |
| H6 | Move the workflow store to `workflow/filelog` on statestore + filelock; pure `classifyWorkflowLog`; Append split into lock → load → admit → publish | workflow-file-adapter-in-domain-package (high), workflow-store-append-oversized | workflow, workflow/filelog | L | medium (log format must stay byte-identical) | H2, H3, H5 | **New**; prec. C20 |
| H7 | Move ChainStore to `roles/filechain` on statestore | roles-chainstore-adapter-in-domain-package (high) | roles, cmd | M | low | H2 | **New** |
| H8 | `BindingStore` port; move Store + nofollow to `projection/fsstore`; stateHome from cmd; lockWait as a field (removes projection→workflow infrastructure edge) | projection-store-adapter-in-domain-package, projection-domain-imports-workflow-for-state-and-types | projection, cmd | M | medium | H2, H3, H6 | **New** |
| H9 | Capability purity: PresenceProber → `capability/presence`; CheckEvidence → a pure check in `capability` behind a `TestCatalog` port, with the `go/parser` scanner as `capabilitytest.NewCatalog` (batch 5); drops the capability→workflow edge | capability-presence-adapter-colocated, capability-evidence-reads-filesystem (low), capability-domain-imports-mixed-workflow | capability, cmd | S | low | H6 | **New** |
| H10 | Shaper fs adapter: `ContainedSource` + `ClearanceStore` ports; move `contained*`, `clearance_store`, file halves of BindGoal/LoadHandoff to `shaper/fsadapter`; open hook becomes a struct field; `cli_support.go` → `disclosure.go` (S1 `ClearanceStore` in batch 5; S2 `ContainedSource`, the read and the hook field, and S3 the `disclosure.go` rename in batch 6, which left `shaper` free of `os`, `runtime`, `syscall` and `unsafe`) | shaper-file-io-in-domain-package (high), shaper-global-test-hook, shaper-cli-disclosures-in-domain | shaper, cmd | L | medium | H2, H4 | **New** |
| H11 | Prespec: `PrespecCore` → `cmd/prespec.go`; clock and entropy injected; NewID removed | prespec-cli-handler-in-domain, prespec-time-now-and-crypto-rand | prespec, cmd | S | low | H1 | **New** |
| H12 | reviewreceipt: dedupe scan, typed `ApprovedReceipt` + pure Parse, constant config (slice a, S). Then ports TransactionStores/ReceiptSource/ReceiptSink/ChangeCatalog and a `reviewreceipt/fsstore` adapter on gitprov (slice b, L; verify the hook cwd is always the toplevel) (both slices in batch 7; slice b in two commits, the ports and the adapter, then the callers; the hook cwd is not always the toplevel, so `gitprov.Locate` finds the worktree from any directory inside it; `reviewreceipt` is free of `os` and `os/exec`) | reviewreceipt-domain-coupled-to-git-and-fs (high), own-git-resolution, duplicated-store-scan, wide-tuple-return, global-store-path-var | reviewreceipt, cmd, tools/archive-anchor-gate | S+L | medium (delivery hook) | H2 | **New** |
| H13 | Pure `engine/contract` (one Parse, one strict list parser); gate/propagator/runtime/cmd consume it; fix the nil-error return at `gate.go:224`; literals; drop legacy Config fields (batch 8: `engine/contract` is the one `Parse` and the one strict list parser, in the domain ring; `gate`, `propagator`, `runtime` and `cmd` consume it, `gate` no longer imports `propagator`, the unreachable nil-error branch went with the second pass over the frontmatter, and `gate.Config` keeps only `Contracts` and `WorkContext`; the context metadata is parsed apart, by `contract.ParseContext`, and only the readers that use it (the gate and the OpenCode contracts that carry context) validate it, so `propagate`, the status check and the OpenCode minimalism contract ignore it as before; the strictness change is in `CHANGELOG.md`) | gate-frontmatter-reparsed-and-second-list-parser, gate-depends-on-registry-writer, propagator-hosts-shared-contract-parser, gate-latent-nil-error | contract (new), gate, propagator, runtime, cmd | M | medium (lenient→strict, D4) | H1 | **New** |
| H14 | `engine/hookwire` adapter: one PreToolUse/UserPromptSubmit decoder and allow/deny/updatedInput encoders; gate, projection, shaper/guard, skills/approve_guard, reviewreceipt/hook take typed values; the edit-tool list becomes Gate input (batch 9: `engine/hookwire`, in the adapter ring, imports nothing of the module and is the one home of the hook's JSON tags (a test reads the module and fails on a tag of the protocol anywhere else). Slice 1 moved `projection` and `gate` onto it: `projection.GateInput` has the edit tools and a `ToolCall`, `gate.Rewrite` takes a `Call`, and neither knows JSON; `cmd` supplies the edit tools (`gatedEditTools`) and maps each decoded value and each decision (`cmd/hook_translate.go`). Slice 2 moved the guards: `shaper.DecideGuard` takes a `GuardCall`, `skills.DecideApproveGuard` an `ApproveGuardCall`, `reviewreceipt.Service.CheckCommand` a command, and each returns a verdict; the bound of the approve guard's input moved to `cmd`. The decoders are several, not one, on purpose: each reads exactly the fields its consumer always read, because each hook decides for itself what to do with an input it cannot use, and the goldens pin it; batch 10 gave the clearance guard's read and the Agent decoder a named bound (`hookwire.MaxToolCallBytes`, `MaxAgentCallBytes`), read the arguments of a call only for a memory query (`ToolCall.ReadQuery`), and scoped the compatibility wording of the clearance denial to the one guard (`hookwire.ClearanceGuardDetail`, which finds the decoder's type name by the type, so renaming it changes no byte)) | gate-duplicated-hook-wire-structs, gate-hook-wire-format-in-policy, projection-hook-wire-format-in-domain, skills-approve-guard-hook-format | hookwire (new) + 5 consumers, cmd | L (2 slices) | medium (hook output bytes; golden tests) | H13 | **New** |

### Phase C: skills domain (Phase 9 C19 and C21 live here)

| ID | Title | Violations covered | Packages | Size | Risk | Deps | Phase 9 relation |
|---|---|---|---|---|---|---|---|
| H15 | `RegistryRepository` port + pure `Registry.Validate`; parse/serialize → `skills/registryyaml` (no Validate call inside); replace the 14 call sites; pipkg uses the port (file- and dir-backed) (batch 10: `skills.RegistryRepository` (`Load`, `Decode`, `Encode`) is the domain's and `skills/registryyaml` implements it; `Registry.Validate` is pure and the domain applies it (`ReadRegistry`, `DecodeRegistry`), the adapter holds no call to it (a test reads the package and fails on one); a decoder that fails hands back the entries that were whole before the fault, so the domain names the first fault in the order of the file, as the file was always read; the verbs, `pipkg` and the Pi runtime adapter take the repository they are given and `cmd` constructs it (`registry_repository.go`); 58 golden files recorded from the unchanged program pin what every verb prints and leaves, and none changed with the move; the one difference found is the words `pipkg` says for a registry path that is a directory) | skills-registry-yaml-in-domain (high), pipkg-registry-direct-parse | skills, registryyaml, pipkg, cmd | L | medium | H1 | **= C19 part 1** (architecture only) |
| H16 | Q5 reader policy in the adapter: dispatch on version, tolerant of unknown fields, must-understand set, versioned decoders (batch 10: the file's `version` is found first and selects the decoder (`decoders`, with the field table of each version in `schemas`), and a version no decoder is for is refused, naming it; a key the reader does not know is left out and said (`Registry.Unread`), the verbs that read warn and go on, and `add`, `remove` and `Encode` refuse a registry that has one; the must-understand set of version 1 is computed from the schema table (`registryyaml.MustUnderstand("1")`) and explained field by field at the top of the package; in a shape the reader does not read, a field of the set is refused, naming the field and the line) | Q5 | registryyaml | M | medium (behavior change, already decided in Q5) | H15 | **= C19 part 2**; closes #312 |
| H17 | Skills fs adapter `skills/skillsfs`: osProjectFS, productionInstallEnv, ScanSkillFiles walk, manifest open, install_source walk, writes via atomicfile; one Deps built in `cmd/main.go`; Locker gains `Exists`; drop `os` from `zero_fetch_test.go` | skills-concrete-fs-in-domain (high), skills-lock-reread-uses-os | skills, skillsfs, cmd | XL (≈3 slices by verb group) | medium | H2, H15 | **New**; should precede C21's declaration verb |
| H18 | `ProjectIdentity` port + ProjectID; adapters explicit → `.git/config` origin → dir name chained in cmd; delete `installCwd` | skills-project-identity-inline (medium), skills-installcwd-global | skills, cmd, identity adapter | M | medium | H15, D2 | **= C21** (verb R-046/R-047 writes through H15) |
| H19 | Skills JSON codecs: use jsonstrict (duplicate-key + UTF-8 checks); ApprovalRecordStore/ProjectLockStore in the skillsfs adapter | skills-json-records-codec-dup | skills, skillsfs | S | medium (now rejects duplicate keys, D4) | H17 | **New** |
| H20 | Skills CLI split: `skills/app` use cases + CLI adapter in cmd; one strict flag parser | skills-cli-in-domain (XL), skills-flag-parser-dup | skills, skills/app, cmd | XL (verb by verb, ≈4–5 slices) | medium (install/add flag strictness, D4) | H15–H19 | **New** |
| H21 | Split `project_register.go` (1538 lines) into plan_register/revise/retire, stager, adapter; break up the 100+-line planners; also planSkill, prepareInstall, SyncManifest | skills-project-register-god-file | skills | L | low | H17 | **New** |
| H22 | pathguard: pure half stays, `pathguard/fsresolve` adapter; delete the skills wrappers; pipkg uses pathguard (adds the dangling-symlink refusal test) | pathguard-pure-and-fs-mixed (low), skills-pathguard-wrappers, pipkg-resolve-ancestors-dup | pathguard, skills, pipkg | S | low | H17 | **New** |

### Phase D: runtime projection (Phase 9 C18 and C10-Pi live here)

| ID | Title | Violations covered | Packages | Size | Risk | Deps | Phase 9 relation |
|---|---|---|---|---|---|---|---|
| H23 | Runtime registry: `Factory(Config) Adapter`, explicit registration in cmd (no init globals), unregistered target is an error; one target vocabulary (capability owns it; runtime and skills validate against it); delete NewFoundationAdapter/runtimeAdapterForTarget (fixes `--config-root` being ignored for pi); HOME/XDG resolved once in cmd | runtime-adapter-registry-duplicated-if-chains, capability-target-vocabulary-duplicates-runtime, skills-runtime-targets-hardcoded (low), runtime-home-resolution-duplicated | runtime, capability, skills, cmd | M | medium | H1 | **= C18** (D3 settles self-registration) |
| H24 | Pi adapter: `CommandRunner` (LookPath, Run with deadline) replaces LABDRIAN_PI_BIN and the skip-env seams; parse piSettings once; explicit home; `PackageBuilder` port to pipkg with `Options{DeployRef}` | runtime-pi-subprocess-no-commandrunner-port (medium), runtime-pi-ambient-home-and-duplicated-settings-reads, runtime-pi-adapter-calls-pipkg-with-hidden-env, pipkg-env-config-seam | runtime/pi, pipkg, cmd | M | medium (live Pi; must never run real `pi`) | H23 | **= C10 (Pi CommandRunner part)** |
| H25 | pipkg `SourceRepo` port + exec-git adapter; shared SKILL.md frontmatter reader exported by skills (kept separate from contract frontmatter); file split | pipkg-git-exec-inline, pipkg-frontmatter-dup, pipkg-oversized-file | pipkg, skills, runtime/pi | L | medium | H15, H24 | **New** |
| H26 | `runtime/core` split (vocabulary, port, prompt rules; no os); pure opencode contract parsing over a content source; split pi.go/opencode.go | runtime-domain-prompt-rules-colocated, runtime-oversized-adapter-files | runtime | M | low | H13, H23 | **New** |
| H27 | Settings: pure Document merge/remove + File adapter on atomicfile (slice a); the five legacy families on hookFamily (slice b); GuardCommandMarker moved to a constants package, which drops the settings→shaper→gitprov edge; ClaudeAdapter depends on a HookInstaller interface | settings-god-file-merge-and-io, settings-five-families-not-on-hookfamily, settings-shaper-dependency (corrected), runtime-claude-adapter-depends-on-settings-concrete | settings, runtime/claude | M+M | medium (settings.json bytes; coordinate with C7) | H2 | **New**; coordinate with C7 (hook upgrade) |

### Phase E: composition root cleanup (engine)

| ID | Title | Violations covered | Packages | Size | Risk | Deps | Phase 9 relation |
|---|---|---|---|---|---|---|---|
| H28 | Propagate use case with typed `Outcome` (Absent/Empty/Unchanged/Written) over a RegistryStore port; the retry loop branches on Outcome instead of stderr/stdout text | cmd-propagate-retry-protocol-in-root (medium) | propagator/app, cmd | L | medium (hook path) | H3, H13 | **New** |
| H29 | `gitfs` RepoLocator adapter (RepoKey, pointer parsing shared with gitprov; the RepoKey rule stays pure in projection); `BindWorkflow` service and `projection.HookService` with typed refusals and an injected clock | cmd-git-reader-and-repo-key-in-root, cmd-workflow-bind-usecase-in-root, cmd-projection-hook-usecases-in-root | projection, gitfs, gitprov, cmd | L (2 slices) | medium | H8 | **New** |
| H30 | Status/doctor package over settings helpers (delete `innerHookContainsBinary`); pure `runtime.AggregateStatus`; options struct for parseRuntimeArgs | cmd-status-settings-parsing-duplicated, cmd-runtime-status-policy-in-root | cmd, runtime, settings | M | low | H23, H27 | **New** |
| H31 | cmd `deps` struct replacing the 9 global seams (before* hooks become decorators on fakes); split main.go (2023 lines) per subcommand; synctrigger ChildArgv injected; shaper input fallback → shaper; agent-child env resolved in main | cmd-global-test-seams-in-production, cmd-main-go-god-file (medium), synctrigger-knows-cmd-cli-grammar (low), cmd-shaper-input-fallback | cmd, synctrigger, shaper | M+M | low | H28–H30 | **New** |

### Phase F: workflow profile typing (prepares C20)

| ID | Title | Violations covered | Packages | Size | Risk | Deps | Phase 9 relation |
|---|---|---|---|---|---|---|---|
| H32 | Typed profile data: memory default (scope + sources) and review dependency on WorkflowProfile; `memoryscope.DefaultFor(profile)`; delete `gentleReviewProfiles`; single declarative stage/check table; one stage-prefix rule; remove dead ResolveForCompatibility/ValidateResume | workflowprofile-policies-are-untyped-prose, memoryscope-profile-defaults-keyed-by-name, workflow-gentle-review-profiles-hardcoded, workflowprofile-required-stages-checks-duplicate-catalog, workflowprofile-dead-resume-and-compat-api | workflowprofile, memoryscope, workflow | M | low | H5 | **New**; **prec. C20** (it defines the snapshot content) |
| H33 | Profile snapshot + digest in the created event (event version bump; v1 still accepted); RecordStage/Verify use the snapshot; catalog drift becomes a named finding | workflow-profile-name-only-no-snapshot-q6 (medium) | workflow | L | medium (format) | H5, H6, H32 | **= C20** |

### Phase G: longterm-mem

| ID | Title | Violations covered | Packages | Size | Risk | Deps | Phase 9 relation |
|---|---|---|---|---|---|---|---|
| L1 | `internal/memory` domain (Observation, Row, Standing, Edge, tokenizer, snippet); engram only maps rows; consumer-owned reader ports (Lister/Searcher/StandingReader/CoverageReader) | engram-domain-model-lives-in-sqlite-adapter (high), engram-no-reader-port (high), promote-sync-deps-concrete-engram, mcpserver-leaks-engram-type | engram, memory, promote, query, staleness, skillstale, mcpserver | L (2 slices: types, then ports) | medium | lm fitness test (H1) | **New**; prec. L3–L6 |
| L2 | promote: `AddressAllocator` port (removes promote→vault); Clock injected (delete nowFunc); `ActionNone` as the zero value | promote-spawns-concrete-vault-runner (high), promote-nowfunc-global-seam, promote-action-zero-value-means-created | promote, vault, cmd | M | low | L1 | **New** |
| L3 | `VaultRepository` port + `internal/vaultfs` adapter, in 4 slices (precedence store, address map, pages, sync state/log/index); a single VaultLayout; ops Doctor/Status consume the port | promote-vault-filesystem-inline (high, XL), promote-layout-constants-duplicated, ops-duplicates-promote-vault-layout, ops-direct-fs-reads | promote, vaultfs, ops | XL (4 slices) | medium | L1, L2 | **New**; strongly recommended before C12 vault docs |
| L4 | query ports: Searcher/StandingReader, `EmbeddingIndexRepository`, `VaultRetriever` with query-owned types and a typed not-provisioned outcome; IndexRebuilder; split query.go (953 lines) | query-concrete-engram-vecindex (high), vault-no-retriever-indexer-ports, query-file-oversized | query, vault, vecindex, cmd | L | medium | L1 | **New** |
| L5 | skillstale: pure Detect over Lock, bodies, facts, `CommandResolver`, now; `SkillSource` adapter; now/PATH from cmd; golden lock fixture shared with engine/skills | skillstale-ambient-env-clock-fs (high), skillstale-lock-format-duplicated-across-modules, skillstale-detect-long | skillstale, cmd, engine/skills tests | M | low | L1 | **Partly = C10** (the project-context port for procedural presence should be shaped like SkillSource here) |
| L6 | staleness: `RepoEvidence` port (tree index + repohistory adapter); Detect reuses ClassifyPaths | staleness-walks-filesystem, staleness-detect-duplicates-classifypaths | staleness, repohistory | M | low | L1 | **New** |
| L7 | projectid: `RepositoryInspector` port + fs-git adapter; adoption use case (Established, Ledger, Clock ports) moved out of cmd | projectid-git-fs-in-identity-rules, projectid-adoption-usecase-in-cmd | projectid, identityledger, cmd | M | medium | D2 | **New**; tied to C21 via D2 |
| L8 | Use cases `promote.SyncProject` and `PromoteAndReindex`; observation→vecindex.Row mapping as an index use case; one `newApp` wiring helper; slimmer cmd functions | promote-sync-propagate-orchestration-in-cmd, cmd-repeated-wiring, cmd-long-command-functions | promote, cmd | M | low | L2–L4 | **New** |
| L9 | vaultreg: pure Resolve(flag, env, reg, project); seeding only on explicit write commands; defaults to cmd; `durable.WriteJSON` | vaultreg-resolve-reads-env-and-writes, vaultreg-hardcoded-default-project, vaultreg-atomic-json-writer | vaultreg, durable, cmd | S | low (doctor/status stop creating `vaults.json`) | — | **New** |
| L10 | Seam hygiene: register `InstallStateStore` dependency; vecindex Builder owns dirLocks; vault Rebuild takes a clock; ops testdata → `opstest` | register-saveinstallstate-global-seam, vecindex-global-dir-locks, vault-sentinel-time-now, ops-testdata-production-package | register, vecindex, vault, ops | S | low | — | **New** |
| L11 | Ingestion use case over a `SourceReader` port + fs adapter; amend tasks 2b.2/2b.6 so `ingest` stays pure | ingest-q9-no-port-no-adapter (planning guard) | ingest, sourcefs, cmd | M | low | — | **= C22** |

### Phase H: TUI and bash (driving side)

| ID | Title | Violations covered | Packages | Size | Risk | Deps | Phase 9 relation |
|---|---|---|---|---|---|---|---|
| T1 | **Bug:** target catalog drift. Today "all 3 selected" → `--target all` → the backend also acts on pi. Fix with a TargetCatalog port backed by a backend `targets` subcommand, plus a contract test | tui-target-catalog-drift-from-backend (high) | tui, bin/labdrian-overlay | M | medium | D5 | **New**; aligns with H23's single vocabulary |
| T2 | `BackupQuery` port; latestBackup moved to an adapter (`restore --list` or STATE_DIR-aware) | tui-state-dir-layout-duplicated (high) | tui | S | low | — | **New** |
| T3 | TUI package split (domain/app/adapters/ui); main as composition root; declarative Action fields (ResultKind, RefreshProbe, RequiresBackup, Group), which fixes the "Hooks" header above self-update; structured invocations; single probe parse | tui-runjgo-mixed-roles, tui-newmodel-is-hidden-composition-root (medium), tui-command-string-special-casing, tui-menu-group-header-derived-from-targetagnostic, tui-runbackend-builds-text-transcript, tui-probe-duplicated-parse | tui | L (2 slices) | low | T1, T2 | **New** |
| B1 | Engine `manifest routes --target` command; delete bash route_resolve and the grep/awk manifest parsers | bash-manifest-route-domain-duplicated | skills, cmd, bash | M | medium (installer) | H17 | **New** |
| B2 | Engine `registry repair-sdd` under the same lock and atomic write; doctor delegates registry checks to `engine status`; pass the binary path explicitly | bash-registry-markdown-editor (inferred race), bash-two-doctors | propagator, cmd, bash | M | medium | H28 | **New** |
| B3 | longterm-mem installed-targets ownership moved into engine/runtime (pure decision + filelock store) | bash-longterm-mem-ownership-store | runtime, bash | L | medium | H23 | **New** (defer candidate, D6) |

### Phase I: clean-code tail (low; batch into 2–3 slices)

| ID | Covers | Size |
|---|---|---|
| H34 | goal.Digest owner; shared validateStringArray; one IsSHA256Hex/Sha256Hex; assets accessor + doc; propagator markers moved to the cmd table + line iterator; synctrigger clock/runner + usage dedupe; gadu Artifacts() table and in-memory Check; gitprov FS field (optional); roles handoff Validate split; shaper Evaluate split + shaper.go v2/v3; projection render.go; prespec ULID loop; gadu model ids (optional) | M (2 slices) |
| H35 | Test hygiene: shelltest table-driven harness; installer route_test.go split; consolidate the overlay-script test helpers; TUI main_test split + `backend` build tag | M |
| B4 | Split bin/labdrian-overlay into sourced lib modules; deploy-state to the engine only if drift detection grows | L (after B1–B3) |

### Coverage of the decided Phase 9 tasks

| Phase 9 task | Covered by | Must precede it | Notes |
|---|---|---|---|
| C18 (Q7) | H23 | H1 | Fixes the verified pi `--config-root` divergence |
| C19 (Q5) | H15 + H16 | H1 (H2 helpful) | Correction applied: the architecture move and the reader policy are separate slices. Q5 already decided the policy, so H16 needs no new decision |
| C20 (Q6) | H33 | H5, H6, H32 | H32 defines the typed snapshot content |
| C21 (Q8) | H18 | H15, D2; H17 recommended | Severity corrected to medium (missing port, not a direction violation) |
| C10 (Q11) | H24 (Pi runner), L5 (project-context / SkillSource shape) | H23 | Doctor/detector parts of C10 are outside this audit |
| C22 (Q9) | L11 | — | Amend tasks 2b.2/2b.6 before implementing |

Everything else is new: H1–H14, H17, H19–H22, H25–H32, H34–H35, L1–L10, T1–T3, B1–B4.

## 4. Ordering rationale

1. **Guard first (H1).** Freezing today's edges in a go-list test makes every later cut provable, and blocks regressions like shaper→gitprov.
2. **Shared adapter primitives (H2, H3).** Each domain cut otherwise copies the atomic-write, flock and StateHome code a fifth time. H3 also fixes the only unbounded lock (`cmd/main.go:917`).
3. **Cut domain free (Phase B, then C19 part 1, then H17).** The high-severity items are all wrong-direction edges. Do the cheap ones first (H4, H5, H11 are S), then the store moves. Dependency order matters: workflow (H6) before projection and capability (H8, H9), because both import workflow.
4. **Ports for missing seams (H18/C21, H23/C18, H24/C10, L1–L7).** C18 comes before C10 because the Pi runner rides on the registry's Config. L1 comes before all other longterm-mem work because every consumer imports `engram` types.
5. **Format and behavior adapters (H13, H14, H16/C19-2, H33/C20).** These carry behavior or format risk, so they come after the structure is in place and tests run against fakes.
6. **Composition root cleanup (Phase E, L8, T3, B1–B3).** These need the services from 3–5 to exist first.
7. **Size and duplication tail (H21, H34, H35, B4).** These are mostly mechanical once the logic has moved.

T1 and T2 are user-visible correctness bugs. They can go first, in parallel with Phase A, because they touch only `tui`.

### Enforcement (H1, delivered)

`engine/architecture_test.go` and `longterm-mem/architecture_test.go` hold the guard's two inputs for their module: `rings` declares the layer of every package (domain, application, adapter, root, or support for test-only code), and `knownDebt` lists every violation that exists today, each with the work unit that removes it. The checker itself is the `archguard` module (D7): standard library only, with its own tests, required by engine and longterm-mem from their tests only through a local `replace` (`../archguard`), so neither production build needs it. Its package documentation states what the checker sees and what it cannot see.

- A package with no ring, a ring row with no package, a violation with no `knownDebt` line, and a `knownDebt` line whose violation is gone each fail the test.
- A work unit that removes a violation deletes its `knownDebt` line in the same commit. `knownDebt` only shrinks; a new violation is fixed, not listed.
- When a unit creates a package, it adds the package to `rings`. Adapters go in the adapter ring; a pure split such as `runtime/core` goes in the domain ring.

## 5. Decisions for you

- **D1. Where adapters live in the engine.**
  - (a) A subpackage under the owning domain (`workflow/filelog`, `skills/registryyaml`).
  - (b) A top-level `engine/adapters/<x>` ring.
  - **Recommendation: (a)**, enforced by H1. It is screaming-architecture friendly, and the go-list test keeps the direction decoupled.
- **D2. Shared project-identity normalization (C21 / L7).** `longterm-mem/internal/projectid` cannot be imported by the engine module.
  - (a) Extract a tiny pure `identity` Go module that both modules depend on.
  - (b) Port the logic into the engine, pinned with shared golden test vectors.
  - **Recommendation: (a)**: one owner, both modules depend inward on pure code.
- **D3. Runtime registry registration (C18).** Q7 says "self-registering".
  - (a) Literal `init()` self-registration into a package registry.
  - (b) Each adapter exports a `Register(r *Registry)` that cmd calls explicitly.
  - **Recommendation: (b)**: it avoids a hidden mutable global, and an unregistered target is still an error.
- **D4. Accept the strictness behavior changes?** These are: install/add reject unknown flags (H20), lock and approval records reject duplicate keys (H19), and contract lists use the strict parser (H13).
  - (a) Accept all three, with tests and a changelog line.
  - (b) Keep the lenient behavior where it exists.
  - **Recommendation: (a)**.
- **D5. TUI target source (T1).**
  - (a) A backend `bin/labdrian-overlay targets` subcommand feeds a TargetCatalog port.
  - (b) The TUI sends explicit per-target invocations and never emits `--target all`.
  - **Recommendation: (a)**, with (b) as the immediate stopgap if T1 has to ship before the subcommand.
- **D6. Bash migration scope in Phase 9.**
  - (a) B1 and B2 now (B2 removes an inferred registry race); B3 and B4 later.
  - (b) All of B1–B4 now.
  - (c) Defer all of them.
  - **Recommendation: (a)**.

## 6. Dropped or corrected claims

**Refuted:**
- `installer-test-only-package-name-misleading`: test-only package; a rename would be churn with no gain.
- `memoryscope-newplan-size`: 52 lines, under the ~80-line bar.
- `bin-overlay-legacy-duplicate`: deliberately vendored from upstream (`README.md:95`, staged at `bin/labdrian-overlay:1436-1437`); deleting it would break sync.
- `tui-package-level-style-vars`: immutable, never reassigned, so not a seam.

**Merged as duplicates:**
- pipkg-registry-direct-parse → H15
- runtime-atomic-write-duplicated and cmd-atomic-write-flock-duplicated → H2/H3
- capability-target-vocabulary → H23
- workflow-own-flock and filelock-package-var-syscall-seam → H3
- roles-chainstore-duplicates-statehome and shaper-clearance-store-duplicated-plumbing → H2
- memoryscope-profile-defaults → H32
- gate-depends-on-registry-writer and propagator-hosts-shared-contract-parser → H13
- gitprov-observation-used-as-domain-type → H4
- pathguard-pure-and-fs-in-one-package → H22
- promote-sync-deps-concrete-engram → L1
- query-run-function-long → L4
- cmd-hosts-usecases → L7/L8

**Severity corrections applied:**
- Downgraded from high to medium:
  - skills-project-identity-inline
  - runtime-pi-subprocess
  - workflow-profile-snapshot
  - cmd-propagate, cmd-workflow-bind and cmd-git-reader
  - tui-newmodel
- Downgraded to low:
  - skills-runtime-targets-hardcoded
  - capability-evidence (test tooling)
  - synctrigger-cli-grammar
  - pathguard split (optional until skills drops `os`)
- Upgraded from low to medium: cmd-main-go-god-file.
- Reversed: settings-shaper-dependency was "no change needed"; it is a real edge that pulls in gitprov transitively.

Inferences that are still unverified and flagged in their units: the bash registry lock race (B2) and the TUI STATE_DIR divergence (T2). The reviewreceipt cwd-is-toplevel assumption was checked in H12b and does not hold: the hook's working directory is `$CLAUDE_PROJECT_DIR`, the directory the session started in, which can be any directory of the repository, so the adapter locates the worktree through gitprov from wherever it is.

Nothing in the repo was written, edited or committed.

## Decisions taken (user, 2026-10-01)

- D1 (adapter location): A — adapters live in a subpackage under their owning domain (e.g. workflow/filelog, skills/registryyaml); the H1 architecture test enforces inward dependencies.
- D2 (shared project-identity rule): A — extract a small pure Go module `identity` holding only the normalization rule (no I/O); engine and longterm-mem both depend on it; each module keeps its own .git/config reader adapter.
- D3 (runtime adapter registration): B — each runtime adapter exports Register(r *Registry); the composition root (cmd) calls it per runtime; no init() and no package-level mutable registry; an unregistered target is an error. (User first answered A, then corrected to B when the conflict with the clean-code principle was pointed out.)
- D4 (strictness changes): A — accept all three: unknown install/add flags are errors (H20), lock and approval records reject duplicate JSON keys (H19), contract lists use the strict parser (H13); each with a test, a clear error message, and a changelog line.
- D5 (TUI target source, bug T1): A — the backend exposes a `targets` subcommand as the single source of the target list; the TUI consumes it through a TargetCatalog port and keeps no list of its own; a contract test pins both sides (B, explicit per-target invocations, only as an interim patch if T1 must ship first).
- D6 (bash migration scope): B — all of B1–B4 in Phase 9: B1 manifest routes in the engine, B2 registry repair and doctor in the engine under the lock, B3 longterm-mem installed-targets ownership in engine/runtime, B4 split bin/labdrian-overlay into sourced modules.
- D7 (shared architecture checker, raised by H1): A — extract the checker into the small Go module `archguard` (own go.mod, standard library only); engine and longterm-mem require it from their tests only through a local `replace`, and each keeps its own `rings` and `knownDebt`. The checker is not duplicated.
