import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { Plugin } from "@opencode/plugin";
import * as Tool from "@opencode/plugin/promise/tool";

// The v2 tool that launches a child session, and the input field that names the agent it launches.
// The contract is injected by rewriting input.prompt in execute.before, which reaches the child.
const INJECTED_TOOL = "subagent";
const AGENT_FIELD = "agent";

function isRecord(value) {
  return typeof value === "object" && value !== null;
}

function hasExactLine(prompt, line) {
  return prompt.split("\n").some((candidate) => candidate.trim() === line);
}

function normalizePromptConfig(config) {
  const promptConfig = isRecord(config) ? config.prompt_config : undefined;
  if (!isRecord(promptConfig)) return undefined;
  if (Array.isArray(promptConfig.contracts)) {
    const contracts = [];
    for (const contract of promptConfig.contracts) {
      const normalized = normalizeContractConfig(contract);
      if (normalized) contracts.push(normalized);
    }
    if (contracts.length === 0) return undefined;
    return { contracts };
  }
	  const legacy = normalizeContractConfig({
	    contract_path: promptConfig.contract_path,
	    injection_point: promptConfig.injection_point,
	    included_phases: promptConfig.included_phases,
	    excluded_phases: promptConfig.excluded_phases,
	    ...(Object.prototype.hasOwnProperty.call(promptConfig, "language_context")
	      ? { language_context: promptConfig.language_context }
	      : {}),
	    ...(Object.prototype.hasOwnProperty.call(promptConfig, "activation_context")
	      ? { activation_context: promptConfig.activation_context }
	      : {}),
	    ...(Object.prototype.hasOwnProperty.call(promptConfig, "context_operator")
	      ? { context_operator: promptConfig.context_operator }
	      : {}),
	  });
  if (!legacy) return undefined;
  return { contracts: [legacy] };
}

function normalizeContractConfig(promptConfig) {
  if (!isRecord(promptConfig)) return undefined;
  if (typeof promptConfig.contract_path !== "string") return undefined;
  if (typeof promptConfig.injection_point !== "string") return undefined;
  if (!Array.isArray(promptConfig.included_phases)) return undefined;
  if (!Array.isArray(promptConfig.excluded_phases)) return undefined;
  if (!promptConfig.included_phases.every((item) => typeof item === "string")) return undefined;
  if (!promptConfig.excluded_phases.every((item) => typeof item === "string")) return undefined;
  if ("context_operator" in promptConfig) return undefined;
  if ("language_context" in promptConfig && !isStringArray(promptConfig.language_context)) return undefined;
  if ("activation_context" in promptConfig && !isStringArray(promptConfig.activation_context)) return undefined;
  return {
    contractPath: promptConfig.contract_path,
    injectionHeader: promptConfig.injection_point,
    includedPhases: new Set(promptConfig.included_phases),
    excludedPhases: new Set(promptConfig.excluded_phases),
    languageContext: new Set((promptConfig.language_context ?? []).map((item) => item.toLowerCase())),
    activationContext: new Set((promptConfig.activation_context ?? []).map((item) => item.toLowerCase())),
  };
}

function isStringArray(value) {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}

function injectPrompt(prompt, config) {
  if (hasExactLine(prompt, config.contractPath)) return prompt;
  if (hasExactLine(prompt, config.injectionHeader)) {
    const lines = prompt.split("\n");
    const out = [];
    for (const line of lines) {
      out.push(line);
      if (line.trim() === config.injectionHeader) out.push(config.contractPath);
    }
    return out.join("\n");
  }
  const separator = prompt.endsWith("\n") ? "\n" : "\n\n";
  return prompt + separator + config.injectionHeader + "\n" + config.contractPath + "\n";
}

function stripPrompt(prompt, config) {
  if (!hasExactLine(prompt, config.contractPath)) return prompt;
  return prompt
    .split("\n")
    .filter((line) => line.trim() !== config.contractPath)
    .join("\n");
}

