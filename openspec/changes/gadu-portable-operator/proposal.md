# Proposal: gadu-portable-operator

## Why

GADU — the high-judgment operator persona — does not travel. It is an ORPHAN. Today it
exists ONLY as a hand-made `~/.claude/agents/GADU.md` (mtime 2026-06-23) on one machine. It
is NOT embedded in the `gentle-ai` binary (`rg 'GADU'` on the binary returns zero matches)
and it is NOT tracked by this overlay. Nothing reproduces it; nothing protects it.

That orphan status has two consequences:

1. **No durability.** `gentle-ai` embeds a FIXED agent catalog (`sdd-*`, `jd-*`, `review-*`)
   and writes it to `~/.claude/agents/`. A `gentle-ai sync` / `gentle-ai upgrade` rewrites
   that directory from the binary's catalog. Because GADU is not in the binary and not
   overlay-managed, an upgrade can clobber the hand-made file with no recovery path. We
   CANNOT fix this upstream — `gentle-ai` is a stripped Go binary we cannot fork or rebuild.

2. **No portability.** `gentle-ai` does NOT translate agents per runtime (confirmed via
   `~/.codex/AGENTS.md`). Its cross-runtime pattern is: the persona travels as a SKILL, and
   the runtime's NATIVE subagent spawning (Agent tool in Claude Code, task dispatch in
   opencode, `spawn_agent` in Codex) supplies the isolated context, model tier, and
   parallelism. GADU has no skill form, so it cannot be invoked as an operator in
   opencode or Codex at all.

The overlay already solves durable, multi-runtime distribution — but only for skills. Its
installer (`bin/labdrian-overlay`) hardcodes the `skills/` source prefix in five functions
(`cmd_apply` L386, `cmd_bootstrap` L186, `cmd_capture` L293/L308, `cmd_status` L438/L443,
`cmd_sync_check` L483/L510/L530) and routes every tracked file to the three skills
destinations in `TARGET_PATHS` (L17): `~/.claude/skills`, `~/.config/opencode/skills`,
`~/.codex/skills`. There is NO `agents/` source directory and NO `~/.claude/agents`
destination today.

This change makes GADU a first-class, reproducible, durable, multi-runtime operator by
giving the overlay a single canonical persona source, a generator that emits both delivery
shapes from it, and a new installer route for the Claude Code agent file.

## What Changes

This change adds a canonical GADU persona, a generator, the two generated artifacts, and the
installer surgery needed to distribute and protect them. It introduces NO `gentle-ai` binary
change (impossible — stripped upstream) and NO SDD-orchestrator wiring.

1. **Add** a single canonical GADU persona source (ONE source of truth). Its body is ported
   verbatim in substance from the existing `~/.claude/agents/GADU.md`: the six non-negotiable
   defining traits (Judgment, Red-team, No sycophancy, Highest-probability path, Autonomy /
   agent orchestration, Source-grounded), the Voice section, Signature capabilities
   (adversarial review + parallel fan-out, loaded as portable skills), the Safety baseline,
   and the Memory section.

2. **Add** a generator step that reads the canonical source and emits BOTH delivery shapes:
   - `agents/GADU.md` — the Claude Code native agent file (frontmatter `name`, `description`,
     `model: opus`, `tools`, persona body).
   - `skills/gadu-operator/SKILL.md` — the persona skill loaded into a native subagent on
     demand in opencode and Codex.
   The generator is part of THIS change and ships with its own test. DRY by construction:
   both outputs derive from one source, so there is zero hand-maintained drift.

3. **Add** a new `agents/` distribution route to `bin/labdrian-overlay`: a new `agents/`
   source directory and a new `~/.claude/agents` destination, with routing added to
   `cmd_apply`, `cmd_capture`, `cmd_status`, and `cmd_sync_check`, and manifest support that
   distinguishes agent-routed files from skill-routed files. The `skills/gadu-operator/`
   tree rides the EXISTING skills route and lands in all three skills directories unchanged.

4. **Add** a sync-durability guard so `agents/GADU.md` and `skills/gadu-operator/SKILL.md`
   both survive `gentle-ai sync` / `gentle-ai upgrade` without being clobbered, registered
   in `overlay.manifest` as overlay-managed entries.

### Capabilities

#### New

- **`gadu-canonical-source`** — the single source-of-truth GADU persona spec. Owns the
  persona body (six traits, Voice, Signature capabilities, Safety baseline, Memory). The
  only place GADU's persona content is authored.

