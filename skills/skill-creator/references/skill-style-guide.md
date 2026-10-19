# LLM-first Skill Style Guide

Use this guide when creating or refactoring skills in this repo. A skill is a **runtime instruction contract for an LLM**, not human-facing documentation: it tells the model when to activate, what rules are non-negotiable, how to decide, what to do, and what to return.

## Required Structure

Every `SKILL.md` MUST use this order unless a section is truly irrelevant:

1. **Frontmatter** — complete metadata for skill discovery.
2. **Activation Contract** — exact situations that load the skill.
3. **Hard Rules** — constraints the LLM MUST NOT violate.
4. **Decision Gates** — short tables or bullets for branching choices.
5. **Execution Steps** — ordered operational workflow.
6. **Output Contract** — required final format or artifacts.
7. **References** — local files only; supporting detail lives outside the skill.

`## Compact Rules` is not required. The skill registry indexes skill names, triggers, scopes, and paths; agents load the full `SKILL.md` as the source of truth.

## Frontmatter Rules

- `description` MUST be one physical line, YAML-safe, and quoted.
- Put trigger words first: `"Trigger: ... . {What the skill does}."`
- `description` length is machine-checked; see the generated rule table below for the exact bounds.
- Include complete `name`, `description`, `license`, `metadata.author`, and `metadata.version`.
- Do NOT add a `Keywords` section; discovery uses frontmatter.

## Body Budget

- Target **180–450 tokens** for the skill body (human-judgement guidance, not machine-checked).
- Recommended and hard maximums are machine-checked; see the generated rule table below for the exact bounds. Move examples, schemas, and background into `assets/` or `references/` to stay under them.

## Machine-Checked Lint Rules

Run `labdrian skills lint <path>` to check a skill against every rule below, or `labdrian skills lint --rules` to print this table yourself.

<!-- BEGIN GENERATED: skill-lint-rules (source: engine/skills/lint.go; print with `labdrian skills lint --rules`) -->
| ID | Severity | Check |
|---|---|---|
| `frontmatter-fence` | hard | The file starts with a `---` line and has a closing `---` line. |
| `required-fields` | hard | `name`, `description`, `license`, `metadata.author` and `metadata.version` are present and non-empty after unquoting. |
| `description-one-line` | hard | `description` is a single physical line: no block scalar (`>`, `\|`, `>-`, `\|-`, `>+`, `\|+`) and no indented continuation line. |
| `description-max` | hard | `description` is at most 250 characters (runes, after unquoting). |
| `body-hard-budget` | hard | Estimated body tokens are at most 1000. |
| `description-should` | advisory | `description` is at most 160 characters. |
| `body-recommended` | advisory | Estimated body tokens are at most 700. |
| `section-order` | advisory | The canonical H2 headings that are present (Activation Contract, Hard Rules, Decision Gates, Execution Steps, Output Contract, References) appear in canonical order. |
| `incident-log-shape` | advisory | The body contains an ISO date, an observation reference, or a PR/issue number. |
| `banned-shell-utility` | advisory | An inline code span, or a line inside a fenced code block, whose first word is `cat`, `grep`, `find`, `sed` or `ls`. |
| `home-path-leak` | advisory | An absolute home-directory path (`/home/<name>/`, `/Users/<name>/` or `C:\Users\<name>\`) appears anywhere in the file. |
<!-- END GENERATED: skill-lint-rules -->

## Writing Rules

### DO

- Write imperative runtime instructions: “Load X”, “Check Y”, “Return Z”.
- Lead with the activation trigger and hard constraints.
- Use compact tables for decision gates.
- Keep examples minimal and executable.
- Link to local supporting files for details.

### DON'T

- Explain history, motivation, or tutorial background.
- Duplicate long docs inside the skill.
- Add generic advice the LLM cannot execute.
- Use external URLs as primary references.
- Hide critical rules below examples.

## Supporting Files

- Use `assets/` for templates, schemas, fixtures, or generated examples.
- Use `references/` for local docs that explain concepts or edge cases.
- Keep references stable and relative to the skill directory when possible.

## Registry Behavior

- `gentle-ai skill-registry refresh` indexes skills; it does not summarize or rewrite them.
- The registry records `name`, `description` trigger text, scope, and exact `SKILL.md` path.
- Delegators pass matching paths to subagents, and subagents read the full skill before work.
- Use `skill-improver` to audit and refactor existing skills against this guide.

## Quality Gates

- Frontmatter is complete, quoted, single-line, and trigger-preserving.
- Required sections exist in the expected order.
- Hard rules are testable or observable.
- Decision gates cover meaningful forks only.
- Output contract tells the LLM exactly what to return.
- References point to local files.

## Refactor Checklist

- [ ] Move explanatory prose to local references.
- [ ] Collapse repeated rules into one hard rule.
- [ ] Replace prose branches with a decision table.
- [ ] Trim examples to the smallest useful case.
- [ ] Recheck description length and trigger words.