function mutatePrompt(prompt, subagentType, config, workContext) {
	  const promptConfig = normalizePromptConfig(config);
	  if (!promptConfig) return prompt;
	  const invocationWorkContext = resolveInvocationWorkContext(workContext);
	  let nextPrompt = prompt;
	  for (const contract of promptConfig.contracts) {
	    if (contract.excludedPhases.has(subagentType)) {
	      nextPrompt = stripPrompt(nextPrompt, contract);
	      continue;
	    }
	    if (!contract.includedPhases.has(subagentType)) continue;
	    if (contextRequired(contract) && !workContextMatches(invocationWorkContext, contract)) continue;
	    nextPrompt = injectPrompt(nextPrompt, contract);
	  }
	  return nextPrompt;
}

function resolveInvocationWorkContext(workContext) {
	  return isRecord(workContext) ? workContext : undefined;
}

function contextRequired(contract) {
  return contract.languageContext.size > 0 || contract.activationContext.size > 0;
}

function workContextMatches(workContext, contract) {
  if (!isRecord(workContext) || workContext.trusted !== true) return false;
  if (contract.languageContext.size > 0 && !setIntersects(contract.languageContext, workContext.languages)) return false;
  if (contract.activationContext.size > 0 && !setIntersects(contract.activationContext, workContext.activations)) return false;
  if (!isStringArray(workContext.work_kinds) || workContext.work_kinds.length === 0) return false;
  if (!hasString(workContext.work_kinds, "application-code")) return false;
  return true;
}

function setIntersects(want, got) {
  if (!isStringArray(got)) return false;
  return got.some((item) => want.has(item.toLowerCase()));
}

function hasString(items, want) {
  return items.some((item) => item.toLowerCase() === want);
}

// injectContract rewrites the prompt of a launch of the injected tool in place, which is what
// reaches the child session.
function injectContract(call, config) {
  if (!isRecord(call) || call.tool !== INJECTED_TOOL) return;
  const input = call.input;
  if (!isRecord(input)) return;
  if (typeof input.prompt !== "string") return;
  if (typeof input[AGENT_FIELD] !== "string") return;

  input.prompt = mutatePrompt(input.prompt, input[AGENT_FIELD], config, extractInvocationWorkContext(input));
}

function extractInvocationWorkContext(input) {
  return isRecord(input.work_context) ? input.work_context : undefined;
}

// recordActive writes the active marker: which version, hash and prompt config this load runs.
async function recordActive(pluginPath, configRoot, config) {
  const pluginHash = createHash("sha256")
    .update(await readFile(pluginPath, "utf8"))
    .digest("hex");
  await writeFile(
    join(configRoot, "labdrian-runtime-parity.active.json"),
    JSON.stringify(
      {
        active_version: config.installed_version,
        active_hash: pluginHash,
        active_prompt_config_hash: config.prompt_config_hash,
        plugin_path: pluginPath,
        config_root: configRoot,
      },
      null,
      2,
    ),
  );
}

// --- the hook bridge -------------------------------------------------------------------------
//
// Each OpenCode event goes to the engine command that answers the same event for Claude Code, with
// --source opencode, and the answer is turned into what OpenCode understands. No policy is decided
// here: the engine decides, this file only carries the question and the answer.
//
// The engine is an executable the bridge cannot trust to answer. It fails open, as a hook that cannot
// run does in Claude Code, and it says so once (the pending note), on the next result or model call
// that can carry it.

