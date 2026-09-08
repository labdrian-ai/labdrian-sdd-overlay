# Delta for longterm-mem-promotion

## MODIFIED Requirements

### Requirement: Promotion Eligibility Predicate

ID: R-007
Traces to: longterm-mem R-007

WHILE promotion runs for project P, the longterm-mem promotion writer SHALL
treat an Engram observation as eligible only if it is pinned, OR is
explicitly targeted by a promote call, OR carries a non-empty `topic_key`
whose first path segment (the substring before the first `/`, or the whole
value when it contains no `/`) is not exactly `sdd`, `review`, or `delivery`.
An observation's `type` value and its `revision_count` SHALL NOT be used as
automatic eligibility criteria.

(Previously: eligibility was pinned, OR type in `decision`/`architecture`/
`pattern`, OR revision count >= 3, OR explicitly targeted. The type- and
revision-count-based automatic criteria are retired; a curated `topic_key`
outside the excluded prefixes now gates automatic eligibility instead.)

#### Scenario: Pinned observation is eligible

- GIVEN an observation that is pinned
- WHEN eligibility is evaluated
- THEN it is eligible

#### Scenario: Explicit promote call overrides the automatic criteria

- GIVEN an observation named by an explicit promote call
- WHEN eligibility is evaluated for that call
- THEN it is eligible regardless of type, pin state, revision count, or
  `topic_key`

#### Scenario: Untopiced observation is not automatically eligible

- GIVEN an observation with an empty or absent `topic_key`, of type
  `decision`, not pinned, not explicitly targeted
- WHEN eligibility is evaluated
- THEN it is not eligible

#### Scenario: Curated topic_key is eligible regardless of type or revision count

- GIVEN an observation of type `discovery`, with a revision count of 0, not
  pinned, not explicitly targeted, and a `topic_key` of
  `longterm-mem/promotion-eligibility-policy` (first segment `longterm-mem`)
- WHEN eligibility is evaluated
- THEN it is eligible

#### Scenario: sdd-prefixed topic_key is excluded

- GIVEN an observation with `topic_key` `sdd/some-change/tasks`, not pinned,
  not explicitly targeted
- WHEN eligibility is evaluated
- THEN it is not eligible

#### Scenario: review-prefixed topic_key is excluded

- GIVEN an observation with `topic_key` `review/some-change/verdict`, not
  pinned, not explicitly targeted
- WHEN eligibility is evaluated
- THEN it is not eligible

#### Scenario: delivery-prefixed topic_key is excluded

- GIVEN an observation with `topic_key` `delivery/some-change`, not pinned,
  not explicitly targeted
- WHEN eligibility is evaluated
- THEN it is not eligible

#### Scenario: Prefix exclusion matches only the exact first path segment

- GIVEN an observation with `topic_key` `sdd-init/some-project/context` and,
  separately, an observation with `topic_key` `sddx/notes`, neither pinned
  nor explicitly targeted
- WHEN eligibility is evaluated for each
- THEN both are eligible, because their first path segments (`sdd-init` and
  `sddx`) are not an exact match for the excluded segment `sdd`

#### Scenario: High-revision, decision-typed, unpinned, untopiced observation is not eligible

- GIVEN an observation of type `decision` with a revision count of 5, an
  empty `topic_key`, not pinned, not explicitly targeted
- WHEN eligibility is evaluated
- THEN it is not eligible, because type and revision count no longer confer
  eligibility on their own

#### Scenario: Pinned observation overrides both the untopiced and prefix exclusions

- GIVEN a pinned observation with `topic_key` `review/some-change/verdict`
- WHEN eligibility is evaluated
- THEN it is eligible

### Requirement: Sync Promotes Unpromoted-or-Revised Observations

ID: R-009
Traces to: longterm-mem R-009

WHEN `sync` runs for project P, the longterm-mem component SHALL promote
every eligible observation for P that is either unpromoted or whose current
revision count exceeds the revision last promoted, applying eligibility per
the Promotion Eligibility Predicate (R-007).

(Previously: same trigger condition, but eligibility was evaluated under the
prior type/revision-count-based predicate.)

#### Scenario: Never-promoted eligible observation is promoted

- GIVEN an eligible, never-promoted observation for project P
- WHEN sync runs for P
- THEN it is promoted

#### Scenario: Revised eligible observation is re-promoted

- GIVEN an eligible observation already promoted at revision 2, now at
  revision 3
- WHEN sync runs
- THEN it is re-promoted

#### Scenario: Unchanged eligible observation is a no-op

- GIVEN an eligible observation already promoted at its current revision
  (unchanged)
- WHEN sync runs
- THEN it is not re-promoted

#### Scenario: sync --dry-run transcript is the acceptance evidence for the narrowed predicate

- GIVEN a `sync --dry-run` run for `labdrian-sdd-overlay` after the
  Promotion Eligibility Predicate (R-007) change lands
- WHEN the resulting promoted-set transcript is compared against the
  pre-change sample of 282 automatically-eligible observations (182
  `sdd/`-prefixed, 58 untopiced, 4 `delivery/`-prefixed, 1 `review/`-prefixed)
- THEN none of those 245 rows appear in the post-change promoted set unless
  independently pinned or explicitly promoted, and the transcript is
  reviewed as the acceptance evidence for R-001 through R-003
