---
name: security-audit
description: "Defensive security guidance and source-first vulnerability review. Trigger: security questions, focused reviews, vulnerability research, audits or pen tests."
license: MIT
metadata:
  author: cloudflare
  version: "c1c8a8c1471069fb0e188eeaff69b8e8db6564a8"
---

# Security Audit

Vendored from https://github.com/cloudflare/security-audit-skill at commit
`c1c8a8c1471069fb0e188eeaff69b8e8db6564a8` (MIT, see `LICENSE`). The only adaptation: the upstream
`SKILL.md` body moved, byte for byte, to `WORKFLOW.md`, so that this file fits the overlay's lint
budget. Where `WORKFLOW.md` says "this `SKILL.md`", it means this directory; where any other file of
this skill points to a rule "in `SKILL.md`" (the budget gate, the incomplete-run rule, the
promotion procedure), that rule is in `WORKFLOW.md`.

## Before anything else

Read [WORKFLOW.md](WORKFLOW.md) in full. It is the skill: operating modes, execution safety, full
audit setup and planning, core principles, the six-phase workflow and its anti-patterns. Load the
companion files it names only when the selected mode and target need them.

## Operating modes (summary; WORKFLOW.md governs)

- **Guidance mode** (default): for security questions, focused reviews, methodology or triage, use
  only the relevant parts. Do not run the six phases, create an output directory or write audit
  artifacts.
- **Full audit mode**: only when the user explicitly asks to audit or pen-test a codebase, asks for
  a full, comprehensive or end-to-end security review, or requests report artifacts.
- If the request could mean either, ask one focused question before creating files.

## Safety that always applies (summary; WORKFLOW.md governs)

- Source inspection is read-only. Run target-controlled code only inside an OS-enforced sandbox
  with no external network, an allowlisted environment, a read-only target and explicit resource
  limits; otherwise do not execute it and report the missing control.
- Never install dependencies, probe deployed endpoints or external services, use real
  credentials, publish artifacts, or continue past the minimum local effect that shows a defect.

## Files

| File | Use |
| --- | --- |
| `WORKFLOW.md` | The upstream skill body: read first |
| `RECONNAISSANCE.md`, `HUNTING.md`, `VALIDATION-AND-REPORTING.md` | Phases 1, 2, 3-6 |
| `ATTACK-CLASSES.md` and the domain companions (`WEB-PROTOCOL-AND-AUTH.md`, `AI-AND-LLM.md`, ...) | Selected per target |
| `report-schema.json`, `validate-findings.cjs`, `validate-coverage-ledger.cjs` | Full audit output and its validation (Node.js, no dependencies, read-only) |