- **`gadu-generator`** — a generator that reads `gadu-canonical-source` and emits both
  `agents/GADU.md` (Claude Code agent) and `skills/gadu-operator/SKILL.md` (persona skill),
  guaranteeing the two delivery shapes never drift. Ships with its own test.

- **`overlay-agent-route`** — a new agent-file distribution path in `bin/labdrian-overlay`:
  `agents/` source directory, `~/.claude/agents` destination, routing in apply/capture/
  status/sync-check, and manifest support for routing agents vs skills.

#### Modified

- **`bin/labdrian-overlay`** — gains the `agents/` route alongside the existing hardcoded
  `skills/` route across `cmd_apply`, `cmd_capture`, `cmd_status`, `cmd_sync_check` (and the
  manifest/path helpers they depend on). The skills route behavior is preserved unchanged.
- **`overlay.manifest`** — gains the GADU agent and skill entries and the routing
  distinction (agent-routed vs skill-routed) needed by the new path; existing two-column
  `path  managed|custom` semantics are preserved.

### Requirements (EARS)

- **R-001**: There SHALL be exactly ONE canonical GADU persona source; the persona body
  SHALL NOT be authored in `agents/GADU.md` or `skills/gadu-operator/SKILL.md` directly.
- **R-002**: The generator SHALL read the canonical source and emit BOTH `agents/GADU.md`
  and `skills/gadu-operator/SKILL.md`, and the two outputs SHALL carry the same persona body
  (no hand-maintained divergence).
- **R-003**: The generator SHALL ship with an automated test that fails if either generated
  artifact is missing, stale, or diverges from the canonical source.
- **R-004**: The emitted `agents/GADU.md` SHALL be a valid Claude Code agent file with
  `name`, `description`, `model: opus`, and `tools` frontmatter, invocable as `@GADU`.
- **R-005**: The emitted `skills/gadu-operator/SKILL.md` SHALL be a valid persona skill that
  can be loaded into a native subagent on demand in opencode and Codex.
- **R-006**: `bin/labdrian-overlay` SHALL route `agents/`-prefixed tracked files to the
  `~/.claude/agents` destination while continuing to route `skills/`-prefixed files to the
  three skills destinations.
- **R-007**: The `agents/` route SHALL be honored by `cmd_apply`, `cmd_capture`,
  `cmd_status`, and `cmd_sync_check`, and `overlay.manifest` SHALL record whether each entry
  is agent-routed or skill-routed.
- **R-008**: `agents/GADU.md` and `skills/gadu-operator/SKILL.md` SHALL be registered as
  overlay-managed and SHALL survive a `gentle-ai sync` / `gentle-ai upgrade` without being
  clobbered (re-deployable by the overlay after upgrade).
- **R-009**: This change SHALL NOT wire GADU into the SDD orchestrator's auto-spawn flow and
  SHALL NOT emit native opencode/Codex agent-definition files.

## Impact

- **New files**: the canonical GADU persona source; the generator (plus its test);
  generated `agents/GADU.md`; generated `skills/gadu-operator/SKILL.md`.
- **Modified files**: `bin/labdrian-overlay` (new `agents/` route across four functions and
  their path/manifest helpers); `overlay.manifest` (GADU entries + agent-vs-skill routing).
- **New distribution destination**: `~/.claude/agents` (first non-skills destination the
  overlay writes to).
- **Affected actors**: the maintainer who installs GADU as `@GADU` in Claude Code and loads
  `gadu-operator` as a subagent in opencode/Codex; the overlay installer (new route);
  `gentle-ai sync`/`upgrade` (now non-destructive to GADU). No `gentle-ai` binary change, no
  SDD engine phase change, no orchestrator runtime change.

## Scope

### In scope (first slice)

- ONE canonical GADU persona source, body ported from `~/.claude/agents/GADU.md` (six traits,
  Voice, Signature capabilities, Safety baseline, Memory).
- A generator that emits both `agents/GADU.md` and `skills/gadu-operator/SKILL.md`, with its
  own test.
- The `skills/gadu-operator/` skill riding the existing overlay skills route into all three
  skills directories.
- A NEW `agents/` installer route in `bin/labdrian-overlay` (source dir, `~/.claude/agents`
  destination, routing in apply/capture/status/sync-check, manifest agent-vs-skill support).
