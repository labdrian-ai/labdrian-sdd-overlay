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

import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, isAbsolute, join, resolve, sep } from "node:path";
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

// Shaper clearance deny guard. Mirrors engine/shaper/guard.go exactly
// (GuardCommandMarker, GuardStoreMarker, GuardMatches): a bash tool call
// that names the clearance record entry point or the clearance store path is
// blocked, so the model cannot record a clearance for the human. It matches
// command and path text only, so it is a speed bump, not a security boundary: any
// process running as the same OS user, including any installed Pi
// extension, can still forge a clearance record, and a clearance is not a
// signature. Covered: bash input.command, and write/edit input.path inside
// the store (like the Claude Write|Edit guard). Other tools, and relative
// paths that do not spell the store segment, are not inspected.
const SHAPER_GUARD_COMMAND_MARKER = "shaper clearance record";
const SHAPER_GUARD_STORE_MARKER = "labdrian/shaper-clearance";
const SHAPER_GUARD_REASON =
  "labdrian shaper clearance guard: recording a clearance or touching the clearance store is reserved for the human " +
  "through the Pi /shaper-clear dialog; the model must not run it. This guard matches command and path text only, so it is a speed bump, " +
  "not a security boundary: any process running as the same OS user can still forge a clearance record, and a clearance is not a signature.";

// matchesShaperClearanceGuard reports whether text names the clearance
// record entry point (after collapsing whitespace and shell line
// continuations) or the clearance store path.
export function matchesShaperClearanceGuard(text) {
  if (typeof text !== "string") return false;
  const normalized = text.split("\\\n").join(" ").split(/\s+/).filter((part) => part.length > 0).join(" ");
  return normalized.includes(SHAPER_GUARD_COMMAND_MARKER) || text.includes(SHAPER_GUARD_STORE_MARKER);
}

// shaperClearanceToolCall is the tool_call handler: {block, reason} for a
// guarded bash call or a write/edit whose path is inside the clearance
// store, undefined otherwise.
export function shaperClearanceToolCall(event) {
  if (!isRecord(event)) return undefined;
  if (event.toolName === "bash") {
    const command = readStringPath(event, ["input", "command"]);
    if (command === undefined || !matchesShaperClearanceGuard(command)) return undefined;
    return { block: true, reason: SHAPER_GUARD_REASON };
  }
  if (event.toolName === "write" || event.toolName === "edit") {
    const path = readStringPath(event, ["input", "path"]);
    if (path === undefined || !path.includes(SHAPER_GUARD_STORE_MARKER)) return undefined;
    return { block: true, reason: SHAPER_GUARD_REASON };
  }
  return undefined;
}

// Shaper clearance dialog: the /shaper-clear command (P3-S6). It is the
// only host channel that records a clearance, and it is deliberately narrow:
//
//   - It runs only in Pi's TUI (ctx.mode === "tui"). In RPC mode ctx.hasUI is
//     true too, but whatever drives stdin answers the dialogs, not a human,
//     and gentle-pi agent children run that way; they are also refused by
//     GENTLE_PI_AGENTS_CHILD=1. print/json have no dialogs at all.
//   - It captures the session identity (the sessionManager object, its
//     session id and file) before any dialog and refuses when it changed
//     afterwards, the pattern gentle-pi's foreign-target grants use.
//   - It shows the Go-rendered view bytes from 'gentle-ai-overlay shaper
//     assess --view' verbatim, after checking their SHA-256 against the
//     view_sha256 that 'shaper assess' reports, and never re-renders them.
//     Go refuses a view holding a terminal control or invisible formatting
//     rune (view_unpresentable); the gate aborts on that refusal or on any
//     exit that carries no view, and repeats the same rune check itself
//     before any dialog, so such bytes never reach the TUI.
//   - It collects a non-blank reason and evidence for every flag, shows the
//     trust-limit disclosure, and then takes an explicit Decline or Affirm.
//     Dialog titles and labels are fixed text plus the Go-generated flag id
//     and kind only, checked against Go's identifier shape; model-authored
//     text (flag items, project_id, goal_id) never becomes a title or label.
//   - It pipes the record JSON to 'gentle-ai-overlay shaper clearance record
//     --stdin' over stdin (never argv), which re-derives and checks every
//     digest, and it surfaces that command's exit status.
//
// It never claims the human's identity: the record says only that the pi
// TUI channel captured the decision, with verified=false. The command
// handler cannot tell who invoked it: any installed Pi extension can dispatch
// /shaper-clear and inject or consume keystrokes before a dialog sees them,
// and any process running as the same OS user can write a record directly.
// This is not a signature.

