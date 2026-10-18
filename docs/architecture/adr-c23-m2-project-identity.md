# ADR: one shared project identity for the workflow, Engram and longterm-mem (C23, M2)

- **Status:** Accepted and implemented, 2026-10-18. The engine and longterm-mem both name projects
  with `identity/resolve` through `identity/gitfs`.
- **Decided by:** the owner, on 2026-10-18.
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M2 (gap G7). M1's record is
  `docs/architecture/adr-c23-m1-memory-sources.md`.
- **Implemented by:** batch 53 of phase 9.

## Context

Before M2 the same repository had up to three names, chosen by three different rules.

**The engine** named a project for the procedural skills with a chain of its own: `--project-id`,
then the origin remote normalized by `identity.NormalizeRemote`, then the basename of the current
directory (`engine/skills/projectidentity/projectidentity.go` at caa9148: `Explicit`, `GitOrigin`,
`DirectoryName`, `Chain`). It never read `.longterm-mem-project`.

**longterm-mem** names a project with `projectid.Resolver.Resolve`
(`longterm-mem/internal/projectid/projectid.go:117`): the declared file `.longterm-mem-project`, then
the normalized origin, then the realpath of the git common directory (`RuleCommonDir`, `:73-77` and
`:133`). Outside git it fails with `ErrNotARepository` (`:86-89`, from
`longterm-mem/internal/projectidfs/projectidfs.go:88`).

**The workflow** knew only its slug, the `--project` of `workflow create`, which keys its directory
(`workflows/<project_id>/`). The memory gate compared a longterm-mem query's `project` with a plan
resolved for that slug, so a workflow `proj-1` permitted only queries for `"proj-1"`, a name
longterm-mem never gives a repository.

**Engram** names a project by its own detection (`mem_current_project`), whose result for a git
remote is the last path segment of the repository.

## Decision

### The six decisions

1. **One resolver, in the shared module.** `identity/resolve` holds the rule and its port;
   `identity/gitfs` is the one disk adapter. The order is fixed, first match wins:
   `--project-id`, then `.longterm-mem-project` read only at the root of the main checkout, then the
   origin remote normalized by `identity.NormalizeRemote`, then the basename of the main checkout's
   root. The rule is `Resolver.Resolve` (`identity/resolve/resolve.go:118`); the declared file is
   `DeclaredFileName` (`:34`) read by `Declared` (`:149`); the remote by `Remote` (`:164`); the last
   fallback is `MainCheckoutName` (`:175`). The port is `Repositories` (`:79`) and its adapter
   `gitfs.Repositories` (`identity/gitfs/gitfs.go:35`, `:55`, `:92`, `:105`), which carries the walk-up,
   the `.git` pointer following and the HEAD check that used to be the engine's `GitOrigin`. The
   engine reaches it through `sharedProjectRules` (`engine/cmd/skills_deps.go:51`), for the skills
   (`newProjectIdentity`, `:44`) and for `workflow create` (`engine/cmd/workflow.go:233`).
2. **The slug stays the directory key; the identity is a field.** `workflows/<project_id>/` is
   unchanged and still checked by `ValidateIdentifier`. The created event gains an optional
   `project_identity` (`engine/workflow/workflow.go:207`, bounded by `MaxProjectIdentityLength` at
   `:93` and checked by `validateProjectIdentity` at `:571`), carried into `State.ProjectIdentity`
   (`engine/workflow/statemachine.go:68`, set at `:212`). The memory plan is resolved for the
   identity (`engine/projection/context.go:119`), so the context and the gate name the same project.
   A workflow created before M2 has no identity: its plan still names the slug, and the gate allows
   its queries with one warning (`engine/projection/gate.go:181`). No migration.
3. **Engram, for M4 only.** M2 gives `identity.EngramName` (`identity/engram.go:13`), the last path
   segment of an identity, with a table test. Nothing calls it yet; the gate does not check Engram.
4. **Outside git, the directory's name.** With no flag and no declared file, a directory that is in
   no repository is named by its basename (`identity/resolve/resolve.go:124`). longterm-mem is to
   work outside git instead of failing with `ErrNotARepository`.
5. **`workflow create` refuses a goal of another project.** When `goal.project_id` differs from the
   `--project` slug, create fails and appends nothing (`ErrGoalProjectMismatch`,
   `engine/workflow/lifecycle.go:31`, checked at `:476`); otherwise it records the identity (`:523`).
