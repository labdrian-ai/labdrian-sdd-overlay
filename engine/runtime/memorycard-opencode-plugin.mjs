// labdrian-memorycard-plugin: the memorycard plugin of the labdrian OpenCode adapter (S11).
//
// A new main session starts with the output of `memorycard context`, and each subagent's final text goes
// to `memorycard capture` on stdin, as in Claude Code. memorycard is the only memory store here. A
// memorycard that is absent, exits non-zero or does not answer in time never breaks the session: the
// failure is logged and the session goes on.
//
// The context is asked for once per session and goes onto the system prompt, which precedes every
// message: it is the same bytes for the whole session, so the cache of the conversation holds. A failed
// ask is not repeated, so the block is never inserted mid-session. Child sessions are not asked: roles
// query memorycard themselves.
//
// Event shapes, as read from the OpenCode 2.2.7 binary and NOT yet observed with a model (T17): the
// `context` event is { sessionID, system: [...] } and takes pushed { type: "text", text } parts; the
// `execute.after` call is { tool, sessionID, id, input, status, result: { output?, content, metadata? } }.
// A shape that is not these is logged once per process, never ignored silently.
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { Plugin } from "@opencode/plugin";

const RECORD_FILE = "memorycard-opencode.json";
const ACTIVE_FILE = "memorycard-opencode.active.json";
const DEFAULT_PROGRAM = "memorycard";
// TIMEOUT_VARIABLE overrides how long memorycard may take to answer, in milliseconds.
const TIMEOUT_VARIABLE = "LABDRIAN_MEMORYCARD_TIMEOUT_MS";
const DEFAULT_TIMEOUT_MS = 20000;
// After a failed capture the next ones wait this long, so that a broken memorycard costs one failure.
const RETRY_AFTER_MS = 60000;
const KILL_GRACE_MS = 2000;
const OUTPUT_LIMIT = 1024 * 1024;
// CAPTURE_LIMIT bounds the bytes sent to `memorycard capture`.
const CAPTURE_LIMIT = 256 * 1024;
// SESSION_LIMIT bounds the sessions kept: past it the oldest is forgotten.
const SESSION_LIMIT = 128;
const SUBAGENT_TOOL = "subagent";

function isRecord(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// log reports a failure without raising it. OpenCode gives a plugin no logger, so it is one line on stderr.
function log(message) {
  try {
    console.error(`memorycard plugin: ${message}`);
  } catch {
    // Nothing is worth a failure here.
  }
}

// warnedShapes are the kinds of unexpected shape already logged by this process.
const warnedShapes = new Set();

// unexpected logs a shape the plugin does not know, once per kind and process.
function unexpected(kind, detail) {
  if (warnedShapes.has(kind)) return;
  warnedShapes.add(kind);
  log(`unexpected ${kind} (${detail}); the shapes were read from the OpenCode 2.2.7 binary and are unverified with a model`);
}

function timeoutMs() {
  const value = Number.parseInt(process.env[TIMEOUT_VARIABLE] ?? "", 10);
  return Number.isFinite(value) && value > 0 ? value : DEFAULT_TIMEOUT_MS;
}

// runMemorycard runs `program args` in cwd and resolves to { ok, stdout } or { ok: false, failure }; it
// never rejects. Its output is decoded as UTF-8 across chunks and limited in bytes. A run that outlives
// the timeout is ended, with SIGKILL when it ignores SIGTERM.
function runMemorycard(program, args, cwd) {
  return new Promise((resolve) => {
    let settled = false;
    let stdout = "";
    let stdoutBytes = 0;
    let stderr = "";
    let child;
    let timer;
    const finish = (reply) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(reply);
    };
    try {
      child = spawn(program, args, { cwd, stdio: ["ignore", "pipe", "pipe"] });
    } catch (error) {
      finish({ ok: false, failure: `could not start ${program}: ${error.message}` });
      return;
    }
    timer = setTimeout(() => {
      child.kill("SIGTERM");
      setTimeout(() => child.kill("SIGKILL"), KILL_GRACE_MS).unref();
      // A grandchild that outlives the kill must not hold the pipes, and with them the session.
      child.stdout.destroy();
      child.stderr.destroy();
      child.unref();
      finish({ ok: false, failure: `${program} ${args.join(" ")} did not answer in ${timeoutMs()} ms` });
    }, timeoutMs());
    child.on("error", (error) => finish({ ok: false, failure: `could not run ${program}: ${error.message}` }));
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      if (stdoutBytes < OUTPUT_LIMIT) {
        stdout += chunk;
        stdoutBytes += Buffer.byteLength(chunk);
      }
    });
    child.stderr.on("data", (chunk) => {
      if (stderr.length < 4096) stderr += chunk;
    });
    child.on("close", (code) => {
      if (code === 0) finish({ ok: true, stdout });
      else finish({ ok: false, failure: `${program} ${args.join(" ")} exited ${code}: ${stderr.trim()}` });
    });
  });
}