// SHAPER_FORGERY_DISCLOSURE mirrors engine/shaper.ForgeryDisclosure exactly.
export const SHAPER_FORGERY_DISCLOSURE =
  "Trust limit: this clearance is bound to exact content by SHA-256 digests only; it is not a signature. " +
  "Any process running as the same OS user, including any installed Pi extension, can forge a clearance record, " +
  "and the Claude Code and Pi deny guards are speed bumps, not a security boundary. " +
  "Readiness grants no execution authority, and the roadmap's full Phase 3 plan outcome is not yet met.";

const SHAPER_CLEAR_USAGE = "usage: /shaper-clear --handoff <rel> --goal <rel> [--root <abs>] (root defaults to the session cwd)";
const SHAPER_DECISION_OPTIONS = ["Decline", "Affirm"];
const SHAPER_VIEW_UNPRESENTABLE = "view_unpresentable";
// Go-generated flag identifiers (engine/shaper readiness.go): the kind is a
// FlagKind constant and the id is "<kind>:<field>:<1-based index>".
const SHAPER_FLAG_KIND_PATTERN = /^[a-z][a-z0-9_]*$/;
const SHAPER_FLAG_ID_PATTERN = /^[a-z][a-z0-9_]*:[a-z][a-z0-9_]*:[1-9][0-9]*$/;
const UNICODE_FORMAT_CONTROL = /^\p{Cf}$/u;

// isUnpresentableCodePoint mirrors engine/shaper.unpresentableRune: a C0
// control other than LF and TAB, DEL, a C1 control, or a Unicode format
// control (category Cf: bidi controls, zero-width characters).
export function isUnpresentableCodePoint(cp) {
  if (cp === 0x0a || cp === 0x09) return false;
  if (cp < 0x20 || cp === 0x7f || (cp >= 0x80 && cp <= 0x9f)) return true;
  return UNICODE_FORMAT_CONTROL.test(String.fromCodePoint(cp));
}

// firstUnpresentable returns {offset, codePoint} for the first unpresentable
// code point in text (offset in code points), or undefined.
export function firstUnpresentable(text) {
  let offset = 0;
  for (const ch of text) {
    const cp = ch.codePointAt(0);
    if (isUnpresentableCodePoint(cp)) return { offset, codePoint: cp };
    offset++;
  }
  return undefined;
}

function formatCodePoint(cp) {
  return `U+${cp.toString(16).toUpperCase().padStart(4, "0")}`;
}

// escapeUnpresentable replaces every unpresentable code point with its
// visible U+XXXX name, so a notification quoting subprocess output can never
// carry a terminal control or invisible formatting rune.
function escapeUnpresentable(text) {
  let out = "";
  for (const ch of String(text)) {
    const cp = ch.codePointAt(0);
    out += isUnpresentableCodePoint(cp) ? `<${formatCodePoint(cp)}>` : ch;
  }
  return out;
}

// resolveOverlayBinary returns the gentle-ai-overlay path the engine installs
// and its status check reads (engine/cmd/main.go: $HOME/.claude/bin/
// gentle-ai-overlay), resolving home as $HOME, then os.homedir().
export function resolveOverlayBinary(env) {
  const home = typeof env?.HOME === "string" && env.HOME.length > 0 ? env.HOME : homedir();
  return join(home, ".claude", "bin", "gentle-ai-overlay");
}