6. **A directory name is declared, not adopted** (the owner, 2026-10-18, answering whether a
   directory name the chain answered may be adopted on the identity ledger's word: "No; la
   solución es declararlo"). The main checkout's name, rule `directory`
   (`longterm-mem/internal/projectid/projectid.go:74`), stays loose on the identity ledger: it is
   recorded but never adopted on the ledger's word alone. A repository with no remote and no
   declaration that is renamed therefore starts a new identity; to keep its memory, the owner
   declares the old name in `.longterm-mem-project`. Pinned by
   `TestResolveProjectFlag_ARenamedRepositoryDoesNotAdoptItsOldDirectoryName`
   (`longterm-mem/cmd/longterm-mem/project_resolve_test.go:368`), which also shows the declaration
   restoring the old name; a repository moved without being renamed keeps its name
   (`TestResolveProjectFlag_AMovedRepositoryKeepsItsName`, `:339`).

### Alternatives considered

The owner rejected each of these. The last column states what the option would have meant given the
code as it stands; it is not a quotation of the owner's reasons.

| Decision | Rejected | What choosing it would have meant |
|---|---|---|
| 1 | Q8 for everyone, drop `.longterm-mem-project` | Every repository that declared a name would be renamed, and its promoted pages would stop matching queries. |
| 1 | Two chains plus a mapping in the workflow or the gate | Both rules would keep drifting, and the gate would need a translation table it cannot verify. |
| 2 | The canonical id encoded as the directory | `github.com/acme/widget` is no valid identifier, so the directory key would need an escaping scheme and every existing workflow would move. |
| 2 | Nested directories | `workflows/github.com/acme/widget/` would change the store's layout and every reader of it. |
| 3 | Ask Engram at gate time (`mem_current_project`) | The pure gate would call a process, and its answer depends on the server's own working directory. |
| 3 | Write the canonical name into Engram and consolidate | Existing Engram memory would be rewritten, an irreversible change to stored data. |
| 4 | Refuse outside git | A plain directory would get no identity at all, and longterm-mem would stay unusable there. |
| 4 | The cwd's name only in the engine | The two programs would disagree outside git, which is the gap M2 closes. |
| 5 | Warn and continue | A workflow whose goal names another project would be created, and its plan would describe the wrong memory. |
| 5 | No check | As before M2: the goal's `project_id` and the slug were never compared. |
| 6 | Adopt directory names from the ledger | A same-named repository (two checkouts called `api`, say) would silently take the other's memory. |
| 6 | Keep the common-dir realpath in longterm-mem for repositories with no remote | The two programs would name such a repository differently again, against decision 1. |

## Consequences

- **The engine's names change where the rule changed.** A repository with no origin is named by its
  main checkout's root, not by the current directory: below the root (`demo/sub`) the project is
  `demo`, not `sub`, and a linked worktree is named after the main checkout. A declared
  `.longterm-mem-project` at the main root now wins over the origin. A declared file that is not one
  non-empty line is refused with the `projectid:` error longterm-mem already gives. The identity
  golden of the engine pins each case (`engine/cmd/testdata/project-identity-golden/`).
- **The workflow log format changes one way.** A log written by an older engine replays unchanged,
  with no identity. A log written now carries `project_identity`, and an older engine refuses it,
  because it reads only the fields it knows.
- **`workflow create` warns, not refuses, when the identity cannot be resolved** (a broken `.git`
  pointer, a declared file that is not one line) **or cannot be recorded** (a name the log refuses,
  such as one with a control character or over 512 runes; `workflow.ValidateProjectIdentity`, checked
  in `engine/cmd/workflow.go` before `Create`). It records no identity, and the gate treats the
  workflow as one created before M2. This keeps create working in every layout it worked in before;
  decision 5 is the only new refusal. This is an implementation choice of batch 53, not an owner
  decision (accepted by the coordinator on 2026-10-18): the owner's decisions say what create
  records, not what it does when nothing can be resolved.
- **The gate stays total.** An unknown identity is an allow with one warning; scope none still
  denies every query.
- **M4** turns on the Engram project check of M1 decision 4. It compares the `project` an Engram
  read names with `identity.EngramName(state.ProjectIdentity)`, not with the identity itself, since
  Engram names a remote repository by its last segment. A workflow with no identity is an allow, as
  for longterm-mem.
- **longterm-mem resolves with the shared chain.** `projectid.Resolver.Resolve`
  (`longterm-mem/internal/projectid/projectid.go:127`) delegates to `identity/resolve`, and
  `projectidfs.Inspector` is an alias of `gitfs.Repositories`
  (`longterm-mem/internal/projectidfs/projectidfs.go:21`); its own walk-up and config read are gone.
  The old common-dir realpath (`RuleCommonDir`, `projectid.go:80`) is still offered for adoption,
  strict, after the chain's names (`derivableNames`, `longterm-mem/internal/projectid/adopt.go:170`),
  so memory stored under it stays reachable; the MCP server stays excluded from cwd adoption.
- **`stale` refuses outside git.** It checks memories against a repository's history, and a
  directory in no repository now has a project, so it looks for the repository first and refuses
  with exit 2 when there is none (`longterm-mem/cmd/longterm-mem/cmd_stale.go:42`).
- **A submodule is its own main checkout.** A `.git` file whose git directory has no `commondir`
  file is a submodule, so its main checkout is its own working tree; only a linked worktree takes
  its main checkout from the shared git directory (`commonDirOfPointer`,
  `identity/gitfs/gitfs.go:121`, used at `:68-73`). Before this a submodule with no origin was named
  `modules`.
- **The disk stays out of the pure module's users.** archguard judges `identity` as one pure module,
  but `identity/gitfs` reads the file system, so a guard fails when any longterm-mem package other
  than `internal/projectidfs` (and the composition root) imports it
  (`longterm-mem/projectidfs_adapter_guard_test.go:40`). archguard is unchanged.
- **Other visible changes.** `skills-stale` of a root in no repository with no `--project` now acts
  on the directory's name and reports the missing lock (exit 1) instead of refusing (exit 2), as
  decision 4 implies. `skills install` in a repository whose `.longterm-mem-project` is not one line
  now fails (exit 1) even with an origin, since the declared file comes first (decision 1). The
  identity ledger of a repository with a remote also records its directory name, as a loose entry
  that is never adopted (decision 6). The `longterm-mem` README and the package comment of
  `internal/identityledger` describe the shared chain.
- **Left open.** `engine/gitfs/locator.go` and `engine/gitprov/gitprov.go` still read `gitdir:` and
  `commondir` on their own, for the binding's repository key and for provenance, not for the
  project identity; this predates batch 53.
- **Accepted costs.** A repository that declares nothing and has no origin changes name in
  longterm-mem once, from the common dir's realpath to the directory's name (memory stored under
  the realpath stays reachable through adoption); after a rename it starts a new identity unless
  the old name is declared (decision 6). Workflows created before M2 are not checked by the gate
  until they are recreated.
