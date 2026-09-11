# Research: pi-runtime-target

schema: gentle-ai.sdd-research/v1 | revision: 2 | outcome: done | Engram: `sdd/pi-runtime-target/research` (obs #3336)

## Questions

Pi package format; extension API and system-prompt mutation; MCP config precedence and package-shipped MCP; skills format and pi-claude-bridge forwarding; `pi -ns` semantics.

## Sources

| id | class | title | publisher | URL / path | accessed |
|----|-------|-------|-----------|------------|----------|
| S1 | documentation | Extensions | Pi docs mirror | https://hochej.github.io/pi-mono/coding-agent/extensions/ | 2026-09-10 |
| S2 | documentation | Pi Packages | Pi docs mirror | https://hochej.github.io/pi-mono/coding-agent/packages/ | 2026-09-10 |
| S3 | documentation | Skills | Pi docs mirror | https://hochej.github.io/pi-mono/coding-agent/skills/ | 2026-09-10 |
| S4 | documentation | docs/pi.md | Gentleman-Programming/gentle-ai | https://github.com/Gentleman-Programming/gentle-ai/blob/main/docs/pi.md | 2026-09-10 |
| S5 | documentation | README | elidickinson/pi-claude-bridge | https://github.com/elidickinson/pi-claude-bridge/blob/main/README.md | 2026-09-10 |
| S6 | source code | gentle-ai.ts | gentle-pi (installed) | ~/.pi/agent/npm/node_modules/gentle-pi/extensions/gentle-ai.ts | 2026-09-10 |
| S7 | source code + README | pi-mcp-adapter 2.32.1 | nicobailon (installed) | ~/.pi/agent/npm/node_modules/pi-mcp-adapter/README.md | 2026-09-10 |
| S8 | source code | internal/agents/pi/adapter.go | gentle-ai 2.7.0 (local clone) | EffectiveCodeGraphMCPPath | 2026-09-10 |
| S9 | documentation | packages.md, extensions.md, skills.md, usage.md | @earendil-works/pi-coding-agent (installed) | ~/.npm-global/lib/node_modules/@earendil-works/pi-coding-agent/docs/ | 2026-09-10 |

## Validated claims

- C1 A package declares extensions/skills/prompts/themes via the `pi` key in package.json or conventional directories; installs from `npm:`, `git:` or a local path; local paths are referenced in place. [S2]
- C2 Skills are `SKILL.md` directories with `name` (matches dir) and `description`; discovered from global, project, package `skills/` and `--skill`. [S3]
- C3 `tool_call` can observe and block a tool call; it cannot mutate the input. [S1]
- C4 `before_agent_start` receives the agent name(s) and `event.systemPrompt` and may return `{ systemPrompt }`; gentle-pi uses it to inject into `sdd-*` agents. A per-phase contract gate for `sdd-tasks`/`sdd-apply` is deterministic in Pi through this hook. [S6]
- C5 MCP precedence (later wins): `~/.config/mcp/mcp.json`, `~/.agents/mcp.json`, `~/.agents/mcp/mcp.json`, `<pi agent dir>/mcp.json`, `.mcp.json`, `.pi/mcp.json`. A package may ship MCP servers by declaring `"pi": {"mcp": "./mcp.json"}` in package.json, a package-relative PATH (or array of paths) to a file with the normal top-level `mcpServers` object; names are prefixed with the sanitized package name; user/project config has higher precedence. [S7, S8]
- C6 pi-claude-bridge forwards Pi skills into the Claude backend prompt (`appendSkills`, default true). [S5]
- C7 `--no-extensions` disables extension discovery and `--no-skills` disables skill discovery; neither has an `-ns` alias, and there is no dedicated detection API. gentle-ai's docs/pi.md wording about `pi -ns` was inaccurate against the installed Pi. [S9; corrects S4]
- C8 Packages are removed with `pi remove <source>` where source is the install string (the local path for a path package); install and remove write the user settings packages list. [S9]
- C9 An extension is a module exporting `default function (pi) { pi.on("before_agent_start", handler) }`; `before_agent_start` handlers chain, each receiving the accumulated `event.systemPrompt`; documented discovery examples are `.ts` files loaded through jiti. [S9, S6]

## Contradictions and uncertainty

The research question assumed prompt mutation through `tool_call`; S1 contradicts it and S6 resolves it with `before_agent_start`. Revision 2 corrects three claims the design-gate validator caught against the installed Pi docs (S9): the `pi.mcp` shape, the `pi remove` verb, and the `--no-extensions` flag. The `@fractaal` fork README could not be fetched (HTTP 403); its behaviour is inferred from the upstream project and the installed sources. The docs mirror may lag Pi 0.85.x; installed sources are authoritative for this machine.

## Product choices (non-authoritative)

Gate through a `before_agent_start` extension composed with gentle-pi's handler; longterm-mem shipped as package MCP (`pi.mcp`) or registered in a Pi-readable config file; the package installed from a local path during development.