// parseShaperClearArgs parses the /shaper-clear argument string. It fails
// loud on an unknown token, a flag without a value, a missing --handoff or
// --goal, or a non-absolute root.
export function parseShaperClearArgs(args, cwd) {
  const tokens = typeof args === "string" ? args.trim().split(/\s+/).filter((t) => t.length > 0) : [];
  const out = { root: cwd, handoff: "", goal: "" };
  for (let i = 0; i < tokens.length; i++) {
    const flag = tokens[i];
    if (flag !== "--root" && flag !== "--handoff" && flag !== "--goal") {
      return { error: `unexpected argument ${JSON.stringify(flag)}; ${SHAPER_CLEAR_USAGE}` };
    }
    if (i + 1 >= tokens.length) return { error: `${flag} requires a value; ${SHAPER_CLEAR_USAGE}` };
    out[flag.slice(2)] = tokens[++i];
  }
  if (!out.handoff) return { error: `--handoff is required; ${SHAPER_CLEAR_USAGE}` };
  if (!out.goal) return { error: `--goal is required; ${SHAPER_CLEAR_USAGE}` };
  if (typeof out.root !== "string" || !isAbsolute(out.root)) {
    return { error: `the worktree root must be an absolute path, got ${JSON.stringify(out.root)}` };
  }
  return out;
}

// runOverlay spawns the binary with an argv array (no shell), optionally
// writes stdinText, and resolves {code, stdout (Buffer), stderr, error}.
function runOverlay(binary, argv, stdinText) {
  return new Promise((done) => {
    let child;
    try {
      child = spawn(binary, argv, { stdio: [stdinText === undefined ? "ignore" : "pipe", "pipe", "pipe"], shell: false });
    } catch (err) {
      done({ code: null, stdout: Buffer.alloc(0), stderr: "", error: String(err?.message ?? err) });
      return;
    }
    const out = [];
    const errOut = [];
    let spawnError;
    child.stdout.on("data", (chunk) => out.push(chunk));
    child.stderr.on("data", (chunk) => errOut.push(chunk));
    child.on("error", (err) => {
      spawnError = String(err?.message ?? err);
    });
    child.on("close", (code) => {
      done({ code, stdout: Buffer.concat(out), stderr: Buffer.concat(errOut).toString("utf8"), error: spawnError });
    });
    if (stdinText !== undefined) {
      child.stdin.on("error", () => {});
      child.stdin.end(stdinText, "utf8");
    }
  });
}

function captureSession(ctx) {
  try {
    const manager = ctx?.sessionManager;
    if (!isRecord(manager) || typeof manager.getSessionId !== "function") return undefined;
    const id = manager.getSessionId();
    if (typeof id !== "string" || id.length === 0) return undefined;
    const file = typeof manager.getSessionFile === "function" ? manager.getSessionFile() : undefined;
    return { manager, id, file };
  } catch {
    return undefined;
  }
}

function sameSession(ctx, before) {
  const now = captureSession(ctx);
  return now !== undefined && now.manager === before.manager && now.id === before.id && now.file === before.file;
}

function tell(ctx, message, type) {
  try {
    if (typeof ctx?.ui?.notify === "function") ctx.ui.notify(message, type);
  } catch {
    // Notification is best effort; the returned result carries the outcome.
  }
}

function describeExit(res) {
  return res.error ? `could not run (${res.error})` : `exit ${res.code}`;
}

// assessOk accepts the assess exit codes that carry a derived subject:
// 0 (ready) and 3 (draft). 2 (invalid) and anything else is refused.
function assessOk(res) {
  return !res.error && (res.code === 0 || res.code === 3);
}

function isBlank(value) {
  return typeof value !== "string" || value.trim().length === 0;
}

