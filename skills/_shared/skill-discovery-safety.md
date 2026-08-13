---
applies_to_phases: [sdd-explore, sdd-tasks, sdd-apply, sdd-verify]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-archive]
injection_point: "## Skills to load before work"
---
# Skill Discovery Safety Contract

> Advisory scope: this contract is injected into the phases that resolve skills or discover
> files (`sdd-explore`, `sdd-tasks`, `sdd-apply`, `sdd-verify`). `engine/gate/gate.go` matches
> `subagent_type` against the frontmatter above to decide whether to inject, and `propagate`
> fails when `applies_to_phases` is absent or empty — the frontmatter is load-bearing, not
> documentation. `.atl/skill-registry.md` is generated from it, so it is the output of this
> binding rather than its source.

These three rules are NON-NEGOTIABLE. They exist because a swallowed `command not found`
error from a missing discovery tool (`fd`/`eza`) was once misread as "0 skills" and escalated
into the false conclusion that "skills are virtual, not files." That conclusion was WRONG. The
registry is correct and `SKILL.md` files do exist. Follow these rules to never repeat it.

## 1. Registry-authoritative

Skill and file presence is determined by the **registry** — Engram (`mem_search` →
`mem_get_observation` on `skill-registry`) or `.atl/skill-registry.md` — NOT by a filesystem
scan.

- An empty or failed filesystem scan is **INCONCLUSIVE**, never proof of "zero", "absent", or
  "virtual".
- NEVER conclude that skills are missing, virtual, or not files because a scan returned
  nothing. If a scan returns nothing, re-run it with portable tools and report the tool and
  the exact error FIRST.
- The registry is the source of truth for what exists. If the registry lists a skill, that
  skill exists; resolve it by its registered path.

## 2. Fail-loud

A discovery command must never hide its own failure.

- NEVER append a bare `2>/dev/null` to a discovery command. You must SEE `command not found`
  and other errors. A swallowed error on a discovery command is a defect.
- NEVER append `|| true` to a discovery command to mask a genuine failure. `|| true` is
  acceptable only to tolerate a *known, expected* absent-tool case AND only when the
  absence is reported, not silenced.
- A missing required tool or an empty/unreadable registry MUST produce a visible, actionable
  message — never a silent no-op that downstream logic reads as "zero".

## 3. Portable discovery

Never assume `fd`, `eza`, `bat`, or `sd` exist on the host.

- Guard every discovery command with `command -v <tool>` before calling it.
- Fall back to portable POSIX tools (`find`, `ls`, `test -f`, `grep`, `cat`) when the
  preferred tool is absent. The standard tools are valid fallbacks, not banned.
- Example pattern:
  `command -v fd >/dev/null 2>&1 && fd <args> || find <equivalent-args>` — and even here,
  do not silence the failure path of the fallback.
