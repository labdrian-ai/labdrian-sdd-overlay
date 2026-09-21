# Procedural Skill Registration Specification

## Purpose

Move a linted draft into the project tier, where the agent is autonomous:
write the skill into each confirmed runtime target directory inside the
project it is working in, stamp declared provenance, record ownership
evidence in a project-owned lock file, and commit the result as one
reviewable, revertable unit. Promotion of a project skill into the global
overlay `skills/` tree stays a separate, human-gated procedure over the
existing `engine skills add` path.

## Requirements

### Requirement: Codex Discovery Is Confirmed Before Codex Support Is Claimed

`.agents/skills/` is not a Codex-only target: it is written unconditionally
because Pi reads it once the project is trusted, regardless of the Codex
smoke-test outcome (decision (e)). The Codex smoke test therefore gates
only whether Codex support is *claimed* for that target, not whether the
target is written. The system MUST perform and record an explicit smoke
test — placing a throwaway skill under a temporary repository's
`.agents/skills/<id>/` and observing whether Codex 0.148.0 lists or loads
it — with the exact command and its output recorded in the design record,
before any registration code path claims Codex support for
`.agents/skills/`. If the smoke test does not confirm discovery, the system
MUST record Codex support as a known gap while still writing
`.agents/skills/<id>/SKILL.md` for Pi, and MUST NOT block registration for
Claude Code or Pi.

#### Scenario: Confirmed Codex discovery claims Codex support

- GIVEN the recorded smoke test shows Codex 0.148.0 lists or loads a skill
  placed under a temp repo's `.agents/skills/<id>/`
- WHEN the registration target list is built
- THEN `.agents/skills/` is written, and Codex support is claimed for it,
  subject to decision (e)'s trust-prompt tradeoff

#### Scenario: Unconfirmed Codex discovery still writes the shared Pi target

- GIVEN the recorded smoke test shows Codex 0.148.0 does not list or load a
  skill placed under `.agents/skills/<id>/`
- WHEN the registration target list is built
- THEN `.agents/skills/` is still written, because Pi requires it
  independently of the Codex result
- AND registration still proceeds for `.claude/skills/`
- AND Codex support is recorded as a known gap, not claimed

#### Scenario: No registration code path claims Codex support before the test is recorded

- GIVEN the repository state before the Codex smoke test has been run and
  recorded
- WHEN the registration code is inspected
- THEN no code path claims Codex support for `.agents/skills/` on an
  unverified assumption

### Requirement: Multi-Target Project-Tier Registration

The system MUST write a linted SKILL.md into exactly two configured
project-local runtime target directories inside the project the agent is
working in: `.claude/skills/<id>/SKILL.md` always, and
`.agents/skills/<id>/SKILL.md` always — the latter unconditionally serving
Pi (after project trust) and additionally serving Codex once the Codex
discovery check has confirmed support, per revised decision (e). The system
MUST NOT write `.pi/skills/<id>/SKILL.md` under any condition. The system
MUST reuse the existing R-055 traversal guard and the existing
validate-before-write discipline from `engine/skills/install.go` for every
target write, and MUST leave `engine/skills/install.go`'s own behavior
unchanged.

#### Scenario: Registration writes to both project-tier targets

- GIVEN a linted draft ready for registration
- WHEN registration runs
- THEN the SKILL.md is written to `.claude/skills/<id>/SKILL.md` and
  `.agents/skills/<id>/SKILL.md`
- AND no file is written to `.pi/skills/<id>/SKILL.md`

#### Scenario: Registration output discloses the Pi project-trust prompt

- GIVEN a registration that writes `.agents/skills/<id>/SKILL.md` in a
  project Pi has not yet trusted
- WHEN registration completes
- THEN the registration output discloses that Pi will ask for project
  trust the next time it starts in that project

#### Scenario: A path-traversal id is rejected

- GIVEN a candidate id containing a path-traversal sequence (for example
  `../../etc`)
- WHEN registration attempts to write it
- THEN the write is refused by the reused R-055 traversal guard
- AND no file is written to any target