// capped is text cut to at most CAPTURE_LIMIT bytes at a character boundary, and whether it was cut.
function capped(text) {
  const bytes = Buffer.from(text, "utf8");
  if (bytes.length <= CAPTURE_LIMIT) return { bytes, cut: false };
  let end = CAPTURE_LIMIT;
  while (end > 0 && (bytes[end] & 0xc0) === 0x80) end--;
  return { bytes: bytes.subarray(0, end), cut: true, from: bytes.length };
}

// startCapture starts `memorycard capture` with the text on stdin and does not wait for it: the process
// is detached and does not keep this one alive, so the session never waits for memorycard. A failure is
// logged and pauses the captures that follow.
function startCapture(state, text) {
  const { bytes, cut, from } = capped(text);
  if (cut) log(`capture input truncated from ${from} to ${bytes.length} bytes`);
  let child;
  try {
    child = spawn(state.program, ["capture"], { cwd: state.directory, stdio: ["pipe", "ignore", "pipe"], detached: true });
  } catch (error) {
    state.pausedUntil = Date.now() + RETRY_AFTER_MS;
    log(`could not start ${state.program}: ${error.message}`);
    return;
  }
  child.unref();
  let stderr = "";
  let failed = false;
  const fail = (reason) => {
    if (failed) return;
    failed = true;
    state.pausedUntil = Date.now() + RETRY_AFTER_MS;
    log(`${state.program} capture ${reason}`);
  };
  const timer = setTimeout(() => {
    fail(`did not finish in ${timeoutMs()} ms`);
    child.kill("SIGTERM");
  }, timeoutMs());
  timer.unref();
  child.on("error", (error) => {
    clearTimeout(timer);
    fail(`could not run: ${error.message}`);
  });
  child.stderr.setEncoding("utf8");
  child.stderr.on("data", (chunk) => {
    if (stderr.length < 4096) stderr += chunk;
  });
  child.stderr.unref?.();
  child.stdin.on("error", () => {
    // memorycard closed stdin early; its exit status says what happened.
  });
  child.on("close", (code) => {
    clearTimeout(timer);
    if (code !== 0 && code !== null) fail(`exited ${code}: ${stderr.trim()}`);
  });
  child.stdin.end(bytes);
}

// textOf is the text of a finished tool result: its output when it has one, else its text parts. It is
// undefined for a result that has neither a text output nor a content list.
function textOf(result) {
  if (!isRecord(result)) return undefined;
  if (typeof result.output === "string" && result.output.trim() !== "") return result.output;
  if (!Array.isArray(result.content)) return typeof result.output === "string" ? "" : undefined;
  return result.content
    .filter((part) => isRecord(part) && part.type === "text" && typeof part.text === "string")
    .map((part) => part.text)
    .join("\n");
}

// isChildSession says whether OpenCode reports a parent for the session (the parity plugin's detection).
// A session that cannot be told is taken as a main one, and the answer is not kept when it could not be had.
async function isChildSession(ctx, session, sessionID) {
  if (session.child !== undefined) return session.child;
  try {
    const info = await ctx.session.get({ sessionID });
    const parent = isRecord(info) ? (info.parentID ?? (isRecord(info.session) ? info.session.parentID : undefined)) : undefined;
    session.child = typeof parent === "string" && parent !== "";
    return session.child;
  } catch {
    return false;
  }
}

