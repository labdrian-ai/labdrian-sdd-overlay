# Manifest — Final: Mission for labdrian-sdd-overlay

## Mission Document

### Agent mission
The agent’s job in this project is to keep the overlay safe, deterministic, and maintainable while preserving local customizations over time and across runtime targets.

### What counts as a good result
- Reproducible `overlay` operations (`status`, `sync-check`, `apply`, `capture`) across runtimes.
- Changes that reduce hidden drift and make behavior explicit in code, tests, or manifest.
- Incremental changes aligned to existing SDD flow and covered by verifiable tests.

### What counts as a bad result
- Editing upstream/vendor artifacts directly or bypassing manifest-driven merge/deploy semantics.
- Expanding scope by stacking unrelated changes without manifest or architecture alignment.
- Introducing governance rules that leak into non-SDD tasks and degrade developer ergonomics.

### Stage success criteria
By the end of this stage:
- The project has a finalized manifest (context, mission, rules) with binding operational expectations.
- Agents and reviewers can reproduce the project’s constraints from those documents.
- The next architecture/risk tradeoff discussion has a single source of truth.

### What is not success yet
- Delivering a feature without a manifest-constrained architecture context.
- Assuming existing implementation is sufficient without explicit, auditable rules.

### Operational priority
1. Finalize project manifest.
2. Derive stable architecture constraints.
3. Generate and maintain roadmap sequence from manifest + architecture.

### Expected outcome
A repository-level operating contract that standardizes how every future SDD change must be scoped, merged, validated, and reviewed.
