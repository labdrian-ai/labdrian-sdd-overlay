---
name: project-inception
description: "Project inception trilogy: project inception, bootstrap completo, manifest architect roadmap, quiero hacer un proyecto inception completo. Use inception-pipeline first for tier routing."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use for the trilogy via inception-pipeline or directly. Receive project, parent-resolved store mode, capabilities, locators, `inception_mode` and optional `requirements_brief_ref`. Read `../_shared/pre-sdd-contracts.md` for identities and language.

## Hard Rules

Sequence producers; do not author their documents, write code or execute the ODD role chain. Write new artifacts in English; preserve user quotes. Do not ask the parent to choose storage again. Preserve supported project-file placement and the existing `engram | openspec | hybrid | none` placement values: historical `engram` means memory-only placement, never a new Engram write or export.

For current memory operations use the supplied memorycard capabilities: project-scoped search then get the full record, merge updates and readback. Keep identities `project/{project}/manifest/{context,mission,rules}`, `project/{project}/architect/final`, `project/{project}/roadmap`. Pass placement and locators to producers; this text grants no execution permission.

## Decision Gates

`full` is the default. In `reuse`, read both manifest and architecture read-only and check both exist and are non-empty. If either is missing, report fallback to `full` before sequencing. Otherwise never rewrite either foundation and use roadmap `incremental-insert`. Require the manifest is final enough before architecture and the architecture is final enough before roadmap; return the concrete blocker for missing or incomplete inputs; ask the parent at most one question at a time. Do not expand the existing inline-context exception.

## Execution Steps

1. If `requirements_brief_ref` is supplied, load the full `project/{project}/requirements/{change}` record and seed from its EARS requirements, scope and handoff; request citation in manifest goals and the roadmap item's `Derivado de` field. Inherit the upstream change slug unchanged.
2. In `full`, invoke `../project-manifest/SKILL.md`, then `../project-architect/SKILL.md` with the manifest and resolved inputs. Carry both foundation results and exact locators forward; stop on a blocker.
3. Invoke `../roadmap-maker/SKILL.md` with project, resolved artifact store mode and foundation locators: `build` for `full`, `incremental-insert` for successful `reuse`. Keep its input gates; incremental insert preserves roadmap history and actuals.
4. Return control to the parent. For the next implementation entry, the parent follows `../inception-pipeline/SKILL.md` Chain entry and `../role-chain/SKILL.md`; this coordinator runs neither.

## Output Contract

Return `completed`, `blocked` or `partial`, the requested and effective mode and any fallback, exact produced and used artifact locators, producer outcomes, concrete blocker or missing input, and the next parent handoff. Do not claim inspection executed a producer.
