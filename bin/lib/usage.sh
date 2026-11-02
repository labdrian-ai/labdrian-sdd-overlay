# shellcheck shell=bash
#
# usage
#
# The text of --help.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

usage() {
  cat <<'EOF'
Usage: labdrian <command> [options]

Commands:
  bootstrap                        Initialize overlay repo and branches
  capture [--target claude|opencode|codex] [--from-backup DIR]
                                   Capture managed files from target (default: claude).
                                   Pushes the 'upstream' mirror branch to origin.
  apply [--target claude|opencode|codex|pi|all]
                                   Merge upstream and deploy to target(s) (default: all).
                                   Pushes 'main' to origin immediately after the merge, so a
                                   local apply never strands main ahead of origin.
                                   After a merge, run 'skills validate': each global skill the
                                   merge changed needs a human approval (see skills approve
                                   below), which an agent cannot give.
  self-update                    Fast-forward ONLY local 'main' (never the current branch),
                                   then return to the branch you were on. Converges to the
                                   latest published release tag (never past it, even when
                                   origin/main carries untagged commits beyond that tag); before
                                   the first release tag exists, falls back to legacy
                                   origin/main-HEAD convergence. Refuses (exit 1) on a dirty
                                   tracked tree, a local main ahead of origin/main, or no
                                   'origin' remote. Untracked files never block it.
  update                           Read-only: report the latest published release and each
                                   target's recorded version (up-to-date/behind/never deployed).
                                   Refreshes only cached tags/remote refs -- never a branch head,
                                   the working tree, target files, or state.
  restore --target claude|opencode|codex [--list] [--backup TIMESTAMP]
                                   DESTRUCTIVE: rolls a single target back from one of its
                                   retained backups (up to 3, auto-pruned; default: most
                                   recent). Refuses --target all. --list shows retained
                                   backups (timestamp, version) without changing anything.
                                   --backup TIMESTAMP picks a specific one. Exits non-zero
                                   without touching any file when the target has no backups.
                                   Performs zero git operations.
  targets                          Read-only: print the target catalog, one target per line as
                                   <name><TAB><kind> (kind: copy or package), in the order
                                   --target all expands to. Takes no arguments. Front-ends
                                   read this list instead of keeping their own, so they never
                                   act on a target they did not show.
  version (also: --version)       Read-only: print this clone's current release version
                                   (from local main) and each target's recorded deployed
                                   version, naming any that are behind. Never-deployed
                                   targets are reported honestly, never fabricated.
  status [--target claude|opencode|codex|pi|all]
                                   Show drift between repo and target(s) (default: all)
  sync-check [--target claude|opencode|codex|pi|all] [--check-origin|--fetch]
                                   Validate gentle-ai sync state per target (default: all).
                                   Also reports REPO_BEHIND_ORIGIN=<count|NA> on each VERDICT
                                   line: how many commits local main is behind this repo's own
                                   origin/main. Default is a cached-ref comparison (no network
                                   call); pass --check-origin (alias --fetch) to run
                                   'git fetch origin' first for a live count. NA means the
                                   comparison is unavailable (no origin remote, no cached ref,
                                   or a requested fetch failed) -- other checks still complete.
  install-hooks                    Build + deploy the engine binary, then wire the
                                   deterministic-scoping hooks, the workflow projection
                                   hooks, the skills approve guard, the review
                                   stop-hook (on Stop: asks the agent for each thing
                                   the first commit with unfinished review steps
                                   misses, once for each session and commit), and the builder guard (on the Agent
                                   tool: denies launching the builder on a task the
                                   shaper did not design) into
                                   ~/.claude/settings.json. Re-run it after an
                                   upgrade that adds a hook family, then restart Claude Code
                                   to load the hooks.
                                   If CODEX_HOME (default ~/.codex) already exists, merge
                                   the Codex SessionStart memory-instructions hook into
                                   hooks.json, with an atomic .bak backup. Trust it once
                                   in the Codex TUI (/hooks, key t); config.toml is untouched.
                                   DELIBERATE ACTION — not called by 'apply'. Run once to
                                   activate; the hooks are INERT until this command runs.
                                   Also best-effort installs discovery tools (bat rg fd sd
                                   eza) via Homebrew so agents never hit the silent-empty
                                   footgun. Set OVERLAY_SKIP_TOOL_INSTALL=1 to opt out.
  uninstall-hooks                  Remove our hook entries from ~/.claude/settings.json
                                   (the review stop-hook on Stop and the builder guard
                                   on Agent among them),
                                   and our Codex SessionStart hook from CODEX_HOME/hooks.json
                                   (default ~/.codex), leaving foreign bytes intact.
  status-hooks                     Read-only check of overlay installation health: binary
                                   present, hooks wired, contract readable. Exits 0 if all
                                   OK, 1 if any check fails, 2 for warnings alone (a missing
                                   or drifted review stop-hook or builder guard is one,
                                   until install-hooks is re-run).
                                   Codex is reported only if its home exists; presence
                                   does not prove trust (/hooks, key t). Read-only.
  doctor [--fix]                   Read-only host-toolchain preflight: checks go (required),
                                   gentle-ai, bat/rg/fd/sd/eza, the engine binary, and the
                                   skill-registry of the project, which the engine judges
                                   (`status --only registry`). Prints PASS/WARN/FAIL per check;
                                   exits non-zero only on a hard FAIL. Safe to run at any time.
                                   --fix: after the checks, attempt to install any missing
                                   discovery tools (bat rg fd sd eza) via Homebrew (best-effort,
                                   non-fatal), then re-check and report the result.
  repair-sdd-registry [--cwd DIR] [--target claude|opencode|codex] [--no-refresh]
                                   Refresh and repair .atl/skill-registry.md so existing
                                   sdd-* phase executor skills stay resolvable after
                                   gentle-ai sync/skill-registry refresh. The repair is the
                                   engine's (registry repair-sdd): it takes the lock the
                                   propagate hook takes and writes the registry atomically,
                                   so it needs the engine binary (built on first use).
  skill-registry refresh [--cwd DIR] [--target claude|opencode|codex] [--no-repair]
                                   Run gentle-ai skill-registry refresh, then immediately
                                   repair sdd-* executor rows for the selected target
                                   (default target: codex), as repair-sdd-registry does.
  tui                              Launch the Bubbletea TUI front-end
  install-alias [name]             Symlink a command (default: labdrian) into ~/.local/bin
  skills <verb>                    Forward to the engine's skills registry core
                                   list      print sorted registry entries
                                   status    print count summary
                                   validate  cross-check registry vs overlay.manifest,
                                             and skills/ on disk vs overlay.manifest
                                             (requires --source-root); every global skill
                                             needs a valid approval record of all its files
                                             (see approve below); none is excused
                                   install   install project-scoped skills into <cwd>/.claude/skills and
                                             <cwd>/.agents/skills, replacing only what it installed and left
                                             unmodified. It records each file it writes (with its SHA-256) in the
                                             project lock, refuses, naming the path, a hand-edited file or a
                                             directory it did not install, reports every refusal and then writes
                                             nothing for any skill, and on a second run with no source change
                                             writes nothing and prints "unchanged: <id>"
                                   adopt     record skill directories already in the project as installed, only
                                             when they are exactly the current source; nothing is adopted silently
                                             (a directory stays foreign to install until skills adopt records it).
                                             It writes the project lock and no skill file
                                   uninstall [--dry-run] <id>
                                             remove from <cwd> a skill install put there: every
                                             file its install record lists, in both runtimes, then
                                             the record (the lock file stays). It refuses, naming
                                             the path, a hand-edited file, an id with no install
                                             record, or a procedural skill (project-retire's), and
                                             then removes nothing; a file the record does not list
                                             is left, with its directory (note: kept <dir>)
                                   add <id> [--repo <url>] [--ref <sha>]
                                             register a skill: custom (no --repo) or
                                             external=vendored provenance (--repo set).
                                             The overlay NEVER fetches; --repo/--ref are
                                             inert labels for a file you already committed.
                                             Refused unless skills/<id>/.approval.json holds a
                                             valid approval for every file of the skill (see
                                             approve below); nothing is written on refusal.
                                   remove <id> unregister a skill from registry + manifest
                                   sync-manifest  regenerate */SKILL.md rows from skills.registry.yaml
                                   approve --id <id> --approver <label>
                                             record a human approval of every file of skills/<id>/
                                             in skills/<id>/.approval.json, bound to one digest of
                                             all of them; the engine cannot prove a human ran it,
                                             so the record only proves the files match
                                             The digest is the SHA-256 of the lines
                                             "<sha256 of the file> <x or ->  <path>" (the format
                                             of sha256sum, with x after the digest for a file whose
                                             owner execute bit is set and - for any other) of every
                                             regular file of the skill's directory, at any depth,
                                             sorted by path, except .approval.json itself. Setting
                                             or clearing that bit (chmod u+x) makes the approval
                                             stale; the other permission bits are not covered.
                                             To recompute it by hand:
                                               (cd skills/<id> && find . -type f ! -path ./.approval.json |
                                                sed 's|^\./||' | LC_ALL=C sort |
                                                while IFS= read -r f; do
                                                  if [ -n "$(find "./$f" -perm -100)" ]; then m=x; else m=-; fi
                                                  echo "$(sha256sum < "$f" | cut -c1-64) $m  $f"
                                                done | sha256sum)
                                             A link, or any entry that is not a regular file,
                                             cannot be covered: the skill is APPROVAL_UNVERIFIABLE
                                             until it is replaced with a regular file or removed.
                                             A record of version 1 covered SKILL.md alone: it is
                                             APPROVAL_STALE, and approving the skill again writes
                                             a record of version 2
                                             A skill that fails the hard lint is refused, except
                                             a lint-baseline skill (one of the 34 global skills
                                             that predate the lint budget; the list exempts them
                                             from that lint only, never from approval): for those,
                                             only the size and description findings
                                             (body-hard-budget, description-max,
                                             description-one-line) are printed as "warning:" lines
                                             on stderr, the approval is still recorded, and the
                                             exit is 0; any other hard finding, such as a missing
                                             front matter, refuses a baseline skill too
                                             A PreToolUse hook (install-hooks) denies the agent running
                                             it or writing the record by hand: a speed bump, not a
                                             security boundary. A person runs it in a terminal.
                                   lint <path> | --rules
                                             lint a SKILL.md file against the authoritative rule
                                             table, or print the table; exit 1 on any hard finding
                                   project-register --project-root <abs> --candidate <key> [--dry-run] <draft-file>
                                             register an agent-owned, project-tier procedural skill:
                                             write <root>/.claude/skills and <root>/.agents/skills and
                                             the project lock; --dry-run prints the plan, writes nothing
                                   project-revise   --project-root <abs> --candidate <key> [--dry-run] <draft-file>
                                             revise an agent-owned project skill (same outputs)
                                   project-status   --project-root <abs> [<id>]
                                             report project-tier ownership and global supersession
                                             from the project lock (read-only)
                                   project-retire   --project-root <abs> [--dry-run] [--reason <text>] [--absorbed-into <id>] <id>
                                             retire an agent-owned project skill; an explicit decision
                                             that nothing in the engine takes for you; --dry-run prints
                                             the removal plan
                                   Project-tier skills stay autonomous: they need no approval record.
                                   The lint baseline (used by approve above and in the README) is the
                                   global skills that existed at the Phase 8 base (main at 53f545d, the
                                   commit the global skill lifecycle was built on) and still do: 37 at
                                   the base, 34 since gadu-operator and gadu-orchestrate were retired
                                   and prespec-malandra was replaced by clarify-request. It is a list
                                   of ids for the lint, and excuses no skill from approval.
                                   Locking: add, remove, sync-manifest and approve hold an exclusive lock, and
                                   validate and install a shared one, on .skills.registry.yaml.lock beside the
                                   registry (created by the first write, never removed, git-ignored; shared
                                   holders never create it, so both work on a read-only overlay). A lock that stays
                                   taken for 2 seconds means another skills command is in progress: the verb
                                   changes nothing and reports it with exit 2 (retry). Exit 1 is unchanged: the
                                   verb ran or was refused, or the lock could not be created at all.
                                   Project lock: project-register, project-revise, project-retire, install, adopt and uninstall also
                                   take an exclusive lock on the project root directory itself (--project-root, or
                                   the working directory for install, adopt and uninstall), so no file is created in the project; a
                                   busy one is exit 2 too, and a filesystem that cannot lock a directory refuses the verb
                                   (exit 1). A verb that holds both takes the overlay lock first, then the project lock,
                                   so install and adopt can wait up to twice the lock bound (2 seconds each) when both
                                   are busy. project-status takes a shared lock on the project. A writer refuses, with
                                   nothing locked, when the registry it would lock does not exist.
  shaper <verb>                    Forward Shaper handoff verbs to the engine unchanged
                                   assess    read-only readiness (exit 0 ready, 3 draft, 2 invalid)
                                   clearance record --stdin  store a human decision (Pi /shaper-clear)
  roles <verb>                     Forward reusable-role handoff verbs to the engine unchanged.
                                   Roles are data only; no verb launches an agent or grants authority.
                                   validate --file PATH                       read-only: parse and validate one record
                                   next --project ID --goal ID --chain ID     read-only: allowed next roles
                                   resume --project ID --goal ID --chain ID   read-only: current role, interrupted?, reason
                                   append --project ID --goal ID --chain ID --stdin
                                             append one record, read only from stdin; a completed review-report
                                             record is held to the files review record and verify stored in the
                                             repository of the working directory: it needs review-diff and
                                             review-result evidence, the review-result and the verifier-report carry
                                             the sha256 of the stored files, which must still be a result and a
                                             verdict of it, and the review-diff the range and the
                                             diff_sha256 of the stored result; evidence that is missing or differs is
                                             exit 1, and nothing is stored; a completed review-exemption record is
                                             held in the same way to the exemption review exempt stored (review-diff
                                             and review-exemption evidence; the exemption must still hold)
                                   match-shaper --root PATH --handoff PATH    report which Shaper v3 roles match the vocabulary
                                   builder-guard                              not run by a person: the Claude Code hook on the Agent tool
                                             that install-hooks wires (the builder guard); it denies launching
                                             the builder on a prompt that cites no task the shaper designed,
                                             or outside the running workflow of the cited feature whose
                                             role chain hands the work to the builder (run workflow open,
                                             or append the hop the chain waits for)
                                             (exit 0 always, 1 usage error)
  review <verb>                    Forward review binding and storage verbs to the engine unchanged.
                                   bind --base REV --head REV [--cwd REPO]
                                             bind a candidate: full SHAs, diff_sha256 and changed paths as JSON; make
                                             the detached review worktree beside the repository; a range whose base is
                                             its head covers no commit and is refused (exit 0 bound, 1 error)
                                   record --feature NAME --base REV --head REV --rows ID,ID [--result FILE|-] [--cwd REPO]
                                             hold a reviewer's reply to the binding recomputed from git and store it
                                             with the lens rows the reviewer was sent (--rows is required: ids of
                                             roles/reviewer/rows.json, separated by commas) as
                                             odd/review-results/NAME/HEAD12-N.json, never over a file; print the
                                             record skeleton for roles append (exit 0 stored, 2 reply refused, 1 error)
                                   verify --feature NAME --stored RESULT [--verdicts FILE|-] [--cwd REPO]
                                             hold a verifier's reply to the stored result, decide whether the review
                                             blocks and store it beside the result as HEAD12-N.verifier.json, never
                                             over a file; print the record skeleton with evidence verifier-report and
                                             to_role filled (builder when it blocks, delivery when it is clear); one
                                             correction round per candidate (exit 0 stored, 2 reply refused, 1 error)
                                   check --feature NAME [--head REV] [--base REV | --onto REV] [--cwd REPO]
                                             recompute from git the candidate of each result stored for the head
                                             (default HEAD), hold the results and verdicts to it, and refuse unless
                                             one result binds the head and every one is clear: a result that blocks,
                                             or a severe finding nobody checked, refuses, because a check that did
                                             not run never passes; an exemption that still holds clears a head that
                                             has no result; print the files used as JSON, store nothing; with --base
                                             REV, the integration base of the delivery, check a chain: reviews are
                                             per commit, so the head is cleared only when clear results or
                                             exemptions cover REV..head as an unbroken chain, each range starting
                                             where the one before ended; a gap, a range that is not clear or a walk
                                             that never reaches REV refuses; print the ranges used as JSON; an
                                             exemption whose base is not the first parent of the head refuses;
                                             --onto REV names the integration branch of the delivery and starts
                                             the chain at the one merge base of REV and the head, as --base of that
                                             commit does (not with --base; none or several merge bases refuse)
                                             (exit 0 cleared, 1 refused or error)
                                   exempt --feature NAME --base REV --head REV --risk none|ITEMS [--cwd REPO]
                                             exempt a head from review on the evidence of the verifier and the risk
                                             the agent declares; reads the verifier's report from stdin:
                                             {"checks": [{"command": ..., "result": pass, fail or not-run, "output": ...}]}
                                             --risk is none or the numbers 1 to 6 of the high-risk items (1 data or
                                             irreversible effects, 2 security, 3 contracts others consume, 4 concurrency,
                                             5 delivery or environment, 6 no test would catch a regression) separated
                                             by commas; a head that names any item is never exempt; a report that is
                                             missing or not clear is refused, because a check that did not run never
                                             passes; an exemption is for one commit: --base must be the first parent
                                             of --head, or it is refused; store HEAD12-N.exempt.json, never over a
                                             file; print the record skeleton for roles append with payload_kind
                                             review-exemption; review check clears an exempted head, but a result that
                                             blocks wins over any exemption (exit 0 stored, 2 report refused, 1 error)
                                   sweep --feature NAME --base REV --head REV [--cwd REPO]
                                             run the checks T1 to T4 of the sweeper once on the one commit REV..head
                                             (--base must be the first parent of --head) and store the findings as
                                             HEAD12-N.sweep.json, never over a file: T1 the paths the commit deleted
                                             or renamed and the removed names and flags the head still mentions, T2 the
                                             new lines of deadcode, staticcheck, errcheck, nilerr and shellcheck over
                                             copies of the base and the head kept under the state home, and go mod tidy -diff over each
                                             module whose go.mod or go.sum it changes or whose .go files lose an import,
                                             a finding being a required module the head no longer needs, T3 the working
                                             tree's git status, T4 the paths the commit changes whose name ends .orig
                                             or .bak and the test fixtures it leaves unread;
                                             a tool that cannot run stores nothing (it needs deadcode, staticcheck,
                                             errcheck, nilerr, shellcheck and go on PATH (Go 1.23 or newer), and the module cache of
                                             a module whose dependencies the commit touches); a second sweep of the repository
                                             while one runs is refused; print outcome (clean or fired), path, sha256 and the
                                             candidates the sweeper answers for as JSON
                                             (exit 0 stored, 2 refused or a check that did not run, 1 usage error)
                                   record --role sweeper|polisher --feature NAME --base REV --head REV [--result FILE|-] [--cwd REPO]
                                             store the sweeper's report (it must answer once for each candidate of a
                                             fired sweep of the head) as HEAD12-N.sweeper.json, or one polisher judge's
                                             findings as HEAD12-N.polisher.json, for the one commit REV..head; print the
                                             path and sha256 as JSON, with no skeleton for roles append; --rows with
                                             another role is a usage error; --role reviewer is the default and is the
                                             record verb above (exit 0 stored, 2 report refused, 1 error)
                                   anchor --merge REV [--cwd REPO]
                                             decide whether the merge REV lands a tree that stored reviews cleared
                                             (CI runs it as review anchor --merge HEAD on every pull request and
                                             every push to main): the merge must have exactly two parents, the first
                                             the base and the second the head of the pull request, and they exactly
                                             one merge base, or it is refused; the features are the feature documents
                                             odd/tasks/NAME.md that the commits the pull request adds change, and a
                                             merge that changes none passes as no-feature; the merge is reviewed when
                                             one feature's chain clears (checked as review check --base does), and
                                             then the tree of the merge must equal the tree of the head, or it is
                                             refused; print the outcome (verified or no-feature) and the features as
                                             JSON, store nothing
                                             (exit 0 verified or no-feature, 1 refused or error)
                                   launch --runtime claude|codex --feature NAME --base REV --head REV --rows ID,ID --model MODEL [--cwd REPO]
                                             the one verb that starts a reviewer, on Codex read-only and on Claude
                                             Code with no tool: read the brief
                                             from stdin, bind the candidate as review bind does and run
                                             codex exec --sandbox read-only --model <model> --cd <worktree>
                                             --skip-git-repo-check --ephemeral --ignore-user-config with the reviewer
                                             instructions of the deployed Codex agent file (a missing one says run:
                                             labdrian-overlay apply --target codex) and the brief, then record its
                                             last message as review record does, printing what it prints;
                                             with --runtime claude run instead one tool-free claude --print
                                             --strict-mcp-config in the worktree over the body of the deployed Claude agent file (a missing one
                                             says run: labdrian-overlay apply --target claude), the brief and its
                                             ## Frozen context (git diff --no-renames --function-context), and record
                                             its stdout; --runtime must be claude or codex: on Pi the parent launches
                                             the reviewer agent natively; --model is required; refused before the
                                             reviewer starts (exit 1): a candidate over the 150000-byte limit of one launch,
                                             and a brief that is empty, not UTF-8, or that does not name the full
                                             base, the full head, the diff_sha256 and the worktree path of the
                                             binding; a codex that fails or leaves no last message is an error that
                                             names codex and repeats what it printed on stderr, and nothing is
                                             stored; a reply refused for its shape is asked for again once, the
                                             brief followed by ## Your previous reply was refused and the refusal
                                             (its first 1024 bytes), unless that is over the reply limit; a
                                             refusal of the second reply, or of the binding or the inspection, is
                                             not asked again, and a re-ask the reviewer does not answer leaves
                                             the first refusal, with the re-ask's failure said after it
                                             (exit 0 stored, 2 reply refused, 1 usage error or any other failure)
                                   proposals [--cwd REPO]
                                             print, and never run, what the owner may remove: the worktrees other
                                             than the main one and the one of --cwd, not locked, that git marks
                                             prunable (git worktree prune) or whose HEAD the default branch has
                                             (git worktree remove PATH), and the local branches merged into the
                                             default branch other than it and the one checked out at --cwd
                                             (git branch -d NAME), as one JSON object {default, worktrees,
                                             branches}; never --force or -D; a repository with no default branch,
                                             or a detached HEAD at --cwd, is refused; store and delete nothing:
                                             the parent relays each proposal to the owner and runs a proposed
                                             command only after the owner confirms it
                                             (exit 0 listed, 1 usage error, refusal or any other failure)
                                   stop-hook
                                             not run by a person: the Claude Code Stop hook that install-hooks wires
                                             (the review stop-hook). On a branch other than the default one, it blocks
                                             the stop for the first commit that misses anything review check asks of
                                             it, with the command for each thing it misses, asking for each thing once
                                             for each session and commit, and never for the whole branch; it lets the
                                             stop happen on anything it cannot tell (exit 0 always, 1 usage error)
                                   Only launch starts an agent, the reviewer on Codex; no other verb launches an
                                   agent or dispatches work.
  memory <verb>                    Forward memory-directive verbs to the engine unchanged.
                                   Read-only: no verb queries or writes memory; query execution
                                   belongs to a runtime adapter outside this overlay (Phase 7).
                                   plan --profile NAME [--goal PATH] [--goal-directive PATH] [--handoff-directive PATH]
                                             resolve a workflow profile's default memory directive,
                                             narrow it with an optional Goal-supplied then
                                             handoff-supplied directive, and print the plan as JSON
  workflow <verb>                  Forward standalone workflow lifecycle verbs to the engine unchanged.
                                   Local bookkeeping under $XDG_STATE_HOME/labdrian/workflows/<project>/<workflow>.jsonl;
                                   no verb requires Gentle AI, gentle-pi, a runtime, memory, or auth (an
                                   unavailable dependency is recorded on the event, and one is recorded available
                                   only when its file or binary is seen by stat, never opened, run, or trusted), and no
                                   verb executes a workflow step or check (that is Phase 7).
                                   create --project ID --workflow ID --goal PATH --profile NAME [--role-chain ID]
                                             create a workflow, binding the Goal's digest, the profile, and
                                             (if given) the role chain's current head digest
                                   start|pause|resume|status --project ID --workflow ID
                                             advance or read status; status never appends anything
                                   stage  --project ID --workflow ID --stage NAME
                                             record the profile's next declared stage
                                   verify --project ID --workflow ID --goal PATH
                                             structural-only re-verification; --goal is re-read fresh, never
                                             a path recorded at create
                                   close  --project ID --workflow ID --outcome completed|abandoned [--reason TEXT]
                                             completed requires an immediately preceding successful verify;
                                             abandoned is legal from any non-closed state
                                   bind   --project ID --workflow ID
                                             bind this git repository (every worktree of it) to an open
                                             workflow; a closed or non-owned workflow is refused, and so is
                                             replacing a binding to a workflow that is still active or
                                             unreadable (unbind first)
                                   unbind    remove this repository's binding; idempotent
                                   binding   read-only: show this repository's binding and the bound
                                             workflow's status
                                   open   --feature F --task T [--goal PATH]
                                             open the workflow of a task the shaper marked: append the
                                             shaper and estimator records of the role chain F-t, then
                                             create, start, record the five pre-work stages and bind the
                                             workflow F-t (goal odd/goals/F.json unless --goal); refuses
                                             before writing an invalid goal, an unmarked task, a chain with
                                             records or an existing workflow (exit 2)
                                   exit 0 success, 2 refused/invalid (illegal transition, verify failure,
                                   non-owned state; for the binding verbs also: no git repository, a
                                   workflow that cannot be bound, a binding file that is not ours or cannot be used
                                   (foreign, malformed, unavailable), another bind or unbind in progress (busy),
                                   a binding another process changed meanwhile), 1 usage error
  validate-entry-contract          Validate a pre-SDD entry candidate with the version-matched
    --schema PATH --instance PATH  schema and deterministic semantic checks. Relative paths are
                                   resolved from the caller's working directory. Preserves validator
                                   exit codes 2-6 and does not modify installed skill roots.
  deterministic-checks             Run the hardcoded v1 check set (gofmt, go vet, staticcheck,
    <normalize|check> [--top-n N]  deadcode) via the overlay-owned runner. 'check' never mutates;
    [--out-dir DIR]                'normalize' applies fixers. Exit 0=passed, 1=verification_failed,
                                   3=procedural_tooling_failed; propagated unchanged from the runner.
  archive-reconcile                Detect completed SDD changes under openspec/changes/ (every
    [--repo PATH]                  tasks.md checkbox checked) that were never archived, and report
    [--known-gaps PATH]            whether their delta capabilities were promoted into
                                   openspec/specs/. A change whose completion signal the guard
                                   cannot read is reported as undetermined, never skipped.
                                   --known-gaps names a ledger of changes already known to fail;
                                   a listed change that stops failing is reported as a stale
                                   waiver, so the ledger cannot outlive the gap it records. The
                                   path is resolved from YOUR current directory, since the guard
                                   runs there -- pass it repository-relative.
                                   Exit 0=clean, 1=stranded change found, 2=usage, 3=could not
                                   determine (an unreadable signal, an unreadable ledger, or a
                                   stale waiver); propagated unchanged from the guard.
  gadu-generate [--check]          Regenerate the role agents (agents/<role>.md,
                                   pi/agents/<role>.md, codex/agents/<role>.toml) from
                                   roles/<role>/body.md under OVERLAY_DIR. The verb keeps the
                                   name it had when the same run also wrote the GADU operator.
                                   Optional --check verifies freshness without writing.
  longterm-mem install|status|uninstall
    [--target claude|opencode|codex|all] [--purge]
                                   Build (install only), copy to the fixed path
                                   $STATE_DIR/bin/longterm-mem, and report/record
                                   per-runtime (claude, opencode, codex) MCP
                                   registration via the engine runtime lifecycle
                                   surface (D4, R-014/R-015). status/uninstall never
                                   build. The binary is removed only when --target
                                   all leaves zero install-state targets, or --purge.
                                   The record of the installed targets is the
                                   engine's ('engine installed-targets', under
                                   $STATE_DIR/longterm-mem/installed-targets and a
                                   lock beside it): install and uninstall need the
                                   engine, built on first use like the rest.
                                   Exits: install 0=every named target registered,
                                   1=at least one named target was not registered.
                                   status 0=the engine reports a supported status,
                                   2=it reports a non-supported one (the report
                                   succeeded; what it reported is unhealthy -- never
                                   1, which would claim the report itself failed).
                                   uninstall 0=converged, 1=an unregister call failed
                                   so a target stays tracked as installed. Any
                                   subcommand exits 1 on a usage error or a missing
                                   prerequisite (die).
  longterm-mem sync-trigger --event session-end|archive --cwd DIR
                                   Forward to the engine's sync-trigger verb, which
                                   detaches a bounded longterm-mem sync for the given
                                   project directory and never blocks the caller. If
                                   the engine binary is missing or not executable,
                                   warns and skips instead of building it. Always
                                   exits 0 (R-003) -- safe to call from a SessionEnd
                                   hook or archive closure-feedback with no further
                                   guard needed.
  --help                           Show this help

Targets:
  claude   → ~/.claude/skills
  opencode → ~/.config/opencode/skills
  codex    → ~/.codex/skills

Hook activation notes:
  - 'labdrian apply' does NOT call install-hooks. Deploy of skills and hook
    wiring are intentionally separate so skills can be updated without
    mutating ~/.claude/settings.json.
  - Run 'labdrian install-hooks' once after initial setup (or after a binary
    update) to activate deterministic scoping.
  - status-hooks is now read-only and fails loudly when the engine binary is
    absent (run 'labdrian install-hooks').
EOF
}
