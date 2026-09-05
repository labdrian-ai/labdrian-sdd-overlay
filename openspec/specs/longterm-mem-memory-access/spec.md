# Longterm-Mem Memory Access Specification

## Purpose

Defines the read-only foundation of the `longterm-mem` component: its
standalone module boundary, direct read access to Engram's mid-term memory,
and per-project resolution and invocation of the long-term vault. Covers the
no-CLI-shelling and query-scoping constraints on the Engram read path,
vault-registry resolution (explicit override, per-project default, and
rejection when unconfigured), the canonical resolution of project identity
from a directory, and the vault query invoke/parse contract including its
not-provisioned failure mode.

## Requirements

### Requirement: Standalone Module Outside engine/

ID: R-001
Traces to: longterm-mem R-001

The longterm-mem component SHALL exist as a standalone module with its own
dependency manifest, located outside the `engine/` zero-dependency boundary.

#### Scenario: Component builds as an independent module

- GIVEN the labdrian-sdd-overlay repository
- WHEN longterm-mem is built
- THEN it compiles as an independent module under `longterm-mem/` capable of
  declaring third-party dependencies

#### Scenario: engine/'s zero-dependency gate stays green

- GIVEN longterm-mem's module manifest adds third-party dependencies
- WHEN `engine/skills`'s zero-fetch import-allowlist test runs
- THEN it still passes unmodified, and `engine/`'s own dependency manifest
  carries no third-party requirement

### Requirement: Read-Only Engram Connection

ID: R-002
Traces to: longterm-mem R-002

The longterm-mem component SHALL read Engram observations only via a
read-only connection to the Engram database, defaulting to Engram's standard
database location and overridable via configuration for tests.

#### Scenario: Default connection is read-only

- GIVEN no override is configured
- WHEN longterm-mem opens its Engram connection
- THEN it connects read-only to Engram's default database location and any
  write attempt on that connection fails

#### Scenario: Overridden connection stays read-only

- GIVEN a test fixture overrides the database path
- WHEN longterm-mem opens its Engram connection
- THEN it connects read-only to the overridden path instead

### Requirement: Mid-Term Query Scoping

ID: R-020
Traces to: longterm-mem R-020

WHEN mid-term observations are queried for project P, the longterm-mem
component SHALL return only observations where the record is not
soft-deleted and belongs to project P.

#### Scenario: Soft-deleted and other-project observations are excluded

- GIVEN a fixture set of active, soft-deleted, and other-project observations
- WHEN observations are queried for project P
- THEN only active observations belonging to P are returned

### Requirement: No CLI Shelling to Engram

ID: R-021
Traces to: longterm-mem R-021

The longterm-mem component SHALL NOT invoke Engram's `search` or `save` CLI
commands.

#### Scenario: No subprocess call to Engram's CLI

- GIVEN any longterm-mem read or write path that touches Engram
- WHEN it executes
- THEN no subprocess invocation of Engram's `search` or `save` command occurs

### Requirement: Vault Resolution From Configuration

ID: R-003
Traces to: longterm-mem R-003

WHEN a vault is resolved for project P and the vault-registry configuration
names a vault path for P, the longterm-mem component SHALL use that
configured path.

#### Scenario: Configured override is used

- GIVEN project `some-other-project` has a vault-registry entry pointing at
  a vault path
- WHEN its vault is resolved
- THEN longterm-mem uses that configured path

### Requirement: Default Vault for labdrian-sdd-overlay

ID: R-022
Traces to: longterm-mem R-022

WHERE project P is `labdrian-sdd-overlay` and no explicit override is
configured for P, the longterm-mem component SHALL resolve P's vault via a
pre-seeded default entry in the vault-registry configuration.

#### Scenario: Default resolves without a code-level constant

- GIVEN a vault-registry configuration with no override for
  `labdrian-sdd-overlay`
- WHEN its vault is resolved
- THEN longterm-mem resolves it via the pre-seeded default entry, and that
  entry is readable and editable like any other configuration row

### Requirement: Vault-Not-Configured Rejection

ID: R-023
Traces to: longterm-mem R-023

IF no vault is configured for project P and no default entry applies to P,
THEN the longterm-mem component SHALL reject the resolution with a
vault-not-configured error.

#### Scenario: Unconfigured, non-default project is rejected, not guessed

