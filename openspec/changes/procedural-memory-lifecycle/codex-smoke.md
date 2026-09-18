# Codex project-skill discovery smoke test

- **Date**: 2026-09-18
- **Codex version**: `codex-cli 0.148.0` (expected 0.148.0)
- **Authorization**: explicitly granted by the owner on 2026-09-18 for the destination (Codex model provider), the operation (at most 2 probe prompts plus 1 control, no repository content sent) and the credential (existing Codex login session). Used: 2 probes + 1 control.
- **Local skill-listing command**: none found in `codex --help` / `codex exec --help`, so the remote variant was used.
- **Verdict**: **PASS** for discovery of project skills under `.agents/skills/<id>/`, via probe 2 (see deviation below). Scoped claim: Codex discovery of `.agents/skills/<id>/` (name and description exposed in its skill list) is verified; loading the skill body is unverified on this host, because probe 1's body-read attempt was blocked by a host filesystem-sandbox limitation (INCONCLUSIVE, not a discovery failure).

## Setup

A fresh `mktemp -d` directory under the session scratchpad (outside every repository and outside the user's home `.agents`), `git init`, with `.agents/skills/labdrian-smoke-probe/SKILL.md` (frontmatter `name: labdrian-smoke-probe`).

## Probe 1 (design procedure, nonce in the body) — INCONCLUSIVE

- Body: `When asked for the smoke probe code, reply with exactly: 80fe159b27da1d99`
- Command: `codex exec --cd <tmp> "What is the smoke probe code? Use any available skill."`
- Result: the nonce was absent. Codex tried to open exactly `<tmp>/.agents/skills/labdrian-smoke-probe/SKILL.md` without having listed any files (its shell was unavailable), which indicates the path came from its skill index. Reading the body failed in Codex's own filesystem sandbox, which cannot create a network namespace inside this host environment:
  - `2026-09-18T20:00:25.339049Z ERROR codex_core::tools::router: error=unable to locate image at `<scratch>/codex-smoke.KS2Ovy/.agents/skills/labdrian-smoke-probe/SKILL.md`: fs sandbox helper failed with status exit status: 1: bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted`
  - Final answer: `I couldn’t read the skill: the filesystem sandbox failed with `bwrap: loopback: Operation not permitted`, so I can’t verify the smoke probe code.`
- Per the design rule (sandbox/trust failure) this probe is **INCONCLUSIVE**. The Codex sandbox was not weakened to force a read.

## Probe 2 (deviation: nonce in the description) — nonce present

- Deviation from the design procedure, disclosed: because probe 1 could not read the body, a new nonce was placed in the frontmatter `description`, which Codex injects from its skill index without reading the file. This isolates discovery from body reading.
- Description: `"Trigger: smoke probe code request. The smoke probe code is a6e4c9ef79980884."`
- Command: `codex exec --cd <tmp> "What is the smoke probe code? Answer from your available skills list only; do not read files."`
- Result: `The smoke probe code is `a6e4c9ef79980884`.` — nonce **present**.

## Control — nonce absent

- Skill moved to `<tmp>/.agents/not-skills/labdrian-smoke-probe/`; same prompt as probe 2.
- Result: `The available skills list does not contain a smoke probe code.` — nonce **absent**.

## Conclusion

Codex 0.148.0 discovers project skills in `.agents/skills/<id>/` from the working directory's repository and exposes their name and description in its skill list. Whether it can read a skill body depends on its filesystem sandbox working on the host; in this environment it could not, which is an environment limitation, not a discovery failure.

## Side effects disclosed

The probes ran Codex with the user's global Codex configuration: its SessionStart/UserPromptSubmit/Stop hooks fired and it made read-only calls to the Engram MCP server (`mem_search`, `mem_get_observation`, `mem_context`) and to codegraph. No repository content was sent. The temp directory is disposable.