#### Scenario: install.go behavior is unchanged

- GIVEN the existing `engine/skills/install.go` test suite
- WHEN this capability's code is added
- THEN `install.go`'s existing tests still pass unmodified, and its
  behavior for Claude-only registry-driven install is unchanged

### Requirement: LintSkill Is a Hard Gate Run on the Stamped Bytes, Before Any Hash or Write

Registration MUST stamp provenance onto the draft's in-memory bytes first,
then run `LintSkill` on that stamped frontmatter and body, and only then
compute the sha256 hash and write to any target directory (stamp, then
lint, then hash, then write — see the design's provenance decision). The
system MUST refuse to hash or write to any target, and MUST NOT write the
lock file, when `LintSkill` returns any hard error against the stamped
bytes; a hard-lint refusal discards the stamped bytes without persisting
them anywhere.

#### Scenario: A hard-lint-failing draft is refused before any hash or write

- GIVEN a draft whose frontmatter is missing `metadata.version`
- WHEN registration stamps the draft and runs `LintSkill` on the stamped
  bytes
- THEN no file is written to any target directory
- AND no hash is computed
- AND the lock file is not updated
- AND the refusal names the hard lint error

#### Scenario: A draft with only warnings proceeds

- GIVEN a draft with zero hard errors and at least one warning
- WHEN registration is attempted
- THEN the write proceeds to every configured target

### Requirement: Provenance Frontmatter Is Stamped on Every Registered Skill

The system MUST stamp every project-tier-registered skill's frontmatter
with `metadata.provenance: procedural`, `metadata.candidate: <topic key>`
identifying the source candidate, and `metadata.author` set to the fixed
constant chosen for agent-authored skills (decision (g)), never the OS
username or a GitHub identity. Stamping MUST happen before `LintSkill`
runs, so that `LintSkill` validates the exact stamped frontmatter that will
be written and hashed.

#### Scenario: Registered skill carries provenance and candidate reference

- GIVEN a draft with a known source candidate topic key
- WHEN it is registered
- THEN the written SKILL.md frontmatter has `metadata.provenance:
  procedural` and `metadata.candidate` equal to that topic key

#### Scenario: metadata.author is the fixed constant, not a personal identity

- GIVEN registration running under any OS user or CI identity
- WHEN the SKILL.md is written
- THEN `metadata.author` equals the documented fixed constant
- AND it does not equal the OS username, git config user, or any GitHub
  handle

### Requirement: Project Lock File Records Ownership Evidence

The system MUST record, for every registered skill, its id, provenance,
source candidate key, and sha256 content hash in a project-owned lock file
separate from `skills-lock.json` (decision (b)), and MUST NOT write
procedural registration entries into `skills-lock.json`, which remains
owned by the external installer.

#### Scenario: A registration test over temp dirs records every field

- GIVEN a `t.TempDir()` project root and a linted draft
- WHEN registration runs
- THEN the project-owned lock file gains an entry with the skill id,
  `metadata.provenance`, the source candidate key, and the sha256 of the
  written content

#### Scenario: skills-lock.json is untouched by procedural registration

- GIVEN a project root containing an existing `skills-lock.json` owned by
  an external installer
- WHEN procedural registration runs
- THEN `skills-lock.json`'s bytes are unchanged

### Requirement: Registration Status Is registered, Distinct From promoted

The system MUST set the record's `Status` to a project-tier value (for
example `registered`) upon successful project-tier registration, and MUST
reserve a distinct global-tier value (for example `promoted`) exclusively
for skills that have completed the human promotion procedure into
`skills/`. The two statuses MUST be distinguishable in every read of the
record and in its `History`.

#### Scenario: Project-tier registration never reports promoted

- GIVEN a skill that has completed only project-tier registration
- WHEN its `Status` is read
- THEN it reads a project-tier value distinct from `promoted`

#### Scenario: Only human promotion sets promoted

- GIVEN a skill that has been promoted into `skills/` via the human
  procedure
