# Skill Registry — labdrian-sdd-overlay

Generated: 2026-06-19
Source: overlay.manifest + skills/ directory scan

## Project Context

- **Product**: Markdown skill governance layer (the `skills/` directory and overlay toolchain)
- **Upcoming change**: `minimalism-contract-lite` — purely Markdown skill/contract authoring
- **Managed files**: vendor-tracked skills in `skills/sdd-spec`, `skills/sdd-tasks`, `skills/sdd-verify`
- **Custom skills**: project-specific additions with no vendor counterpart

---

## Skills Index

### Managed (vendor + overlay)

| Skill | Path | Trigger / Description |
|-------|------|-----------------------|
| sdd-spec | `skills/sdd-spec/SKILL.md` | SDD spec phase; writing specifications with requirements |
| sdd-tasks | `skills/sdd-tasks/SKILL.md` | SDD tasks phase; breaking specs into implementation tasks |
| sdd-verify | `skills/sdd-verify/SKILL.md` | SDD verify phase; validating implementation against specs |
| sdd-verify/report-format | `skills/sdd-verify/references/report-format.md` | Verify phase report format reference |
| sdd-verify/strict-tdd | `skills/sdd-verify/strict-tdd-verify.md` | Strict TDD verification extension |

### Custom (overlay only)

| Skill | Path | Trigger / Description |
|-------|------|-----------------------|
| requirements-from-transcripts | `skills/requirements-from-transcripts/SKILL.md` | Extract structured requirements from chat/interview transcripts |
| kadia-content-guard | `skills/kadia-content-guard/SKILL.md` | Kadia project — content governance rules |
| kadia-ui-fix | `skills/kadia-ui-fix/SKILL.md` | Kadia project — UI fix conventions |
| kadia-visual-qa | `skills/kadia-visual-qa/SKILL.md` | Kadia project — visual QA checklist |
| genesis-delivery-workflow | `skills/genesis-delivery-workflow/SKILL.md` | Genesis project — delivery workflow |
| genesis-design-system | `skills/genesis-design-system/SKILL.md` | Genesis project — design system conventions |
| chat-thread-analyzer | `skills/chat-thread-analyzer/SKILL.md` | Analyze chat threads for patterns/insights |
| project-inception | `skills/project-inception/SKILL.md` | Project inception phase |
| inception-pipeline | `skills/inception-pipeline/SKILL.md` | Full inception pipeline orchestration |
| project-manifest | `skills/project-manifest/SKILL.md` | Project manifest authoring |
| project-architect | `skills/project-architect/SKILL.md` | Architecture document authoring |
| roadmap-maker | `skills/roadmap-maker/SKILL.md` | Roadmap creation |
| sdd-time-estimation | `skills/sdd-time-estimation/SKILL.md` | Time estimation for SDD tasks |

### Shared Contracts

| Artifact | Path | Description |
|----------|------|-------------|
| pre-sdd-contracts | `skills/_shared/pre-sdd-contracts.md` | Contracts that precede SDD phases |
| entry-contract.schema | `skills/_shared/entry-contract.schema.json` | JSON schema for entry contracts |
| actuals-record.schema | `skills/_shared/actuals-record.schema.json` | JSON schema for actuals records |
<!-- BEGIN: minimalism-contract-scope (auto-generated) -->
| minimalism-contract | skills/_shared/minimalism-contract.md | Inject ONLY into sdd-tasks and sdd-apply sub-agent prompts under '## Skills to load before work'. Do NOT inject into sdd-propose/sdd-spec/sdd-design/sdd-verify/sdd-archive. |
<!-- END: minimalism-contract-scope -->

---

## Scope Notes

- **Markdown-only changes** (e.g. `minimalism-contract-lite`): no skill matching by file extension needed. Match by task context only (authoring, refactoring Markdown contracts).
- **Go TUI changes** (`tui/*.go`): match `genesis-delivery-workflow` if relevant; no dedicated Go skill currently registered.
- **Bash CLI changes** (`bin/overlay`): no dedicated skill; use standard SDD phases.

---

## Foundation Artifacts (Engram)

| Artifact | Topic Key | Status |
|----------|-----------|--------|
| project-manifest | `sdd/labdrian-sdd-overlay/project-manifest` | NOT FOUND (not yet created) |
| project-architect | `sdd/labdrian-sdd-overlay/project-architect` | NOT FOUND (not yet created) |
| sdd-init | `sdd-init/labdrian-sdd-overlay` | Written this session |