// ENGINE_VARIABLE names the engine binary the bridge runs. Without it the bridge runs the binary
// `labdrian install-hooks` deploys for Claude Code, bin/lib/locations.sh ENGINE_BINARY.
const ENGINE_VARIABLE = "LABDRIAN_ENGINE_BINARY";
const ENGINE_DEFAULT = join(".claude", "bin", "gentle-ai-overlay");
// TIMEOUT_VARIABLE overrides how long the engine may take to answer, in milliseconds.
const TIMEOUT_VARIABLE = "LABDRIAN_ENGINE_TIMEOUT_MS";
const ENGINE_TIMEOUT_MS = 30000;
// After a timeout the engine is not asked again for twice the timeout, so that a hung engine costs
// one wait, not one for every call.
const ENGINE_PAUSE_FACTOR = 2;
const ENGINE_KILL_GRACE_MS = 2000;
const ENGINE_OUTPUT_LIMIT = 2 * 1024 * 1024;
const ENGINE_SOURCE = "opencode";
const DENY_FALLBACK = "this tool call was denied by a hook.";
// NOTES_LIMIT bounds what the bridge holds for calls that have not completed.
const NOTES_LIMIT = 256;

function engineBinary() {
  const configured = process.env[ENGINE_VARIABLE];
  if (typeof configured === "string" && configured.trim() !== "") return configured.trim();
  return join(homedir(), ENGINE_DEFAULT);
}

function engineTimeoutMs() {
  const configured = Number(process.env[TIMEOUT_VARIABLE]);
  return Number.isFinite(configured) && configured > 0 ? configured : ENGINE_TIMEOUT_MS;
}

function describeFailure(error) {
  return error instanceof Error ? error.message : String(error);
}

// runEngine runs the engine with the envelope on stdin. A string envelope is sent as it is. It resolves to {stdout} when the engine
// exited 0, and to {failure} when it could not be started, could not be given its input, timed out
// ({hung: true}), printed more than ENGINE_OUTPUT_LIMIT or exited otherwise (an engine too old for a
// flag exits 1); it never rejects. An engine that outlives its timeout gets SIGTERM, then SIGKILL.
function runEngine(args, envelope, timeoutMs) {
  let input;
  try {
    // A string is the input as it is (the frozen-context verb reads a plain-text brief); anything else is a hook envelope.
    input = typeof envelope === "string" ? envelope : JSON.stringify(envelope);
  } catch (error) {
    return Promise.resolve({ failure: `the hook input could not be serialized: ${describeFailure(error)}` });
  }
  return new Promise((resolve) => {
    let child;
    let timer;
    // settle answers once and puts the timer away, so that an engine that never started leaves
    // nothing behind that keeps the process alive.
    const settle = (answer) => {
      clearTimeout(timer);
      resolve(answer);
    };
    try {
      child = spawn(engineBinary(), args, { stdio: ["pipe", "pipe", "pipe"] });
    } catch (error) {
      settle({ failure: describeFailure(error) });
      return;
    }
    // end stops the engine, politely and then for good, and answers with the failure.
    let ending = false;
    const end = (answer) => {
      if (ending) return;
      ending = true;
      child.kill("SIGTERM");
      const killer = setTimeout(() => child.kill("SIGKILL"), Math.min(ENGINE_KILL_GRACE_MS, timeoutMs));
      killer.unref();
      child.once("close", () => clearTimeout(killer));
      settle(answer);
    };
    timer = setTimeout(() => {
      end({ failure: `the engine did not answer within ${timeoutMs / 1000} seconds`, hung: true });
    }, timeoutMs);
    const stdout = [];
    const stderr = [];
    let size = 0;
    const collect = (chunks) => (chunk) => {
      size += chunk.length;
      if (size > ENGINE_OUTPUT_LIMIT) {
        end({ failure: "the engine printed more than 2 MiB" });
        return;
      }
      chunks.push(chunk);
    };
    child.stdout.on("data", collect(stdout));
    child.stderr.on("data", collect(stderr));
    child.stdin.on("error", () => {});
    child.on("error", (error) => settle({ failure: describeFailure(error) }));
    child.on("close", (code, signal) => {
      if (code === 0) {
        settle({ stdout: Buffer.concat(stdout).toString("utf8") });
        return;
      }
      const said = Buffer.concat(stderr).toString("utf8").trim();
      settle({ failure: said || `the engine ended with ${code === null ? `signal ${signal}` : `exit ${code}`}` });
    });
    child.stdin.end(input);
  });
}

