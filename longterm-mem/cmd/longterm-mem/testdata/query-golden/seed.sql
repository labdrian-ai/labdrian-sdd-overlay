-- Seed for TestQueryGolden: one project (goldenproj) with every shape the engram reader has to carry
-- (superseded, conflicting, undecided, withdrawn and dangling relations; a long multibyte body; a
-- soft-deleted row, a row of another project, a legacy row without sync_id).
WITH RECURSIVE c(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM c WHERE n<45)
INSERT INTO observations (id, sync_id, session_id, type, title, content, project, topic_key, revision_count, pinned, created_at, updated_at, deleted_at)
SELECT 1, 'sy-1', 's', 'decision', 'Canonical identity of the register writer',
  'The canonical identity rule: the register writer sorts its keys.' || char(10) || '**Where**: internal/old/gone.go, moved.go' || char(10) || 'isolation marker here.',
  'goldenproj', 'architecture/auth-model', 3, 1, '2026-08-01 00:00:00', '2026-08-02 00:00:00', NULL
UNION ALL SELECT 2, 'sy-2', 's', 'bugfix', 'Fixed register writer key order',
  'Fixed the register writer sorting its keys; what conventions apply when editing the register writer.',
  'goldenproj', 'bugfix/register-order', 2, 0, '2026-08-03 00:00:00', '2026-08-03 00:00:00', NULL
UNION ALL SELECT 3, 'sy-3', 's', 'architecture', 'Dragonscale layout',
  (SELECT group_concat('reglas del pingüino, ñandú café; ', '') FROM c) || ' dragonscale needle in the middle ' || (SELECT group_concat('más texto náutico ', '') FROM c),
  'goldenproj', 'architecture/dragon', 1, 0, '2026-08-04 00:00:00', '2026-08-04 00:00:00', NULL
UNION ALL SELECT 4, 'sy-4', 's', 'discovery', 'Deleted dragonscale note',
  'dragonscale deleted body', 'goldenproj', NULL, 1, 0, '2026-08-05 00:00:00', '2026-08-05 00:00:00', '2026-08-06 00:00:00'
UNION ALL SELECT 5, 'sy-5', 's', 'decision', 'Other project dragonscale',
  'dragonscale from another project', 'otherproj', 'architecture/x', 1, 0, '2026-08-05 00:00:00', '2026-08-05 00:00:00', NULL
UNION ALL SELECT 6, 'sy-6', 's', 'session_summary', 'Session summary dragonscale',
  'dragonscale summary of a session', 'goldenproj', NULL, 1, 0, '2026-08-07 00:00:00', '2026-08-07 00:00:00', NULL
UNION ALL SELECT 7, 'sy-7', 's', 'policy', 'Policy with multibyte head',
  (SELECT group_concat('ñandú ', '') FROM c) || 'the policy marker word zebracorn ' || (SELECT group_concat('é', '') FROM c),
  'goldenproj', 'policy/zebra', 1, 1, '2026-08-08 00:00:00', '2026-08-08 00:00:00', NULL
UNION ALL SELECT 8, 'sy-8', 's', 'decision', 'Old cache decision', 'We cache with strategy alpha-cache.', 'goldenproj', 'decision/cache-old', 1, 0, '2026-08-09 00:00:00', '2026-08-09 00:00:00', NULL
UNION ALL SELECT 9, 'sy-9', 's', 'decision', 'New cache decision', 'We cache with strategy beta-cache replacing alpha-cache.', 'goldenproj', 'decision/cache-new', 1, 0, '2026-08-10 00:00:00', '2026-08-10 00:00:00', NULL
UNION ALL SELECT 10, 'sy-10', 's', 'decision', 'Conflict left side', 'gamma-conflict claim one', 'goldenproj', 'decision/conflict-a', 1, 0, '2026-08-11 00:00:00', '2026-08-11 00:00:00', NULL
UNION ALL SELECT 11, 'sy-11', 's', 'decision', 'Conflict right side', 'gamma-conflict claim two', 'goldenproj', 'decision/conflict-b', 1, 0, '2026-08-12 00:00:00', '2026-08-12 00:00:00', NULL
UNION ALL SELECT 12, 'sy-12', 's', 'discovery', 'Pending left', 'delta-pending left', 'goldenproj', NULL, 1, 0, '2026-08-13 00:00:00', '2026-08-13 00:00:00', NULL
UNION ALL SELECT 13, 'sy-13', 's', 'discovery', 'Pending right', 'delta-pending right', 'goldenproj', NULL, 1, 0, '2026-08-14 00:00:00', '2026-08-14 00:00:00', NULL
UNION ALL SELECT 14, NULL, 's', 'discovery', 'Legacy row without sync id', 'epsilon-legacy row', 'goldenproj', NULL, 1, 0, '2026-08-15 00:00:00', '2026-08-15 00:00:00', NULL
UNION ALL SELECT 15, 'sy-15', 's', 'decision', 'Withdrawn relation subject', 'zeta-withdrawn subject', 'goldenproj', 'decision/withdrawn', 1, 0, '2026-08-16 00:00:00', '2026-08-16 00:00:00', NULL;

INSERT INTO memory_relations (sync_id, source_id, target_id, relation, judgment_status, superseded_at) VALUES
 ('r1', 'sy-9', 'sy-8', 'supersedes', 'judged', NULL),
 ('r2', 'sy-10', 'sy-11', 'conflicts_with', 'judged', NULL),
 ('r3', 'sy-12', 'sy-13', 'conflicts_with', 'pending', NULL),
 ('r4', 'sy-1', 'sy-2', 'related', 'judged', NULL),
 ('r5', 'sy-15', 'sy-1', 'supersedes', 'judged', '2026-09-01 00:00:00'),
 ('r6', 'sy-3', 'sy-2', 'scoped', 'judged', NULL),
 ('r7', 'sy-7', NULL, 'related', 'judged', NULL),
 ('r8', 'sy-3', 'sy-7', 'not_conflict', 'judged', NULL),
 ('r9', 'sy-4', 'sy-2', 'supersedes', 'judged', NULL),
 ('r10', 'sy-1', 'sy-4', 'conflicts_with', 'judged', NULL);
