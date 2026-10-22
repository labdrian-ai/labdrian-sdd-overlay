---
name: sdd-time-estimation
description: "Size a work package before committing: prototyper time-box, shaper per-stage ranges, builder slice sizing. Ranges with a stated basis, never a point."
license: Apache-2.0
metadata:
  author: labdrian
  version: "2.0"
---

## Activation Contract

Load before a prototyper spike, a shaper handoff (`estimates[]`), a builder unit, or an inception entry estimate. One body; read your row in Decision Gates.

## Hard Rules

- Three units, independently measured, never blended (R-001/R-002): agent-compute-time, elapsed-calendar-time and human-confirmation-checkpoint-count. The `low_minutes`/`high_minutes` you write are agent-compute-time only, never a calendar ETA or a checkpoint count; a report showing two or more units labels each separately, never summed or averaged into one figure.
- Ranges only. Every range carries its basis (`lines-ref n=<k>` or `no-actuals n=0`) and a confidence (low|medium|high). `n=0` never blocks: estimate and flag it low.
- Size in authored changed lines first (additions + deletions, tests included, generated excluded). Minutes derive from lines last; minutes have no actuals (`n=0`): keep them wide and say so.
- Reference class, this repository: features landed 0.7-4.2x their line forecast, five of seven above 2x; units under 200 lines landed 0.85-1.22x their high bound, over only when test- or fixture-heavy. Start from the class, then adjust for pieces; never from how fast the work feels.
- A package whose high bound exceeds one slice (~400 lines) or carries more than two independently reasoned pieces is split, not estimated.
- A miss above the high bound widens the next range; never anchor low. Ledgers count bookkeeping files: add headroom.

## Decision Gates

| Role | When | Output |
|---|---|---|
| prototyper | before a spike | a box per unknown (lookups or spikes); at the box stop and report `resolved` or `open` plus what would resolve it; no estimate |
| shaper | per handoff stage | pieces, lines range, `low_minutes`/`high_minutes`; an open unknown becomes a `risks[]` item with its mitigation and widens that stage's high |
| builder | per unit, before writing | lines range and pieces; <= ~100 lines: inline; > ~400 or > 2 pieces: slice; re-forecast at each slice boundary against `git diff --stat` |
| inception | entry contract | shaper minutes/60 as `planned_range_hours`, `no-actuals n=0`, low; `complexity` by pieces: 1 low, 2 medium, split high |

## Execution Steps

1. List the pieces (behaviors, files, tests, fixtures, docs). 2. Place the package in the reference class; give a lines range. 3. Split what exceeds a slice. 4. Shaper: minutes, wide, `n=0`. 5. Write basis and confidence beside every range.

## Output Contract

Per package: `pieces`, `lines low-high`, `basis`, `confidence`. Shaper adds `low_minutes`/`high_minutes` per stage; prototyper adds `box` and `unknowns[]` as `resolved|open`.

## References

- `odd/tasks/*.md` forecasts and `openspec/changes/archive/*/tasks.md`: the reference class; update the figures when a ledger closes.