// parseAnswer reads what a hook prints: nothing (the engine's silence, an allow), or one JSON object.
// Anything else is not an answer of the engine.
function parseAnswer(stdout) {
  const text = typeof stdout === "string" ? stdout.trim() : "";
  if (text === "") return { answer: {} };
  try {
    const answer = JSON.parse(text);
    if (isRecord(answer) && !Array.isArray(answer)) return { answer };
  } catch {
    // reported below
  }
  return { failure: "the engine printed an answer that is not a JSON object" };
}

// askEngine sends one event to the engine and resolves to {answer} or {failure}; it never rejects.
// A failure is also the first one of the bridge: it makes the note that says so, which waits until a
// result or a model call can carry it.
// The gate (PreToolUse) is never paused: a pause there would let through a call the engine denies.
async function askEngine(state, args, envelope, gate = false) {
  if (!gate && state.pausedUntil > Date.now()) return noteFailure(state, state.pausedReason);
  const timeoutMs = engineTimeoutMs();
  const reply = await runEngine(args, envelope, timeoutMs);
  if (reply.failure !== undefined) {
    if (reply.hung) {
      state.pausedUntil = Date.now() + ENGINE_PAUSE_FACTOR * timeoutMs;
      state.pausedReason = `${reply.failure}; the bridge stops asking it for ${(ENGINE_PAUSE_FACTOR * timeoutMs) / 1000} seconds except to check tool calls, which each wait for it`;
      return noteFailure(state, state.pausedReason);
    }
    return noteFailure(state, reply.failure);
  }
  const parsed = parseAnswer(reply.stdout);
  return parsed.failure === undefined ? parsed : noteFailure(state, parsed.failure);
}

// noteFailure makes the pending note the first time the engine fails, and returns the failure.
function noteFailure(state, failure) {
  if (!state.noteIssued) {
    state.noteIssued = true;
    state.pendingNote = `labdrian hooks are not running: ${failure}. The calls of this session are not checked by the memory gate until the engine answers.`;
  }
  return { failure };
}

// takeNote hands over the pending note, if there is one, and forgets it: told once.
function takeNote(state) {
  const note = state.pendingNote;
  state.pendingNote = undefined;
  return note === undefined ? [] : [note];
}

// holdWarning keeps what the PreToolUse warned about a call until its result; the oldest are let go
// past NOTES_LIMIT, since a call that never completes must not be held for ever.
function holdWarning(state, id, warning) {
  state.notes.set(id, [warning]);
  while (state.notes.size > NOTES_LIMIT) state.notes.delete(state.notes.keys().next().value);
}

function hookSpecific(answer) {
  return isRecord(answer.hookSpecificOutput) ? answer.hookSpecificOutput : {};
}

// textOf is the text a hook answer addresses to the model or the user, or undefined.
function textOf(value) {
  return typeof value === "string" && value !== "" ? value : undefined;
}

// toolInputFor is the input of a call as Claude Code names its arguments, where OpenCode v2 names
// them otherwise: the read tool takes `path` (Claude's file_path) and the skill tool `id` (Claude's
// skill). The call's own arguments are kept; the value OpenCode acts on is the one the engine judges,
// so it replaces a Claude-named argument the call may also carry.
function toolInputFor(call) {
  const input = call.input;
  if (!isRecord(input)) return input;
  if (call.tool === "read" && typeof input.path === "string") {
    return { ...input, file_path: input.path };
  }
  if (call.tool === "skill" && typeof input.id === "string") {
    return { ...input, skill: input.id };
  }
  return input;
}

function toolEnvelope(event, call) {
  return {
    hook_event_name: event,
    session_id: call.sessionID,
    cwd: process.cwd(),
    tool_name: call.tool,
    tool_input: toolInputFor(call),
    tool_use_id: call.id,
  };
}