// runShaperClear is the /shaper-clear handler. It returns {status, message,
// exitCode?} with status refused, aborted, failed, or recorded, and notifies
// the same message. Only "recorded" means a record command exited 0.
export async function runShaperClear(args, ctx) {
  const finish = (status, rawMessage, type, exitCode) => {
    const message = escapeUnpresentable(rawMessage);
    tell(ctx, message, type);
    return exitCode === undefined ? { status, message } : { status, message, exitCode };
  };
  const refuse = (message) => finish("refused", `shaper clearance refused, nothing shown or recorded: ${message}`, "error");
  const abort = (message) => finish("aborted", `shaper clearance aborted, nothing recorded: ${message}`, "warning");

  if (process.env.GENTLE_PI_AGENTS_CHILD === "1") {
    return refuse("inside a gentle-pi agent child (GENTLE_PI_AGENTS_CHILD=1) no human answers the dialog");
  }
  if (ctx?.mode !== "tui" || ctx?.hasUI !== true) {
    return refuse(`requires the interactive Pi tui; mode ${JSON.stringify(ctx?.mode)} dialogs are answered by a process, not a human`);
  }
  const session = captureSession(ctx);
  if (!session) return refuse("no session identity is available to bind the dialog to");
  const opts = parseShaperClearArgs(args, ctx.cwd);
  if (opts.error) return refuse(opts.error);

  const binary = resolveOverlayBinary(process.env);
  try {
    if (!statSync(binary).isFile()) return refuse(`gentle-ai-overlay at ${binary} is not a regular file`);
  } catch {
    return refuse(`gentle-ai-overlay is not installed at ${binary}`);
  }
  const source = ["--root", opts.root, "--handoff", opts.handoff, "--goal", opts.goal];

  const assessed = await runOverlay(binary, ["shaper", "assess", ...source]);
  if (!assessOk(assessed)) {
    return refuse(`gentle-ai-overlay shaper assess: ${describeExit(assessed)}\n${assessed.stderr}${assessed.stdout.toString("utf8")}`);
  }
  let assessment;
  try {
    assessment = JSON.parse(assessed.stdout.toString("utf8"));
  } catch (err) {
    return refuse(`gentle-ai-overlay shaper assess printed invalid JSON: ${err?.message ?? err}`);
  }
  const subject = assessment?.subject;
  if (!isRecord(subject)) {
    const blockers = Array.isArray(assessment?.blockers) ? assessment.blockers : [];
    const unpresentable = blockers.find((b) => b?.reason === SHAPER_VIEW_UNPRESENTABLE);
    if (unpresentable) {
      return refuse(`the view cannot be shown safely (${SHAPER_VIEW_UNPRESENTABLE}): ${unpresentable.detail}`);
    }
    return refuse(`no clearance subject can be derived (state ${assessment?.state})`);
  }
  const flags = Array.isArray(assessment.flags) ? assessment.flags : [];
  for (const flag of flags) {
    const kind = flag?.kind;
    const id = flag?.id;
    if (typeof kind !== "string" || !SHAPER_FLAG_KIND_PATTERN.test(kind) || typeof id !== "string" || !SHAPER_FLAG_ID_PATTERN.test(id) || !id.startsWith(`${kind}:`)) {
      return refuse(`a flag id or kind is not a Go-generated identifier (id ${JSON.stringify(id)}, kind ${JSON.stringify(kind)})`);
    }
  }

  const viewed = await runOverlay(binary, ["shaper", "assess", ...source, "--view"]);
  if (viewed.stderr.includes(SHAPER_VIEW_UNPRESENTABLE)) {
    return refuse(`gentle-ai-overlay shaper assess --view refused the view as unsafe to show (${SHAPER_VIEW_UNPRESENTABLE}): ${describeExit(viewed)}\n${viewed.stderr}`);
  }
  if (!assessOk(viewed) || viewed.stdout.length === 0) {
    return refuse(`gentle-ai-overlay shaper assess --view printed no view: ${describeExit(viewed)}\n${viewed.stderr}`);
  }
  const viewDigest = createHash("sha256").update(viewed.stdout).digest("hex");
  if (viewDigest !== subject.view_sha256) {
    return refuse(`the view bytes (sha256 ${viewDigest}) do not match the assessed view_sha256 ${subject.view_sha256}; the sources changed between reads`);
  }
  const viewText = viewed.stdout.toString("utf8");
  if (!Buffer.from(viewText, "utf8").equals(viewed.stdout)) {
    return refuse("the view bytes are not valid UTF-8 and cannot be displayed verbatim");
  }
  const hidden = firstUnpresentable(viewText);
  if (hidden) {
    return refuse(`the view holds ${formatCodePoint(hidden.codePoint)} at code point offset ${hidden.offset}, a terminal control or invisible formatting character; it is unpresentable and is refused, not rewritten`);
  }

  const shown = await ctx.ui.editor("Shaper clearance view: verbatim gentle-ai-overlay output (edits are ignored; close to continue)", viewText);
  if (shown === undefined) return abort("the view was closed without continuing");

  const resolutions = [];
  for (const flag of flags) {
    const label = `Flag ${flag.id} (${flag.kind})`;
    const reason = await ctx.ui.input(`${label}: reason`, "why this flag is acceptable");
    if (isBlank(reason)) return abort(`flag ${flag.id} needs a non-blank reason`);
    const evidence = await ctx.ui.input(`${label}: evidence`, "what you checked");
    if (isBlank(evidence)) return abort(`flag ${flag.id} needs non-blank evidence`);
    resolutions.push({ flag_id: flag.id, reason, evidence });
  }

  const acknowledged = await ctx.ui.confirm(
    "Shaper clearance: trust limit",
    `${SHAPER_FORGERY_DISCLOSURE}\n\nThis record claims no human identity: it says only that the pi tui channel captured the decision, with verified=false. Continue to the decision?`,
  );
  if (acknowledged !== true) return abort("the trust limit was not acknowledged");
  const choice = await ctx.ui.select("Shaper clearance decision", SHAPER_DECISION_OPTIONS.slice());
  if (choice !== "Affirm" && choice !== "Decline") return abort("no explicit Decline or Affirm was chosen");

  if (!sameSession(ctx, session)) {
    return refuse("the session identity changed during the dialog");
  }

  const record = {
    version: 1,
    subject: {
      project_id: subject.project_id,
      goal_id: subject.goal_id,
      goal_sha256: subject.goal_sha256,
      handoff_sha256: subject.handoff_sha256,
      provenance_sha256: subject.provenance_sha256,
      view_sha256: subject.view_sha256,
    },
    flag_resolutions: resolutions,
    decision: choice === "Affirm" ? "affirm" : "decline",
    channel: { runtime: "pi", mode: "tui", verified: false },
  };
  const recorded = await runOverlay(binary, ["shaper", "clearance", "record", ...source, "--stdin"], JSON.stringify(record));
  const output = `${recorded.stdout.toString("utf8")}${recorded.stderr}`;
  if (recorded.error || recorded.code !== 0) {
    return finish("failed", `gentle-ai-overlay shaper clearance record failed: ${describeExit(recorded)}\n${output}`, "error", recorded.code ?? -1);
  }
  return finish("recorded", `gentle-ai-overlay shaper clearance record: exit 0\n${output}`, "info", 0);
}

export default function (pi) {
  pi.on("before_agent_start", async (event) => {
    try {
      return injectContractsForEvent(event);
    } catch {
      return {};
    }
  });
  // No try/catch here: a thrown tool_call handler blocks execution, which is
  // the fail-closed outcome this guard wants.
  pi.on("tool_call", async (event) => shaperClearanceToolCall(event));
  if (typeof pi.registerCommand === "function") {
    pi.registerCommand("shaper-clear", {
      description: "Record a shaper clearance decision (Pi TUI only; digest-bound, not a signature)",
      handler: async (args, ctx) => runShaperClear(args, ctx),
    });
  }
}