- WHEN its `Status` is read
- THEN it reads `promoted`
- AND no automated project-tier code path ever sets `promoted`

#### Scenario: Post-promotion project-tier retirement does not change Status

- GIVEN a skill whose `Status` already reads `promoted`
- WHEN the agent afterward removes the now-redundant project-tier copies
  with `project-retire --reason promoted`
- THEN the project-tier files and their project lock entry are deleted
- AND the record's `Status` continues to read `promoted`
- AND a `History` line records the removal without recording a status
  transition

### Requirement: Registration Produces Exactly One Scoped Commit

The system MUST perform the registration write as exactly one git commit
containing only the paths written by that registration (the target
SKILL.md files and the project lock file), using an explicit pathspec, and
MUST refuse to commit — leaving the working tree exactly as it was found —
when the consumer repository has unrelated staged changes at the time
registration is invoked.

#### Scenario: A clean repository produces one scoped commit

- GIVEN a consumer repository with no staged changes before registration
- WHEN registration completes successfully
- THEN exactly one new commit exists
- AND that commit's changed paths are exactly the registered SKILL.md
  files and the project lock file, and nothing else

#### Scenario: Unrelated staged changes cause a refusal, not a commit

- GIVEN a consumer repository with an unrelated file already staged before
  registration is invoked
- WHEN registration is attempted
- THEN no commit is created
- AND the refusal names the unrelated staged change
- AND the unrelated staged change remains staged exactly as it was

#### Scenario: git revert restores the pre-registration state

- GIVEN a successful registration commit
- WHEN `git revert <commit>` is run against it
- THEN the target SKILL.md files and the project lock file entry return to
  their pre-registration state

### Requirement: No File Is Written Under skills/ Except Through the Human add Path

The system MUST NOT write, at the project tier, any file under the
overlay's global `skills/` tree. The only path that writes under `skills/`
MUST be the existing human-invoked `engine skills add` (`AddCore`)
procedure.

#### Scenario: Project-tier registration in the overlay repository itself

- GIVEN the agent working inside the overlay repository (decision (f))
- WHEN project-tier registration runs
- THEN it writes only to `.claude/skills/` and `.agents/skills/`, never to
  `.pi/skills/` and never to `skills/`

#### Scenario: No test writes under skills/ as a side effect

- GIVEN the full registration test suite
- WHEN it runs
- THEN no test writes a file under `skills/` outside the explicit, human
  procedure's own `AddCore` tests

### Requirement: Global Promotion Reuses AddCore Unchanged Apart From the Lint Gate

The system MUST document project-to-overlay promotion as a human-invoked
procedure over the existing `engine skills add` / `AddCore` path, adding no
new CLI surface and no new approval machinery, no TTL, and no implicit
consent from silence. The only behavioral addition to `AddCore` MUST be the
`LintSkill` hard gate defined by the `skill-lifecycle` capability.

#### Scenario: Promotion procedure names an existing command

- GIVEN the documented promotion procedure
- WHEN it is followed to promote a project-registered skill
- THEN the command invoked is `engine skills add`, with no new subcommand
  introduced by this change

#### Scenario: Silence is not consent for promotion

- GIVEN a project-registered skill awaiting promotion with no human action
  taken
- WHEN any amount of time passes
- THEN the skill is not promoted, and no TTL-based or default-approval
  mechanism promotes it automatically

### Requirement: Tests Use Temporary Directories Only

Every automated test for this capability MUST operate exclusively on
`t.TempDir()`-rooted paths and MUST NOT read from or write to a live
`.claude/skills/`, `.agents/skills/`, `.pi/`, `skills-lock.json`, `$HOME`,
or invoke a live runtime binary.

#### Scenario: Test suite touches no live user directory

- GIVEN the full registration test suite
- WHEN it runs on a developer or CI machine with real `.claude/skills/`,
  `.agents/skills/`, `.pi/`, and `skills-lock.json` present
- THEN none of those live paths are modified, and none of their content
  influences a test's pass/fail outcome