// gateToolCall is the PreToolUse: the engine's denial becomes a Tool.Error, which stops the call
// and gives the model the reason; a warning waits for the result of the same call.
async function gateToolCall(call, state) {
  const reply = await askEngine(state, ["projection", "hook", "--event", "PreToolUse", "--source", ENGINE_SOURCE], toolEnvelope("PreToolUse", call), true);
  if (reply.failure !== undefined) return;
  const specific = hookSpecific(reply.answer);
  if (specific.permissionDecision === "deny") {
    throw new Tool.Error({ message: textOf(specific.permissionDecisionReason) ?? DENY_FALLBACK });
  }
  const warning = textOf(reply.answer.systemMessage);
  if (warning !== undefined) holdWarning(state, call.id, warning);
}

// The agent whose launch is given the frozen context of its candidate. OpenCode's reviewer has no shell,
// so it cannot run git for its diff; Claude Code's parent pastes it and Codex's `review launch` appends it.
const FROZEN_CONTEXT_AGENT = "reviewer";
const FROZEN_CONTEXT_VERB = "review frozen-context";

function frozenContextRefusal(failure) {
  // The verb's own refusal is one line that names the verb and says why; anything else (the engine
  // missing, hung, or too old for the verb) gets a sentence that says what was not made.
  if (failure.startsWith(`error: ${FROZEN_CONTEXT_VERB}:`)) return failure;
  return `this reviewer launch was denied: its frozen context (the diff of its candidate) could not be made: ${failure}`;
}

// The verb that makes the skills a launch prompt lists absolute SKILL.md paths. A role on OpenCode cannot use the
// skill tool, so a skill reaches it as the path of its SKILL.md under the header the contracts use.
const SKILL_PATHS_VERB = "roles skill-paths";

function skillPathsRefusal(failure) {
  // The verb's own refusal is one line that names the verb and says what to write; anything else (the engine
  // missing, hung, or too old for the verb) gets a sentence that says what was not checked.
  if (failure.startsWith(`error: ${SKILL_PATHS_VERB}:`)) return failure;
  return `this launch was denied: its skill list could not be checked: ${failure}`;
}

// resolveSkillPaths replaces the prompt of a `subagent` launch with the verb's stdout: the same prompt, each bare
// skill id under the skills header made the path of its SKILL.md. It runs after the gate and before the frozen
// context, so that the diff appended to a reviewer brief is never read as a skill list. It denies the launch when
// the verb refuses, fails, times out or prints nothing. A config with no skills_root (an install older than this
// plugin) leaves every prompt as it is, and so does a call that is not a launch with a prompt. It hands the verb
// the launch's agent (`--agent <id>`) when that field is a non-empty string; the engine decides which agents get
// a skill added from prose (a reviewer or polisher gets none).
async function resolveSkillPaths(call, config) {
  if (!isRecord(call) || call.tool !== INJECTED_TOOL) return;
  const root = isRecord(config) ? config.skills_root : undefined;
  if (typeof root !== "string" || root === "") return;
  const input = call.input;
  if (!isRecord(input) || typeof input.prompt !== "string" || input.prompt === "") return;
  const agent = typeof input[AGENT_FIELD] === "string" && input[AGENT_FIELD] !== "" ? ["--agent", input[AGENT_FIELD]] : [];
  const reply = await runEngine([...SKILL_PATHS_VERB.split(" "), "--skills-root", root, ...agent], input.prompt, engineTimeoutMs());
  if (reply.failure !== undefined) throw new Tool.Error({ message: skillPathsRefusal(reply.failure) });
  if (reply.stdout.trim() === "") throw new Tool.Error({ message: skillPathsRefusal("the engine printed nothing") });
  input.prompt = reply.stdout;
}