// sessionOf is what the plugin keeps of a session; the oldest is forgotten past SESSION_LIMIT.
function sessionOf(state, sessionID) {
  let session = state.sessions.get(sessionID);
  if (session === undefined) {
    if (state.sessions.size >= SESSION_LIMIT) state.sessions.delete(state.sessions.keys().next().value);
    session = {};
    state.sessions.set(sessionID, session);
  }
  return session;
}

// sessionContext asks memorycard for the context of a main session once and keeps the answer, which is
// pushed before each model call. A child session, and a session whose ask failed, keep the empty text for
// good: a later success would change the system prompt mid-session.
async function sessionContext(ctx, sessionID, state) {
  const session = sessionOf(state, sessionID);
  if (session.text !== undefined) return session.text;
  session.asking ??= (async () => {
    if (await isChildSession(ctx, session, sessionID)) return "";
    const reply = await runMemorycard(state.program, ["context"], state.directory);
    if (!reply.ok) {
      log(reply.failure);
      return "";
    }
    return reply.stdout.trim();
  })().then((text) => {
    session.text = text;
    session.asking = undefined;
    return text;
  });
  return session.asking;
}

async function contextRequested(ctx, event, state) {
  if (!isRecord(event) || typeof event.sessionID !== "string" || !Array.isArray(event.system)) {
    unexpected("context event", "want { sessionID, system: [] }");
    return;
  }
  const text = await sessionContext(ctx, event.sessionID, state);
  if (text !== "") event.system.push({ type: "text", text });
}

function toolFinished(call, state) {
  if (!isRecord(call)) {
    unexpected("execute.after call", "want an object");
    return;
  }
  if (call.tool !== SUBAGENT_TOOL || call.status !== "completed") return;
  const text = textOf(call.result);
  if (text === undefined) {
    unexpected("subagent result", "want result.output as a string or result.content as a list");
    return;
  }
  if (text.trim() === "" || state.pausedUntil > Date.now()) return;
  startCapture(state, text);
}

// guarded makes a hook that cannot fail its caller.
const guarded = (handler) => async (event) => {
  try {
    await handler(event);
  } catch (error) {
    log(String(error?.message ?? error));
  }
};

// readRecord reads the record the adapter wrote beside the plugin: the program to run and the version
// the plugin reports as active. A missing or unreadable record leaves memorycard to the PATH.
async function readRecord(configRoot) {
  try {
    const record = JSON.parse(await readFile(join(configRoot, RECORD_FILE), "utf8"));
    return isRecord(record) ? record : {};
  } catch {
    return {};
  }
}

// recordActive writes the active marker: which version and hash this load runs.
async function recordActive(pluginPath, configRoot, record) {
  if (typeof record.installed_version !== "string") return;
  const hash = createHash("sha256").update(await readFile(pluginPath, "utf8")).digest("hex");
  await writeFile(
    join(configRoot, ACTIVE_FILE),
    JSON.stringify({ active_version: record.installed_version, active_hash: hash, plugin_path: pluginPath, config_root: configRoot }, null, 2),
  );
}

export default Plugin.define({
  // The id is also the name of the directory the adapter installs the plugin into.
  id: "memorycard",
  async setup(ctx) {
    // <config>/plugins/memorycard/index.js: the config root is three levels up.
    const pluginPath = fileURLToPath(import.meta.url);
    const configRoot = dirname(dirname(dirname(pluginPath)));
    const record = await readRecord(configRoot);
    try {
      await recordActive(pluginPath, configRoot, record);
    } catch (error) {
      log(`could not write the active marker: ${error.message}`);
    }
    const directory = isRecord(ctx.location) && typeof ctx.location.directory === "string" ? ctx.location.directory : process.cwd();
    const program = typeof record.memorycard_command === "string" && record.memorycard_command !== "" ? record.memorycard_command : DEFAULT_PROGRAM;
    const state = { program, directory, sessions: new Map(), pausedUntil: 0 };
    await ctx.session.hook("context", guarded((event) => contextRequested(ctx, event, state)));
    await ctx.tool.hook("execute.after", guarded((call) => toolFinished(call, state)));
  },
});