- A sync-durability guard so both artifacts survive `gentle-ai sync` / `upgrade`.
- Target outcome = "available to invoke": `@GADU` native agent in Claude Code;
  `gadu-operator` skill loaded into a native subagent on demand in opencode/Codex.

### Out of scope (explicit non-goals, v1)

- NO native opencode/Codex agent-DEFINITION files (`.config/opencode/agents/*.md`,
  `~/.codex/agents/*.toml`). The skill + native subagent covers those runtimes.
- NO SDD-orchestrator auto-spawn integration — GADU is invoke-on-demand, not auto-wired.
- NO change to the `gentle-ai` binary or its embedded agent catalog (impossible upstream).
- NO change to the existing `skills/` route behavior for non-GADU files.

## Acceptance Criteria

- **AC-1**: Editing the canonical source and running the generator updates BOTH
  `agents/GADU.md` and `skills/gadu-operator/SKILL.md`; neither holds independently authored
  persona content.
- **AC-2**: The generator test fails if either artifact is missing, stale, or divergent from
  the canonical source.
- **AC-3**: After `bin/labdrian-overlay apply`, `agents/GADU.md` lands in `~/.claude/agents`
  and `skills/gadu-operator/SKILL.md` lands in all three skills directories.
- **AC-4**: `cmd_status` and `cmd_sync_check` report the GADU agent file and skill correctly
  under the new agent-vs-skill routing.
- **AC-5**: A simulated `gentle-ai sync`/`upgrade` followed by overlay re-apply leaves both
  GADU artifacts present and overlay-managed (not clobbered).
- **AC-6**: No native opencode/Codex agent-definition file and no SDD auto-spawn wiring is
  produced by this change.

## Risks and Open Questions

- **Installer surgery touches multiple functions.** The `agents/` route must be threaded
  through `cmd_apply`, `cmd_capture`, `cmd_status`, and `cmd_sync_check` plus the manifest
  helpers, where the `skills/` prefix is currently hardcoded. Risk of regressing the existing
  skills route. Mitigation: introduce a single route-resolution helper rather than copy-paste
  branching, and cover the skills route with its existing behavior in tests.
- **Drift if the generator is skipped.** If someone edits a generated artifact by hand
  instead of the canonical source, the DRY guarantee breaks. Mitigation: the generator test
  (R-003) fails on divergence; generated files should carry a "do-not-edit, generated from
  canonical source" header.
- **Sync clobber is the core threat.** If the durability guard is incomplete, a
  `gentle-ai upgrade` can still wipe `~/.claude/agents/GADU.md`. Mitigation: overlay-managed
  manifest entries + re-deployable apply; AC-5 explicitly tests the upgrade-then-reapply path.
- **Review budget likely exceeds 400 changed lines.** Canonical source + generator + test +
  two generated artifacts + four-function installer surgery + manifest changes will probably
  exceed the 400-line budget, triggering the chained/stacked-PR decision at tasks time.
- **Generator test harness choice (open question for spec/design).** The repo's canonical
  test command is Go-based (`cd engine && go test ./... && cd tui && go test ./...`) while
  the installer is bash. The spec/design phase must decide whether the generator and its test
  live in Go (runs under the existing command) or bash (needs a separate harness), so strict
  TDD has a real runner.
- **Manifest schema extension.** Adding an agent-vs-skill routing distinction to the
  two-column `path  managed|custom` manifest must stay backward-compatible with existing
  skill entries. Open question: extend the row format vs. infer route from the `agents/` vs
  `skills/` path prefix. To resolve in design.

## Assumptions

- **A-1**: The persona body in `~/.claude/agents/GADU.md` is the intended canonical content
  to port; this change does not redesign the persona, only makes it reproducible and portable.
- **A-2**: Claude Code continues to read agent files from `~/.claude/agents/*.md` with the
  `name`/`description`/`model`/`tools` frontmatter shape the existing file uses.
- **A-3**: opencode and Codex can load `skills/gadu-operator/SKILL.md` into a native subagent
  on demand, consistent with `gentle-ai`'s confirmed persona-as-skill cross-runtime pattern.
- **A-4**: The route inferred from path prefix (`agents/` → `~/.claude/agents`, `skills/` →
  three skills dirs) is sufficient routing granularity for this slice; per-runtime agent
  formats are explicitly out of scope.