// attachFrozenContext replaces the prompt of a reviewer launch with the verb's stdout: the brief the agent
// wrote, then the frozen context of the candidate it names. It denies the launch when the verb refuses,
// fails, times out or prints nothing, since a review without its diff must not run. Every other call is
// left as it is.
async function attachFrozenContext(call) {
  if (!isRecord(call) || call.tool !== INJECTED_TOOL) return;
  const input = call.input;
  if (!isRecord(input) || input[AGENT_FIELD] !== FROZEN_CONTEXT_AGENT) return;
  if (typeof input.prompt !== "string") {
    throw new Tool.Error({ message: frozenContextRefusal("the launch carries no prompt") });
  }
  const reply = await runEngine([...FROZEN_CONTEXT_VERB.split(" "), "--cwd", process.cwd()], input.prompt, engineTimeoutMs());
  if (reply.failure !== undefined) throw new Tool.Error({ message: frozenContextRefusal(reply.failure) });
  if (reply.stdout.trim() === "") throw new Tool.Error({ message: frozenContextRefusal("the engine printed nothing") });
  input.prompt = reply.stdout;
}

// reportToolResult is the PostToolUse: what the engine says about a completed call, what the
// PreToolUse said about it and, if the bridge has one to give, the note that the engine does not answer,
// join the result the model reads.
async function reportToolResult(call, state) {
  const lines = state.notes.get(call.id) ?? [];
  state.notes.delete(call.id);
  if (call.status === "completed") {
    const envelope = { ...toolEnvelope("PostToolUse", call), tool_response: call.result };
    const reply = await askEngine(state, ["projection", "hook", "--event", "PostToolUse", "--source", ENGINE_SOURCE], envelope);
    if (reply.failure === undefined) {
      for (const line of [textOf(hookSpecific(reply.answer).additionalContext), textOf(reply.answer.systemMessage)]) {
        if (line !== undefined) lines.push(line);
      }
    }
    if (canCarry(call.result)) lines.push(...takeNote(state));
  }
  appendToResult(call.result, lines);
}

// canCarry says whether a result has somewhere to put a line the model reads.
function canCarry(result) {
  return isRecord(result) && (Array.isArray(result.content) || typeof result.output === "string");
}

// appendToResult adds the lines to what the model reads of a result: one more text content, or, for
// a result with no content list, the end of its text output.
function appendToResult(result, lines) {
  if (lines.length === 0 || !canCarry(result)) return;
  const text = lines.join("\n");
  if (Array.isArray(result.content)) {
    result.content.push({ type: "text", text });
  } else {
    result.output += `\n${text}`;
  }
}

// --- the session events ---------------------------------------------------------------------
//
// OpenCode has no hook that answers with text for the model, so what the engine says at the start of
// a session and at a prompt reaches the model through the `context` hook, which runs before each model
// call and lets the plugin push text parts onto the system prompt. It reaches main and child sessions
// alike (L10 T3).

const ENGINE_STOP_ARGS = ["review", "stop-hook"];
const STOP_DESCRIPTION = "labdrian stop hook";
const STOP_FALLBACK_REASON = "a stop hook blocked this stop.";

// sessionState is what the bridge keeps of one session: the SessionStart block once it was asked for (start,
// or the question under way, starting), whether the last stop was blocked (Claude Code's stop_hook_active,
// which a prompt of the user clears), the text the bridge itself sent to make the agent continue (ownText)
// and whether the session is a child one.
function sessionState(state, sessionID) {
  let session = state.sessions.get(sessionID);
  if (session === undefined) {
    session = { start: undefined, starting: undefined, continuing: false, ownText: undefined, child: undefined };
    state.sessions.set(sessionID, session);
  }
  return session;
}

function sessionEnvelope(event, sessionID) {
  return { hook_event_name: event, session_id: sessionID, cwd: process.cwd() };
}

