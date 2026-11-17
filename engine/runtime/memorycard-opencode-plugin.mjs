// labdrian-memorycard-plugin: the memorycard plugin of the labdrian OpenCode adapter (S11).
//
// A new session starts with the output of `memorycard context`, and each subagent's final text goes to
// `memorycard capture` on stdin, as in Claude Code. memorycard is the only memory store here. A
// memorycard that is absent, exits non-zero or does not answer in time never breaks the session: the
// failure is logged and the session goes on.
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
// After a failure the context is not asked for again for the session until this long has passed, so
// that a broken or hung memorycard costs one wait, not one for each model call.
const RETRY_AFTER_MS = 60000;
const KILL_GRACE_MS = 2000;
const OUTPUT_LIMIT = 1024 * 1024;
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

function timeoutMs() {
  const value = Number.parseInt(process.env[TIMEOUT_VARIABLE] ?? "", 10);
  return Number.isFinite(value) && value > 0 ? value : DEFAULT_TIMEOUT_MS;
}

// runMemorycard runs `program args` in cwd with stdin as given and resolves to { ok, stdout } or
// { ok: false, failure }; it never rejects. A run that outlives the timeout is ended, with SIGKILL when
// it ignores SIGTERM.
function runMemorycard(program, args, cwd, stdin) {
  return new Promise((resolve) => {
    let settled = false;
    let stdout = "";
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
      child = spawn(program, args, { cwd, stdio: ["pipe", "pipe", "pipe"] });
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
    child.stdout.on("data", (chunk) => {
      if (stdout.length < OUTPUT_LIMIT) stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      if (stderr.length < 4096) stderr += chunk;
    });
    child.stdin.on("error", () => {
      // memorycard closed stdin early; its exit status says what happened.
    });
    child.on("close", (code) => {
      if (code === 0) finish({ ok: true, stdout });
      else finish({ ok: false, failure: `${program} ${args.join(" ")} exited ${code}: ${stderr.trim()}` });
    });
    child.stdin.end(stdin ?? "");
  });
}

// textOf is the text of a finished tool result: its output when it has one, else its text parts.
function textOf(result) {
  if (!isRecord(result)) return "";
  if (typeof result.output === "string" && result.output.trim() !== "") return result.output;
  if (!Array.isArray(result.content)) return "";
  return result.content
    .filter((part) => isRecord(part) && part.type === "text" && typeof part.text === "string")
    .map((part) => part.text)
    .join("\n");
}

// sessionContext asks memorycard for the context of a session once and keeps the answer, which is
// pushed before each model call. A failure is kept for RETRY_AFTER_MS, an answer for good.
async function sessionContext(sessionID, state) {
  const known = state.sessions.get(sessionID);
  if (known?.text !== undefined) return known.text;
  if (known?.retryAt > Date.now()) return "";
  if (known?.asking !== undefined) return known.asking;
  const entry = known ?? {};
  state.sessions.set(sessionID, entry);
  entry.asking = runMemorycard(state.program, ["context"], state.directory, "").then((reply) => {
    entry.asking = undefined;
    if (!reply.ok) {
      entry.retryAt = Date.now() + RETRY_AFTER_MS;
      log(reply.failure);
      return "";
    }
    entry.text = reply.stdout.trim();
    return entry.text;
  });
  return entry.asking;
}

async function contextRequested(event, state) {
  if (!isRecord(event) || typeof event.sessionID !== "string" || !Array.isArray(event.system)) return;
  const text = await sessionContext(event.sessionID, state);
  if (text !== "") event.system.push({ type: "text", text });
}

async function toolFinished(call, state) {
  if (!isRecord(call) || call.tool !== SUBAGENT_TOOL || call.status !== "completed") return;
  const text = textOf(call.result);
  if (text.trim() === "") return;
  const reply = await runMemorycard(state.program, ["capture"], state.directory, text);
  if (!reply.ok) log(reply.failure);
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
    const state = { program, directory, sessions: new Map() };
    await ctx.session.hook("context", guarded((event) => contextRequested(event, state)));
    await ctx.tool.hook("execute.after", guarded((call) => toolFinished(call, state)));
  },
});
