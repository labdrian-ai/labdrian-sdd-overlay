# Manifest — Final: Working Rules for labdrian-sdd-overlay

## Mandatory Rules

1. **Separate domain, rules, and architecture.** Keep project intent, decision criteria, and implementation details explicit per document and do not treat code presence as complete specification.
2. **Do not invent missing definitions.** If behavior is not explicit in docs or code paths, keep it open and explicitly mark it for definition before implementation.
3. **Preserve reproducible merge semantics.** All overlay behavior must remain compatible with `upstream/main` branch separation and deterministic capture/apply workflows.
4. **No direct mutation of upstream/vendor artifacts.** Vendor-like baseline files (for example `bin/overlay`) must remain treated as external baseline; customizations belong to overlay layers and tracked manifests.
5. **Keep governance scoped.** Minimalism/scoping contracts and hook-driven constraints must apply only where explicitly intended; do not leak them into unrelated workflows.
6. **Treat `engine` as the trusted control-plane.** CLI commands and hook logic must be the source of enforcement truth; the TUI presents results and state.
7. **Validate both Go modules.** Any validation, vet, and build checks must execute for `engine` and `tui` in module context.
8. **Track skill/agent changes through manifest.** All skill or agent additions/removals must be represented in `overlay.manifest` and follow deployment validation paths.
9. **Prefer explicit errors and fail-loud behavior for overlay health.** Health checks should reject unsafe states clearly instead of silently continuing.
10. **Prioritize minimal surface change.** Use the smallest change that preserves behavior and safety, especially in shell and hook paths that affect global developer tooling.