// linesOf is what an answer says to the model: the warning, then the context.
function linesOf(reply) {
  if (reply.failure !== undefined) return [];
  return [textOf(reply.answer.systemMessage), textOf(hookSpecific(reply.answer).additionalContext)].filter((line) => line !== undefined);
}

// promptSubmitted is the UserPromptSubmit: the answer is kept until the next prompt of the session,
// which answers for itself. The text the bridge itself sent to continue a stopped agent is not a
// prompt of the user: it asks nothing and clears nothing.
async function promptSubmitted(event, state) {
  const session = sessionState(state, event.sessionID);
  const text = isRecord(event.prompt) && typeof event.prompt.text === "string" ? event.prompt.text : "";
  if (session.ownText !== undefined && text === session.ownText) {
    session.ownText = undefined;
    return;
  }
  session.continuing = false;
  const reply = await askEngine(
    state,
    ["projection", "hook", "--event", "UserPromptSubmit", "--source", ENGINE_SOURCE],
    { ...sessionEnvelope("UserPromptSubmit", event.sessionID), prompt: text },
  );
  // The answer joins the prompt, not the system prompt: the system prompt comes before every message, so
  // what changes with each prompt there would invalidate the cached prefix of the whole conversation.
  const lines = linesOf(reply);
  if (lines.length === 0) return;
  if (isRecord(event.prompt) && typeof event.prompt.text === "string") {
    event.prompt.text = `${event.prompt.text}\n\n${lines.join("\n")}`;
    return;
  }
  warnPromptShape(event.prompt);
}

let promptShapeWarned = false;

// warnPromptShape says once per process that an answer was dropped because the prompt event is not the
// shape read from the OpenCode 2.2.7 binary ({ text: string }). The answer is not moved to the system
// prompt, which must stay the same bytes for the session.
function warnPromptShape(prompt) {
  if (promptShapeWarned) return;
  promptShapeWarned = true;
  let seen = typeof prompt;
  if (Array.isArray(prompt)) seen = "an array";
  else if (isRecord(prompt)) seen = `an object whose text is ${typeof prompt.text}`;
  try {
    console.error(`labdrian runtime-parity plugin: dropped the engine's answer to a prompt: unexpected prompt shape (the prompt is ${seen}; want { text: string })`);
  } catch {
    // Nothing is worth a failure here.
  }
}

// askSessionStart asks the SessionStart once for a session. It resolves to the lines to push and
// whether the engine answered: only an answer is remembered, so a failure is asked again.
async function askSessionStart(sessionID, state) {
  const reply = await askEngine(state, ["projection", "hook", "--event", "SessionStart", "--source", ENGINE_SOURCE], sessionEnvelope("SessionStart", sessionID));
  if (reply.failure !== undefined) return { lines: [], answered: false };
  return { lines: [textOf(hookSpecific(reply.answer).additionalContext)].filter((line) => line !== undefined), answered: true };
}

// startLines are the SessionStart lines of a session. The question is asked once at a time: a model
// call that comes while it is under way waits for it, and neither a failure nor a rejection is kept.
async function startLines(sessionID, session, state) {
  if (session.start !== undefined) return session.start;
  session.starting ??= askSessionStart(sessionID, state).then(
    (asked) => {
      session.starting = undefined;
      if (asked.answered) session.start = asked.lines;
      return asked.lines;
    },
    () => {
      session.starting = undefined;
      return [];
    },
  );
  return session.starting;
}

// contextRequested runs before each model call of a session: the SessionStart block and the note that the
// engine does not answer (once, when the engine is broken) join the system prompt, as parts of type text
// (the shape OpenCode's own plugins push). Nothing that changes with each prompt goes here.
async function contextRequested(event, state) {
  if (!Array.isArray(event.system)) return;
  const session = sessionState(state, event.sessionID);
  const start = await startLines(event.sessionID, session, state);
  for (const text of [...start, ...takeNote(state)]) {
    event.system.push({ type: "text", text });
  }
}

