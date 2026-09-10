// labdrian-gate.ts is the deterministic-scoping contract gate for Pi
// (gentle-pi). It is shipped inside the built labdrian-pi package's
// extensions/ directory and discovered by Pi's jiti-based extension loader
// (never as plain .js -- see design.md A5). It mirrors engine/gate/gate.go
// and the OpenCode plugin (labdrian-runtime-parity-plugin.mjs): on
// before_agent_start for the sdd-tasks/sdd-apply agents only, it injects the
// bare contract PATH LINE for every applicable managed contract, under that
// contract's injection_point header, read from the contract's own
// frontmatter with a strict inline-list parse. It never throws -- any
// failure degrades to a pass-through {} response -- and it never overwrites
// another handler's systemPrompt contribution: it always starts from
// event.systemPrompt (docs/extensions.md:538-539,565 -- handlers chain).
//
// Written in type-annotation-free syntax (valid plain JS too) so it never
// needs a TS toolchain; the same bytes run directly under Node in tests
// (copied to a .mjs fixture) and under Pi's jiti loader in production.
//
// This file has zero TS-specific syntax by design -- do not add type
// annotations, interfaces, or enums here.

import { readFileSync } from "node:fs";
import { dirname, isAbsolute, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

// PHASE_NAMES is the exhaustive set of agents this gate ever touches. Every
// other agent name (or no name at all) is a strict no-op.
const PHASE_NAMES = ["sdd-tasks", "sdd-apply"];

// CONTRACT_RELATIVE_PATHS are package-root-relative paths to the managed
// contracts this build ships under skills/_shared/ (mirrors pipkg.go's
// gateContractFiles). Fixed and non-configurable: the gate never reads an
// externally supplied path.
const CONTRACT_RELATIVE_PATHS = [
  "skills/_shared/minimalism-contract.md",
  "skills/_shared/anti-generic-design.md",
];

const DEFAULT_INJECTION_HEADER = "## Skills to load before work";

function isRecord(value) {
  return typeof value === "object" && value !== null;
}

function readStringPath(value, path) {
  let current = value;
  for (const key of path) {
    if (!isRecord(current)) return undefined;
    current = current[key];
  }
  return typeof current === "string" ? current : undefined;
}

// readAgentStartNames mirrors gentle-pi's own before_agent_start name
// reading exactly (gentle-ai.ts readAgentStartNames), so this gate agrees
// with gentle-pi's own agent-identification surface rather than inventing a
// second one.
function readAgentStartNames(event) {
  return [
    readStringPath(event, ["agentName"]),
    readStringPath(event, ["agent"]),
    readStringPath(event, ["name"]),
    readStringPath(event, ["agent", "name"]),
    readStringPath(event, ["subagent", "name"]),
  ]
    .filter((value) => value !== undefined)
    .map((value) => value.trim())
    .filter((value) => value.length > 0);
}

function phaseFromNames(names) {
  return names.find((name) => PHASE_NAMES.includes(name));
}

// resolveContractPath joins root with relPath and returns the resolved
// absolute path ONLY when it stays contained within root (F1): an absolute
// relPath, or any ".."/"." traversal segment, or a resolved path outside
// root, is rejected by returning undefined -- never throwing.
export function resolveContractPath(root, relPath) {
  if (typeof relPath !== "string" || relPath.length === 0) return undefined;
  if (isAbsolute(relPath)) return undefined;
  const segments = relPath.split(/[\\/]/);
  if (segments.some((segment) => segment === ".." || segment === ".")) return undefined;
  const resolved = resolve(root, relPath);
  const rootWithSep = root.endsWith(sep) ? root : root + sep;
  if (resolved !== root && !resolved.startsWith(rootWithSep)) return undefined;
  return resolved;
}

// splitFrontmatterOnce mirrors Go's strings.SplitN(content, "---", 3): the
// first two "---" delimiters bound the frontmatter block; everything after
// the second delimiter (including any further "---" lines in the body)
// stays a single trailing part.
function splitFrontmatterOnce(content) {
  const parts = [];
  let rest = content;
  for (let i = 0; i < 2; i++) {
    const idx = rest.indexOf("---");
    if (idx === -1) break;
    parts.push(rest.slice(0, idx));
    rest = rest.slice(idx + 3);
  }
  parts.push(rest);
  return parts;
}

// parseStrictInlineList mirrors engine/gate/gate.go's parseStrictInlineList
// (gate.go:249-263): only a bracket-delimited "[a, b, c]" form parses;
// anything else is malformed (returns undefined, never an empty list
// standing in for "absent").
function parseStrictInlineList(raw) {
  const trimmed = raw.trim();
  if (!trimmed.startsWith("[") || !trimmed.endsWith("]")) return undefined;
  const inner = trimmed.slice(1, -1);
  const out = [];
  for (const item of inner.split(",")) {
    const cleaned = item.trim().replace(/^['"]|['"]$/g, "");
    if (cleaned.length > 0) out.push(cleaned);
  }
  return out;
}

// parseFrontmatter reads applies_to_phases/excluded_phases/injection_point
// from a contract's frontmatter with the strict parse above (mirrors
// gate.go:216-263 / design C4). Returns undefined (malformed -> no
// injection, R-004) when the frontmatter block is absent, applies_to_phases
// is absent/malformed, or excluded_phases is present but malformed.
export function parseFrontmatter(content) {
  const parts = splitFrontmatterOnce(content);
  if (parts.length < 3) return undefined;

  let appliesTo;
  let excluded = [];
  let injectionPoint = "";
  for (const rawLine of parts[1].split("\n")) {
    const line = rawLine.trim();
    if (line.startsWith("applies_to_phases:")) {
      appliesTo = parseStrictInlineList(line.slice("applies_to_phases:".length));
      if (appliesTo === undefined) return undefined;
    } else if (line.startsWith("excluded_phases:")) {
      excluded = parseStrictInlineList(line.slice("excluded_phases:".length));
      if (excluded === undefined) return undefined;
    } else if (line.startsWith("injection_point:")) {
      injectionPoint = line.slice("injection_point:".length).trim().replace(/^['"]|['"]$/g, "");
    }
  }
  if (!appliesTo || appliesTo.length === 0) return undefined;
  return { appliesTo, excluded, injectionPoint };
}

function hasExactLine(prompt, line) {
  return prompt.split("\n").some((candidate) => candidate.trim() === line);
}

// injectContractLine mirrors runtime.go's InjectPrompt/gate.go's inject
// exactly (same header-insertion and separator rules), so the same
// contract/header pair produces byte-identical output across Claude,
// OpenCode, and Pi.
export function injectContractLine(prompt, contractPath, injectionHeader) {
  if (hasExactLine(prompt, contractPath)) return prompt;
  if (hasExactLine(prompt, injectionHeader)) {
    const lines = prompt.split("\n");
    const out = [];
    for (const line of lines) {
      out.push(line);
      if (line.trim() === injectionHeader) out.push(contractPath);
    }
    return out.join("\n");
  }
  const separator = prompt.endsWith("\n") ? "\n" : "\n\n";
  return prompt + separator + injectionHeader + "\n" + contractPath + "\n";
}

function resolvePackageRoot() {
  // This file lives at "<packageRoot>/extensions/labdrian-gate.ts".
  return dirname(dirname(fileURLToPath(import.meta.url)));
}

// injectContractsForEvent applies every managed contract to event, returning
// {} (no-op) for any agent other than sdd-tasks/sdd-apply, and
// {systemPrompt} composed on top of event.systemPrompt otherwise. Never
// throws: a contract file that is missing, unreadable, path-uncontained, or
// carries malformed frontmatter is silently skipped (R-004/F1), never
// aborting the other contracts.
export function injectContractsForEvent(event, packageRoot) {
  const phase = phaseFromNames(readAgentStartNames(event));
  if (!phase) return {};

  const root = packageRoot ?? resolvePackageRoot();
  let prompt = typeof event?.systemPrompt === "string" ? event.systemPrompt : "";

  for (const relPath of CONTRACT_RELATIVE_PATHS) {
    const absPath = resolveContractPath(root, relPath);
    if (!absPath) continue;

    let raw;
    try {
      raw = readFileSync(absPath, "utf8");
    } catch {
      continue;
    }

    const meta = parseFrontmatter(raw);
    if (!meta) continue;
    if (meta.excluded.includes(phase)) continue;
    if (!meta.appliesTo.includes(phase)) continue;

    const header = meta.injectionPoint || DEFAULT_INJECTION_HEADER;
    prompt = injectContractLine(prompt, absPath, header);
  }

  return { systemPrompt: prompt };
}

export default function (pi) {
  pi.on("before_agent_start", async (event) => {
    try {
      return injectContractsForEvent(event);
    } catch {
      return {};
    }
  });
}