- GIVEN project `some-new-project` has no vault-registry entry and is not
  `labdrian-sdd-overlay`
- WHEN its vault is resolved
- THEN longterm-mem rejects the resolution with a vault-not-configured error
  rather than guessing a path

### Requirement: Vault Query Invoke and Parse

ID: R-004
Traces to: longterm-mem R-004

WHEN a long-term query is issued for project P, the longterm-mem component
SHALL invoke the vault's retrieval entrypoint inside P's resolved vault with
a top-N result count (overridable via configuration, default 5) and parse
the page address, absolute path, BM25 score, rerank score, and snippet from
its output.

#### Scenario: Default top-N and full field parse

- GIVEN project `labdrian-sdd-overlay` resolves to its provisioned vault
- WHEN a query is issued with no explicit top-N
- THEN longterm-mem invokes the vault's retrieval entrypoint with the
  default top-N and parses all five named fields from its output

#### Scenario: Explicit top-N override

- GIVEN an explicit top-N override
- WHEN a query is issued
- THEN longterm-mem passes that value instead of the default

### Requirement: Vault Query Not-Provisioned Handling

ID: R-024
Traces to: longterm-mem R-024

IF the vault's retrieval entrypoint exits with the not-provisioned status
code, THEN the longterm-mem component SHALL return a `not_provisioned`
status instead of treating it as an error.

#### Scenario: Never-indexed vault maps to not_provisioned, not a generic error

- GIVEN a resolved vault that has never been indexed
- WHEN it is queried
- THEN longterm-mem maps the resulting not-provisioned exit status to a
  `not_provisioned` result rather than a generic failure

### Requirement: Canonical Project Identity Resolution

ID: R-040
Traces to: longterm-mem R-040

WHEN the project identity of a directory is resolved, the longterm-mem
component SHALL apply a first-match-wins chain of a declared project file
read from the main checkout's working-tree root, then the normalized
`origin` remote URL, then the absolute, symlink-resolved git common
directory, and SHALL report which rule produced the identity.

#### Scenario: The same repository resolves identically from every worktree

- GIVEN a repository's main checkout and any number of linked worktrees
- WHEN each directory's project identity is resolved
- THEN every one of them yields the identical identity, independently at
  each of the three chain rules

#### Scenario: A declaration is a property of the repository, not of a worktree

- GIVEN the declared project file present at the main checkout's root but
  absent from a linked worktree, or present on a linked worktree's branch
  but absent from the main checkout's
- WHEN each directory's project identity is resolved
- THEN both directories still yield the identical identity, because the
  declaration is read from the main checkout's root only -- a declaration
  visible from one worktree and not another would itself be the
  fragmentation this requirement exists to prevent

#### Scenario: A repository whose `.git` is a symlink still has an identity

- GIVEN a working tree whose `.git` entry is a symlink to the real git
  directory rather than the directory itself
- WHEN its project identity is resolved
- THEN the repository resolves normally, including its declared file, rather
  than being reported as not a repository

#### Scenario: The git common directory never keeps git's own relative answer

- GIVEN `git rev-parse --git-common-dir` answers relatively from a main
  checkout and absolutely from a linked worktree
- WHEN the common-directory rule produces an identity
- THEN that identity is absolute, free of unresolved parent segments, and
  symlink-resolved, so a symlinked worktree path or a symlinked parent
  directory resolves to the same identity as the real path

#### Scenario: Remote spellings of one repository collapse

- GIVEN the `origin` URL spelled as `git@host:owner/name.git`,
  `https://host/owner/name`, or `https://host/owner/name.git`
- WHEN the remote rule produces an identity
- THEN all spellings yield the identical `host/owner/name` value

#### Scenario: A declared name wins, and garbage in it is rejected

- GIVEN a repository carrying a declared project file
- WHEN its identity is resolved
- THEN the declared name is used in preference to the remote and the common
  directory, and an empty or multi-line declaration is rejected with a
  declared-invalid error rather than silently falling through

#### Scenario: Distinct repositories never collide

- GIVEN two different repositories
- WHEN each one's identity is resolved by the same chain rule
- THEN the two identities differ

#### Scenario: A non-repository directory is a named failure

- GIVEN a directory that is not inside a git repository
- WHEN its project identity is resolved
- THEN resolution fails with a not-a-repository error and produces no
  project name, empty or otherwise

### Requirement: Working-Directory Project Default