// isChildSession says whether OpenCode reports a parent for the session. Claude Code runs the Stop
// hook for the main session only. A session that cannot be told is taken as a main one, and the answer
// is not kept when it could not be had.
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

// sessionIdle is the Stop: when the engine blocks the stop, its reason is sent to the session as a
// synthetic message, which makes the agent take another turn (the experimental idle hook, L10 T3). An
// engine that does not answer lets the session stop, as a failing Stop hook does in Claude Code.
async function sessionIdle(event, ctx, state) {
  const session = sessionState(state, event.sessionID);
  if (await isChildSession(ctx, session, event.sessionID)) return;
  // A synthetic message is answered by its own prompt before the next idle; one still held was never
  // seen, and must not swallow a later prompt that happens to say the same.
  session.ownText = undefined;
  const reply = await askEngine(state, ENGINE_STOP_ARGS, { ...sessionEnvelope("Stop", event.sessionID), stop_hook_active: session.continuing });
  if (reply.failure !== undefined || reply.answer.decision !== "block") {
    session.continuing = false;
    return;
  }
  const text = textOf(reply.answer.reason) ?? STOP_FALLBACK_REASON;
  session.continuing = true;
  session.ownText = text;
  await ctx.session.synthetic({ sessionID: event.sessionID, text, description: STOP_DESCRIPTION });
}

// registerSessionHooks hooks the three session events. A hook that fails must not break the session.
async function registerSessionHooks(ctx, state) {
  const guarded = (handler) => async (event) => {
    if (!isRecord(event) || typeof event.sessionID !== "string") return;
    try {
      await handler(event);
    } catch {
      // The bridge fails open: a session event that cannot be bridged is not worth the session.
    }
  };
  await ctx.session.hook("prompt", guarded((event) => promptSubmitted(event, state)));
  await ctx.session.hook("context", guarded((event) => contextRequested(event, state)));
  await ctx.session.hook("experimental.idle", guarded((event) => sessionIdle(event, ctx, state)));
}

// registerToolHooks hooks the PreToolUse gate and the contract injection into tool calls, and the
// PostToolUse report after them. The gate runs first: a launch it denies is not rewritten. The skill list of a
// launch is made paths next, a reviewer launch is then given its frozen context, and last the contract is
// injected into the prompt that results.
async function registerToolHooks(ctx, config, state) {
  await ctx.tool.hook("execute.before", async (call) => {
    if (!isRecord(call)) return;
    await gateToolCall(call, state);
    await resolveSkillPaths(call, config);
    await attachFrozenContext(call);
    injectContract(call, config);
  });
  await ctx.tool.hook("execute.after", async (call) => {
    if (!isRecord(call)) return;
    try {
      await reportToolResult(call, state);
    } catch {
      // A report that cannot be made must not turn a finished call into a failed one.
    }
  });
}

export default Plugin.define({
  // The id is also the name of the directory the adapter installs the plugin into.
  id: "labdrian-runtime-parity",
  async setup(ctx) {
    // <config>/plugins/labdrian-runtime-parity/index.js: the config root is three levels up.
    const pluginPath = fileURLToPath(import.meta.url);
    const configRoot = dirname(dirname(dirname(pluginPath)));
    const config = JSON.parse(await readFile(join(configRoot, "labdrian-runtime-parity.json"), "utf8"));

    await recordActive(pluginPath, configRoot, config);
    // notes are the warnings of the gate by call id, until the result of the call; sessions is what the
    // bridge keeps of each session; pendingNote is the note that the engine does not answer, until a
    // result or a model call carries it (noteIssued: it was made, and is told once); pausedUntil is when
    // a hung engine is asked again.
    const state = { notes: new Map(), sessions: new Map(), pendingNote: undefined, noteIssued: false, pausedUntil: 0, pausedReason: "" };
    await registerToolHooks(ctx, config, state);
    await registerSessionHooks(ctx, state);
  },
});
