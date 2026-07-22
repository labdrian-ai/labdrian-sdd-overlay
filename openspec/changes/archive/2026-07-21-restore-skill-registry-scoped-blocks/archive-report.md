# Archive Report: Restore Skill Registry Scoped Blocks

## Closure Status

Archived on 2026-07-21 after native dispatcher status reported `reviewGate.result: allow`, `nextRecommended: archive`, no blocked reasons, and 11/11 completed tasks. Verification passed with warnings and reported no critical findings.

## Specification Sync

| Domain | Action | Details |
|---|---|---|
| `skill-registry-propagate` | Updated | Merged 4 added requirements and 5 scenarios; no modified, removed, or renamed requirements. |

The delta was merged into `openspec/specs/skill-registry-propagate/spec.md` before this change folder was moved.

## Review Gate Traceability

- At archive time, native status was `reviewGate.result: allow`; the approved receipt exactly matched authoritative native state and the archived repair candidate.
- Approved recovered lineage: `review-bc87e2c7d46b98f7`, generation 2.
- Base tree: `198395cd1f5c0caf9881001792db59546bb513b2`.
- Final candidate tree: `1ae373337a0ea6065d4236e6a8e3c421e9a305cf`.
- Paths digest: `sha256:c6dae49bf08fa3283581dec1d417db16f8b6c1ee59a501fea4278301bf0289a9`.
- Policy hash: `sha256:34fb63d7f29f8613cd4431382b1057398a4816f8a4c20fc34677fffc80a184f6`.
- Fix delta hash: `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.
- Evidence hash: `sha256:875dd8293119e3b864b27e468d6a5646a9a1709c0d927e1b768a08876e0b6d07`.
- Receipt mirror: `reviews/receipt.json`, byte-for-byte equivalent to the canonical approved receipt.

## Engram Traceability

| Artifact | Topic | Observation ID |
|---|---|---:|
| Proposal | `sdd/restore-skill-registry-scoped-blocks/proposal` | 4563 |
| Specification | `sdd/restore-skill-registry-scoped-blocks/spec` | 4566 |
| Design | `sdd/restore-skill-registry-scoped-blocks/design` | 4569 |
| Tasks | `sdd/restore-skill-registry-scoped-blocks/tasks` | 4581 |
| Apply progress | `sdd/restore-skill-registry-scoped-blocks/apply-progress` | 4585 |
| Verification report | `sdd/restore-skill-registry-scoped-blocks/verify-report` | 4616 |
| Receipt-mirror reconciliation | `review/restore-registry-archive-authority` | 4617 |
| Receipt-mirror session record | `mirror-reconcile-review-bc87e2c7d46b98f7-20260721` | 4620 |

No exact Engram observations exist for the requested review transaction, frozen ledger, or gate-context topics. Canonical native records were read directly from `.git/gentle-ai/review-transactions/v2/review-bc87e2c7d46b98f7/` and the approved receipt mirror was verified directly.

## Intentional Warnings and Follow-ups

1. **Historical lock cleanup is non-reusable**: the archived run restored an absent lock baseline, but that evidence is historical and superseded. Future operations follow the authoritative post-review correction in `tasks.md` and retain the persistent lock path and inode after flock release.
2. **Historical rollback is non-reusable**: the archived run restored its captured baseline without checking for foreign writes. Future operations follow the authoritative corrected guidance in `proposal.md`: compare against the transaction's last written snapshot before restore and preserve current bytes on mismatch.
3. **Historical roadmap status**: at archive time, `openspec/project/roadmap.md` was an unstaged external worktree modification and still recorded the change as planned. This separate PR2 closure completes that roadmap entry without changing the archived repair candidate.

## Archive Verification

- Main specification updated before archival: confirmed.
- Change folder moved to `openspec/changes/archive/2026-07-21-restore-skill-registry-scoped-blocks/`: confirmed.
- Archived artifacts include proposal, specifications, corrected design, tasks, apply progress, verification report, and receipt mirror: confirmed.
- Archived tasks contain 11/11 completed implementation tasks and no unchecked task: confirmed.
- Active changes directory no longer contains this change: confirmed.
