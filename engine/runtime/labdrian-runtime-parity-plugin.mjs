import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { Plugin } from "@opencode/plugin";
import { Tool } from "@opencode/plugin/promise/tool";

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

// ENGINE_VARIABLE names the engine binary the bridge runs. Without it the bridge runs the binary
// `labdrian install-hooks` deploys for Claude Code, bin/lib/locations.sh ENGINE_BINARY.
const ENGINE_VARIABLE = "LABDRIAN_ENGINE_BINARY";
const ENGINE_DEFAULT = join(".claude", "bin", "gentle-ai-overlay");
const ENGINE_TIMEOUT_MS = 30000;
const ENGINE_SOURCE = "opencode";
const DENY_FALLBACK = "this tool call was denied by a hook.";

function engineBinary() {
  const configured = process.env[ENGINE_VARIABLE];
  if (typeof configured === "string" && configured.trim() !== "") return configured.trim();
  return join(homedir(), ENGINE_DEFAULT);
}

function describeFailure(error) {
  return error instanceof Error ? error.message : String(error);
}

// runEngine runs the engine with the envelope on stdin. It resolves to {stdout} when the engine
// exited 0, and to {failure} when it could not be started, timed out or exited otherwise (an engine
// too old for a flag exits 1); it never rejects.
function runEngine(args, envelope) {
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
    timer = setTimeout(() => {
      child.kill();
      settle({ failure: `the engine did not answer within ${ENGINE_TIMEOUT_MS / 1000} seconds` });
    }, ENGINE_TIMEOUT_MS);
    const stdout = [];
    const stderr = [];
    child.stdout.on("data", (chunk) => stdout.push(chunk));
    child.stderr.on("data", (chunk) => stderr.push(chunk));
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
    child.stdin.end(JSON.stringify(envelope));
  });
}

// parseAnswer reads what a hook prints: nothing, or one JSON object.
function parseAnswer(stdout) {
  const text = typeof stdout === "string" ? stdout.trim() : "";
  if (text === "") return {};
  try {
    const answer = JSON.parse(text);
    return isRecord(answer) ? answer : {};
  } catch {
    return {};
  }
}

// askEngine sends one event to the engine. The bridge fails open, as a hook that cannot run does in
// Claude Code, and says so once, so that a session without its hooks is not taken for one with them.
async function askEngine(args, envelope) {
  const reply = await runEngine(args, envelope);
  if (reply.failure !== undefined) return { failure: reply.failure };
  return { answer: parseAnswer(reply.stdout) };
}

function unavailableNote(failure) {
  return `labdrian hooks are not running: ${failure}. The calls of this session are not checked by the memory gate until the engine answers.`;
}

function hookSpecific(answer) {
  return isRecord(answer.hookSpecificOutput) ? answer.hookSpecificOutput : {};
}

// textOf is the text a hook answer addresses to the model or the user, or undefined.
function textOf(value) {
  return typeof value === "string" && value !== "" ? value : undefined;
}

function toolEnvelope(event, call) {
  return {
    hook_event_name: event,
    session_id: call.sessionID,
    cwd: process.cwd(),
    tool_name: call.tool,
    tool_input: call.input,
    tool_use_id: call.id,
  };
}

// gateToolCall is the PreToolUse: the engine's denial becomes a Tool.Error, which stops the call
// and gives the model the reason; a warning waits for the result of the same call.
async function gateToolCall(call, state) {
  const reply = await askEngine(["projection", "hook", "--event", "PreToolUse", "--source", ENGINE_SOURCE], toolEnvelope("PreToolUse", call));
  if (reply.failure !== undefined) {
    if (!state.unavailableReported) {
      state.unavailableReported = true;
      state.notes.set(call.id, [unavailableNote(reply.failure)]);
    }
    return;
  }
  const specific = hookSpecific(reply.answer);
  if (specific.permissionDecision === "deny") {
    throw new Tool.Error({ message: textOf(specific.permissionDecisionReason) ?? DENY_FALLBACK });
  }
  const warning = textOf(reply.answer.systemMessage);
  if (warning !== undefined) state.notes.set(call.id, [warning]);
}

// reportToolResult is the PostToolUse: what the engine says about a completed call, and what the
// PreToolUse said about it, joins the result the model reads.
async function reportToolResult(call, state) {
  const lines = state.notes.get(call.id) ?? [];
  state.notes.delete(call.id);
  if (call.status === "completed") {
    const envelope = { ...toolEnvelope("PostToolUse", call), tool_response: call.result };
    const reply = await askEngine(["projection", "hook", "--event", "PostToolUse", "--source", ENGINE_SOURCE], envelope);
    if (reply.failure !== undefined) {
      if (!state.unavailableReported) {
        state.unavailableReported = true;
        lines.push(unavailableNote(reply.failure));
      }
    } else {
      for (const line of [textOf(hookSpecific(reply.answer).additionalContext), textOf(reply.answer.systemMessage)]) {
        if (line !== undefined) lines.push(line);
      }
    }
  }
  appendToResult(call.result, lines);
}

// appendToResult adds the lines to what the model reads of a result: one more text content, or, for
// a result with no content list, the end of its text output.
function appendToResult(result, lines) {
  if (lines.length === 0 || !isRecord(result)) return;
  const text = lines.join("\n");
  if (Array.isArray(result.content)) {
    result.content.push({ type: "text", text });
  } else if (typeof result.output === "string") {
    result.output += `\n${text}`;
  }
}

// registerToolHooks hooks the PreToolUse gate and the contract injection into tool calls, and the
// PostToolUse report after them. The gate runs first: a launch it denies is not rewritten.
async function registerToolHooks(ctx, config, state) {
  await ctx.tool.hook("execute.before", async (call) => {
    if (!isRecord(call)) return;
    await gateToolCall(call, state);
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
    // notes are what the bridge owes the model about a call, by call id; unavailableReported is whether
    // it has already said the engine does not answer.
    await registerToolHooks(ctx, config, { notes: new Map(), unavailableReported: false });
  },
});