ID: R-041
Traces to: longterm-mem R-041

WHERE a CLI subcommand accepts `--project` and the flag is omitted, the
longterm-mem component SHALL resolve the project from the working directory
via the canonical identity chain, and IF that resolution fails THEN it
SHALL refuse with the existing `--project is required` rejection, naming
why resolution failed.

#### Scenario: An omitted flag resolves from the working directory

- GIVEN the working directory is inside a repository with a resolvable
  identity
- WHEN a subcommand that takes `--project` runs without the flag
- THEN it acts on the resolved project instead of refusing

#### Scenario: An unresolvable directory still refuses

- GIVEN the working directory is not inside a git repository
- WHEN a subcommand that takes `--project` runs without the flag
- THEN it refuses with the `--project is required` rejection, names the
  resolution failure, and passes no project name onward

### Requirement: Project Correspondence Warning

ID: R-042
Traces to: longterm-mem R-042

WHERE `--project` is given explicitly and the working directory is inside a
git repository whose canonical identity does not correspond to it, the
longterm-mem component SHALL report the mismatch and SHALL still act on the
project the operator named.

#### Scenario: A mismatch is reported and the command proceeds

- GIVEN a working directory whose canonical identity is project A
- WHEN a subcommand runs with `--project B`
- THEN it emits a warning naming both B and A, and acts on B

#### Scenario: A corresponding project is silent

- GIVEN a working directory whose canonical identity corresponds to the
  given project, including by the repository's own directory name when the
  identity was derived from a path or a remote
- WHEN a subcommand runs with that `--project`
- THEN no correspondence warning is emitted

### Requirement: MCP Project Field Is Not Working-Directory Derived

ID: R-043
Traces to: longterm-mem R-043

The longterm-mem component SHALL NOT default or validate an MCP tool call's
`project` field against the MCP server's working directory.

#### Scenario: The MCP tool contract is unchanged by working-directory resolution

- GIVEN the MCP server is launched by a host runtime whose working
  directory is unrelated to the project being asked about
- WHEN a `query` or `promote` tool call is handled
- THEN its explicit `project` field is used as given, with no
  working-directory default and no working-directory correspondence check

### Requirement: Established Identity Adoption

ID: R-044
Traces to: longterm-mem R-044

WHERE the project is resolved from the working directory, the longterm-mem
component SHALL use the highest-ranked derivable name that already holds
memory, and SHALL fall back to the identity chain's own answer only when no
derivable name holds any. It SHALL consider only names the repository
itself derives, never names selected by resemblance to one.

#### Scenario: An established derivable name is adopted rather than re-minted

- GIVEN a repository whose `origin` normalizes to `host/owner/name` and
  whose memory already lives under the plain `name`
- WHEN the project is resolved from the working directory
- THEN `name` is used, and the adoption is reported, rather than a second
  identity being created beside the memory that already exists

#### Scenario: Nothing established leaves the chain in charge

- GIVEN a repository none of whose derivable names holds any memory
- WHEN the project is resolved from the working directory
- THEN the identity chain's own first-match-wins answer is used unchanged

#### Scenario: Adoption preserves the identical-across-worktrees property

- GIVEN a repository's main checkout and any number of linked worktrees
- WHEN the project is resolved from each of them
- THEN every one of them adopts the identical name

#### Scenario: A store that cannot be consulted is reported, never read as empty

- GIVEN the vault registry or Engram's database cannot be read
- WHEN the project is resolved from the working directory
- THEN the command reports that an already-established identity may have
  been missed, rather than proceeding as though the repository were new

### Requirement: Derivable Fragments Are Reported For Integration

ID: R-045
Traces to: longterm-mem R-045

WHERE more than one derivable name of a single repository holds memory, the
longterm-mem component SHALL report every name other than the adopted one
as owed an integration into it, and SHALL NOT perform the merge itself.

#### Scenario: Every other established derivable name is named, with its remedy

- GIVEN a repository whose declared name and whose remote-derived name both
  hold memory
- WHEN the project is resolved from the working directory
- THEN the declared name is adopted and the other is reported as owed an
  integration into it, together with the command that performs the merge

#### Scenario: The merge stays with the store that owns it

- GIVEN derivable fragments have been reported
- WHEN longterm-mem completes the command
- THEN Engram's database is not written, its read-only connection contract
  being unchanged
