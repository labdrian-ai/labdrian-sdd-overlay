# Closure feedback

The chain's records are files at `$XDG_STATE_HOME/labdrian/role-chains/<project>/<goal>/<chain>/<seq>.json` (`$HOME/.local/state` when `XDG_STATE_HOME` is unset). Read the closing `reviewer → delivery` record there.

### Plan

Read what the chain left, at the locators the parent named in its prompts (Chain entry). Do NOT modify hop outputs (anything a hop wrote); the one carve-out is Execute item 3. Omit an optional field whose artifact was never named or cannot be read; never guess it.

- **Feature document**: read the parent-named feature document's Specs and builder/delivery Log; confirm the resolved target exists, is readable and is inside the project repository BEFORE the first write. If the target is missing or unreadable, unnamed, or outside the repository, report it and write nothing. Use a recorded realized diff-stat total for `changed_lines`, or compute a realized diff-stat against the base commit; never use a forecast. Historical archive reports remain read-only evidence, never conversion or deletion targets.
- **Verifier reports**: `<head12>-<n>.verifier.json`, stored beside the review results, when the verifier ran.
- **Upstream requirements brief**: use the located upstream requirements brief for `requirement_count` as the count of EARS R-NNN requirements; otherwise omit `requirement_count`. Do not substitute Specs or task counts.
- **`sdd/{change}/pipeline-state`**: the tier-selection outcome and the ambiguity clarifying question if it fired — the only checkpoints inception-pipeline itself durably records.
- **Stored review results**: `odd/review-results/<feature>/<head12>-<n>.json`, where `<feature>` is the name of the feature document `odd/tasks/<feature>.md` (`odd/review-results/README.md` spells out the layout). This is the ONLY place `approved_tree` is read from (see t1). A stored result records the lens rows its launch was sent (`rows`), so `review_lens_count` is the number of rows recorded in the stored results of the change, one for each row of each launch; omit it when no stored result records a row.
- Do NOT source `changed_lines` from the shaper's tasks artifact: its Review Workload Forecast is a pre-implementation planning guard, not an exact diff count (see `sdd-tasks/SKILL.md`).

#### Storage

Use the parent's resolved storage and exact project/topic locators for every closure read and write. For new writes use memorycard; search project-scoped records and get the full record with available operations; merge bookkeeping updates and read back, while actuals follows the exact replacement contract below. Legacy `engram` placement denotes memory-only placement, not permission to write Engram; use an explicitly available historical source for legacy evidence only. Preserve supported project-file placement and existing topic keys and enums.

#### t0

1. If the available provider exposes typed creation metadata, select the exact project and `sdd/{change}/pipeline-state` topic and use its typed `created_at` field — never a rendered footer or updated-at value. Do not invent export commands, metadata fields, or an adapter.
2. Only when typed historical metadata permits it and no matching pipeline-state record exists, t0 falls back to the earliest `created_at` among the change's own `sdd/{change}/*` records; name the fallback source in `variance_vs_plan`. A record with no `topic_key` cannot match. If typed creation metadata is unavailable, otherwise omit unresolved elapsed time and disclose why under R-021; continue with other valid actuals.

#### t1 and the landing anchor

t1 is `landing_commit`'s committer timestamp, once resolved. After delivery and landing, closure records the anchor in versioned prose; it is never discovered from unversioned state.

1. The chain's review results live in the repository, not under `.git/`, and each is bound to its feature by its path (`odd/review-results/<feature>/`). Committed with the feature, they reach a fresh clone.
2. Closure-feedback WRITES the anchor from an explicitly named landing identity; it never discovers one. Delivery's Log entry is written before the landing exists, so nothing the chain wrote can name the landing commit: the parent names the actual landing commit once the change has landed (a merge, squash or fast-forward commit, whatever its parent count), and a landing commit already recorded in versioned prose (a historical report, an earlier Log entry) is used directly. A delivered chain that has not landed is not closed: wait, and write nothing yet. Once delivery succeeded and the landing commit is named, record it in the feature document (Execute item 3):
   - `landing_commit`, always: the explicitly named commit that landed the successful delivery (the last slice to land when delivered as chained PRs);
   - `approved_tree`, ONLY WHEN review ran for this candidate: `git show -s --format=%T <reviewed head>`, the reviewed head being the `candidate.head` of the stored result that the closing `reviewer → delivery` record binds as its `review-result` evidence.
