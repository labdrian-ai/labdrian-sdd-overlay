# Delta for Runtime Roster

## ADDED Requirements

### Requirement: Single Roster Declaration

ID: R-135
Traces to: labdrian-sdd-overlay R-135

The overlay MUST declare every runtime it knows about exactly once, in one
Go declaration, and each entry MUST carry a liveness of exactly `active` or
`dormant`. `active` means the overlay ships a lifecycle adapter for that
runtime. `dormant` means the runtime is known and deliberately unsupported.
No other liveness value is permitted, and a runtime the overlay does not
know MUST NOT appear in the roster at all.

#### Scenario: Liveness is a closed three-state domain

- GIVEN the roster declaration
- WHEN it is read
- THEN every entry's liveness is `active` or `dormant`
- AND a runtime absent from the roster is neither, and is treated as unknown

#### Scenario: Active and dormant are separately enumerable

- GIVEN the roster contains both active and dormant entries
- WHEN a caller asks for the active runtimes
- THEN it receives exactly the active names, in declaration order
- AND the dormant names are reachable through a separate accessor, never
  silently merged into the active list

#### Scenario: Removing a runtime from the roster fails the suite

- GIVEN an active runtime is deleted from the roster declaration
- WHEN the test suite runs
- THEN at least one test fails naming that runtime

### Requirement: Validation Surfaces Are Derived, Not Restated

ID: R-136
Traces to: labdrian-sdd-overlay R-136

The runtime-target domain accepted by `ParseTarget` and the skill-registry
target domain used by entry validation MUST both be derived from the roster
declaration rather than restated as independent literals. Every value of
`install.targets` across the skill registry MUST be an active roster name.

#### Scenario: A new active runtime widens both surfaces without a second edit

- GIVEN a new runtime is added to the roster as `active` with an adapter
- WHEN the runtime target parser and the registry entry validator run
- THEN both accept the new name
- AND neither required a separate literal list to be edited

#### Scenario: A registry entry naming a non-active runtime is refused

- GIVEN a skill registry entry whose `install.targets` names a dormant or
  unknown runtime
- WHEN the registry is validated
- THEN validation fails, the error names the offending entry and value, and
  the error text lists the active roster names

#### Scenario: The shipped registry is checked against the roster

- GIVEN the repository's own `skills.registry.yaml`
- WHEN the drift guard runs
- THEN every `install.targets` value in every entry is an active roster name

### Requirement: A Dormant Runtime Is Refused By Name, Not As An Unknown String

ID: R-137
Traces to: labdrian-sdd-overlay R-137

WHEN a caller names a dormant runtime as a lifecycle target, the overlay MUST
refuse it with an error that identifies it as dormant, distinct from the error
returned for a name the roster does not contain.

#### Scenario: Dormant and unknown produce different errors

- GIVEN the roster lists `pi` as dormant and does not list `future-cli`
- WHEN each is passed as a lifecycle target
- THEN both are refused
- AND the dormant refusal names the runtime and states that it is dormant
- AND the unknown refusal does not claim dormancy

#### Scenario: A dormant runtime has no target constant and no adapter

- GIVEN a dormant roster entry
- WHEN the drift guard inspects the runtime package
- THEN there is no accepted lifecycle target for that name
- AND there is no adapter constructor registered for that name

### Requirement: The Shared Contract Carries A Roster Block That Must Match The Code

ID: R-138
Traces to: labdrian-sdd-overlay R-138

`skills/_shared/review-ledger-contract.md` MUST carry a delimited,
machine-readable roster block listing every roster runtime with its liveness.
That block MUST equal the roster declaration as a set, in both directions,
name by name and liveness by liveness.

#### Scenario: A missing block is a failure, not a skip

- GIVEN the contract has no roster block
- WHEN the drift guard runs
- THEN it fails naming the contract path and the expected delimiters
- AND it does not pass by treating the absent block as vacuously correct

#### Scenario: A runtime present in code and absent from the block fails

- GIVEN a runtime is added to the roster declaration only
- WHEN the drift guard runs
- THEN it fails naming that runtime and the file that omits it

#### Scenario: A runtime present in the block and absent from code fails

- GIVEN a runtime is added to the contract's roster block only
- WHEN the drift guard runs
- THEN it fails naming that runtime and the declaration that omits it

#### Scenario: A liveness disagreement fails

- GIVEN a runtime is `dormant` in the roster declaration and `active` in the
  contract's roster block
- WHEN the drift guard runs
- THEN it fails naming the runtime and both liveness values

### Requirement: Shared Contracts Never Describe A Dormant Runtime As Working

ID: R-139
Traces to: labdrian-sdd-overlay R-139

Every markdown file under `skills/_shared/` MUST NOT contain a line naming a
dormant roster runtime unless that same line also contains the word `dormant`.
The guard MUST match dormant names as case-sensitive whole words. The guard's
failure message MUST name the file, the line number, the runtime, and the two
ways to satisfy it — reword the line, or change the runtime's liveness in the
roster.

#### Scenario: A support claim about a dormant runtime fails the guard

- GIVEN a `skills/_shared/` file contains a line asserting that a dormant
  runtime executes reviewers, or names it as a persistence surface
- WHEN the guard runs
- THEN it fails naming the file, the line number and the runtime

#### Scenario: A correct dormancy clause passes

- GIVEN a line reads "Kilo remains dormant because it has no equivalent
  native path"
- WHEN the guard runs
- THEN it passes, because the line names the runtime and the word `dormant`

#### Scenario: Promoting a runtime to active clears the guard for it

- GIVEN a dormant runtime is reclassified `active` in the roster and given an
  adapter
- WHEN the guard runs
- THEN lines naming it are no longer required to contain `dormant`

### Requirement: The Guard States Its Own Blind Spots

ID: R-140
Traces to: labdrian-sdd-overlay R-140

The prose guard of R-139 MUST be documented, in the guard's own source, as a
tripwire for known runtime names and not as a proof that the contract text is
correct. The documentation MUST name at least these three limits: it cannot
detect a support claim about a runtime absent from the roster; it cannot
detect a claim spanning more than one line; and a legitimate whole-word use
of a dormant runtime's name is a false positive.

#### Scenario: The stated limits are discoverable at the failure site

- GIVEN a reader encounters the guard for the first time
- WHEN they read its source
- THEN the three limits above are stated there
- AND the guard does not describe itself as verifying the contract's prose

#### Scenario: The blind spots are not closed by an allowlist

- GIVEN a legitimate whole-word use of a dormant runtime's name is needed
- WHEN the guard fails on it
- THEN the resolution is to reword the line or change the roster
- AND no per-file or per-phrase exemption list is introduced
