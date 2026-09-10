# Delta for Runtime Lifecycle

## MODIFIED Requirements

### Requirement: Claude Lifecycle Support

The system MUST support Claude runtime `status`, `install`, `update`, and
`uninstall` through the runtime lifecycle command surface, including the
`SessionEnd` sync-trigger hook family alongside existing owned hook state.
(Previously: covered only status/install/update/uninstall without naming the
`SessionEnd` family.)

#### Scenario: Claude install succeeds

- GIVEN Claude settings are writable
- WHEN the user runs runtime install for target `claude`
- THEN the command succeeds
- AND Claude status becomes provably healthy as `supported`

#### Scenario: Claude status is honest

- GIVEN Claude lifecycle state cannot be proven installed and healthy
- WHEN the user runs runtime status for target `claude`
- THEN the command MUST NOT report healthy `supported`

#### Scenario: Claude update refreshes lifecycle state

- GIVEN Claude is already installed by Labdrian
- WHEN the user runs runtime update for target `claude`
- THEN the command succeeds
- AND Claude remains provably healthy as `supported`

#### Scenario: Claude uninstall removes owned lifecycle state

- GIVEN Claude has Labdrian-owned lifecycle entries installed
- WHEN the user runs runtime uninstall for target `claude`
- THEN the command succeeds
- AND subsequent Claude status is not healthy `supported`

#### Scenario: Claude install includes the SessionEnd sync-trigger family

- GIVEN Claude settings are writable
- WHEN the user runs runtime install for target `claude`
- THEN the `SessionEnd` sync-trigger hook entry is installed
- AND Claude status reports it as part of owned lifecycle state

#### Scenario: Claude status reports the SessionEnd family honestly

- GIVEN the `SessionEnd` sync-trigger entry may or may not be installed
- WHEN the user runs runtime status for target `claude`
- THEN status reflects the entry's actual installed/missing state rather
  than assuming it from other hook families

#### Scenario: Claude uninstall removes the SessionEnd sync-trigger entry

- GIVEN the `SessionEnd` sync-trigger entry is installed alongside
  pre-existing `SessionEnd`/`Stop` entries owned by other tools
- WHEN the user runs runtime uninstall for target `claude`
- THEN only the owned `SessionEnd` sync-trigger entry is removed
- AND entries owned by other tools remain intact