3. No review ran when no result is stored for this candidate, and also when the stored result's reviewed head does not resolve in this repository (a squashed branch that was deleted): there is no tree to take. Then `approved_tree` MUST NOT be recorded at all. Never synthesize it from `landing_commit`'s own tree: a check comparing a value against itself can never fail, and reporting an unfailable check as verification is a fabricated assurance.
4. When `approved_tree` IS recorded, the anchor is **verifiable rather than merely asserted**: `git show -s --format=%T <landing_commit>` MUST equal the recorded `approved_tree`. When it does not, the anchor is REJECTED, t1 is omitted, and the mismatch is disclosed in `variance_vs_plan` — never trust an unverified SHA.
5. When `approved_tree` is absent because no review ran, t1 STILL resolves from `landing_commit`'s committer timestamp alone — the measurement is not withheld — but the anchor is recorded as **self-asserted**: used, but with no independent authority to check it against.
6. Never search history for the commit that landed the change.
   - A tree hash MUST be used to **verify** a commit, never to **discover** one. Trees are not unique across commits (a content-preserving merge reproduces its parent's tree; `be2c3ca`, `aa81361` and `459b48d` all carry one tree in this repository), so a tree search returns commits of other changes whichever end of the range is picked, and the "earliest carrier" is a guess, not a rule.
   - Scanning history for whichever commit touched the change's own files (`odd/tasks/<feature>.md`, `odd/review-results/<feature>/`) is no safe substitute: a commit of an unrelated change can touch the same files and silently become the wrong t1. There is no folder-scan fallback anywhere in this rule.
   - A historical record whose archive report names a landing SHA in versioned prose (e.g. "merged as `be2c3ca`"): use that SHA directly. It is the recorded identity; the tree is only a cross-check on it.
   - If a tree hash is all that is available and more than one commit carries it, the anchor is ambiguous: t1 is omitted and the ambiguity is disclosed, never resolved by position.
   - If no `landing_commit` was ever recorded (a change predating this convention), t1 is omitted (R-021), `variance_vs_plan` states that the change predates the anchor convention, and no folder-scan re-derivation is attempted.
7. The resolution outcome — verified, self-asserted, rejected, or absent — MUST be named in `variance_vs_plan`, so no reader mistakes a self-asserted anchor (used, but never independently checked) for a verified one (checked against a stored review result).
8. `tools/archive-anchor-gate` is history-only (owner decision 5, ADR C13-S0): it scans only the archive reports under `openspec/changes/archive/`. Its chain adapter is a separate goal (the ADR's Follow-ups); until then the feature-document anchor has no machine check and the rules above are its only guard.

#### Figures

- **`total_wall_clock_hours`** is measured independently of the compute-hour sum. It runs from the tiering go-ahead checkpoint (`sdd/{change}/pipeline-state`) through merge, not archive: post-merge archive-authorization replies are bookkeeping and MUST NOT count as development time. It MUST include interruption gaps (such as provider rate or session-limit resets), not only active agent-compute time. Never derive it by summing `implementation_hours` + `review_gate_hours` + `post_review_fix_hours`: that sum is the agent-compute-time proxy, a distinct and usually smaller unit that must stay separately labeled.
- **`checkpoint_count`** counts one unit per distinct human round-trip reply (R-015), uniformly regardless of how many decisions a single reply resolved. It shares the boundary of `total_wall_clock_hours`, from the tiering go-ahead through merge; post-merge archive-authorization replies are excluded from this count as post-boundary bookkeeping.
  - The role chain routes every hop from the shaper to delivery, so inception-pipeline does not durably observe every live confirmation exchanged during those hops (mid-design product decisions, judgment-day rounds, chained-PR split confirmation, merge authorization). Do not claim "your own record" of checkpoints you never routed, and do not invent a field to proxy them.
  - Always include the durable floor that `sdd/{change}/pipeline-state` carries: the tiering go-ahead (always 1) plus the ambiguity clarifying question if it fired (0-1). Any blocking ambiguity resolved by asking the user counts, whatever phase surfaced it and whichever tier-selection rule fired, not only a rule-4 tier ambiguity. The floor is guaranteed and never dropped.
  - If a fuller count is reconstructable from the session narrative, fold it into the single `checkpoint_count` total (never a separate field) and itemize in `variance_vs_plan` free text which checkpoints were durably observed (via `pipeline-state`) versus reconstructed from the closure narrative.
  - If no non-durable checkpoints occurred, `variance_vs_plan` MUST state this explicitly zero and MUST still name the other half with it: which checkpoints were durably observed via `pipeline-state`, AND that zero were reconstructed from the closure narrative. The validator's half-disclosure rule rejects prose naming only one half, so "all durably observed" alone and "zero reconstructed" alone are both hard stops.
- **Two more figures** go in `variance_vs_plan` prose, because the schema is closed (R-003/R-004/R-005 forbid adding a property, and `additionalProperties: false` is unchanged). Free text is not machine-verifiable.
  - `interruption_hours: <n>` — the observed interruption duration for this change, naming its cause (session reset, provider rate limit, human unavailability). Record it whenever an interruption occurred. Record `interruption_hours: 0` explicitly when none did, because "no line" and "no interruption" are different facts and a reader cannot tell them apart. No skill reads this figure now (R-011, which excluded interruption-contaminated samples from the round-trip-latency calibration, was retired with that calibration); it stays a record of what interrupted the change.
  - `realized_slices: <n>` — how many review candidates actually landed, counted from what exists and never from the plan: the PRs delivered under the chosen `chain_strategy`, or the stored review results that carried their own review boundary, one per slice (a second blind launch of one head is a second file, not a second slice). `roadmap-maker/assets/roadmap-template.md` has a tracking row for it, so an unwritten figure leaves that row reading a field nobody fills.
  - The planned side of `realized_slices` is the slice count the chain's shaper/estimator record states; there is no structured field for either side in the actuals record. When the planned count is not stated there, write `planned slice count not recorded`. When the realized count cannot be established, write `realized slice count not recorded`. Never reuse the planned number as though it were the outcome: that is how an overrun disappears into a plan that looks like it held.

### Validate

1. Read and reconcile the existing actuals as evidence, preserving its topic identity and metadata. From currently valid sources, construct ONE FINAL closed-schema object: remove or omit now-unsourced optional values, including old elapsed time when a landing or approved anchor is rejected. Before any write, confirm the parent-resolved capability can persist and read back that exact object as a full replacement, including removal or absence; if unavailable, report it and STOP before any write. Do not invent a deletion API or adapter. Compute the actuals object **once**: `implementation_hours`, `review_gate_hours`, `total_wall_clock_hours` when resolved, `post_review_fix_hours`, `approval_decision`, `scope_drift_notes`, `variance_vs_plan`, plus the optional complexity-unit fields `requirement_count`, `changed_lines`, `review_lens_count` and `checkpoint_count` when their source artifact is readable. Those four are recorded for reference only (`sdd-time-estimation` no longer derives a per-unit calibration rate from them): omit any whose source artifact could not be read rather than guessing.
2. Validate it with the **deterministic validator**, never by reading it:

   ```bash
   # -C changes the process working directory, so every path is absolute. Each
   # tool under tools/ is its own Go module; there is no repository-root module.
   root="$(git rev-parse --show-toplevel)"
   go run -C "$root/tools/actuals-record-validator" . \
     --schema "$root/skills/_shared/actuals-record.schema.json" \
     --instance /absolute/path/to/actuals-candidate.json \
     --repo "$root"
   ```

3. Exit `0` is the only result that authorises the write. Any non-zero exit is a hard stop, and so is a missing or unbuildable validator, which is not a pass: report it and write nothing. Reading the JSON, checking that the fields look present, or attesting that you were careful is NOT validation; the record's author cannot be its validator.
4. Beyond schema shape the validator enforces: `approval_decision` inside the closed outcome vocabulary; non-negative hours and counts; the durable-vs-reconstructed disclosure a recorded `checkpoint_count` owes; the measured-vs-reconstructed disclosure a recorded `total_wall_clock_hours` owes; that `review_lens_count` is the number of rows the stored review results of the change record (it counts `odd/review-results/<change_name>/` under `--repo` with the engine's own function and refuses a different count, a count when no stored result backs it, and an omitted count while a stored result records a row; a change reviewed before the rows were stored omits the field; results it cannot read are exit `7`, never a pass); and that `scope_drift_notes` and `variance_vs_plan` carry their own content instead of deferring it to some other section. That last rule exists because it was actually broken: "see the narrative below — this field is summarised there rather than duplicated" satisfies every required-field check JSON Schema can make while recording nothing. Cross-reference in ADDITION to the content, never INSTEAD of it.
5. `total_wall_clock_hours` is validly absent when its anchors do not resolve (R-021), and `implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` are validly absent while no durable source exists for them (R-019) — every other resolved field is still required. If validation fails for any other reason, report and STOP — do not partial-write.
6. The three compute-time fields stay unpopulated (R-019); state why, do not leave it silent. No durable source exists for any of them yet: orchestrator telemetry is session-transient, the `sdd-attempt` ledger has no timestamp fields, and transcripts carry no structured subagent durations. None of the three MAY be filled with `total_wall_clock_hours` or any other elapsed-time proxy while this remains true.

### Execute

Write items 1-3 in order through the parent-resolved storage capabilities. Read back each write; calibration and bookkeeping may merge or append under their existing rules. If one of them fails after another succeeded, report the inconsistency and STOP; never leave the stores silently out of sync.

1. `sdd/{change}/actuals` (the authoritative single actuals notebook): persist the exact validated object as a full replacement. Never merge, patch, or modify actuals after validation. A returned payload differing from the validated candidate is a partial-write failure: report it and STOP, without silently patching it.
2. `project/{project}/estimation-calibration` (append/update — variance patterns; no skill reads it since `sdd-time-estimation` was rewritten).
3. Append a NEW closure-feedback Log entry containing a fenced Markdown block with a `## Cycle Timestamps` heading and a 2-row table to the confirmed feature document — a named, delimited, append-only carve-out to closure-feedback's own "do NOT modify hop outputs" rule. Preserve all prior bytes, task states and the four top-level parts. Delivery's final Log entry stays delivery's close; closure's entry follows delivery's final Log entry by design, so a reader must not take the last Log entry as delivery's. The table records t0 (topic key, record identity, typed `created_at`, primary-or-fallback source when available) and t1 (`landing_commit`, `approved_tree` when recorded, committer timestamp, and verified/self-asserted/rejected/absent outcome); disclose omitted anchors.
4. After items 1-3 succeed, fire the archive-time sync trigger from the project root:

   ```bash
   labdrian longterm-mem sync-trigger --event archive --cwd "$root" || true
   ```

   It is best-effort and informational: it runs only after items 1-3 succeed, and a failed, skipped or missing sync never blocks or reverses closure and is not proof of memorycard sync. The command already exits 0 whatever the sync does (the `sync-trigger` branch of `cmd_longterm_mem` in `bin/lib/longterm_mem.sh`); the trailing `|| true` is a redundant safety net, kept so closure stays non-blocking even if that guarantee regresses.

Downstream consumers read on their next run — you do NOT push to them:
- **roadmap-maker** reads `sdd/{change}/actuals` on its next render.
- **sdd-time-estimation** no longer reads `project/{project}/estimation-calibration`; its reference class comes from the `odd/tasks/*.md` and archived `tasks.md` ledgers.
