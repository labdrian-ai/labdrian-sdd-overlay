package skills

// The registry files the end-to-end tests give to the built engine. They are written out as text,
// because this package cannot import the adapter that encodes a registry (the adapter imports this
// package); TestTheRegistryFixturesAreWhatTheEncoderWrites, in the external test package, decodes
// each and encodes it again and requires the same bytes, so a fixture cannot drift from the format
// the program writes.

// projectCLIRegistry is a minimal overlay registry that lists one skill and matches no candidate
// of the tests that read it: registering a project skill reads the registry for the identity check,
// and an unreadable registry is a refusal that fails closed, so those tests need a readable one.
const projectCLIRegistry = `version: "1"
skills:
  - id: unrelated-skill
    path: unrelated-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`

// installFixtureRegistry is the registry of the install fixture: a global skill that approve can
// target and a project skill admitted to the project p.
const installFixtureRegistry = `version: "1"
skills:
  - id: glob
    path: glob
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
  - id: proj
    path: proj
    source:
      type: custom
    install:
      defaultScope: project
      targets:
        - claude
      allowedProjects:
        - p
    lifecycle:
      updateStrategy: overlay-only
`
